// The synced entities, mirroring server/internal/syncer/entities.go. The
// app's creates send only the fields they set: the device that makes a row
// fills in DEFAULTS, other devices hold the fields sent (the converters in
// replica.svelte.ts default the rest) until a bootstrap reads the whole row,
// which the database filled with the same defaults. A create through the
// REST API logs every field. entities.spec.ts holds
// DEFAULTS to the defaults the API's reference states.

import { hashId } from '../hashid';

/**
 * The version of the synced entities, sent with every push and equal to
 * Schema in entities.go. A server of another version refuses the push
 * with update_required, and the app keeps its edits until it reloads.
 * Raising it needs an upgrade step in outbox.ts for the edits kept.
 */
export const SYNC_SCHEMA = 6;

export type Entity =
	| 'project'
	| 'status'
	| 'label'
	| 'task'
	| 'task_label'
	| 'task_dependency'
	| 'task_assignee'
	| 'checklist_item'
	| 'comment'
	| 'doc_update'
	| 'page'
	| 'page_update'
	| 'attachment'
	| 'page_attachment'
	| 'page_thread'
	| 'page_comment'
	| 'my_day_entry'
	| 'reminder'
	| 'custom_field'
	| 'custom_field_value'
	| 'saved_view'
	| 'template'
	| 'time_entry'
	| 'project_time_entry'
	| 'workspace_theme'
	| 'automation';
export type Values = Record<string, unknown>;

export const DEFAULTS: Record<Entity, Values> = {
	project: {
		name: '',
		icon: '',
		color: 'blue',
		description: '',
		sort_key: '',
		archived_at: null,
		deleted_at: null
	},
	status: { project_id: '', name: '', category: 'todo', sort_key: '', deleted_at: null },
	task: {
		project_id: null,
		status_id: null,
		title: '',
		notes: '',
		sort_key: '',
		completed_at: null,
		deleted_at: null,
		parent_id: null,
		priority: 'none',
		start_date: null,
		due_date: null,
		due_time: null,
		created_by: null,
		recurrence: null,
		recurrence_mode: 'schedule',
		series_id: null,
		estimate_minutes: null,
		// What reports read; the server fills both in (docs/PLAN.md section 3).
		made_at: null,
		started_at: null
	},
	label: { name: '', color: 'blue', sort_key: '', deleted_at: null },
	task_label: { task_id: '', label_id: '', deleted_at: null },
	// task_id waits for blocker_id to be done (docs/PLAN.md section 3).
	task_dependency: { task_id: '', blocker_id: '', deleted_at: null },
	task_assignee: { task_id: '', user_id: '', deleted_at: null },
	checklist_item: { task_id: '', text: '', done: false, sort_key: '', deleted_at: null },
	comment: { task_id: '', author_id: '', body: '', edited_at: null, deleted_at: null },
	// One Yjs update to a task's notes, as base64 (docs.svelte.ts).
	doc_update: { task_id: '', data: '', deleted_at: null },
	page: { project_id: null, parent_id: null, title: '', body: '', sort_key: '', deleted_at: null },
	// One Yjs update to a page's body, as a doc_update is to a task's notes.
	page_update: { page_id: '', data: '', deleted_at: null },
	my_day_entry: { task_id: '', user_id: '', day: '', sort_key: '', deleted_at: null },
	reminder: { task_id: '', user_id: '', remind_at: '', offset_minutes: null, deleted_at: null },
	custom_field: {
		project_id: null,
		name: '',
		type: 'text',
		options: [],
		sort_key: '',
		deleted_at: null
	},
	custom_field_value: { task_id: '', field_id: '', value: null, deleted_at: null },
	saved_view: {
		project_id: null,
		owner_id: null,
		name: '',
		type: 'list',
		config: {},
		display: null,
		sort_key: '',
		deleted_at: null
	},
	// A project or task saved to make more from (lib/templates.ts).
	template: {
		kind: 'task',
		name: '',
		data: { v: 1, tasks: [] },
		saved_by: null,
		saved_at: null,
		sort_key: '',
		deleted_at: null
	},
	// Time logged on a task: typed in, or a timer while running (lib/time.ts).
	time_entry: {
		task_id: '',
		user_id: '',
		day: '',
		minutes: 0,
		note: '',
		started_at: null,
		running: false,
		active_at: null,
		deleted_at: null
	},
	// Time logged to a project itself, not a task: as a time entry, by project.
	project_time_entry: {
		project_id: '',
		user_id: '',
		day: '',
		minutes: 0,
		note: '',
		started_at: null,
		running: false,
		active_at: null,
		deleted_at: null
	},
	// A rule: lib/automations.ts, and internal/automations on the server.
	automation: {
		project_id: null,
		name: '',
		enabled: false,
		enabled_at: null,
		rule: {},
		made_by: '',
		paused_why: null,
		sort_key: '',
		deleted_at: null
	},
	// A theme the workspace offers (lib/theme/workspaceThemes.svelte.ts).
	workspace_theme: {
		name: '',
		body: {},
		source_id: null,
		suggested_at: null,
		published_by: null,
		published_at: null,
		sort_key: '',
		deleted_at: null
	},
	attachment: {
		task_id: '',
		filename: '',
		mime: '',
		size: 0,
		storage_key: '',
		uploaded_by: '',
		deleted_at: null
	},
	page_attachment: {
		page_id: '',
		filename: '',
		mime: '',
		size: 0,
		storage_key: '',
		uploaded_by: '',
		deleted_at: null
	},
	page_thread: {
		page_id: '',
		created_by: '',
		quote: '',
		anchor: '',
		resolved_at: null,
		deleted_at: null
	},
	page_comment: {
		page_id: '',
		thread_id: '',
		author_id: '',
		body: '',
		edited_at: null,
		deleted_at: null
	}
};

