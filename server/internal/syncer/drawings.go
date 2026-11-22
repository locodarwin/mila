package syncer

import (
	"context"
	"database/sql"
	"slices"

	"github.com/locodarwin/mila/server/internal/apperr"
)

// lineKinds are the shapes that join others rather than being joined.
var lineKinds = []string{"arrow", "line", "pen"}

// checkShape keeps a drawing's shapes to their drawing: on its page, at
// most maxDrawingShapes of them, joined only to its other shapes, and
// showing only the page's own files.
func (s *Service) checkShape(ctx context.Context, tx *sql.Tx, r *row, accepted []string) error {
	d := s.st.DB()
	if !r.exists {
		drawing, err := loadRow(ctx, tx, d.Dialect, entities["page_drawing"], r.values["drawing_id"].(string))
		if err != nil {
			return err
		}
		if drawing.values["page_id"] != r.values["page_id"] {
			return refuse(apperr.Invalid, "drawing_id", "A shape must be on its drawing's page.")
		}
		var n int
		if err := tx.QueryRowContext(ctx, rebind(d.Dialect, "SELECT COUNT(*) FROM drawing_shapes WHERE drawing_id = ? AND deleted_at IS NULL"), r.values["drawing_id"]).Scan(&n); err != nil {
			return err
		}
		if n >= maxDrawingShapes {
			return refuse(apperr.Invalid, "", "A drawing holds %d shapes at most.", maxDrawingShapes)
		}
	}
	for _, name := range []string{"from_id", "to_id"} {
		id, _ := r.values[name].(string)
		if id == "" || (r.exists && !slices.Contains(accepted, name)) {
			continue
		}
		end, err := loadRow(ctx, tx, d.Dialect, entities["drawing_shape"], id)
		if err != nil {
			return err
		}
		if end.values["drawing_id"] != r.values["drawing_id"] || id == r.id {
			return refuse(apperr.Invalid, name, "A line can only join shapes of its own drawing.")
		}
		// Lines join shapes, never other lines, so no chain of them loops.
		if slices.Contains(lineKinds, end.values["kind"].(string)) {
			return refuse(apperr.Invalid, name, "A line joins shapes, not other lines.")
		}
	}
	if id, _ := r.values["image_id"].(string); id != "" && (!r.exists || slices.Contains(accepted, "image_id")) {
		file, err := loadRow(ctx, tx, d.Dialect, entities["page_attachment"], id)
		if err != nil {
			return err
		}
		if file.values["page_id"] != r.values["page_id"] {
			return refuse(apperr.Invalid, "image_id", "A picture must be one of its page's files.")
		}
	}
	return nil
}
