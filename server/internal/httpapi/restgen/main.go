// Command restgen writes the REST endpoints for synced rows from the
// entity registry (syncer.Describe): the marked regions of
// shared/openapi.yaml (tags, paths, parameters and schemas) and
// httpapi/rows_gen.go, the one-line methods that hand each operation to
// the generic handlers in rows.go. It runs before oapi-codegen (go
// generate in httpapi/api), so a new synced entity or field gets its
// endpoints from make generate. It fails if it would write a name twice
// or one the hand-written part of the spec already has.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/locodarwin/mila/server/internal/syncer"
)

const endMarker = "# <<< restgen"

func startMarker(region string) string {
	return "# >>> restgen: " + region + " (generated, do not edit)"
}

func main() {
	specPath := flag.String("spec", "../../../../shared/openapi.yaml", "the OpenAPI spec to write into")
	goPath := flag.String("go", "../rows_gen.go", "the Go methods to write")
	flag.Parse()
	log.SetFlags(0)

	raw, err := os.ReadFile(*specPath)
	if err != nil {
		log.Fatal(err)
	}
	spec := string(raw)
	hand := handWritten(spec)
	g := &gen{hand: hand, seen: map[string]bool{}}
	entities := syncer.Describe()
	regions := map[string]string{
		"tags":       g.tags(entities),
		"paths":      g.paths(entities),
		"parameters": g.parameters(entities),
		"schemas":    g.schemas(entities),
	}
	for name, body := range regions {
		if spec, err = replaceRegion(spec, name, body); err != nil {
			log.Fatal(err)
		}
	}
	if err := os.WriteFile(*specPath, []byte(spec), 0o644); err != nil {
		log.Fatal(err)
	}
	src, err := format.Source(g.methods(entities))
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*goPath, src, 0o644); err != nil {
		log.Fatal(err)
	}
}

// handWritten is the spec without its generated regions.
func handWritten(spec string) string {
	var b strings.Builder
	inside := false
	for _, line := range strings.SplitAfter(spec, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "# >>> restgen:"):
			inside = true
		case t == endMarker:
			inside = false
		case !inside:
			b.WriteString(line)
		}
	}
	return b.String()
}

// replaceRegion puts body between a region's markers, indented as the
// start marker is.
func replaceRegion(spec, region, body string) (string, error) {
	start := startMarker(region)
	i := strings.Index(spec, start)
	if i < 0 {
		return "", fmt.Errorf("the spec has no %q marker", start)
	}
	lineStart := strings.LastIndex(spec[:i], "\n") + 1
	indent := spec[lineStart:i]
	j := strings.Index(spec[i:], endMarker)
	if j < 0 {
		return "", fmt.Errorf("the %s region has no end marker", region)
	}
	j += i
	endLine := strings.LastIndex(spec[:j], "\n") + 1
	var b strings.Builder
	for _, line := range strings.SplitAfter(body, "\n") {
		if line == "" {
			continue
		}
		if strings.TrimSpace(line) == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(indent + line)
	}
	return spec[:i+len(start)] + "\n" + b.String() + spec[endLine:], nil
}

type gen struct {
	hand string
	seen map[string]bool
}

// claim fails if a generated name repeats, or is in the hand-written spec.
func (g *gen) claim(kind, name string, handPattern string) {
	key := kind + " " + name
	if g.seen[key] {
		log.Fatalf("restgen would write the %s %s twice", kind, name)
	}
	g.seen[key] = true
	if handPattern != "" && regexp.MustCompile(handPattern).MatchString(g.hand) {
		log.Fatalf("the %s %s is in the hand-written part of the spec already", kind, name)
	}
}

func q(s string) string { return strconv.Quote(s) }

// thousands writes 20000 as 20,000.
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func lowerFirst(s string) string { return strings.ToLower(s[:1]) + s[1:] }

// article is "a" or "an" before a word.
func article(word string) string {
	if strings.ContainsRune("aeiou", rune(word[0])) {
		return "an"
	}
	return "a"
}

func label(name string) string { return strings.ReplaceAll(name, "_", " ") }

func (g *gen) tags(entities []syncer.EntityInfo) string {
	var b strings.Builder
	for _, e := range entities {
		g.claim("tag", e.Collection, `(?m)^  - name: `+regexp.QuoteMeta(e.Collection)+`$`)
		fmt.Fprintf(&b, "- name: %s\n  description: %s\n", e.Collection, q(e.Doc))
	}
	return b.String()
}

