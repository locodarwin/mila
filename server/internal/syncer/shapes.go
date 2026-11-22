package syncer

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/locodarwin/mila/server/internal/accounts"
)

// The shapes of the kJSON fields. A value the app cannot read would break
// the page for everyone who opens it, so a write is refused unless each
// part the app reads has its JSON type. Keys the app does not know are
// left alone, for a newer app. Values arrive decoded with UseNumber.

// viewFilterLists are the parts of a view's filter that hold lists of
// strings (web/src/lib/filter.ts, server/internal/filter).
var viewFilterLists = []string{"projects", "statuses", "statusIds", "priorities", "labels", "assignees", "dependencies", "time"}

// configShape: a view's config is an object whose filter, when present
// and not null, is an object with each part of its type.
func configShape(v any) error {
	config, ok := v.(map[string]any)
	if !ok {
		return errors.New(`must be an object, such as {"filter":{}}`)
	}
	raw, ok := config["filter"]
	if !ok || raw == nil {
		return nil
	}
	filter, ok := raw.(map[string]any)
	if !ok {
		return errors.New("filter must be an object")
	}
	for name, part := range filter {
		if part == nil {
			continue
		}
		switch {
		case name == "text" || name == "due":
			if _, ok := part.(string); !ok {
				return fmt.Errorf("filter.%s must be a string", name)
			}
		case name == "showDone" || name == "myDay":
			if _, ok := part.(bool); !ok {
				return fmt.Errorf("filter.%s must be true or false", name)
			}
		case slices.Contains(viewFilterLists, name):
			if !isStringList(part) {
				return fmt.Errorf("filter.%s must be a list of strings", name)
			}
		case name == "fields":
			conditions, ok := part.([]any)
			if !ok {
				return errors.New("filter.fields must be a list")
			}
			for i, c := range conditions {
				cond, ok := c.(map[string]any)
				_, field := cond["field"].(string)
				_, op := cond["op"].(string)
				if !ok || !field || !op {
					return fmt.Errorf("filter.fields[%d] must be an object with a string field and op", i)
				}
			}
		}
	}
	return nil
}

// displayShape: a view's display is an object (null, the defaults, is
// handled before). Its parts are the app's to judge: what it cannot use
// it shows as not available.
func displayShape(v any) error {
	d, ok := v.(map[string]any)
	if !ok {
		return errors.New("must be an object, or null")
	}
	// A report view's setup: the app reads each part it knows and takes
	// anything else as the page comes, so only its own shape is checked.
	if r, ok := d["report"]; ok && r != nil {
		rep, ok := r.(map[string]any)
		if !ok {
			return errors.New("report must be an object, or null")
		}
		// Its dates are held to the app's window: a shared report from the
		// year 1000 had every reader's device work through 375,000 days.
		for _, k := range []string{"from", "to"} {
			if v, ok := rep[k]; ok && v != nil {
				day, ok := v.(string)
				if _, err := time.Parse(time.DateOnly, day); !ok || err != nil || day < earliestReportDay || day > "2999-12-31" {
					return fmt.Errorf("report.%s must be a day from %s, as YYYY-MM-DD, or null", k, earliestReportDay)
				}
			}
		}
	}
	return nil
}

// earliestReportDay is EARLIEST_DAY in web/src/lib/reportSetup.svelte.ts.
const earliestReportDay = "2000-01-01"

// optionsShape: a field's choices are a list of objects, each with an ID
// of its own, a name and a color of the palette.
func optionsShape(v any) error {
	list, ok := v.([]any)
	if !ok {
		return errors.New("must be a list")
	}
	seen := map[string]bool{}
	for i, item := range list {
		o, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("[%d] must be an object with id, name and color", i)
		}
		id, ok := o["id"].(string)
		if !ok || id == "" || len(id) > 64 {
			return fmt.Errorf("[%d].id must be a string of 1 to 64 characters", i)
		}
		if seen[id] {
			return fmt.Errorf("[%d].id %q is used twice", i, id)
		}
		seen[id] = true
		if _, ok := o["name"].(string); !ok {
			return fmt.Errorf("[%d].name must be a string", i)
		}
		if color, ok := o["color"].(string); !ok || !slices.Contains(palette, color) {
			return fmt.Errorf("[%d].color must be one of %v", i, palette)
		}
	}
	return nil
}

