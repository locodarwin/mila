// The table view: its columns, what clicking a heading does to the sort,
// the rows with their subtasks, the summary under each column, and what
// dropping a row in another group writes. Pure, so the rules are tested
// without a replica: the view reads what these need from it and passes it in.
import { storedSort } from './viewDisplay';
import type {
	CustomField,
	FieldValue,
	Grouping,
	Priority,
	Project,
	SortRule,
	StatusCategory,
	SummaryFn,
	TableDisplay,
	Task,
	ViewDisplay
} from './sync/entities';
import { CATEGORIES, PRIORITY_ORDER, BUILTIN_FIELDS, type Section } from './viewDisplay';
import { formatDuration, remaining, sayDuration } from './duration';

// Columns

/** What a column holds, which decides how it is drawn, edited, sorted and summed up. */
export type ColumnKind =
	| 'title'
	| 'status'
	| 'priority'
	| 'people'
	| 'date'
	| 'project'
	| 'labels'
	| 'count'
	| 'repeat'
	| 'text'
	| 'url'
	| 'number'
	| 'select'
	| 'multi'
	| 'checkbox'
	| 'user'
	| 'duration';

export interface TableColumn {
	/** "title", a built-in field's name, or a custom field's ID. */
	id: string;
	label: string;
	kind: ColumnKind;
	/** The custom field, for a custom field's column. */
	field?: CustomField;
	width: number;
	/** What a click on the heading sorts by, or null when it cannot sort. */
	sort: Omit<SortRule, 'dir'> | null;
	/** Whether its cells open a picker; the rest open the task. */
	editable: boolean;
	/** Numbers line up on the right. */
	end: boolean;
}

/** A place in a row: a column drawn, or a stretch of columns left out. */
export type ColumnSlot =
	{ key: string; col: TableColumn; at: number } | { key: string; gap: number };

/**
 * What a row draws, for a table scrolled to `left` in a `view` wide: the
 * title, the columns within a screenful either side, and any in `keep` (a
 * cell with focus must stay in the page). The rest are one wide empty cell
 * each side, since a workspace's fields can make a table of many columns,
 * and drawing them all makes it slow to scroll.
 */
export function columnSlots(
	columns: TableColumn[],
	left: number,
	view: number,
	keep: readonly (number | null)[]
): ColumnSlot[] {
	const out: ColumnSlot[] = [];
	const from = left - view;
	const to = left + 2 * view;
	let x = 0;
	let gap = 0;
	const close = (at: number) => {
		if (gap) out.push({ key: `gap:${at - gap}`, gap });
		gap = 0;
	};
	columns.forEach((col, at) => {
		if (at === 0 || keep.includes(at) || (x + col.width > from && x < to)) {
			close(at);
			out.push({ key: col.id, col, at });
		} else gap++;
		x += col.width;
	});
	close(columns.length);
	return out;
}

/** The built-in columns after the title, in the order a new table shows them. */
export const TABLE_ORDER = [
	'status',
	'priority',
	'assignees',
	'due',
	'start',
	'project',
	'labels',
	'subtasks',
	'checklist',
	'comments',
	'repeat',
	'reminders',
	'estimate',
	'logged',
	'remaining'
];

const BUILTIN_KIND: Record<string, ColumnKind> = {
	status: 'status',
	priority: 'priority',
	assignees: 'people',
	due: 'date',
	start: 'date',
	project: 'project',
	labels: 'labels',
	subtasks: 'count',
	checklist: 'count',
	comments: 'count',
	repeat: 'repeat',
	reminders: 'count',
	estimate: 'duration',
	logged: 'duration',
	remaining: 'duration'
};

const FIELD_KIND: Record<CustomField['type'], ColumnKind> = {
	text: 'text',
	url: 'url',
	number: 'number',
	date: 'date',
	select: 'select',
	multi_select: 'multi',
	checkbox: 'checkbox',
	user: 'user'
};

export const MIN_WIDTH = 60;
export const MAX_WIDTH = 640;

