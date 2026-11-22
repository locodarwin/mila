package syncer_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/syncer"
)

func shapeRow(page, drawing, kind string, extra map[string]any) map[string]any {
	row := map[string]any{
		"page_id": page, "drawing_id": drawing, "kind": kind,
		"box": map[string]any{"x": -40, "y": 16.5, "w": 160, "h": 72}, "sort_key": "a0",
	}
	for k, v := range extra {
		row[k] = v
	}
	return row
}

func TestDrawingsKeepToTheirPage(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		project, page := uuid.NewString(), uuid.NewString()
		other := uuid.NewString()
		main := syncer.MainPageID(project)
		for _, r := range w.push("owner", op("project", project, later(0), newProject("Garden")),
			op("page", main, later(1), newPage(project, nil, "Garden")),
			op("page", page, later(2), newPage(project, main, "Plan")),
			op("page", other, later(2), newPage(project, main, "Other"))) {
			wantStatus(t, r, "applied")
		}
		drawing, box, arrow := uuid.NewString(), uuid.NewString(), uuid.NewString()
		for _, r := range w.push("member",
			op("page_drawing", drawing, later(3), map[string]any{"page_id": page, "name": "Flow", "made_by": w.users["member"]}),
			op("drawing_shape", box, later(4), shapeRow(page, drawing, "rect", map[string]any{"text": "Start"})),
			op("drawing_shape", arrow, later(5), shapeRow(page, drawing, "arrow", map[string]any{
				"from_id": box, "points": []any{[]any{0, 0}, []any{40, -12.5}}, "style": map[string]any{"route": "curved", "end": "arrow"},
			}))) {
			wantStatus(t, r, "applied")
		}
		refused := map[string]map[string]any{
			"a shape on another page":   shapeRow(other, drawing, "rect", nil),
			"a line to another line":    shapeRow(page, drawing, "line", map[string]any{"to_id": arrow}),
			"a line to itself":          nil,
			"a kind no app draws":       shapeRow(page, drawing, "hexagon", nil),
			"a box out of shape":        shapeRow(page, drawing, "rect", map[string]any{"box": map[string]any{"x": 1, "y": 2}}),
			"a box of negative size":    shapeRow(page, drawing, "rect", map[string]any{"box": map[string]any{"x": 1, "y": 2, "w": -1, "h": 3}}),
			"a style out of shape":      shapeRow(page, drawing, "rect", map[string]any{"style": map[string]any{"route": "zigzag"}}),
			"points that are not pairs": shapeRow(page, drawing, "pen", map[string]any{"points": []any{[]any{1, 2, 3}}}),
		}
		for name, row := range refused {
			id := uuid.NewString()
			if row == nil {
				row = shapeRow(page, drawing, "line", map[string]any{"from_id": id})
			}
			res := w.push("member", op("drawing_shape", id, later(6), row))[0]
			if res.Status != "rejected" {
				t.Errorf("%s was %s", name, res.Status)
			}
		}
		// A picture is one of its own page's files.
		file := uuid.NewString()
		_, err := w.svc.CreateAttachment(context.Background(), w.users["owner"], w.ws, syncer.NewAttachment{ID: file, PageID: other, Filename: "a.png", Mime: "image/png", StorageKey: "k", Size: 1})
		must(t, err)
		res := w.push("member", op("drawing_shape", uuid.NewString(), later(7), shapeRow(page, drawing, "image", map[string]any{"image_id": file})))[0]
		if res.Status != "rejected" || !strings.Contains(res.Message, "page's files") {
			t.Errorf("a picture of another page's file: %s %s", res.Status, res.Message)
		}
		// A guest of the project draws too, as they comment.
		w.share(project, true)
		wantStatus(t, w.push("guest", op("drawing_shape", uuid.NewString(), later(8), shapeRow(page, drawing, "sticky", map[string]any{"text": "Hello"})))[0], "applied")
	})
}