/**
 * Fields where the write with the lowest clock wins instead of the highest
 * (firstWins on the server): the creator of a task two devices made at
 * once under the same derived ID.
 */
export const FIRST_WINS: Partial<Record<Entity, string[]>> = { task: ['created_by'] };

/** Rows that hang off a task and go with it. */
export const TASK_CHILDREN: Entity[] = [
	'task_label',
	'task_assignee',
	'checklist_item',
	'comment',
	'attachment',
	'custom_field_value'
];

export type Priority = 'none' | 'low' | 'medium' | 'high' | 'urgent';

export type StatusCategory = 'backlog' | 'todo' | 'in-progress' | 'done' | 'canceled';

/** The statuses every new project starts with. */
export const STARTER_STATUSES: [string, StatusCategory][] = [
	['Backlog', 'backlog'],
	['To do', 'todo'],
	['In progress', 'in-progress'],
	['Done', 'done'],
	['Canceled', 'canceled']
];

export interface Project {
	id: string;
	name: string;
	icon: string;
	color: string;
	description: string;
	sortKey: string;
	archivedAt: string | null;
}

export interface Status {
	id: string;
	projectId: string;
	name: string;
	category: StatusCategory;
	sortKey: string;
}

/**
 * A wiki page: in a project's wiki, or the workspace's (projectId null).
 * A wiki's main page has no parent and an ID made from its project's or
 * workspace's (pages.ts); its title is never shown.
 */
export interface Page {
	id: string;
	projectId: string | null;
	parentId: string | null;
	title: string;
	/** The page as Markdown, rendered by the server from its live document. */
	body: string;
	sortKey: string;
	deletedAt: string | null;
}

