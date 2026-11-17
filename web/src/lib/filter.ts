// Saved views' filters. Pure, so the rules are tested without a replica. The
// server's copy (server/internal/filter) serves calendar feeds of a view;
// shared/filter-vectors.json holds the two to the same answers.
import type {
	DueFilter,
	FieldOp,
	FieldType,
	FieldValue,
	Priority,
	Status,
	StatusCategory,
	Task,
	TimeCondition,
	ViewFilter
} from './sync/entities';
export { filterOf, UNREADABLE } from './viewConfig';

/** What the filter needs to know about a task beyond its own fields. */
export interface FilterContext {
	me: string;
	today: string; // YYYY-MM-DD
	category: (task: Task) => StatusCategory;
	labels: (taskId: string) => string[];
	assignees: (taskId: string) => string[];
	/** A custom field's type, or undefined for one deleted or out of sight. */
	fieldType?: (fieldId: string) => FieldType | undefined;
	/** A task's stored value for a field, or undefined for none. */
	value?: (taskId: string, fieldId: string) => FieldValue | undefined;
	/** Whether a task waits for one not done. */
	blocked?: (taskId: string) => boolean;
	/** Whether an open task holds up one not done. */
	blocking?: (taskId: string) => boolean;
	/** The time logged on a task. */
	entries?: (taskId: string) => TimeFact[];
	/** Whether the viewer picked a task for My Day, today or on an earlier day. */
	inMyDay?: (taskId: string) => boolean;
}

/** What the filter reads of a piece of time logged. */
export interface TimeFact {
	userId: string;
	day: string;
	minutes: number;
}

/** The status categories, in the order the panel offers them. */
export const CATEGORY_LABELS: { value: StatusCategory; label: string }[] = [
	{ value: 'backlog', label: 'Backlog' },
	{ value: 'todo', label: 'To do' },
	{ value: 'in-progress', label: 'In progress' },
	{ value: 'done', label: 'Done' },
	{ value: 'canceled', label: 'Canceled' }
];

/** A status the panel offers by name, standing for every status of that name. */
export interface StatusChoice {
	name: string;
	ids: string[];
}

/**
 * The statuses a filter offers beside the categories, from the statuses of
 * the projects in view, in their order: one choice for each name (any
 * case), leaving out a status named for its own category (a project's To
 * do, which the category's choice already picks).
 */
export function statusChoices(statuses: Status[]): StatusChoice[] {
	const key = (s: string) => s.trim().toLowerCase();
	const label = new Map(CATEGORY_LABELS.map((c) => [c.value, key(c.label)]));
	const byName = new Map<string, StatusChoice>();
	for (const s of statuses) {
		const k = key(s.name);
		if (!k || label.get(s.category) === k) continue;
		const choice = byName.get(k);
		if (choice) choice.ids.push(s.id);
		else byName.set(k, { name: s.name.trim(), ids: [s.id] });
	}
	return [...byName.values()];
}

/** The time part's conditions, in the order the panel offers them. */
export const TIME_CONDITIONS: TimeCondition[] = [
	'estimated',
	'unestimated',
	'over',
	'recent',
	'mine'
];

/** The operators each type of field offers, in the order the panel lists them. */
export const FIELD_OPS: Record<FieldType, FieldOp[]> = {
	text: ['contains', 'not_contains', 'empty', 'not_empty'],
	url: ['contains', 'not_contains', 'empty', 'not_empty'],
	number: ['eq', 'lt', 'lte', 'gt', 'gte', 'empty', 'not_empty'],
	date: ['on', 'before', 'after', 'between', 'empty', 'not_empty'],
	select: ['any', 'none_of', 'empty', 'not_empty'],
	user: ['any', 'none_of', 'empty', 'not_empty'],
	multi_select: ['any', 'all', 'none_of', 'empty', 'not_empty'],
	checkbox: ['checked', 'unchecked']
};