// pathParam is the component parameter naming a row: the hand-written one
// when the spec has it already (TaskId), else a generated one.
func (g *gen) pathParam(e syncer.EntityInfo) string { return e.Type + "Id" }

func (g *gen) hasHandParam(name string) bool {
	return regexp.MustCompile(`(?m)^    ` + name + `:`).MatchString(g.hand)
}

// patchable are the fields a PATCH may set.
func patchable(e syncer.EntityInfo) []syncer.FieldInfo {
	var out []syncer.FieldInfo
	for _, f := range e.Fields {
		if !f.Immutable && !f.ServerSet {
			out = append(out, f)
		}
	}
	return out
}

// hasPatch says whether an entity gets PATCH: not when deleted_at is all
// it could change (a pair's label or assignee is added and removed).
func hasPatch(e syncer.EntityInfo) bool {
	p := patchable(e)
	return len(p) != 1 || p[0].Name != "deleted_at"
}

func ref(kind, name string) string {
	return fmt.Sprintf(`{ $ref: "#/components/%s/%s" }`, kind, name)
}

func (g *gen) paths(entities []syncer.EntityInfo) string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	errorResp := func() { w("      default: %s", ref("responses", "Error")) }
	jsonBody := func(status, desc, schema string, headers ...string) {
		w("      %q:", status)
		w("        description: %s", q(desc))
		if len(headers) > 0 {
			w("        headers:")
			for _, h := range headers {
				w("          %s: { schema: { type: string } }", h)
			}
		}
		w("        content:")
		w("          application/json:")
		w("            schema: %s", schema)
	}
	for _, e := range entities {
		one, many := label(e.Name), strings.ReplaceAll(e.Collection, "-", " ")
		list := "/workspaces/{workspaceId}/" + e.Collection
		item := list + "/{" + lowerFirst(e.Type) + "Id}"
		g.claim("path", list, `(?m)^  `+regexp.QuoteMeta(list)+`:`)
		g.claim("path", item, `(?m)^  `+regexp.QuoteMeta(item)+`:`)
		operation := func(method, id, summary, desc string) {
			g.claim("operationId", id, `operationId: `+id+`\b`)
			w("  %s:", method)
			w("    tags: [%s]", e.Collection)
			w("    operationId: %s", id)
			w("    summary: %s", q(summary))
			w("    description: %s", q(desc))
		}
		row := ref("schemas", e.Type)

		w("%s:", list)
		desc := fmt.Sprintf("The %s you can see, a page at a time in ID order (not the order they were made: IDs are random). Sort by sort_key, compared byte by byte, or by created_at yourself. Filters match a field exactly; an empty value matches null.", many)
		if e.Parent != "" {
			desc += " Without include_deleted, rows hanging off a deleted task are left out with it."
		}
		operation("get", "list"+e.Plural, "List "+many, desc)
		w("    parameters:")
		for _, p := range []string{"WorkspaceId", "ListAfter", "ListLimit", "IncludeDeleted"} {
			w("      - %s", ref("parameters", p))
		}
		for _, name := range e.Filters {
			d := "Only rows whose " + name + " is this"
			if field(e, name).Nullable {
				d += "; empty for none"
			}
			w("      - name: %s", name)
			w("        in: query")
			w("        description: %s", q(d+"."))
			w("        schema: { type: string }")
		}
		w("    responses:")
		jsonBody("200", "A page of "+many+".", ref("schemas", e.Type+"Page"))
		errorResp()
		if !e.ServerCreated {
			desc := fmt.Sprintf("Makes %s %s through the same merge as sync, so every device gets it. Fields left out take their defaults; sort_key, left out, puts it after every other %s. With an Idempotency-Key and no id, a retry names the same row and answers 200 with it.", article(one), one, one)
			if len(e.Pair) == 2 {
				desc = fmt.Sprintf("Adds %s %s. Its ID is made from %s and %s, so adding the same pair again answers 200 with the row it has, brought back if it was deleted.", article(one), one, e.Pair[0], e.Pair[1])
			}
			operation("post", "create"+e.Type, "Make "+article(one)+" "+one, desc)
			w("    parameters:")
			w("      - %s", ref("parameters", "WorkspaceId"))
			w("      - %s", ref("parameters", "IdempotencyKey"))
			w("    requestBody:")
			w("      required: true")
			w("      content:")
			w("        application/json:")
			w("          schema: %s", ref("schemas", e.Type+"Create"))
			w("    responses:")
			jsonBody("201", "Made.", row, "Location", "ETag")
			jsonBody("200", "The row was there already (a pair, or a retry with the same Idempotency-Key).", row, "ETag")
			errorResp()
		}

		w("%s:", item)
		itemParams := func(header string) {
			w("    parameters:")
			w("      - %s", ref("parameters", "WorkspaceId"))
			w("      - %s", ref("parameters", g.pathParam(e)))
			w("      - %s", ref("parameters", header))
		}
		operation("get", "get"+e.Type, "Get "+article(one)+" "+one,
			fmt.Sprintf("One %s. A deleted one comes back with deleted_at set until it is purged.", one))
		itemParams("IfNoneMatch")
		w("    responses:")
		jsonBody("200", "OK", row, "ETag")
		w("      \"304\":")
		w("        description: Not modified since the ETag given in If-None-Match.")
		errorResp()
		if hasPatch(e) {
			desc := "Sets the fields given and leaves the rest. Each field is last writer wins, as on every device; send If-Match with the ETag you read to change the row only if no one has since. deleted_at: null brings a deleted one back."
			if e.Name == "task" {
				desc += " Moving a task to another project takes project_id and status_id together (one of the new project's statuses, or null), and then the same for each of its subtasks, which must stay in their parent's project."
			}
			operation("patch", "update"+e.Type, "Change "+article(one)+" "+one, desc)
			itemParams("IfMatch")
			w("    requestBody:")
			w("      required: true")
			w("      content:")
			w("        application/json:")
			w("          schema: %s", ref("schemas", e.Type+"Patch"))
			w("    responses:")
			jsonBody("200", "The row as it stands now.", row, "ETag")
			errorResp()
		}
		desc = "Sets deleted_at. Deleting what is deleted already changes nothing."
		if e.Name == "task" || e.Name == "project" {
			desc += " It stays in the trash for 30 days, and PATCH deleted_at: null brings it back."
		}
		operation("delete", "delete"+e.Type, "Delete "+article(one)+" "+one, desc)
		itemParams("IfMatch")
		w("    responses:")
		w("      \"204\":")
		w("        description: Deleted.")
		errorResp()
		w("")
	}
	return b.String()
}