export interface Task {
	id: string;
	projectId: string | null;
	statusId: string | null;
	parentId: string | null;
	title: string;
	notes: string;
	sortKey: string;
	completedAt: string | null;
	priority: Priority;
	startDate: string | null;
	dueDate: string | null;
	dueTime: string | null;
	createdBy: string | null;
	/** A repeat rule (see lib/recurrence.ts), or null. */
	recurrence: string | null;
	recurrenceMode: 'schedule' | 'completion';
	/** The first task of the series this one repeats, or null. */
	seriesId: string | null;
	/** The task's own estimate in minutes (its subtasks have theirs), or null. */
	estimateMinutes: number | null;
	/** When the task was made, as the server says; null only if its history was lost. */
	madeAt: string | null;
	/** When it first went to a status of kind in-progress, as the server says, or null. */
	startedAt: string | null;
}

/**
 * Time logged on a task by one person (docs/PLAN.md section 3): typed in,
 * or a timer, which runs while running is true and counts from startedAt;
 * its minutes are 0 until it stops.
 */
/**
 * A theme a workspace offers: a copy of one of its publisher's own
 * themes, under a name of its own. The one suggested to everyone is the
 * one suggested last.
 */
export interface Automation {
	id: string;
	/** Null for the whole workspace. */
	projectId: string | null;
	name: string;
	enabled: boolean;
	/** The rule as stored; read it with readRule. */
	rule: unknown;
	madeBy: string;
	/** Why Mila stopped it, if it did. */
	pausedWhy: string | null;
	sortKey: string;
}

export interface WorkspaceTheme {
	id: string;
	name: string;
	/** The theme document as stored; read it with workspaceDoc. */
	body: unknown;
	/** The publisher's own theme it is a copy of, for Update from my theme. */
	sourceId: string | null;
	suggestedAt: string | null;
	publishedBy: string | null;
	publishedAt: string | null;
	sortKey: string;
}

/**
 * Time someone logged: on a task (a time_entry row), or with taskId null,
 * on a project itself (a project_time_entry row), whose project it names.
 */
export interface TimeEntry {
	id: string;
	taskId: string | null;
	/** The project for time on a project itself; null for time on a task. */
	projectId: string | null;
	userId: string;
	/** The day it counts toward, YYYY-MM-DD. */
	day: string;
	minutes: number;
	note: string;
	startedAt: string | null;
	running: boolean;
	/** While running, when its person last did something in Mila. */
	activeAt: string | null;
}

export interface Label {
	id: string;
	name: string;
	color: string;
	sortKey: string;
}

export interface ChecklistItem {
	id: string;
	taskId: string;
	text: string;
	done: boolean;
	sortKey: string;
}

export interface Comment {
	id: string;
	taskId: string;
	authorId: string;
	body: string;
	editedAt: string | null;
	createdAt: string; // the HLC's time, which is when it was written
}

/** Comments pinned to some words of a wiki page. */
export interface PageThread {
	id: string;
	pageId: string;
	createdBy: string;
	/** The words it was pinned to. */
	quote: string;
	/** Where it is pinned in the page's live document, or empty to go by its words. */
	anchor: string;
	resolvedAt: string | null;
	createdAt: string;
}

/** A comment in a thread on a wiki page. */
export interface PageComment {
	id: string;
	threadId: string;
	authorId: string;
	body: string;
	editedAt: string | null;
	createdAt: string;
}

/**
 * A reminder on a task, which only its user sees. One made relative to the
 * due moment keeps its offset and follows the task's due date and time;
 * remindAt is then the time that works out to.
 */
export interface Reminder {
	id: string;
	taskId: string;
	remindAt: string;
	/** Minutes before the due moment, or null for a fixed time. */
	offsetMinutes: number | null;
}

/** A file on a task, or with pageId instead, on a wiki page. */
export interface Attachment {
	id: string;
	taskId: string;
	pageId?: string;
	filename: string;
	mime: string;
	size: number;
	uploadedBy: string;
}

export type FieldType =
	'text' | 'number' | 'date' | 'select' | 'multi_select' | 'checkbox' | 'url' | 'user';