/** A column's width before anyone resizes it. */
export function defaultWidth(id: string, kind: ColumnKind): number {
	if (kind === 'title') return 300;
	if (id === 'project') return 130;
	switch (kind) {
		case 'status':
			return 150;
		case 'priority':
		case 'people':
		case 'user':
			return 120;
		case 'date':
			return 120;
		case 'labels':
		case 'multi':
			return 170;
		case 'text':
		case 'url':
			return 180;
		case 'select':
			return 140;
		case 'number':
		case 'count':
			return 96;
		case 'duration':
			return 112;
		case 'checkbox':
			return 90;
		default:
			return 110;
	}
}

const clampWidth = (px: number) => Math.round(Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, px)));

/** The built-in sort keys a column's heading sorts by. */
const BUILTIN_SORT: Record<string, SortRule['by']> = {
	title: 'title',
	status: 'status',
	priority: 'priority',
	due: 'due',
	start: 'start',
	project: 'project',
	estimate: 'estimate',
	logged: 'logged',
	remaining: 'remaining'
};
const SORTABLE_FIELDS = new Set(['text', 'url', 'number', 'date', 'select', 'user', 'checkbox']);
const EDITABLE: Set<ColumnKind> = new Set([
	'status',
	'priority',
	'people',
	'date',
	'project',
	'labels',
	'text',
	'url',
	'number',
	'select',
	'multi',
	'checkbox',
	'user'
]);

/**
 * A table's layout as stored, with every part of the wrong shape left
 * out: a display is the app's to judge (the server checks only that it
 * is an object), so nothing malformed reaches the page.
 */
export function readTable(display: ViewDisplay | null | undefined): TableDisplay {
	const t = display?.table;
	if (typeof t !== 'object' || t === null || Array.isArray(t)) return {};
	const raw = t as Record<string, unknown>;
	const out: TableDisplay = {};
	if (Array.isArray(raw.order)) out.order = raw.order.filter((x) => typeof x === 'string');
	if (Array.isArray(raw.widths))
		out.widths = raw.widths.filter(
			(w): w is { col: string; px: number } =>
				typeof w?.col === 'string' && typeof w?.px === 'number' && Number.isFinite(w.px)
		);
	if (Array.isArray(raw.summaries))
		out.summaries = raw.summaries.filter(
			(s): s is { col: string; fn: SummaryFn } =>
				typeof s?.col === 'string' && typeof s?.fn === 'string'
		);
	return out;
}

/**
 * The columns of a table: the title, then the shown fields in the table's
 * own order where it has one and the default order for the rest.
 */
export function tableColumns(
	shown: string[],
	table: TableDisplay,
	fields: CustomField[]
): TableColumn[] {
	const on = new Set(shown);
	const defaults = [
		...TABLE_ORDER.filter((id) => on.has(id)),
		...fields.filter((f) => on.has(f.id)).map((f) => f.id)
	];
	const order = [
		...new Set([...(table.order ?? []).filter((id) => on.has(id) && id !== 'title'), ...defaults])
	];
	const widths = new Map((table.widths ?? []).map((w) => [w.col, w.px]));
	const make = (id: string): TableColumn | null => {
		const field = fields.find((f) => f.id === id);
		const kind = id === 'title' ? 'title' : field ? FIELD_KIND[field.type] : BUILTIN_KIND[id];
		if (!kind) return null;
		const label =
			id === 'title'
				? 'Task'
				: (field?.name ?? BUILTIN_FIELDS.find((f) => f.id === id)?.label ?? id);
		const sort: TableColumn['sort'] = field
			? SORTABLE_FIELDS.has(field.type)
				? { by: 'field', field: field.id }
				: null
			: BUILTIN_SORT[id]
				? { by: BUILTIN_SORT[id] }
				: null;
		const stored = widths.get(id);
		return {
			id,
			label,
			kind,
			...(field ? { field } : {}),
			width: stored === undefined ? defaultWidth(id, kind) : clampWidth(stored),
			sort,
			editable: EDITABLE.has(kind),
			end: kind === 'number' || kind === 'count' || kind === 'duration'
		};
	};
	return ['title', ...order].map(make).filter((c): c is TableColumn => c !== null);
}

/** The table's layout with the columns after the title in a new order. */
export function withOrder(table: TableDisplay, order: string[]): TableDisplay | undefined {
	return tidy({ ...table, order: order.filter((id) => id !== 'title') });
}