func field(e syncer.EntityInfo, name string) syncer.FieldInfo {
	for _, f := range e.Fields {
		if f.Name == name {
			return f
		}
	}
	log.Fatalf("%s has no field %s", e.Name, name)
	return syncer.FieldInfo{}
}

func (g *gen) parameters(entities []syncer.EntityInfo) string {
	var b strings.Builder
	shared := []struct{ name, def string }{
		{"ListAfter", `{ name: after, in: query, description: "The ID of the last row of the page before; the next field of that page.", schema: { type: string } }`},
		{"ListLimit", `{ name: limit, in: query, description: "How many rows, from 1 to 500.", schema: { type: integer, minimum: 1, maximum: 500, default: 100 } }`},
		{"IncludeDeleted", `{ name: include_deleted, in: query, description: "true to list deleted rows too, which the trash still holds.", schema: { type: boolean, default: false } }`},
		{"IfMatch", `{ name: If-Match, in: header, description: "An ETag from a GET, POST or PATCH: change the row only if it is unchanged since. A mismatch answers 412.", schema: { type: string } }`},
		{"IfNoneMatch", `{ name: If-None-Match, in: header, description: "An ETag you hold: answers 304 if the row is unchanged.", schema: { type: string } }`},
		{"IdempotencyKey", `{ name: Idempotency-Key, in: header, description: "Any string of yours: sending the same request with the same key again (a retry) does not make or change it twice. Kept for 90 days.", schema: { type: string, maxLength: 200 } }`},
	}
	for _, p := range shared {
		g.claim("parameter", p.name, `(?m)^    `+p.name+`:`)
		fmt.Fprintf(&b, "%s: %s\n", p.name, p.def)
	}
	for _, e := range entities {
		name := g.pathParam(e)
		if g.hasHandParam(name) {
			continue // TaskId, ProjectId and the like are shared with older paths
		}
		g.claim("parameter", name, "")
		fmt.Fprintf(&b, "%s: { name: %sId, in: path, required: true, schema: { type: string } }\n", name, lowerFirst(e.Type))
	}
	return b.String()
}