// valueShape checks a task's value for a custom field against the field's
// type, as the app writes it.
func valueShape(fieldType string, v any) error {
	switch fieldType {
	case "text", "url", "select", "user":
		if _, ok := v.(string); !ok {
			return fmt.Errorf("a %s field's value must be a string", fieldType)
		}
	case "number":
		n, ok := v.(json.Number)
		if !ok {
			return errors.New("a number field's value must be a number")
		}
		if f, err := strconv.ParseFloat(string(n), 64); err != nil || math.IsInf(f, 0) {
			return errors.New("a number field's value must be a finite number")
		}
	case "date":
		s, ok := v.(string)
		if !ok || !datePattern.MatchString(s) {
			return errors.New("a date field's value must be a date like 2026-10-28")
		}
		if _, err := time.Parse(time.DateOnly, s); err != nil {
			return errors.New("a date field's value must be a date like 2026-10-28")
		}
	case "multi_select":
		if !isStringList(v) {
			return errors.New("a multi_select field's value must be a list of choice IDs")
		}
	case "checkbox":
		if _, ok := v.(bool); !ok {
			return errors.New("a checkbox field's value must be true or false")
		}
	}
	return nil
}

func isStringList(v any) bool {
	list, ok := v.([]any)
	if !ok {
		return false
	}
	for _, x := range list {
		if _, ok := x.(string); !ok {
			return false
		}
	}
	return true
}

// maxTemplateTasks bounds the tasks one template holds.
const maxTemplateTasks = 500

// maxTemplatePages bounds the pages one page template holds.
const maxTemplatePages = 100

// templateShape: a template's snapshot (web/src/lib/templates.ts) is an
// object of version 1 with a list of at most maxTemplateTasks tasks, each
// an object with a string id and title, and for a project template a
// project object. The rest is the app's to read.
func templateShape(v any) error {
	t, ok := v.(map[string]any)
	if !ok {
		return errors.New("must be an object")
	}
	if n, ok := t["v"].(json.Number); !ok || n.String() != "1" {
		return errors.New("v must be 1")
	}
	tasks, ok := t["tasks"].([]any)
	if !ok {
		return errors.New("tasks must be a list")
	}
	if len(tasks) > maxTemplateTasks {
		return fmt.Errorf("holds %d tasks; a template holds %d at most", len(tasks), maxTemplateTasks)
	}
	for i, x := range tasks {
		o, ok := x.(map[string]any)
		_, id := o["id"].(string)
		title, isTitle := o["title"].(string)
		if !ok || !id || !isTitle {
			return fmt.Errorf("tasks[%d] must be an object with a string id and title", i)
		}
		// As a task's own: making from the template would be refused otherwise.
		if n := utf8.RuneCountInString(title); title != trimmed(title) || n < 1 || n > entities["task"].field("title").max {
			return fmt.Errorf("tasks[%d]'s title must be 1 to %d characters, with no spaces at either end", i, entities["task"].field("title").max)
		}
	}
	if pages, ok := t["pages"]; ok && pages != nil {
		list, ok := pages.([]any)
		if !ok {
			return errors.New("pages must be a list")
		}
		if len(list) > maxTemplatePages {
			return fmt.Errorf("holds %d pages; a template holds %d at most", len(list), maxTemplatePages)
		}
		for i, x := range list {
			o, ok := x.(map[string]any)
			_, id := o["id"].(string)
			title, isTitle := o["title"].(string)
			_, isBody := o["body"].(string)
			if !ok || !id || !isTitle || !isBody {
				return fmt.Errorf("pages[%d] must be an object with a string id, title and body", i)
			}
			if n := utf8.RuneCountInString(title); n > entities["page"].field("title").max {
				return fmt.Errorf("pages[%d]'s title must be at most %d characters", i, entities["page"].field("title").max)
			}
		}
	}
	if p, ok := t["project"]; ok && p != nil {
		if _, ok := p.(map[string]any); !ok {
			return errors.New("project must be an object")
		}
	}
	return nil
}

// themeShape: a workspace theme's body is a theme document that passes
// every check a person's own theme does (accounts.ValidateTheme), so the
// app reads both the same way.
func themeShape(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return errors.New("must be a theme document")
	}
	if _, err := accounts.ValidateTheme(raw); err != nil {
		why, _ := strings.CutPrefix(err.Error(), "This theme cannot be saved: ")
		return fmt.Errorf("is not a theme Mila can use: %s", why)
	}
	return nil
}