/** The table's layout with one column at a new width. */
export function withWidth(table: TableDisplay, col: string, px: number): TableDisplay | undefined {
	const widths = (table.widths ?? []).filter((w) => w.col !== col);
	widths.push({ col, px: clampWidth(px) });
	return tidy({ ...table, widths });
}

/** The table's layout back to the default order and widths, keeping its summaries. */
export function withoutLayout(table: TableDisplay): TableDisplay | undefined {
	return tidy({ summaries: table.summaries ?? [] });
}

/** Where a column goes when it moves one place left or right among the movable ones. */
export function moveColumn(order: string[], col: string, by: -1 | 1): string[] | null {
	const at = order.indexOf(col);
	const to = at + by;
	if (at < 0 || to < 0 || to >= order.length) return null;
	const next = [...order];
	next.splice(at, 1);
	next.splice(to, 0, col);
	return next;
}

/** Puts a column before another (or last, for null). */
export function placeColumn(order: string[], col: string, before: string | null): string[] {
	const rest = order.filter((id) => id !== col);
	const at = before === null ? rest.length : rest.indexOf(before);
	rest.splice(at < 0 ? rest.length : at, 0, col);
	return rest;
}

/** A layout with its empty parts left out; undefined when nothing is left. */
function tidy(t: TableDisplay): TableDisplay | undefined {
	const out: TableDisplay = { ...t };
	for (const k of ['order', 'widths', 'summaries'] as const) {
		const v = out[k];
		if (Array.isArray(v) && v.length === 0) delete out[k];
	}
	return Object.keys(out).length ? out : undefined;
}

// Sorting from the headings

const keyOf = (r: unknown) => {
	const rule = r as SortRule;
	return rule?.by === 'field' ? `field:${rule.field}` : String(rule?.by);
};
const ruleKey = (r: Omit<SortRule, 'dir'>) => (r.by === 'field' ? `field:${r.field}` : r.by);

/** Where a column stands in the sort: its direction and place, or null. */
export function columnSort(
	rules: SortRule[],
	col: TableColumn
): { dir: 'asc' | 'desc'; index: number } | null {
	if (!col.sort) return null;
	const k = ruleKey(col.sort);
	const index = rules.findIndex((r) => keyOf(r) === k);
	return index < 0 ? null : { dir: rules[index].dir, index };
}

/**
 * The sort after a click on a heading: ascending, then descending, then
 * not sorted by it. A plain click makes the column the only key; with
 * Shift it is added after the others, up to three, or turned in place.
 * Rules this app cannot use are kept on a Shift-click, as the Display
 * panel keeps them.
 */
export function headingSort(
	stored: unknown[],
	col: Omit<SortRule, 'dir'>,
	add: boolean
): unknown[] {
	const k = ruleKey(col);
	const at = stored.findIndex((r) => keyOf(r) === k);
	const current = at < 0 ? null : (stored[at] as SortRule);
	const next: SortRule | null = !current
		? { ...col, dir: 'asc' }
		: current.dir === 'asc'
			? { ...col, dir: 'desc' }
			: null;
	if (!add) return next ? [next] : [];
	if (at >= 0)
		return next ? stored.map((r, i) => (i === at ? next : r)) : stored.filter((_, i) => i !== at);
	return stored.length < 3 && next ? [...stored, next] : stored;
}

export { storedSort };

// Rows

export interface TableRow {
	/** The group and task, since a task can be in two groups. */
	key: string;
	task: Task;
	/** 0 for the group's own tasks, 1 for their subtasks, and so on. */
	depth: number;
	/** The task's parent among the rows, or null at the top. */
	parent: string | null;
	/** How many subtasks it has in the table. */
	kids: number;
	open: boolean;
	group: string;
}

/**
 * A group's rows: each task, then its subtasks under it (in the order
 * `kids` gives, the view's own), unless it is folded.
 */