export const FIELD_TYPES: { value: FieldType; label: string }[] = [
	{ value: 'text', label: 'Text' },
	{ value: 'number', label: 'Number' },
	{ value: 'date', label: 'Date' },
	{ value: 'select', label: 'Select' },
	{ value: 'multi_select', label: 'Multi-select' },
	{ value: 'checkbox', label: 'Checkbox' },
	{ value: 'url', label: 'Link' },
	{ value: 'user', label: 'Person' }
];

/** A choice of a select field. Values refer to it by ID, so it can be renamed. */
export interface FieldOption {
	id: string;
	name: string;
	color: string;
}

export interface CustomField {
	id: string;
	projectId: string | null; // null: every project in the workspace
	name: string;
	type: FieldType;
	options: FieldOption[];
	sortKey: string;
}

/**
 * A field's value on a task: a string for text, URL, date, select and
 * person; a number; a boolean; or a list of option IDs.
 */
export type FieldValue = string | number | boolean | string[] | null;

export type ViewType = 'list' | 'board' | 'calendar' | 'table' | 'timeline' | 'report';

export type DueFilter = 'overdue' | 'today' | 'week' | 'dated' | 'none';

/** What a saved view shows. Every part is optional; an empty filter shows everything. */
export type DependencyFilter = 'blocked' | 'blocking';

export interface ViewFilter {
	text?: string;
	projects?: string[]; // 'inbox' for tasks in no project
	statuses?: StatusCategory[];
	/**
	 * Statuses picked by ID, a project's own; one part with statuses: a task
	 * matches if its category is in statuses or its status is here.
	 */
	statusIds?: string[];
	priorities?: Priority[];
	labels?: string[];
	assignees?: string[]; // 'me' is whoever is looking
	due?: DueFilter;
	showDone?: boolean;
	/** In the viewer's My Day: picked for today or an earlier day. */
	myDay?: boolean;
	/** blocked, blocking or both; either matches (lib/filter.ts). */
	dependencies?: DependencyFilter[];
	/** Conditions on estimates and time logged; every one must hold (lib/filter.ts). */
	time?: TimeCondition[];
	/** Conditions on custom fields; every one must hold (lib/filter.ts). */
	fields?: FieldCondition[];
}

/**
 * A condition on time: the task has an estimate of its own, or none; more
 * is logged on it than its estimate; time was logged on it in the seven
 * days to today; or by whoever looks.
 */
export type TimeCondition = 'estimated' | 'unestimated' | 'over' | 'recent' | 'mine';

/**
 * The operators a field condition can use. Which apply depends on the
 * field's type (FIELD_OPS in lib/filter.ts).
 */
export type FieldOp =
	| 'contains'
	| 'not_contains'
	| 'eq'
	| 'lt'
	| 'lte'
	| 'gt'
	| 'gte'
	| 'on'
	| 'before'
	| 'after'
	| 'between'
	| 'any'
	| 'all'
	| 'none_of'
	| 'checked'
	| 'unchecked'
	| 'empty'
	| 'not_empty';

/**
 * A condition on a custom field. The value depends on the operator: text,
 * a number, a date bound ("YYYY-MM-DD" or a day offset from today), two
 * bounds for between, or option or user IDs ("me" for whoever is looking).
 */
export interface FieldCondition {
	field: string;
	op: FieldOp;
	value?: unknown;
}

export type SortBy =
	| 'manual'
	| 'title'
	| 'priority'
	| 'due'
	| 'start'
	| 'created'
	| 'updated'
	| 'completed'
	| 'status'
	| 'project'
	| 'estimate'
	| 'logged'
	| 'remaining'
	| 'field';

export interface SortRule {
	by: SortBy;
	/** The custom field's ID, for by: 'field'. */
	field?: string;
	dir: 'asc' | 'desc';
}

export type GroupBy =
	'none' | 'project' | 'status' | 'priority' | 'assignee' | 'label' | 'due' | 'field';