// shapeKinds are what a drawing's shapes may be (web/src/lib/drawing.ts).
var shapeKinds = []string{"rect", "ellipse", "diamond", "cylinder", "text", "sticky", "arrow", "line", "pen", "image"}

// maxCoordinate bounds where a shape may be in its drawing, either way.
const maxCoordinate = 1_000_000

// coordinate: a finite JSON number within maxCoordinate of the origin.
func coordinate(v any) bool {
	n, ok := v.(json.Number)
	if !ok {
		return false
	}
	f, err := n.Float64()
	return err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) && math.Abs(f) <= maxCoordinate
}

var boxSchema = []string{"type: object", "required: [x, y, w, h]", "properties:",
	"  x: { type: number }", "  y: { type: number }", "  w: { type: number, minimum: 0 }", "  h: { type: number, minimum: 0 }"}

// boxShape: a shape's box is {x, y, w, h}, numbers, its size not negative.
func boxShape(v any) error {
	box, ok := v.(map[string]any)
	if !ok {
		return errors.New(`must be an object, such as {"x":0,"y":0,"w":160,"h":80}`)
	}
	for _, k := range []string{"x", "y", "w", "h"} {
		if !coordinate(box[k]) {
			return fmt.Errorf("%s must be a number within %d of 0", k, maxCoordinate)
		}
	}
	for _, k := range []string{"w", "h"} {
		if f, _ := box[k].(json.Number).Float64(); f < 0 {
			return fmt.Errorf("%s must not be negative", k)
		}
	}
	return nil
}

// maxPoints bounds a pen stroke's points.
const maxPoints = 5_000

var pointsSchema = []string{"type: array", "maxItems: 5000", "items:", "  type: array", "  minItems: 2", "  maxItems: 2", "  items: { type: number }"}

// pointsShape: a list of at most maxPoints [x, y] pairs of numbers.
func pointsShape(v any) error {
	list, ok := v.([]any)
	if !ok {
		return errors.New("must be a list of [x, y] points")
	}
	if len(list) > maxPoints {
		return fmt.Errorf("holds %d points at most", maxPoints)
	}
	for i, p := range list {
		pair, ok := p.([]any)
		if !ok || len(pair) != 2 || !coordinate(pair[0]) || !coordinate(pair[1]) {
			return fmt.Errorf("point %d must be [x, y], two numbers", i+1)
		}
	}
	return nil
}

// drawingStyleChoices are the parts of a shape's style that take one of a
// few words; the colors are names the app reads, any string to it.
var drawingStyleChoices = map[string][]string{
	"size":     {"s", "m", "l"},
	"route":    {"straight", "elbow", "curved"},
	"start":    {"none", "arrow"},
	"end":      {"none", "arrow"},
	"dash":     {"solid", "dashed", "dotted"},
	"fromSide": {"top", "right", "bottom", "left"},
	"toSide":   {"top", "right", "bottom", "left"},
}

var drawingStyleSchema = []string{"type: object", "properties:",
	"  fill: { type: string }", "  stroke: { type: string }",
	"  rounded: { type: boolean }", "  bold: { type: boolean }", "  italic: { type: boolean }",
	"  size: { type: string, enum: [s, m, l] }", "  route: { type: string, enum: [straight, elbow, curved] }",
	"  start: { type: string, enum: [none, arrow] }", "  end: { type: string, enum: [none, arrow] }",
	"  dash: { type: string, enum: [solid, dashed, dotted] }",
	"  fromSide: { type: string, enum: [top, right, bottom, left] }", "  toSide: { type: string, enum: [top, right, bottom, left] }"}

// drawingStyleShape: an object whose known parts have their types; the
// rest is left for a newer app.
func drawingStyleShape(v any) error {
	style, ok := v.(map[string]any)
	if !ok {
		return errors.New("must be an object")
	}
	for name, part := range style {
		switch name {
		case "fill", "stroke":
			if s, ok := part.(string); !ok || len(s) > 40 {
				return fmt.Errorf("%s must be a color's name", name)
			}
		case "rounded", "bold", "italic":
			if _, ok := part.(bool); !ok {
				return fmt.Errorf("%s must be true or false", name)
			}
		default:
			if choices, known := drawingStyleChoices[name]; known {
				if s, ok := part.(string); !ok || !slices.Contains(choices, s) {
					return fmt.Errorf("%s must be one of %s", name, strings.Join(choices, ", "))
				}
			}
		}
	}
	return nil
}