export function groupRows(
	section: Section,
	kids: (task: Task) => Task[],
	folded: Set<string>
): TableRow[] {
	const out: TableRow[] = [];
	const walk = (task: Task, depth: number, parent: string | null, seen: Set<string>) => {
		if (seen.has(task.id)) return;
		const inner = new Set(seen).add(task.id);
		const children = kids(task).filter((c) => !inner.has(c.id));
		const open = !folded.has(task.id);
		out.push({
			key: `${section.id}/${task.id}`,
			task,
			depth,
			parent,
			kids: children.length,
			open,
			group: section.id
		});
		if (open) for (const c of children) walk(c, depth + 1, task.id, inner);
	};
	for (const t of section.tasks) walk(t, 0, null, new Set());
	return out;
}

/** Every task in a group, subtasks too, folded or not, each once: what its summaries count. */
export function groupTasksDeep(section: Section, kids: (task: Task) => Task[]): Task[] {
	const out: Task[] = [];
	const seen = new Set<string>();
	const walk = (task: Task) => {
		if (seen.has(task.id)) return;
		seen.add(task.id);
		out.push(task);
		for (const c of kids(task)) walk(c);
	};
	for (const t of section.tasks) walk(t);
	return out;
}

// Dropping a row in a group

/** What dropping a task in a group sets: what the group stands for. */
export type GroupWrite =
	| { status: string }
	| { category: StatusCategory }
	| { priority: Priority }
	| { project: string | null }
	| { field: string; value: FieldValue };

/**
 * What a group stands for, to write on a task dropped in it, or null when
 * it stands for nothing a drop can set (labels, assignees, due dates: a
 * task can have several, or the date would be a guess).
 */
export function groupWrite(group: Grouping, sectionId: string): GroupWrite | null {
	const [kind, ...rest] = sectionId.split(':');
	const value = rest.join(':');
	switch (group.by) {
		case 'status':
			if (kind === 's') return { status: value };
			if (kind === 'c') return { category: value as StatusCategory };
			return null;
		case 'priority':
			return kind === 'prio' ? { priority: value as Priority } : null;
		case 'project':
			return kind === 'p' ? { project: value === 'inbox' ? null : value } : null;
		case 'field':
			if (kind !== 'f' || !group.field) return null;
			return { field: group.field, value: value === 'none' ? null : value };
	}
	return null;
}

/** The sort keys around where a task lands among others, leaving itself out. */
export function landing(
	siblings: Task[],
	taskId: string,
	index: number
): { before: string | null; after: string | null } {
	const others = siblings.filter((t) => t.id !== taskId);
	const before = others[index - 1]?.sortKey ?? null;
	const after = others[index]?.sortKey ?? null;
	return { before, after: after === before ? null : after };
}

// Summaries

export const SUMMARY_LABELS: Record<SummaryFn, string> = {
	none: 'None',
	count: 'Count',
	filled: 'Filled in',
	sum: 'Sum',
	avg: 'Average',
	min: 'Smallest',
	max: 'Largest',
	range: 'Date range',
	earliest: 'Earliest',
	latest: 'Latest',
	overdue: 'Overdue',
	bar: 'Distribution'
};

/** The summaries a column can give, None first. */
export function summaryChoices(col: TableColumn): SummaryFn[] {
	switch (col.kind) {
		case 'title':
			return ['none', 'count'];
		case 'number':
		case 'count':
		case 'duration':
			return ['none', 'sum', 'avg', 'min', 'max', 'filled'];
		case 'date':
			return ['none', 'range', 'earliest', 'latest', 'overdue', 'filled'];
		case 'status':
		case 'priority':
		case 'project':
		case 'people':
		case 'select':
		case 'user':
			return ['none', 'bar', 'filled'];
		default:
			return ['none', 'filled'];
	}
}

/** A new table's summaries: a count under the title, the status bar, sums under numbers, the range of due dates. */
export function defaultSummary(col: TableColumn): SummaryFn {
	if (col.kind === 'title') return 'count';
	if (col.kind === 'status') return 'bar';
	if (col.kind === 'number' || col.kind === 'duration') return 'sum';
	if (col.id === 'due') return 'range';
	return 'none';
}

/** The summary a column gives: the table's choice where it can give it, else the default. */
export function summaryOf(table: TableDisplay, col: TableColumn): SummaryFn {
	const stored = table.summaries?.find((s) => s.col === col.id)?.fn;
	return stored && summaryChoices(col).includes(stored) ? stored : defaultSummary(col);
}

