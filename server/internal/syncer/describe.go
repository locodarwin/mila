package syncer

import (
	"strings"
)

// EntityInfo describes a synced entity for what is generated from the
// registry (httpapi/restgen: the REST endpoints in shared/openapi.yaml)
// and for tests.
type EntityInfo struct {
	Name string // "custom_field"
	// Type is the Go and schema name ("CustomField"); Collection is the
	// path segment ("custom-fields") and Plural its Go form
	// ("CustomFields").
	Type, Collection, Plural string
	// Doc says what the entity is, for the API reference.
	Doc    string
	Fields []FieldInfo
	// Pair names the two fields a pair's ID is made from (PairID); a POST
	// then adds or brings back the pair's row.
	Pair []string
	// Filters are the fields a list can be filtered on.
	Filters []string
	// ServerCreated rows are made by the server only (attachments, by an
	// upload).
	ServerCreated bool
	// Parent names the field holding the task a row hangs off.
	Parent string
	// Private names the field holding the one user who sees the row.
	Private string
	// GuestWritable rows can be changed by guests in their projects;
	// WorkspaceWide rows (and WideWhenNull ones with no project) are
	// seen by every member.
	GuestWritable, WorkspaceWide, WideWhenNull bool
	// Owner names the field holding who alone may change the row.
	Owner string
}

// FieldInfo describes one field.
type FieldInfo struct {
	Name string
	// Kind is the field's kind: string, text, enum, time, ref, bool, int,
	// date, clock, self, member or json.
	Kind                                     string
	Required, Nullable, Immutable, ServerSet bool
	Min, Max                                 int
	Enum                                     []string
	Ref                                      string // the entity a ref points at
	// Default is the value a REST create fills in, when HasDefault.
	Default    any
	HasDefault bool
	Doc        string
	// Schema is a json field's shape for the API reference: lines of
	// OpenAPI under the property ("type: object"). Empty is any JSON.
	Schema []string
}

var kindNames = map[kind]string{
	kString: "string", kText: "text", kEnum: "enum", kTime: "time", kRef: "ref", kBool: "bool",
	kInt: "int", kDate: "date", kClock: "clock", kSelf: "self", kMember: "member", kJSON: "json",
}

// collections are the entities' REST path segments.
var collections = map[string]string{
	"project": "projects", "status": "statuses", "label": "labels", "custom_field": "custom-fields",
	"saved_view": "saved-views", "task": "tasks", "task_label": "task-labels", "task_dependency": "task-dependencies", "task_assignee": "task-assignees",
	"checklist_item": "checklist-items", "comment": "comments", "attachment": "attachments",
	"my_day_entry": "my-day-entries", "reminder": "reminders", "custom_field_value": "custom-field-values",
	"automation": "automations", "page": "pages", "page_attachment": "page-attachments", "page_thread": "page-threads", "page_comment": "page-comments", "page_drawing": "page-drawings", "drawing_shape": "drawing-shapes", "template": "templates",
	"time_entry": "time-entries", "project_time_entry": "project-time-entries", "workspace_theme": "workspace-themes",
}