export function addDays(day: string, n: number): string {
	const [y, m, d] = day.split('-').map(Number);
	const date = new Date(y, m - 1, d + n);
	return `${String(date.getFullYear()).padStart(4, '0')}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
}

function dueMatches(due: DueFilter, task: Task, today: string): boolean {
	const d = task.dueDate;
	switch (due) {
		case 'none':
			return d === null;
		case 'overdue':
			return d !== null && d < today;
		case 'today':
			return d !== null && d <= today;
		case 'week':
			return d !== null && d <= addDays(today, 7);
		case 'dated':
			return d !== null;
	}
	return false;
}

// A filter as stored may hold anything a broken or newer app wrote. Every
// part is read for its type: undefined or null leaves it unset, a value of
// the wrong JSON type is malformed. A malformed part counts as set and
// matches nothing, as does a filter that is not an object, the way the
// server's copy reads them.
const BAD = Symbol('malformed');
type Part<T> = T | undefined | typeof BAD;

function listPart(v: unknown): Part<string[]> {
	if (v === undefined || v === null) return undefined;
	return strings(v) ? v : BAD;
}
function stringPart(v: unknown): Part<string> {
	if (v === undefined || v === null) return undefined;
	return typeof v === 'string' ? v : BAD;
}
function boolPart(v: unknown): Part<boolean> {
	if (v === undefined || v === null) return undefined;
	return typeof v === 'boolean' ? v : BAD;
}
function fieldsPart(v: unknown): Part<unknown[]> {
	if (v === undefined || v === null) return undefined;
	return Array.isArray(v) ? v : BAD;
}

interface Parts {
	text: Part<string>;
	projects: Part<string[]>;
	statuses: Part<string[]>;
	statusIds: Part<string[]>;
	priorities: Part<string[]>;
	labels: Part<string[]>;
	assignees: Part<string[]>;
	due: Part<string>;
	showDone: Part<boolean>;
	myDay: Part<boolean>;
	dependencies: Part<string[]>;
	time: Part<string[]>;
	fields: Part<unknown[]>;
}

/** A filter's parts by type; undefined for a filter that is not an object (null is unset). */
function partsOf(f: unknown): Parts | undefined {
	if (f === undefined || f === null) f = {};
	if (!isObject(f)) return undefined;
	return {
		text: stringPart(f.text),
		projects: listPart(f.projects),
		statuses: listPart(f.statuses),
		statusIds: listPart(f.statusIds),
		priorities: listPart(f.priorities),
		labels: listPart(f.labels),
		assignees: listPart(f.assignees),
		due: stringPart(f.due),
		showDone: boolPart(f.showDone),
		myDay: boolPart(f.myDay),
		dependencies: listPart(f.dependencies),
		time: listPart(f.time),
		fields: fieldsPart(f.fields)
	};
}

const some = (list: Part<string[]>): list is string[] => Array.isArray(list) && list.length > 0;

/** Whether a task passes a filter. Within a part any choice matches; every part must. */
export function matches(task: Task, f: ViewFilter, ctx: FilterContext): boolean {
	const p = partsOf(f);
	if (!p || Object.values(p).includes(BAD)) return false;
	const category = ctx.category(task);
	const done = category === 'done' || category === 'canceled';
	const statuses = p.statuses as string[] | undefined;
	// A finished task shows when asked for: by showDone, by a finished
	// category, or by its own status picked (only a finished status picked
	// can reach one; the status part then decides).
	const picked = task.statusId !== null && some(p.statusIds) && p.statusIds.includes(task.statusId);
	if (done && !p.showDone && !picked && !statuses?.some((s) => s === 'done' || s === 'canceled'))
		return false;
	if ((some(p.statuses) || some(p.statusIds)) && !picked && !statuses?.includes(category))
		return false;
	if (some(p.projects) && !p.projects.includes(task.projectId ?? 'inbox')) return false;
	if (some(p.priorities) && !p.priorities.includes(task.priority as Priority)) return false;
	if (some(p.labels)) {
		const on = ctx.labels(task.id);
		if (!p.labels.some((l) => on.includes(l))) return false;
	}
	if (some(p.assignees)) {
		const on = ctx.assignees(task.id);
		const want = p.assignees.map((a) => (a === 'me' ? ctx.me : a));
		if (!want.some((a) => on.includes(a))) return false;
	}
	if (p.due && !dueMatches(p.due as DueFilter, task, ctx.today)) return false;
	if (p.myDay === true && !ctx.inMyDay?.(task.id)) return false;
	if (some(p.dependencies)) {
		const blocked = p.dependencies.includes('blocked') && Boolean(ctx.blocked?.(task.id));
		const blocking = p.dependencies.includes('blocking') && Boolean(ctx.blocking?.(task.id));
		if (!blocked && !blocking) return false;
	}
	if (some(p.time) && !p.time.every((c) => timeMatches(c, task, ctx))) return false;
	const text = (p.text as string | undefined)?.trim().toLowerCase();
	if (text && !`${task.title}\n${task.notes}`.toLowerCase().includes(text)) return false;
	const fields = p.fields as unknown[] | undefined;
	if (fields && !fields.every((c) => matchesField(c, task, ctx))) return false;
	return true;
}

/**
 * One of the time part's conditions, every one of which must hold, unlike
 * the other lists: an unknown one matches nothing.
 */
function timeMatches(condition: string, task: Task, ctx: FilterContext): boolean {
	const entries = ctx.entries?.(task.id) ?? [];
	switch (condition) {
		case 'estimated':
			return task.estimateMinutes !== null;
		case 'unestimated':
			return task.estimateMinutes === null;
		case 'over':
			return (
				task.estimateMinutes !== null &&
				entries.reduce((sum, e) => sum + e.minutes, 0) > task.estimateMinutes
			);
		case 'recent': {
			const from = addDays(ctx.today, -6);
			return entries.some((e) => e.day >= from && e.day <= ctx.today);
		}
		case 'mine':
			return entries.some((e) => e.userId === ctx.me);
	}
	return false;
}

// Field conditions. The server's copy follows the same rules in the same
// order, and shared/filter-vectors.json holds the two to them.
//
// - Empty means no value: absent, null, "", [] or false, as setFieldValue
//   has it. A task whose project lacks the field has no value.
// - A condition on a field this viewer does not know (deleted, or in a
//   project they cannot see) is evaluated as if no task had a value: empty,
//   not_contains, none_of and unchecked match, the rest match nothing. A
//   guest then sees a shared view's tasks within their projects, never more.
// - A malformed condition matches nothing, like an unknown due: not an
//   object with a string field and op, an operator the type lacks, or a
//   value of the wrong JSON type (a date that is not a real YYYY-MM-DD, an
//   offset that is not a whole number of days within ten years).
// - Unset conditions are ignored and not counted: any, all or none_of with
//   no choices, contains or not_contains with no words. Only a condition
//   that can be read (a string field and op) is unset; any other counts.
// - Keys are read exactly as written ("field", not "Field"). A field of a
//   type this app does not know matches nothing.
// - not_contains and none_of match tasks with no value; unchecked matches
//   any value that is not true. A stored value of the wrong shape never
//   matches a comparison, but still counts as a value for empty and
//   not_empty.

const NEGATIVE: FieldOp[] = ['empty', 'not_contains', 'none_of', 'unchecked'];
const DAY = /^\d{4}-\d{2}-\d{2}$/;
const MAX_OFFSET = 3660;

const isObject = (v: unknown): v is Record<string, unknown> =>
	typeof v === 'object' && v !== null && !Array.isArray(v);

/** Whether a stored value counts as one. */
export const hasValue = (v: unknown) =>
	v !== undefined && v !== null && v !== '' && v !== false && !(Array.isArray(v) && v.length === 0);

/** Whether a condition has nothing chosen yet, so it is ignored. */
function unset(c: Record<string, unknown>): boolean {
	const v = c.value;
	switch (c.op) {
		case 'any':
		case 'all':
		case 'none_of':
			return v === undefined || v === null || (Array.isArray(v) && v.length === 0);
		case 'contains':
		case 'not_contains':
			return v === undefined || v === null || (typeof v === 'string' && v.trim() === '');
	}
	return false;
}

function realDay(s: string): boolean {
	if (!DAY.test(s)) return false;
	const [y, m, d] = s.split('-').map(Number);
	const leap = (y % 4 === 0 && y % 100 !== 0) || y % 400 === 0;
	const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][m - 1];
	return days !== undefined && d >= 1 && d <= days;
}

/** A date bound as a day, or undefined if it is not a valid one. */
function bound(v: unknown, today: string): string | undefined {
	if (typeof v === 'string') return realDay(v) ? v : undefined;
	if (typeof v === 'number' && Number.isInteger(v) && Math.abs(v) <= MAX_OFFSET)
		return addDays(today, v);
	return undefined;
}

const finite = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v);
const strings = (v: unknown): v is string[] =>
	Array.isArray(v) && v.every((x) => typeof x === 'string');

/** Whether a task passes one field condition. */
/** An object with a string field and op: a condition that can be read. */
const wellFormed = (c: unknown): c is Record<string, unknown> & { field: string; op: string } =>
	isObject(c) && typeof c.field === 'string' && typeof c.op === 'string';

export function matchesField(cond: unknown, task: Task, ctx: FilterContext): boolean {
	if (!wellFormed(cond)) return false;
	if (unset(cond)) return true;
	const op = cond.op as FieldOp;
	const type = ctx.fieldType?.(cond.field);
	if (!type) return NEGATIVE.includes(op);
	// A type from a newer app has no operators here: the condition is malformed.
	if (!FIELD_OPS[type]?.includes(op)) return false;
	const stored: unknown = ctx.value?.(task.id, cond.field);
	const has = hasValue(stored);
	const v = cond.value;
	switch (op) {
		case 'empty':
			return !has;
		case 'not_empty':
			return has;
		case 'checked':
			return stored === true;
		case 'unchecked':
			return stored !== true;
		case 'contains':
		case 'not_contains': {
			if (typeof v !== 'string') return false;
			const needle = v.trim().toLowerCase();
			const found = typeof stored === 'string' && stored.toLowerCase().includes(needle);
			if (op === 'contains') return found;
			return !has || (typeof stored === 'string' && !found);
		}
		case 'eq':
		case 'lt':
		case 'lte':
		case 'gt':
		case 'gte': {
			if (!finite(v) || !finite(stored)) return false;
			if (op === 'eq') return stored === v;
			if (op === 'lt') return stored < v;
			if (op === 'lte') return stored <= v;
			if (op === 'gt') return stored > v;
			return stored >= v;
		}
		case 'on':
		case 'before':
		case 'after':
		case 'between': {
			let from: string | undefined;
			let to: string | undefined;
			if (op === 'between') {
				if (!Array.isArray(v) || v.length !== 2) return false;
				from = bound(v[0], ctx.today);
				to = bound(v[1], ctx.today);
			} else from = to = bound(v, ctx.today);
			if (from === undefined || to === undefined) return false;
			if (typeof stored !== 'string' || !DAY.test(stored)) return false;
			if (op === 'on') return stored === from;
			if (op === 'before') return stored < from;
			if (op === 'after') return stored > from;
			return from <= stored && stored <= to;
		}
		case 'any':
		case 'all':
		case 'none_of': {
			if (!strings(v)) return false;
			const set = new Set(type === 'user' ? v.map((x) => (x === 'me' ? ctx.me : x)) : v);
			if (type === 'multi_select') {
				if (!Array.isArray(stored)) return op === 'none_of' && !has;
				if (op === 'any') return stored.some((x) => set.has(x as string));
				if (op === 'all') return [...set].every((x) => stored.includes(x));
				return !stored.some((x) => set.has(x as string));
			}
			if (typeof stored !== 'string') return op === 'none_of' && !has;
			return op === 'any' ? set.has(stored) : !set.has(stored);
		}
	}
	return false;
}

/** How many parts of a filter are set, for a badge. A malformed part counts. */
export function filterCount(f: ViewFilter): number {
	const p = partsOf(f);
	if (!p) return 1;
	// A part counts when it is malformed or when it holds something.
	const set = <T>(v: Part<T>, on: (v: T) => boolean) =>
		v === BAD || (v !== undefined && on(v)) ? 1 : 0;
	// Categories and statuses picked by ID are one row of the panel, so one
	// part; each of them malformed counts too.
	const statusRow =
		(p.statuses === BAD ? 1 : 0) +
		(p.statusIds === BAD ? 1 : 0) +
		(some(p.statuses) || some(p.statusIds) ? 1 : 0);
	const lists: Part<string[]>[] = [
		p.projects,
		p.priorities,
		p.labels,
		p.assignees,
		p.dependencies,
		p.time
	];
	return (
		statusRow +
		lists.filter((l) => l === BAD || some(l)).length +
		set(p.due, (d) => d !== '') +
		set(p.text, (t) => t.trim() !== '') +
		set(p.showDone, (on) => on) +
		set(p.myDay, (on) => on) +
		(p.fields === BAD
			? 1
			: (p.fields ?? []).filter((c: unknown) => !(wellFormed(c) && unset(c))).length)
	);
}

/**
 * What a table or timeline keeps of a filter's answer: a task that matches,
 * or one with a subtask somewhere under it that does, so the match shows in
 * its place in the tree. Each task is worked out once, and a loop ends.
 */
export function keepsMatches(
	hit: (task: Task) => boolean,
	subtasks: (taskId: string) => Task[]
): (task: Task) => boolean {
	const known = new Map<string, boolean>();
	const keeps = (task: Task): boolean => {
		const seen = known.get(task.id);
		if (seen !== undefined) return seen;
		known.set(task.id, false);
		const out = hit(task) || subtasks(task.id).some(keeps);
		known.set(task.id, out);
		return out;
	};
	return keeps;
}