/** The table's layout with a column's summary chosen; a choice equal to the default is not stored. */
export function withSummary(
	table: TableDisplay,
	col: TableColumn,
	fn: SummaryFn
): TableDisplay | undefined {
	const summaries = (table.summaries ?? []).filter((s) => s.col !== col.id);
	if (fn !== defaultSummary(col)) summaries.push({ col: col.id, fn });
	return tidy({ ...table, summaries });
}

/** What summing up needs to know beyond the tasks. */
export interface SummaryContext {
	today: string;
	category: (task: Task) => StatusCategory;
	projects: Project[];
	assignees: (taskId: string) => string[];
	labels: (taskId: string) => string[];
	values: Map<string, Map<string, FieldValue>>;
	/** A person's name, members or not. */
	nameOf: (userId: string) => string;
	/** How many subtasks, checklist items, comments or reminders a task has. */
	count: (task: Task, what: string) => number;
	formatDate: (date: string) => string;
	formatNumber: (n: number) => string;
	/** The minutes logged on a task itself; none when not given. */
	logged?: (taskId: string) => number;
}

/** One part of a distribution bar. */
export interface BarPart {
	key: string;
	label: string;
	count: number;
	/** A CSS color: a token or a palette color. */
	color: string;
}

export type Summary =
	{ kind: 'text'; text: string; said: string } | { kind: 'bar'; parts: BarPart[]; said: string };

const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

/** A task's value in a column, or null when it is empty. */
export function cellValue(
	col: TableColumn,
	t: Task,
	ctx: SummaryContext
): string | number | boolean | string[] | null {
	if (col.field) {
		const v = ctx.values.get(col.field.id)?.get(t.id);
		if (v === undefined || v === null || v === '' || v === false) return null;
		if (Array.isArray(v)) return v.length ? v : null;
		return v;
	}
	switch (col.id) {
		case 'title':
			return t.title || null;
		case 'status':
			return ctx.category(t);
		case 'priority':
			return t.priority === 'none' ? null : t.priority;
		case 'assignees': {
			const a = ctx.assignees(t.id);
			return a.length ? a : null;
		}
		case 'due':
			return t.dueDate;
		case 'start':
			return t.startDate;
		case 'project':
			return t.projectId;
		case 'labels': {
			const l = ctx.labels(t.id);
			return l.length ? l : null;
		}
		case 'repeat':
			return t.recurrence;
		case 'estimate':
			return t.estimateMinutes;
		case 'logged':
			return ctx.logged?.(t.id) || null;
		case 'remaining': {
			// What is left; a task past its estimate has none left.
			const left = remaining(t.estimateMinutes, ctx.logged?.(t.id) ?? 0);
			return left && left.left;
		}
		default: {
			const n = ctx.count(t, col.id);
			return n ? n : null;
		}
	}
}

const PALETTE = ['blue', 'green', 'orange', 'purple', 'teal', 'pink', 'yellow', 'red'];