// property writes one field's schema.
func property(b *bytes.Buffer, e syncer.EntityInfo, f syncer.FieldInfo, indent string, withDefault, readOnly bool) {
	p := func(format string, args ...any) { fmt.Fprintf(b, indent+"  "+format+"\n", args...) }
	fmt.Fprintf(b, "%s%s:\n", indent, f.Name)
	desc := f.Doc
	switch f.Kind {
	case "string", "text":
		p("type: string")
		if f.Min > 0 {
			p("minLength: %d", f.Min)
		}
		if f.Max > 0 {
			p("maxLength: %d", f.Max)
		}
	case "enum":
		p("type: string")
		p("enum: [%s]", strings.Join(f.Enum, ", "))
	case "time":
		p("type: string")
		p("format: date-time")
	case "ref":
		p("type: string")
		if desc == "" {
			desc = fmt.Sprintf("The ID of %s %s in this workspace.", article(label(f.Ref)), label(f.Ref))
		}
	case "bool":
		p("type: boolean")
	case "int":
		p("type: integer")
		p("format: int64")
		p("minimum: 0")
		if f.Max > 0 {
			p("maximum: %d", f.Max)
		}
	case "date":
		p("type: string")
		p("format: date")
	case "clock":
		p("type: string")
		p("pattern: %s", q(`^([01]\d|2[0-3]):[0-5]\d$`))
		if desc == "" {
			desc = "A time of day, 24-hour, such as 14:30."
		}
	case "self":
		p("type: string")
		if desc == "" {
			desc = "The ID of the user who made it: you. Left out, it is filled in."
			if !f.Required {
				desc = "The ID of the user who made it, or null for rows from before it was kept. Left out of a create, it is you."
			}
			if e.Private == f.Name {
				desc = "The ID of the one user who sees it: you. Null makes it shared."
				if !f.Nullable {
					desc = "The ID of the one user who sees it: you."
				}
			}
		}
	case "member":
		p("type: string")
		if desc == "" {
			desc = "The ID of a member of this workspace."
		}
	case "json":
		// Its shape from the registry, or any JSON value.
		for _, line := range f.Schema {
			p("%s", line)
		}
		if desc == "" {
			desc = "Any JSON value."
		}
		if f.Max > 0 {
			desc = strings.TrimSpace(desc + fmt.Sprintf(" At most %s bytes as compact JSON.", thousands(f.Max)))
		}
	default:
		log.Fatalf("restgen does not know the field kind %q (%s.%s)", f.Kind, e.Name, f.Name)
	}
	if f.Nullable {
		p("nullable: true")
	}
	if f.Immutable && !readOnly {
		desc = strings.TrimSpace(desc + " Set when the row is made; it never changes.")
	}
	if readOnly {
		p("readOnly: true")
	}
	// A create fills in its maker rather than a default.
	if withDefault && f.HasDefault && (f.Kind != "self" || e.Private == f.Name) {
		raw, err := json.Marshal(f.Default)
		if err != nil {
			log.Fatal(err)
		}
		p("default: %s", raw)
	}
	if desc != "" {
		p("description: %s", q(desc))
	}
}

