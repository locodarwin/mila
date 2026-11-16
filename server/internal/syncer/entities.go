package syncer

import (
	"errors"
	"strings"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/recur"
)

// The synced entities and their fields. The table drives validation, the
// row SQL and the merge, so adding a field means adding it here, in a
// migration for both dialects, and in web/src/lib/sync/entities.ts.

// Schema is the version of the synced entities, which every push names:
// a device running another version is refused with "update required"
// before any of its ops is applied, and keeps them until it has
// reloaded. Raise it, with SYNC_SCHEMA in web/src/lib/sync/entities.ts,
// whenever ops from an older app would be refused or misread (a new
// required field, a renamed or removed one), and add the step in
// web/src/lib/sync/outbox.ts that rewrites the edits an older app kept.
// TestSchemaFingerprint fails when the fields change, as a reminder to
// decide. 5 made a pair's ID a hash of the pair (ids.Pair) instead of the
// two IDs XORed: an app of 4 would make pair rows under IDs refused now.
const Schema = 6

type kind int

const (
	kString kind = iota // one line, trimmed, length-limited
	kText               // free text, length-limited
	kEnum
	kTime   // RFC 3339 in JSON, a timestamp in the database
	kRef    // the ID of another entity in the same workspace
	kBool   // true or false
	kInt    // a whole number
	kDate   // a calendar date, "2026-10-28"
	kClock  // a time of day, "14:30"
	kSelf   // the user making the change; set once, by them
	kMember // the ID of a member of the workspace
	kJSON   // any JSON value, stored as text, length-limited
)

type field struct {
	name      string
	kind      kind
	required  bool // must be given when the row is created
	nullable  bool
	immutable bool // set at creation, never changed
	serverSet bool // only the server sets it
	max       int  // characters, for strings and text; the largest kInt
	min       int
	enum      []string
	ref       string // entity a kRef points at
	// check further validates a string field's value (a repeat rule).
	check func(string) error
	// shape checks a kJSON field's value, decoded with UseNumber, for the
	// JSON types the app reads (shapes.go); schema is that shape for the
	// API reference, as lines of OpenAPI under the property.
	shape  func(any) error
	schema []string
	// firstWins: on a row that exists, a write to this immutable field
	// with an older clock is kept instead of refused, when it comes from
	// another making of the row (madeTogether). Two people can make the
	// same row at once (the next task of a series, which has a derived
	// ID), and whoever made it first by the clock is its creator on every
	// device, whichever push arrives first. Any other older write is
	// refused, and a newer one is dropped. A row that predates the field
	// keeps it empty.
	firstWins bool
	// def is the value a REST create fills in when the field is not given,
	// the same as the migration's DEFAULT and DEFAULTS in entities.ts. A
	// nullable field's is null without saying so; required fields have
	// none. It is not part of what Fingerprint sums up.
	def any
	// doc says what the field means where its name does not, for the API
	// reference.
	doc string
}

type entity struct {
	name   string
	table  string
	fields []field
	// projectOf names the field holding the row's project, or "id" for a
	// project itself; it decides what guests can see.
	projectOf string
	// guests may write rows of this entity inside projects they can see.
	guestWritable bool
	// parent names the field holding the task a row belongs to; the row
	// is in the task's project and its history is the task's.
	parent string
	// parentEntity is what parent names when it is not a task: a page,
	// for the rows of a page's document.
	parentEntity string
	// workspaceWide rows (labels) are seen by every member, guests too.
	workspaceWide bool
	// owner names the field holding the user who alone may change the row
	// (a comment's author); admins may still delete it.
	owner string
	// serverCreated rows (attachments) are made only by the server.
	serverCreated bool
	// private names the field holding the user who alone sees the row
	// (My Day entries); a row whose field is null is not private (a
	// shared saved view).
	private string
	// wideWhenNull rows with no project belong to the whole workspace
	// (a custom field for every project), not the inbox.
	wideWhenNull bool
	// quiet rows leave no history of their own: a document's updates,
	// whose notes have it (docs.go).
	quiet bool
	// membersOnly rows (templates) belong to the workspace and are never
	// seen or written by guests.
	membersOnly bool
	// adminsEdit: admins may change an owner's rows too (time logged),
	// not only delete them.
	adminsEdit bool
	// adminsOnly rows (workspace themes) are made and changed by owners
	// and admins alone; everyone else only reads them.
	adminsOnly bool
	// alsoIn names a second task field: the row is in that task's
	// project too, and a guest sees it only when they see both (a
	// dependency between projects).
	alsoIn string
	// internal rows sync to devices but are not offered by the REST API:
	// a document's Yjs updates, which the API reads and writes as Markdown
	// through the notes.
	internal bool
}

