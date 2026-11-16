package actions

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// NewStatus is one of a new project's statuses.
type NewStatus struct {
	Name     string `json:"name"`
	Category string `json:"category"`
}

// starters are a new project's statuses unless others are given, as the
// app makes them (STARTER_STATUSES in web/src/lib/sync/entities.ts).
var starters = []NewStatus{
	{"Backlog", "backlog"}, {"To do", "todo"}, {"In progress", "in-progress"}, {"Done", "done"}, {"Canceled", "canceled"},
}

// CreateProject is what create-project takes.
type CreateProject struct {
	Name     string      `json:"name"`
	Statuses []NewStatus `json:"statuses"`
}

// Made is a new project with its statuses.
type Made struct {
	Project  *syncer.Record
	Statuses []StatusNamed
	Batch    string
}

// CreateProject makes a project with the app's five starting statuses, or
// with the ones given, in their order.
func (s *Service) CreateProject(ctx context.Context, c Caller, in CreateProject) (Made, error) {
	r, err := s.start(ctx, c)
	if err != nil {
		return Made{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Made{}, &apperr.Error{Kind: apperr.Invalid, Field: "name", Message: "A project needs a name."}
	}
	statuses := in.Statuses
	if statuses == nil {
		statuses = starters
	}
	if len(statuses) == 0 {
		return Made{}, &apperr.Error{Kind: apperr.Invalid, Field: "statuses", Message: "A project needs at least one status; leave statuses out for the usual five."}
	}
	for _, st := range statuses {
		if strings.TrimSpace(st.Name) == "" {
			return Made{}, &apperr.Error{Kind: apperr.Invalid, Field: "statuses", Message: "Each status needs a name."}
		}
	}
	project, err := r.write(syncer.Write{Entity: "project", Mode: syncer.Create, Fields: map[string]json.RawMessage{"name": raw(name)}})
	if err != nil {
		return Made{}, err
	}
	out := Made{Project: project, Batch: r.batch, Statuses: []StatusNamed{}}
	for _, st := range statuses {
		rec, err := r.write(syncer.Write{Entity: "status", Mode: syncer.Create, Fields: map[string]json.RawMessage{
			"project_id": raw(project.ID), "name": raw(strings.TrimSpace(st.Name)), "category": raw(st.Category),
		}})
		if err != nil {
			return Made{}, err
		}
		out.Statuses = append(out.Statuses, StatusNamed{ID: rec.ID, Name: str(*rec, "name"), Category: str(*rec, "category")})
	}
	return out, nil
}

// DeleteStatus is what delete-status takes: the status by ID, or by name
// with its project; and where its tasks go, by ID or name, if not to
// another status of the same kind.
type DeleteStatus struct {
	Status  string  `json:"status"`
	Project *string `json:"project"`
	To      *string `json:"to"`
}

// Rehomed is what deleting a status did with its tasks.
type Rehomed struct {
	Moved int
	To    StatusNamed
	Batch string
}

// DeleteStatus moves a status's tasks to another of the project's, as the
// app does (one of the same kind, else its first), then deletes it.
func (s *Service) DeleteStatus(ctx context.Context, c Caller, in DeleteStatus) (Rehomed, error) {
	r, err := s.start(ctx, c)
	if err != nil {
		return Rehomed{}, err
	}
	var gone *syncer.Record
	if given(in.Project) {
		project, err := r.project("project", *in.Project)
		if err != nil {
			return Rehomed{}, err
		}
		id, err := r.status("status", project, in.Status)
		if err != nil {
			return Rehomed{}, err
		}
		if gone, err = s.sync.Get(ctx, c.User, c.Workspace, "status", id); err != nil {
			return Rehomed{}, err
		}
	} else {
		gone, err = s.sync.Get(ctx, c.User, c.Workspace, "status", strings.TrimSpace(in.Status))
		if apperr.KindOf(err) == apperr.NotFound {
			return Rehomed{}, &apperr.Error{Kind: apperr.NotFound, Field: "status", Message: "There is no such status that you can see. Give its ID, or its name with its project."}
		}
		if err != nil {
			return Rehomed{}, err
		}
	}
	if !live(*gone) {
		return Rehomed{}, &apperr.Error{Kind: apperr.Invalid, Field: "status", Message: "That status is deleted already."}
	}
	project := str(*gone, "project_id")
	others, err := r.all("status", map[string]string{"project_id": project})
	if err != nil {
		return Rehomed{}, err
	}
	// In the project's order, as the app reads them.
	slices.SortFunc(others, func(a, b syncer.Record) int {
		return cmp.Or(cmp.Compare(str(a, "sort_key"), str(b, "sort_key")), cmp.Compare(a.ID, b.ID))
	})
	var target, first *syncer.Record
	for i := range others {
		st := &others[i]
		if !live(*st) || st.ID == gone.ID {
			continue
		}
		if first == nil {
			first = st
		}
		if target == nil && str(*st, "category") == str(*gone, "category") {
			target = st
		}
	}
	if target == nil {
		target = first
	}
	if given(in.To) {
		id, err := r.status("to", project, *in.To)
		if err != nil {
			return Rehomed{}, err
		}
		if id == gone.ID {
			return Rehomed{}, &apperr.Error{Kind: apperr.Invalid, Field: "to", Message: "Its tasks can't go to the status being deleted."}
		}
		for i := range others {
			if others[i].ID == id {
				target = &others[i]
			}
		}
	}
	if target == nil {
		return Rehomed{}, &apperr.Error{Kind: apperr.Invalid, Field: "status", Message: "A project needs at least one status."}
	}
	tasks, err := r.all("task", map[string]string{"status_id": gone.ID})
	if err != nil {
		return Rehomed{}, err
	}
	out := Rehomed{To: StatusNamed{ID: target.ID, Name: str(*target, "name"), Category: str(*target, "category")}, Batch: r.batch}
	for _, t := range tasks {
		if !live(t) {
			continue
		}
		if _, err := r.write(syncer.Write{Entity: "task", ID: t.ID, Mode: syncer.Update,
			Fields: map[string]json.RawMessage{"status_id": raw(target.ID)}}); err != nil {
			return Rehomed{}, err
		}
		out.Moved++
	}
	if _, err := r.write(syncer.Write{Entity: "status", ID: gone.ID, Mode: syncer.Delete}); err != nil {
		return Rehomed{}, err
	}
	return out, nil
}