func (g *gen) schemas(entities []syncer.EntityInfo) string {
	var b bytes.Buffer
	schema := func(name string) {
		g.claim("schema", name, `(?m)^    `+name+`:`)
		fmt.Fprintf(&b, "%s:\n", name)
	}
	for _, e := range entities {
		// The row.
		schema(e.Type)
		fmt.Fprintf(&b, "  description: %s\n  type: object\n", q(e.Doc))
		required := []string{"id"}
		for _, f := range e.Fields {
			required = append(required, f.Name)
		}
		required = append(required, "created_at", "updated_at")
		fmt.Fprintf(&b, "  required: [%s]\n  properties:\n", strings.Join(required, ", "))
		b.WriteString("    id: { type: string, readOnly: true }\n")
		for _, f := range e.Fields {
			property(&b, e, f, "    ", false, f.ServerSet)
		}
		b.WriteString("    created_at: { type: string, format: date-time, readOnly: true }\n")
		b.WriteString("    updated_at: { type: string, format: date-time, readOnly: true }\n")

		// A page of rows.
		schema(e.Type + "Page")
		fmt.Fprintf(&b, "  type: object\n  required: [items]\n  properties:\n    items:\n      type: array\n      items: %s\n", ref("schemas", e.Type))
		b.WriteString("    next:\n      type: string\n      description: \"Pass as after for the next page; absent when this page came back short.\"\n")

		// What a create takes.
		if !e.ServerCreated {
			schema(e.Type + "Create")
			b.WriteString("  type: object\n  additionalProperties: false\n")
			var req []string
			for _, f := range e.Fields {
				if f.Required && f.Kind != "self" && f.Name != "sort_key" && !f.ServerSet {
					req = append(req, f.Name)
				}
			}
			fmt.Fprintf(&b, "  required: [%s]\n  properties:\n", strings.Join(req, ", "))
			idDoc := "A new ID of yours (letters, digits and dashes, at most 64), or left out for one the server makes. A UUID should be drawn (versions 1 to 7 of RFC 9562): one of the shapes Mila works IDs out in (version 8, or another variant) is refused unless it is the one Mila would work out for the row."
			if len(e.Pair) == 2 {
				idDoc = fmt.Sprintf("Made from %s and %s; leave it out.", e.Pair[0], e.Pair[1])
			}
			fmt.Fprintf(&b, "    id:\n      type: string\n      description: %s\n", q(idDoc))
			for _, f := range e.Fields {
				if !f.ServerSet {
					property(&b, e, f, "    ", true, false)
				}
			}
		}

		// What a PATCH takes.
		if hasPatch(e) {
			schema(e.Type + "Patch")
			b.WriteString("  type: object\n  additionalProperties: false\n  minProperties: 1\n  properties:\n")
			for _, f := range patchable(e) {
				property(&b, e, f, "    ", false, false)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// methods is rows_gen.go: each operation handed to rows.go.
func (g *gen) methods(entities []syncer.EntityInfo) []byte {
	var b bytes.Buffer
	b.WriteString(`// Code generated by httpapi/restgen from the entity registry. DO NOT EDIT.

package httpapi

import (
	"net/http"

	"github.com/locodarwin/mila/server/internal/httpapi/api"
)
`)
	for _, e := range entities {
		idType := "api." + g.pathParam(e)
		fmt.Fprintf(&b, "\n// %s\n\n", e.Plural)
		fmt.Fprintf(&b, "func (h *handlers) List%s(w http.ResponseWriter, r *http.Request, ws api.WorkspaceId, _ api.List%sParams) {\n\th.listRows(w, r, %q, ws)\n}\n\n", e.Plural, e.Plural, e.Name)
		if !e.ServerCreated {
			mode := "syncer.Create"
			if len(e.Pair) == 2 {
				mode = "syncer.Upsert"
			}
			fmt.Fprintf(&b, "func (h *handlers) Create%s(w http.ResponseWriter, r *http.Request, ws api.WorkspaceId, _ api.Create%sParams) {\n\th.createRow(w, r, %q, ws, %s)\n}\n\n", e.Type, e.Type, e.Name, mode)
		}
		fmt.Fprintf(&b, "func (h *handlers) Get%s(w http.ResponseWriter, r *http.Request, ws api.WorkspaceId, id %s, _ api.Get%sParams) {\n\th.getRow(w, r, %q, ws, id)\n}\n\n", e.Type, idType, e.Type, e.Name)
		if hasPatch(e) {
			fmt.Fprintf(&b, "func (h *handlers) Update%s(w http.ResponseWriter, r *http.Request, ws api.WorkspaceId, id %s, _ api.Update%sParams) {\n\th.patchRow(w, r, %q, ws, id)\n}\n\n", e.Type, idType, e.Type, e.Name)
		}
		fmt.Fprintf(&b, "func (h *handlers) Delete%s(w http.ResponseWriter, r *http.Request, ws api.WorkspaceId, id %s, _ api.Delete%sParams) {\n\th.deleteRow(w, r, %q, ws, id)\n}\n", e.Type, idType, e.Type, e.Name)
	}
	out := b.Bytes()
	if bytes.Contains(out, []byte("syncer.")) {
		out = bytes.Replace(out, []byte(`"github.com/locodarwin/mila/server/internal/httpapi/api"`),
			[]byte("\"github.com/locodarwin/mila/server/internal/httpapi/api\"\n\t\"github.com/locodarwin/mila/server/internal/syncer\""), 1)
	}
	return out
}