// pageBodyDoc says what a page's body is, for the API reference.
const pageBodyDoc = "The page as Markdown. The page itself is a live document that merges what everyone types; the server writes it here as Markdown after every change, and a write here becomes a change to it that replaces only the blocks that differ."

// themeBodyDoc says what a workspace theme's body is.
const themeBodyDoc = "The theme document, as GET /me/themes gives a person's own: a built-in base and the tokens it changes. It is checked as strictly as a person's own theme is; its id and name are the app's to replace with the row's."

// MaxThemeBytes bounds a workspace theme's body, as a person's own.
const MaxThemeBytes = accounts.MaxThemeBytes

// sortKeyDoc explains every sort_key.
const sortKeyDoc = "Orders rows in their list: a fractional key (web/src/lib/sync/sortkey.ts) compared byte by byte. On create, leave it out to put the row after every other row of its kind."

var palette = []string{"red", "orange", "amber", "yellow", "lime", "green", "teal", "cyan", "blue", "indigo", "purple", "pink"}

var statusCategories = []string{"backlog", "todo", "in-progress", "done", "canceled"}

var priorities = []string{"none", "low", "medium", "high", "urgent"}

var entities = map[string]*entity{
	"project": {
		name: "project", table: "projects", projectOf: "id",
		fields: []field{
			{name: "name", kind: kString, required: true, min: 1, max: 200},
			{name: "icon", kind: kString, max: 32, def: ""},
			{name: "color", kind: kEnum, enum: palette, def: "blue"},
			{name: "description", kind: kText, max: 10_000, def: ""},
			{name: "sort_key", kind: kString, required: true, min: 1, max: 64, check: validateKey, doc: sortKeyDoc},
			{name: "archived_at", kind: kTime, nullable: true},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	},
	"status": {
		name: "status", table: "statuses", projectOf: "project_id",
		fields: []field{
			{name: "project_id", kind: kRef, ref: "project", required: true, immutable: true},
			{name: "name", kind: kString, required: true, min: 1, max: 60},
			{name: "category", kind: kEnum, required: true, enum: statusCategories},
			{name: "sort_key", kind: kString, required: true, min: 1, max: 64, check: validateKey, doc: sortKeyDoc},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	},
	"task": {
		name: "task", table: "tasks", projectOf: "project_id", guestWritable: true,
		fields: []field{
			{name: "project_id", kind: kRef, ref: "project", nullable: true},
			{name: "status_id", kind: kRef, ref: "status", nullable: true},
			{name: "title", kind: kString, required: true, min: 1, max: 500},
			{name: "notes", kind: kText, max: 100_000, def: ""},
			{name: "sort_key", kind: kString, required: true, min: 1, max: 64, check: validateKey, doc: sortKeyDoc},
			{name: "completed_at", kind: kTime, nullable: true},
			{name: "deleted_at", kind: kTime, nullable: true},
			{name: "parent_id", kind: kRef, ref: "task", nullable: true},
			{name: "priority", kind: kEnum, enum: priorities, def: "none"},
			{name: "start_date", kind: kDate, nullable: true},
			{name: "due_date", kind: kDate, nullable: true},
			{name: "due_time", kind: kClock, nullable: true},
			{name: "created_by", kind: kSelf, nullable: true, immutable: true, firstWins: true},
			{name: "recurrence", kind: kString, nullable: true, max: 200, check: recur.Validate,
				doc: "How the task repeats: an RFC 5545 RRULE without the prefix, such as FREQ=WEEKLY;BYDAY=MO,TH. Null for a task that does not repeat."},
			{name: "recurrence_mode", kind: kEnum, enum: []string{recur.Schedule, recur.Completion}, def: recur.Schedule,
				doc: "schedule: the next task falls on the rule's next date; completion: it is counted from the day this one is completed."},
			// The first task of the series; not a reference, since that task
			// may be long gone from the trash.
			{name: "series_id", kind: kString, nullable: true, max: 64, check: validID,
				doc: "The ID of the first task of a repeating series, which may be long gone from the trash; null on the first task itself."},
			{name: "estimate_minutes", kind: kInt, nullable: true, max: MaxEstimate,
				doc: "How long the task's own work should take, in minutes; its subtasks have their own. Null for no estimate."},
			// What reports read for trends (docs/PLAN.md section 3, Release
			// 3.0, Reporting). The server fills both in (fillTaskDates).
			{name: "made_at", kind: kTime, nullable: true, immutable: true,
				doc: "When the task was made. The server fills it in from the edit that makes the task; an import may give the original time instead. Null only for a task whose history was lost."},
			{name: "started_at", kind: kTime, nullable: true, serverSet: true,
				doc: "When the task first went to a status of kind in-progress, set by the server; null if it never has."},
		},
	},
	"label": {
		name: "label", table: "labels", workspaceWide: true,
		fields: []field{
			{name: "name", kind: kString, required: true, min: 1, max: 60},
			{name: "color", kind: kEnum, enum: palette, def: "blue"},
			{name: "sort_key", kind: kString, required: true, min: 1, max: 64, check: validateKey, doc: sortKeyDoc},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	},
	"task_label": {
		name: "task_label", table: "task_labels", parent: "task_id", guestWritable: true,
		fields: []field{
			{name: "task_id", kind: kRef, ref: "task", required: true, immutable: true},
			{name: "label_id", kind: kRef, ref: "label", required: true, immutable: true},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	},
	"task_dependency": {
		name: "task_dependency", table: "task_dependencies", parent: "task_id", alsoIn: "blocker_id", guestWritable: true,
		fields: []field{
			{name: "task_id", kind: kRef, ref: "task", required: true, immutable: true, doc: "The task that waits."},
			{name: "blocker_id", kind: kRef, ref: "task", required: true, immutable: true, doc: "The task it waits for: it can start once this one is done."},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	},
	"task_assignee": {
		name: "task_assignee", table: "task_assignees", parent: "task_id", guestWritable: true,
		fields: []field{
			{name: "task_id", kind: kRef, ref: "task", required: true, immutable: true},
			{name: "user_id", kind: kMember, required: true, immutable: true},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	},
	"checklist_item": {
		name: "checklist_item", table: "checklist_items", parent: "task_id", guestWritable: true,
		fields: []field{
			{name: "task_id", kind: kRef, ref: "task", required: true, immutable: true},
			{name: "text", kind: kString, required: true, min: 1, max: 500},
			{name: "done", kind: kBool, def: false},
			{name: "sort_key", kind: kString, required: true, min: 1, max: 64, check: validateKey, doc: sortKeyDoc},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	},
	"comment": {
		name: "comment", table: "comments", parent: "task_id", guestWritable: true, owner: "author_id",
		fields: []field{
			{name: "task_id", kind: kRef, ref: "task", required: true, immutable: true},
			{name: "author_id", kind: kSelf, required: true, immutable: true},
			{name: "body", kind: kText, required: true, min: 1, max: 20_000},
			{name: "edited_at", kind: kTime, nullable: true},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	},
	"doc_update": {
		name: "doc_update", table: "doc_updates", parent: "task_id", guestWritable: true, quiet: true, internal: true,
		fields: []field{
			{name: "task_id", kind: kRef, ref: "task", required: true, immutable: true},
			// One Yjs update (version 1) to the task's notes, as base64. About
			// 3 MB at most, which a document merged into one row stays under.
			{name: "data", kind: kText, required: true, immutable: true, min: 1, max: 4_000_000, check: validUpdate},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	},
	"page": {
		name: "page", table: "pages", projectOf: "project_id", guestWritable: true,
		fields: []field{
			// Fixed: a page whose project changed would reach people who
			// never had the rest of its wiki. Null is the workspace's wiki,
			// which guests never see.
			{name: "project_id", kind: kRef, ref: "project", nullable: true, immutable: true},
			// Null only for a wiki's main page (pages.go).
			{name: "parent_id", kind: kRef, ref: "page", nullable: true},
			{name: "title", kind: kString, max: 200, def: ""},
			{name: "body", kind: kText, max: 1_000_000, def: "", doc: pageBodyDoc},
			{name: "sort_key", kind: kString, required: true, min: 1, max: 64, check: validateKey, doc: sortKeyDoc},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	},
	"page_update": {
		name: "page_update", table: "page_updates", parent: "page_id", parentEntity: "page", guestWritable: true, quiet: true, internal: true,
		fields: []field{
			{name: "page_id", kind: kRef, ref: "page", required: true, immutable: true},
			// One Yjs update to the page's body, as a doc_update's data is.
			{name: "data", kind: kText, required: true, immutable: true, min: 1, max: 4_000_000, check: validUpdate},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	},
	// Comments pinned to text in a page. A thread is where they are pinned and
	// whether they are resolved; anyone who can comment may change that.
	"page_thread": {
		name: "page_thread", table: "page_threads", parent: "page_id", parentEntity: "page", guestWritable: true,
		fields: []field{
			{name: "page_id", kind: kRef, ref: "page", required: true, immutable: true},
			{name: "created_by", kind: kSelf, required: true, immutable: true},
			{name: "quote", kind: kText, required: true, immutable: true, min: 1, max: 2_000,
				doc: "The words the thread was pinned to when it was made. It goes back to them if its pin stops resolving."},
			{name: "anchor", kind: kString, immutable: true, max: 2_000, def: "",
				doc: "Where the thread is pinned: positions in the page's live document, which only the app reads. Empty pins it by its quote."},
			{name: "resolved_at", kind: kTime, nullable: true},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	},
	"page_comment": {
		name: "page_comment", table: "page_comments", parent: "page_id", parentEntity: "page", guestWritable: true, owner: "author_id",
		fields: []field{
			{name: "page_id", kind: kRef, ref: "page", required: true, immutable: true},
			{name: "thread_id", kind: kRef, ref: "page_thread", required: true, immutable: true},
			{name: "author_id", kind: kSelf, required: true, immutable: true},
			{name: "body", kind: kText, required: true, min: 1, max: 20_000},
			{name: "edited_at", kind: kTime, nullable: true},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	},
	"attachment": {
		name: "attachment", table: "attachments", parent: "task_id", guestWritable: true, serverCreated: true,
		fields: []field{
			{name: "task_id", kind: kRef, ref: "task", required: true, immutable: true},
			{name: "filename", kind: kString, required: true, min: 1, max: 255},
			{name: "mime", kind: kString, required: true, serverSet: true, min: 1, max: 255},
			{name: "size", kind: kInt, required: true, serverSet: true},
			{name: "storage_key", kind: kString, required: true, serverSet: true, min: 1, max: 255},
			{name: "uploaded_by", kind: kSelf, required: true, immutable: true},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	},
	// A file on a wiki page, as an attachment is on a task.
	"page_attachment": {
		name: "page_attachment", table: "page_attachments", parent: "page_id", parentEntity: "page", guestWritable: true, serverCreated: true,
		fields: []field{
			{name: "page_id", kind: kRef, ref: "page", required: true, immutable: true},
			{name: "filename", kind: kString, required: true, min: 1, max: 255},
			{name: "mime", kind: kString, required: true, serverSet: true, min: 1, max: 255},
			{name: "size", kind: kInt, required: true, serverSet: true},
			{name: "storage_key", kind: kString, required: true, serverSet: true, min: 1, max: 255},
			{name: "uploaded_by", kind: kSelf, required: true, immutable: true},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	},
}

func init() {
	entities["my_day_entry"] = &entity{
		name: "my_day_entry", table: "my_day_entries", parent: "task_id", guestWritable: true,
		owner: "user_id", private: "user_id",
		fields: []field{
			{name: "task_id", kind: kRef, ref: "task", required: true, immutable: true},
			{name: "user_id", kind: kSelf, required: true, immutable: true},
			{name: "day", kind: kDate, required: true},
			{name: "sort_key", kind: kString, required: true, min: 1, max: 64, check: validateKey, doc: sortKeyDoc},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	}
	// A person's reminder on a task, which only they see. A reminder made
	// relative to the due moment keeps offset_minutes, and the server fires
	// it at the due moment less the offset whatever remind_at says, so it
	// follows the task when anyone reschedules it.
	entities["reminder"] = &entity{
		name: "reminder", table: "reminders", parent: "task_id", guestWritable: true,
		owner: "user_id", private: "user_id",
		fields: []field{
			{name: "task_id", kind: kRef, ref: "task", required: true, immutable: true},
			{name: "user_id", kind: kSelf, required: true, immutable: true},
			{name: "remind_at", kind: kTime, required: true},
			{name: "offset_minutes", kind: kInt, nullable: true, max: MaxReminderOffset,
				doc: "For a reminder relative to the task's due moment, the minutes before it. The server fires it then, whatever remind_at says, so it follows the task when it is rescheduled."},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	}
}

func init() {
	// Time logged on a task (docs/PLAN.md section 3): typed in, or a
	// timer. Its person edits it, and so do admins.
	entities["time_entry"] = &entity{
		name: "time_entry", table: "time_entries", parent: "task_id", guestWritable: true,
		owner: "user_id", adminsEdit: true,
		fields: []field{
			{name: "task_id", kind: kRef, ref: "task", required: true, immutable: true},
			{name: "user_id", kind: kSelf, required: true, immutable: true, doc: "Whose time it is."},
			{name: "day", kind: kDate, required: true, doc: "The day it counts toward, in its person's own calendar; a timer counts toward the day it started."},
			{name: "minutes", kind: kInt, max: MaxEntryMinutes, def: 0, doc: "How long, in whole minutes. 0 while a timer runs."},
			{name: "note", kind: kString, max: 500, def: ""},
			{name: "started_at", kind: kTime, nullable: true, doc: "When a timer started; null for time typed in."},
			{name: "running", kind: kBool, def: false, doc: "True while a timer runs. A person has one running at a time: starting another stops it."},
			{name: "active_at", kind: kTime, nullable: true, doc: "While a timer runs, when its person last did something in Mila, so a timer left running can be stopped then."},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	}
}

func init() {
	// Time logged to a project itself, not to one of its tasks (docs/PLAN.md
	// section 3): a meeting, planning, travel. Typed in, or a timer, as a
	// task's. Who sees it follows the project; its person edits it, and so
	// do admins.
	entities["project_time_entry"] = &entity{
		name: "project_time_entry", table: "project_time_entries", projectOf: "project_id", guestWritable: true,
		owner: "user_id", adminsEdit: true,
		fields: []field{
			// Fixed: a row that changed project would reach people who never had it.
			{name: "project_id", kind: kRef, ref: "project", required: true, immutable: true},
			{name: "user_id", kind: kSelf, required: true, immutable: true, doc: "Whose time it is."},
			{name: "day", kind: kDate, required: true, doc: "The day it counts toward, in its person's own calendar; a timer counts toward the day it started."},
			{name: "minutes", kind: kInt, max: MaxEntryMinutes, def: 0, doc: "How long, in whole minutes. 0 while a timer runs."},
			{name: "note", kind: kString, max: 500, def: ""},
			{name: "started_at", kind: kTime, nullable: true, doc: "When a timer started; null for time typed in."},
			{name: "running", kind: kBool, def: false, doc: "True while a timer runs. A person has one running at a time, on a task or a project: starting another stops it."},
			{name: "active_at", kind: kTime, nullable: true, doc: "While a timer runs, when its person last did something in Mila, so a timer left running can be stopped then."},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	}
}

// MaxEstimate is the largest estimate, in minutes: 999 hours.
const MaxEstimate = 999 * 60

// MaxEntryMinutes is the most one entry of logged time holds: a day.
const MaxEntryMinutes = 24 * 60

// MaxReminderOffset is the longest a reminder can come before the due
// moment, in minutes: a year.
const MaxReminderOffset = 366 * 24 * 60

func init() {
	entities["custom_field"] = &entity{
		name: "custom_field", table: "custom_fields", projectOf: "project_id", wideWhenNull: true,
		fields: []field{
			{name: "project_id", kind: kRef, ref: "project", nullable: true, immutable: true},
			{name: "name", kind: kString, required: true, min: 1, max: 60},
			{name: "type", kind: kEnum, required: true, immutable: true,
				enum: []string{"text", "number", "date", "select", "multi_select", "checkbox", "url", "user"}},
			{name: "options", kind: kJSON, max: 10_000, def: []any{}, shape: optionsShape,
				schema: []string{"type: array", "items:", "  type: object", "  required: [id, name, color]", "  properties:",
					"    id: { type: string, minLength: 1, maxLength: 64, description: \"Unique within the field; a task's value names it.\" }",
					"    name: { type: string }",
					"    color: { type: string, enum: [" + strings.Join(palette, ", ") + "] }"},
				doc: "A select or multi_select field's choices, each with an id of its own, a name and a color. Empty for other types."},
			{name: "sort_key", kind: kString, required: true, min: 1, max: 64, check: validateKey, doc: sortKeyDoc},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	}
	entities["custom_field_value"] = &entity{
		name: "custom_field_value", table: "custom_field_values", parent: "task_id", guestWritable: true,
		fields: []field{
			{name: "task_id", kind: kRef, ref: "task", required: true, immutable: true},
			{name: "field_id", kind: kRef, ref: "custom_field", required: true, immutable: true},
			{name: "value", kind: kJSON, required: true, max: 10_000,
				schema: []string{"oneOf:", "  - { type: string }", "  - { type: number }", "  - { type: boolean }", "  - { type: array, items: { type: string } }"},
				doc:    "The task's value for the field, as its type has it: text and url a string, number a number, date a date string such as \"2026-10-28\", select a choice's id, multi_select an array of choice ids, checkbox true or false, user a member's ID. To clear a value, delete the row."},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	}
	entities["template"] = &entity{
		name: "template", table: "templates", membersOnly: true,
		fields: []field{
			{name: "kind", kind: kEnum, required: true, immutable: true, enum: []string{"project", "task", "page"}},
			{name: "name", kind: kString, required: true, min: 1, max: 120},
			{name: "data", kind: kJSON, required: true, max: 1_000_000, shape: templateShape,
				schema: []string{"type: object"}, doc: templateDoc},
			{name: "saved_by", kind: kSelf, nullable: true, doc: "Who saved it last."},
			{name: "saved_at", kind: kTime, nullable: true, doc: "When it was saved last."},
			{name: "sort_key", kind: kString, required: true, min: 1, max: 64, check: validateKey, doc: sortKeyDoc},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	}
	entities["automation"] = &entity{
		name: "automation", table: "automations", membersOnly: true,
		fields: []field{
			// Where it works, not who sees it: as with templates, no guest gets one, so
			// it has no projectOf, which would hand it to the project's guests. Fixed,
			// since what a rule may reach is decided by who saved it there.
			{name: "project_id", kind: kRef, ref: "project", nullable: true, immutable: true,
				doc: "The project it works in, or null for the whole workspace."},
			{name: "name", kind: kString, required: true, min: 1, max: 80},
			{name: "enabled", kind: kBool, required: true},
			{name: "enabled_at", kind: kTime, nullable: true, doc: "When it was last turned on. It acts on due dates from then on."},
			{name: "rule", kind: kJSON, required: true, max: 40_000, shape: automationShape,
				schema: []string{"type: object"}, doc: automationDoc},
			{name: "made_by", kind: kSelf, required: true, doc: "Who last saved its rule, whom it runs as."},
			{name: "paused_why", kind: kString, nullable: true, max: 200, doc: "Why Mila stopped it, if it did."},
			{name: "sort_key", kind: kString, required: true, min: 1, max: 64, check: validateKey, doc: sortKeyDoc},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	}
	entities["workspace_theme"] = &entity{
		name: "workspace_theme", table: "workspace_themes", workspaceWide: true, adminsOnly: true,
		fields: []field{
			{name: "name", kind: kString, required: true, min: 1, max: 60},
			{name: "body", kind: kJSON, required: true, max: MaxThemeBytes, shape: themeShape,
				schema: []string{"type: object"}, doc: themeBodyDoc},
			{name: "source_id", kind: kString, nullable: true, max: 64, check: validID,
				doc: "The id of the publisher's own theme this is a copy of (GET /me/themes), for updating it from that theme again; null when it has none."},
			{name: "suggested_at", kind: kTime, nullable: true,
				doc: "When it was suggested to everyone. The workspace suggests one theme at a time: the one suggested last. Null when it is not suggested."},
			{name: "published_by", kind: kSelf, nullable: true, doc: "Who published it, or updated it, last."},
			{name: "published_at", kind: kTime, nullable: true, doc: "When it was published, or updated, last."},
			{name: "sort_key", kind: kString, required: true, min: 1, max: 64, check: validateKey, doc: sortKeyDoc},
			{name: "deleted_at", kind: kTime, nullable: true, doc: "When it was unpublished."},
		},
	}
	entities["saved_view"] = &entity{
		name: "saved_view", table: "saved_views", projectOf: "project_id", wideWhenNull: true,
		owner: "owner_id", private: "owner_id",
		fields: []field{
			// Fixed, like owner_id: a row that changed scope would reach
			// people who never had the rest of it.
			{name: "project_id", kind: kRef, ref: "project", nullable: true, immutable: true},
			{name: "owner_id", kind: kSelf, nullable: true, immutable: true},
			{name: "name", kind: kString, required: true, min: 1, max: 80},
			{name: "type", kind: kEnum, required: true, enum: []string{"list", "board", "calendar", "table", "timeline", "report"}},
			{name: "config", kind: kJSON, required: true, max: 20_000, shape: configShape,
				schema: []string{"type: object"}, doc: configDoc},
			// Sort, grouping, board columns, table layout and visible fields; null is the
			// defaults. Optional, so apps from before still create views.
			{name: "display", kind: kJSON, nullable: true, max: 10_000, shape: displayShape,
				schema: []string{"type: object"}, doc: displayDoc},
			{name: "sort_key", kind: kString, required: true, min: 1, max: 64, check: validateKey, doc: sortKeyDoc},
			{name: "deleted_at", kind: kTime, nullable: true},
		},
	}
}

// templateDoc says what a template's data holds.
const templateDoc = `The snapshot, as the app saves and reads it (web/src/lib/templates.ts): {"v": 1, "tasks": [...], "project": {...}}, or for a page template {"v": 1, "tasks": [], "pages": [...]}, each page with its id, parent, title and body, the page itself first. Each task is an object with its id (the ID of the task it was saved from), title, parent (another task's id, or null), notes, priority, status (a status's id in the project, or null), start and due (whole days from the template's first date, or null), due_time, recurrence, recurrence_mode, estimate (minutes, or null), labels ([{"id", "name", "color"}]), values (field ID to value), checklist ([text]), assignees (member IDs, when kept) and waits (the ids of tasks it waits for). A project template's project holds name, description, icon, color, statuses ([{"id", "name", "category"}]), fields ([{"id", "name", "type", "options"}]), views ([{"name", "type", "config", "display"}]) and pages ([{"id", "parent", "title", "body"}]); a task template has none, and its first task is the task, due in "due" days from the day it is used. At most 500 tasks and 1,000,000 bytes; 200 templates in a workspace.`

// configDoc and displayDoc say what a saved view's JSON fields hold, for
// the API reference; web/src/lib/filter.ts and viewDisplay.ts read them.
const configDoc = `The view's filter, under "filter": {"filter": {...}}. {} (or a null filter) shows every open task. Every part of the filter is optional, and a task must pass each part given:

- text: a string; words in the title or notes, any case.
- projects, statuses, priorities, labels, assignees: lists of strings; any one matches. projects holds project IDs or "inbox"; statuses holds status categories (backlog, todo, in-progress, done, canceled); priorities holds none, low, medium, high or urgent; labels holds label IDs; assignees holds member IDs or "me".
- statusIds: a list of status IDs; it and statuses are one part, so a task matches if its status's category is in statuses or its status is in statusIds. A finished status here shows the finished tasks in it.
- due: overdue, today (due today or before), week (within seven days), dated (has a due date) or none.
- myDay: true keeps the tasks in My Day of whoever looks, picked for today or an earlier day.
- showDone: true shows done and canceled tasks, which are otherwise hidden unless statuses or statusIds asks for them.
- dependencies: a list holding blocked (the task waits for one not done), blocking (an open task holds up one not done), or both; either matches.
- time: a list of conditions on estimates and time logged, every one of which must hold: estimated (the task has an estimate of its own), unestimated (it has none), over (more time logged on it than its estimate), recent (time logged on it in the seven days to today) and mine (time logged on it by whoever looks).
- fields: a list of conditions on custom fields, each {"field": <field ID>, "op": <operator>, "value": <what to compare with>}; every one must hold. The operators by type: text and url contains or not_contains (value: words); number eq, lt, lte, gt or gte (value: a number); date on, before, after (value: a date string, or a whole number of days from today) or between (value: two of those); select and user any or none_of (value: a list of choice or member IDs, "me" for whoever looks); multi_select any, all or none_of; checkbox checked or unchecked; and every type but checkbox empty or not_empty (no value).

A part of the wrong type is refused.`

const displayDoc = `How the view lays out what its filter shows, or null for the defaults:

- sort: up to three rules in turn, each {"by": title, priority, due, start, created, updated, completed, status, project, estimate, logged, remaining or field, "field": <field ID> when by is field, "dir": asc or desc}. None (or by manual) is the manual order. A field to sort by is of type text, url, number, date, select, user or checkbox.
- group (list, table and timeline views): {"by": none, project, status, priority, assignee, label, due or field, "field": <ID of a select or user field>}.
- columns (board views): {"by": status, priority, due or field, "field": <ID of a select or user field>}. Status is for a project's board only, and across projects only a workspace field. due is a column for what is past, each of the next seven days, later and no date.
- fields: {"list": {"show": [...], "hide": [...]}, "board": {...}, "table": {...}}, what rows, cards and table columns show beyond each type's defaults: custom field IDs or the built-in names status, project, priority, labels, subtasks, checklist, comments, repeat, reminders, assignees, start, due, estimate, logged and remaining (the last three are a task's own estimate, the time logged on it, and what is left of the estimate). A table shows every custom field unless it is hidden.
- table (table views): {"order": [...], "widths": [{"col": ..., "px": 60 to 640}], "summaries": [{"col": ..., "fn": ...}]}: the order of the columns after the title, their widths, and what each column's summary row works out where it differs from the default (fn: none, count, filled, sum, avg, min, max, range, earliest, latest, overdue or bar). Columns are named as in fields, and the title column is "title".
- timeline (timeline views): {"zoom": day, week, month or quarter}, how much time the timeline shows across its width; week when absent.
- report (report views): {"tab": status, workload or flow, "period": weeks6, months3, year or range, "from" and "to": the days of a range, "project": a project ID or null}, how the Reports page is set up: the tab it opens on, the weeks it counts, and, for a view of no project, the one project it reports on (null for all). The view's filter narrows the tasks it counts, as everywhere.

What the app cannot use (a deleted field, a rule from a newer app) it shows as not available and otherwise ignores.`

// syncOrder is the order bootstrap reads entities in: parents first.
var syncOrder = []string{"project", "status", "label", "custom_field", "saved_view", "task", "task_label", "task_dependency", "task_assignee", "checklist_item", "comment", "doc_update", "page", "page_update", "page_thread", "page_comment", "attachment", "page_attachment", "my_day_entry", "reminder", "custom_field_value", "template", "time_entry", "project_time_entry", "workspace_theme", "automation"}

func validID(s string) error {
	if !idPattern.MatchString(s) {
		return errors.New("is not a valid ID")
	}
	return nil
}

func (e *entity) field(name string) *field {
	for i := range e.fields {
		if e.fields[i].name == name {
			return &e.fields[i]
		}
	}
	return nil
}