export interface Grouping {
	by: GroupBy;
	field?: string;
}

export type ColumnsBy = 'status' | 'priority' | 'due' | 'field';

export interface Columns {
	by: ColumnsBy;
	field?: string;
}

/** Overrides of a view type's default fields: built-in names or custom field IDs. */
export interface FieldShow {
	show?: string[];
	hide?: string[];
}

/** What a summary under a table column works out for its group (lib/table.ts). */
export type SummaryFn =
	| 'none'
	| 'count'
	| 'filled'
	| 'sum'
	| 'avg'
	| 'min'
	| 'max'
	| 'range'
	| 'earliest'
	| 'latest'
	| 'overdue'
	| 'bar';

/**
 * A table's own layout: the order of its columns after the title, their
 * widths in pixels, and the summary each column gives its group where it
 * differs from the default. Columns are named as shown fields are: a
 * built-in name or a custom field's ID, always in a value position.
 */
export interface TableDisplay {
	order?: string[];
	widths?: { col: string; px: number }[];
	summaries?: { col: string; fn: SummaryFn }[];
}

/**
 * How a saved view lays out what its filter shows: sort, grouping (lists),
 * columns (boards) and the fields on rows and cards. A view's display is
 * its own synced field, apart from config, so edits to the two merge
 * separately. Every reference in it is a bare ID in a value position, so a
 * restore with fresh IDs can remap it.
 */
export interface ViewDisplay {
	/** At most three; absent or empty is manual order. */
	sort?: SortRule[];
	/** Lists and tables. */
	group?: Grouping;
	/** Boards only. */
	columns?: Columns;
	fields?: { list?: FieldShow; board?: FieldShow; table?: FieldShow };
	/** Tables only. */
	table?: TableDisplay;
	/** Timelines only: { zoom: day, week, month or quarter } (lib/timeline.ts). */
	timeline?: { zoom?: unknown };
	/** Report views only: the Reports page's tab, period and project (lib/reportSetup.svelte.ts). */
	report?: unknown;
	/** Keys from a newer app, kept on save. */
	[key: string]: unknown;
}

export interface SavedView {
	id: string;
	projectId: string | null;
	ownerId: string | null; // null: shared with the workspace
	name: string;
	type: ViewType;
	filter: ViewFilter;
	/** {} when the row has none: the defaults. */
	display: ViewDisplay;
	sortKey: string;
}

/**
 * The ID of a row that stands for a pair (a label on a task, a person
 * assigned to it, a dependency, a task in someone's day, a field's value):
 * a hash of the two IDs in order, so the same pair makes the same row on
 * every device, and nobody can choose two IDs of their own that make
 * another's. server/internal/ids holds the same, and ids_test.go and
 * entities.spec.ts the same answers.
 */
export function pairId(a: string, b: string): string {
	return a && b ? hashId(`${a}/${b}`) : '';
}

/**
 * The two UUIDs XORed: how a pair's ID was made before pairId (sync schema 4
 * and earlier), and still how a wiki's main page's ID is made (pages.ts).
 */
export function xorId(a: string, b: string): string {
	const x = a.replaceAll('-', '');
	const y = b.replaceAll('-', '');
	let hex = '';
	for (let i = 0; i < 32; i += 2) {
		hex += (parseInt(x.slice(i, i + 2), 16) ^ parseInt(y.slice(i, i + 2), 16))
			.toString(16)
			.padStart(2, '0');
	}
	return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

/** The entities whose rows stand for a pair, and the two fields of it. */
export const PAIRS: Partial<Record<Entity, readonly [string, string]>> = {
	task_label: ['task_id', 'label_id'],
	task_assignee: ['task_id', 'user_id'],
	task_dependency: ['task_id', 'blocker_id'],
	my_day_entry: ['task_id', 'user_id'],
	custom_field_value: ['task_id', 'field_id']
};