/** Works out a column's summary over a group's tasks, or null for none. */
export function summarize(
	fn: SummaryFn,
	col: TableColumn,
	tasks: Task[],
	ctx: SummaryContext
): Summary | null {
	if (fn === 'none') return null;
	if (fn === 'count') {
		const s = plural(tasks.length, 'task');
		return { kind: 'text', text: s, said: s };
	}
	const values = tasks.map((t) => cellValue(col, t, ctx));
	if (fn === 'filled') {
		const n = values.filter((v) => v !== null).length;
		return {
			kind: 'text',
			text: `${n} of ${tasks.length}`,
			said: `${n} of ${tasks.length} filled in`
		};
	}
	if (fn === 'sum' || fn === 'avg' || fn === 'min' || fn === 'max') {
		const nums = values.filter((v): v is number => typeof v === 'number' && Number.isFinite(v));
		if (!nums.length) return { kind: 'text', text: '', said: 'No numbers' };
		const n =
			fn === 'sum'
				? nums.reduce((a, b) => a + b, 0)
				: fn === 'avg'
					? nums.reduce((a, b) => a + b, 0) / nums.length
					: fn === 'min'
						? Math.min(...nums)
						: Math.max(...nums);
		if (col.kind === 'duration') {
			const left = col.id === 'remaining' && fn === 'sum' ? ' left' : '';
			return {
				kind: 'text',
				text: formatDuration(n) + left,
				said: `${SUMMARY_LABELS[fn]} ${sayDuration(n)}${left}`
			};
		}
		const text = ctx.formatNumber(n);
		return { kind: 'text', text, said: `${SUMMARY_LABELS[fn]} ${text}` };
	}
	if (fn === 'range' || fn === 'earliest' || fn === 'latest' || fn === 'overdue') {
		const dates = values.filter((v): v is string => typeof v === 'string').sort();
		if (fn === 'overdue') {
			const n = tasks.filter((t, i) => {
				const v = values[i];
				return typeof v === 'string' && v < ctx.today && t.completedAt === null;
			}).length;
			return { kind: 'text', text: n ? `${n} overdue` : 'None overdue', said: `${n} overdue` };
		}
		if (!dates.length) return { kind: 'text', text: '', said: 'No dates' };
		const first = ctx.formatDate(dates[0]);
		const last = ctx.formatDate(dates[dates.length - 1]);
		if (fn === 'earliest') return { kind: 'text', text: first, said: `Earliest ${first}` };
		if (fn === 'latest') return { kind: 'text', text: last, said: `Latest ${last}` };
		const text = first === last ? first : `${first} to ${last}`;
		return { kind: 'text', text, said: `From ${first} to ${last}` };
	}
	// A distribution bar: one part per value, in the column's own order,
	// with the tasks that have none last.
	const parts = barParts(col, tasks, values, ctx);
	const said = parts.map((p) => `${p.count} ${p.label}`).join(', ');
	return { kind: 'bar', parts, said };
}

function barParts(
	col: TableColumn,
	tasks: Task[],
	values: ReturnType<typeof cellValue>[],
	ctx: SummaryContext
): BarPart[] {
	const counts = new Map<string, number>();
	let none = 0;
	for (const v of values) {
		const keys = v === null ? [] : Array.isArray(v) ? v : [String(v)];
		if (!keys.length) none++;
		for (const k of keys) counts.set(k, (counts.get(k) ?? 0) + 1);
	}
	let order: { key: string; label: string; color: string }[] = [];
	let noneLabel = 'None';
	switch (col.kind) {
		case 'status':
			order = CATEGORIES.map((c) => ({
				key: c.value,
				label: c.label,
				color: `var(--status-${c.value})`
			}));
			break;
		case 'priority':
			order = PRIORITY_ORDER.filter((p) => p.value !== 'none').map((p) => ({
				key: p.value,
				label: p.label,
				color: `var(--priority-${p.value})`
			}));
			noneLabel = 'No priority';
			break;
		case 'project':
			order = ctx.projects.map((p) => ({
				key: p.id,
				label: p.name,
				color: `var(--palette-${p.color})`
			}));
			noneLabel = 'Inbox';
			break;
		case 'select':
			order = (col.field?.options ?? []).map((o) => ({
				key: o.id,
				label: o.name,
				color: `var(--palette-${o.color})`
			}));
			break;
		case 'people':
		case 'user': {
			const ids = [...counts.keys()].sort((a, b) => ctx.nameOf(a).localeCompare(ctx.nameOf(b)));
			order = ids.map((id, i) => ({
				key: id,
				label: ctx.nameOf(id),
				color: `var(--palette-${PALETTE[i % PALETTE.length]})`
			}));
			noneLabel = 'Nobody';
			break;
		}
	}
	const parts: BarPart[] = order
		.filter((o) => counts.get(o.key))
		.map((o) => ({ ...o, count: counts.get(o.key)! }));
	// Values that name nothing known (a deleted option) count with the empty ones.
	const known = new Set(order.map((o) => o.key));
	for (const [k, n] of counts) if (!known.has(k)) none += n;
	if (none)
		parts.push({ key: 'none', label: noneLabel, count: none, color: 'var(--border-strong)' });
	return parts;
}