// entityDocs say what each entity is, for the API reference.
var entityDocs = map[string]string{
	"project":      "A project: a list of tasks with its own statuses. A new project has no statuses; make them too (the app makes Backlog, To do, In progress, Done and Canceled).",
	"status":       "A step a project's tasks move through, in a category that says what it means (done and canceled close a task).",
	"label":        "A label any task in the workspace can carry. Every member sees every label, guests too.",
	"custom_field": "A field of the user's own on tasks: in one project, or with no project_id, in every project.",
	"saved_view":   "A saved filter and how it lays out what it shows. With owner_id set, only its owner sees it.",
	"task": "A task: in a project or, with no project_id, in the inbox; with parent_id, a subtask, which lives in its parent's project.\n\n" +
		"To move a task with subtasks to another project, move the parent first (project_id, with a status_id of the new project, or null), then each subtask the same way. Until a subtask is moved it stays where it was and can still be edited there; a write that sets a subtask's project_id or parent_id is refused unless it puts the subtask in its parent's project.",
	"task_label":         "A label on a task. Its ID is made from the pair, so adding the same label again brings back the same row.",
	"task_dependency":    "A task waiting for another to be done (blocked by it): task_id waits for blocker_id. The two may be in different projects of the workspace; a guest sees the row only when they see both tasks. Its ID is made from the pair, as a label's on a task is. A dependency that would close a loop (the blocker already waits for the task, directly or down a chain) is refused.",
	"task_assignee":      "A member assigned to a task. Its ID is made from the pair, as a label's on a task is.",
	"checklist_item":     "A step of a task's checklist.",
	"comment":            "A comment on a task. Only its author changes it; admins may delete it.",
	"attachment":         "A file on a task. Upload one with POST /workspaces/{workspaceId}/tasks/{taskId}/attachments, and fetch its bytes from .../attachments/{attachmentId}/file.",
	"my_day_entry":       "A task in the user's My Day, which only they see. Its ID is made from the task and the user.",
	"reminder":           "A reminder on a task, which only its owner sees and gets.",
	"custom_field_value": "A task's value for a custom field. Its ID is made from the task and the field.",
	"page": "A wiki page: in a project's wiki, or with no project_id, the workspace's (which guests do not see). A wiki's main page has no parent_id and an ID made from its project's or workspace's ID (XOR with 2f5c6c0e-8a9b-4c7e-9d3a-6b1e4f2a7c58); it cannot be deleted, and its title is not shown, since it takes its project's or workspace's name. Every other page has a parent_id in the same wiki. Deleting a page does not delete its subpages: delete them too.\n\n" +
		"body is Markdown, written as the editor writes it (checklists, tables, images, and boxes as > [!NOTE]); a write replaces it whole and is kept as a version of its own in the page's history (GET .../pages/{pageId}/versions). A link to a page is a link to its address in the app (/wiki?page=<id>, or /projects/<projectId>?tab=wiki&page=<id>) and to a task is /?task=<id>; when either is renamed, links whose text was its old name take the new one. An image of a file attached to the page or a task is ![name](/api/v1/workspaces/<workspaceId>/attachments/<attachmentId>/file).",
	"automation":         "A rule: when something happens to a task, and it fits, the changes someone chose. Not for guests.",
	"template":           "A project, a task or a wiki page saved to make more from, as the app's Settings, Templates shows them. Members and admins see a workspace's templates; guests never do. data is the snapshot; the app makes the rows from it.",
	"time_entry":         "Time logged on a task by one person: typed in, or a timer. A timer runs while running is true, its minutes 0 until it stops; set running to false with minutes, the time since started_at, to stop it. A person has one timer running at a time, so the app stops any other first. Its person changes it, and so do admins. Everyone who sees the task sees its time.",
	"project_time_entry": "Time logged to a project itself by one person, not to one of its tasks: a meeting, planning, travel. Typed in, or a timer, as a time entry is on a task: a timer runs while running is true, its minutes 0 until it stops. A person has one timer running at a time, on a task or a project, so the app stops any other first. Its person changes it, and so do admins. Everyone who sees the project sees its time.",
	"workspace_theme":    "A theme the workspace offers everyone in it, guests too, as the app's Settings, Workspace themes shows them: nobody is switched to one. Owners and admins publish, change and unpublish them (set deleted_at); a workspace offers 10 at most.",
	"page_thread":        "Comments pinned to some words of a wiki page. Anyone who can read the page can comment and can resolve or reopen a thread. Only an owner or admin can delete one that holds other people's comments.",
	"page_comment":       "A comment in a thread on a wiki page. Only its author changes it; admins may delete it.",
	"page_drawing":       "A drawing in a wiki page, which the page's body names with <!-- mila:drawing <id> --> and a picture of it. Its shapes are drawing_shape rows. A page holds 50 drawings at most.",
	"drawing_shape":      "A shape of a drawing: a box, ellipse, diamond, cylinder, text, sticky note, line, arrow, pen stroke or picture, with its place, words and look. A line or arrow on shapes names them in from_id and to_id and follows them. Shapes are drawn in sort_key order. A drawing holds 2,000 shapes at most.",
	"page_attachment":    "A file on a wiki page, as an attachment is on a task. Upload one with POST /workspaces/{workspaceId}/pages/{pageId}/attachments, and fetch its bytes from /workspaces/{workspaceId}/attachments/{attachmentId}/file, as a task's.",
}

// camel turns "my-day-entries" or "my_day_entry" into "MyDayEntries" or
// "MyDayEntry".
func camel(s string) string {
	var b strings.Builder
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == '-' }) {
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return b.String()
}

// Describe lists the synced entities in sync order.
func Describe() []EntityInfo {
	out := make([]EntityInfo, 0, len(syncOrder))
	for _, name := range syncOrder {
		e := entities[name]
		if e.internal {
			continue
		}
		info := EntityInfo{
			Name: name, Type: camel(name), Collection: collections[name], Plural: camel(collections[name]), Doc: entityDocs[name],
			ServerCreated: e.serverCreated, Parent: e.parent, Private: e.private, Owner: e.owner,
			GuestWritable: e.guestWritable, WorkspaceWide: e.workspaceWide, WideWhenNull: e.wideWhenNull,
		}
		if p, ok := pairs[name]; ok {
			info.Pair = []string{p[0], p[1]}
		}
		for i := range e.fields {
			f := &e.fields[i]
			fi := FieldInfo{
				Name: f.name, Kind: kindNames[f.kind], Required: f.required, Nullable: f.nullable,
				Immutable: f.immutable, ServerSet: f.serverSet, Min: f.min, Max: f.max, Enum: f.enum, Ref: f.ref, Doc: f.doc,
				Schema: f.schema,
			}
			fi.Default, fi.HasDefault = defaultOf(f)
			info.Fields = append(info.Fields, fi)
			if filterable(f) {
				info.Filters = append(info.Filters, f.name)
			}
		}
		out = append(out, info)
	}
	return out
}
