// How a saved view lays out the tasks its filter shows: sort, grouping
// (lists), columns (boards) and the fields on rows and cards. Pure, so the
// rules are tested without a replica: the view page reads what these need
// from the replica once per change and passes it in.
import { addDays } from './filter';
import { bySortKey } from './sync/sortkey';
import type {
	Columns,
	CustomField,
	FieldShow,
	FieldValue,
	Grouping,
	GroupBy,
	Label,
	Priority,
	Project,
	SortBy,
	SortRule,
	Status,
	StatusCategory,
	Task,
	ViewDisplay,
	ViewType
} from './sync/entities';

export { addDays };

/** The one collator for titles and names: case-blind, numbers by value. */
const collator = new Intl.Collator(undefined, { sensitivity: 'base', numeric: true });

export const CATEGORIES: { value: StatusCategory; label: string }[] = [
	{ value: 'backlog', label: 'Backlog' },
	{ value: 'todo', label: 'To do' },
	{ value: 'in-progress', label: 'In progress' },
	{ value: 'done', label: 'Done' },
	{ value: 'canceled', label: 'Canceled' }
];

export const PRIORITY_ORDER: { value: Priority; label: string }[] = [
	{ value: 'urgent', label: 'Urgent' },
	{ value: 'high', label: 'High' },
	{ value: 'medium', label: 'Medium' },
	{ value: 'low', label: 'Low' },
	{ value: 'none', label: 'No priority' }
];

const PRIORITY_RANK: Record<Priority, number | null> = {
	urgent: 4,
	high: 3,
	medium: 2,
	low: 1,
	none: null
};

/** The fields a row or card can show, in the order they are drawn. */
export const BUILTIN_FIELDS: { id: string; label: string }[] = [
	{ id: 'status', label: 'Status' },
	{ id: 'project', label: 'Project' },
	{ id: 'priority', label: 'Priority' },
	{ id: 'labels', label: 'Labels' },
	{ id: 'subtasks', label: 'Subtasks' },
	{ id: 'checklist', label: 'Checklist' },
	{ id: 'comments', label: 'Comments' },
	{ id: 'repeat', label: 'Repeat' },
	{ id: 'reminders', label: 'Reminders' },
	{ id: 'assignees', label: 'Assignees' },
	{ id: 'start', label: 'Start date' },
	{ id: 'due', label: 'Due date' },
	{ id: 'estimate', label: 'Estimate' },
	{ id: 'logged', label: 'Time logged' },
	{ id: 'remaining', label: 'Remaining' }
];

const LIST_FIELDS = [
	'status',
	'priority',
	'labels',
	'subtasks',
	'checklist',
	'comments',
	'repeat',
	'reminders',
	'assignees',
	'due',
	'estimate',
	'logged'
];
/** The kinds of view that choose their shown fields. */
export type FieldsKind = 'list' | 'board' | 'table';

/**
 * What a row, card or table shows when the view says nothing: for lists and
 * boards what they always showed; a table also shows every custom field.
 */
export const DEFAULT_FIELDS: Record<FieldsKind, string[]> = {
	list: LIST_FIELDS,
	board: LIST_FIELDS.filter((f) => f !== 'status'),
	table: ['status', 'priority', 'assignees', 'due', 'project', 'estimate', 'logged']
};

const BUILTIN_IDS = new Set(BUILTIN_FIELDS.map((f) => f.id));

/**
 * Whether a kind of view shows a field when it says nothing about it. A
 * board across projects shows each card's project, as nothing else there says it.
 */
export function shownByDefault(kind: FieldsKind, id: string, across = false): boolean {
	return (
		DEFAULT_FIELDS[kind].includes(id) ||
		(kind === 'table' && !BUILTIN_IDS.has(id)) ||
		(kind === 'board' && across && id === 'project')
	);
}

/** What sorting by each key is called, and its two directions in words. */
export const SORT_KEYS: { by: Exclude<SortBy, 'field'>; label: string }[] = [
	{ by: 'manual', label: 'Manual order' },
	{ by: 'title', label: 'Title' },
	{ by: 'priority', label: 'Priority' },
	{ by: 'due', label: 'Due date' },
	{ by: 'start', label: 'Start date' },
	{ by: 'created', label: 'Created' },
	{ by: 'updated', label: 'Last changed' },
	{ by: 'completed', label: 'Completed' },
	{ by: 'status', label: 'Status' },
	{ by: 'project', label: 'Project' },
	{ by: 'estimate', label: 'Estimate' },
	{ by: 'logged', label: 'Time logged' },
	{ by: 'remaining', label: 'Remaining' }
];

/** The field types a view can sort by; a multi-select has no one value to sort on. */
const SORTABLE = new Set(['text', 'url', 'number', 'date', 'select', 'user', 'checkbox']);
/** The field types a list can group by and a board can have as columns. */
const GROUPABLE = new Set(['select', 'user']);

/** The direction a new rule starts with. */
export function defaultDir(by: SortBy): 'asc' | 'desc' {
	return by === 'priority' ||
		by === 'created' ||
		by === 'updated' ||
		by === 'completed' ||
		by === 'estimate' ||
		by === 'logged'
		? 'desc'
		: 'asc';
}

/** A direction in words, for the panel and what is said aloud. */
export function dirLabel(rule: SortRule, field?: CustomField): string {
	const asc = rule.dir !== 'desc';
	switch (rule.by) {
		case 'title':
			return asc ? 'A to Z' : 'Z to A';
		case 'priority':
			return asc ? 'Lowest first' : 'Highest first';
		case 'due':
		case 'start':
			return asc ? 'Soonest first' : 'Latest first';
		case 'created':
		case 'updated':
		case 'completed':
			return asc ? 'Oldest first' : 'Newest first';
		case 'estimate':
		case 'logged':
		case 'remaining':
			return asc ? 'Least first' : 'Most first';
		case 'field':
			switch (field?.type) {
				case 'number':
					return asc ? 'Lowest first' : 'Highest first';
				case 'date':
					return asc ? 'Earliest first' : 'Latest first';
				case 'checkbox':
					return asc ? 'Unchecked first' : 'Checked first';
				case 'text':
				case 'url':
				case 'user':
					return asc ? 'A to Z' : 'Z to A';
			}
	}
	return asc ? 'First to last' : 'Last to first';
}

/** What a sort rule sorts by, in words. */
export function sortLabel(rule: SortRule, fields: CustomField[]): string {
	if (rule.by === 'field')
		return fields.find((f) => f.id === rule.field)?.name ?? 'A deleted field';
	return SORT_KEYS.find((k) => k.by === rule.by)?.label ?? 'Not available';
}

/** "Sorted by due date, soonest first", or "Manual order". */
export function sortSaid(rules: SortRule[], fields: CustomField[]): string {
	if (!rules.length) return 'Manual order';
	return (
		'Sorted by ' +
		rules
			.map((r) => {
				const field = fields.find((f) => f.id === r.field);
				const name = r.by === 'field' ? sortLabel(r, fields) : sortLabel(r, fields).toLowerCase();
				return `${name}, ${dirLabel(r, field).toLowerCase()}`;
			})
			.join(', then ')
	);
}

export const GROUP_LABELS: Record<Exclude<GroupBy, 'field'>, string> = {
	none: 'No grouping',
	project: 'Project',
	status: 'Status',
	priority: 'Priority',
	assignee: 'Assignee',
	label: 'Label',
	due: 'Due date'
};

/** "Grouped by priority", or "Not grouped". */
export function groupSaid(group: Grouping, fields: CustomField[]): string {
	if (group.by === 'none') return 'Not grouped';
	if (group.by === 'field')
		return `Grouped by ${fields.find((f) => f.id === group.field)?.name ?? 'a field'}`;
	return `Grouped by ${GROUP_LABELS[group.by].toLowerCase()}`;
}

/** What resolveDisplay needs to know. */
export interface DisplayLookups {
	/** The view's project, or null for a view across projects. */
	projectId: string | null;
	/** The custom fields the view's tasks can have, in order. */
	fields: CustomField[];
	/** Without a project, whether its tasks all have none (the Inbox), so cards need not say so. */
	inbox?: boolean;
	/** A board's columns when the view says none: a list's own (Upcoming's days). */
	columns?: Columns;
}

/** Whether a view's tasks come from more than one project, so cards say whose. */
export const acrossProjects = (look: DisplayLookups): boolean => !look.projectId && !look.inbox;

/** A view's display with every default filled in and every reference checked. */
export interface Resolved {
	/** Empty is manual order. */
	sort: SortRule[];
	group: Grouping;
	columns: Columns;
	/** The fields rows, cards or columns show: built-in names and field IDs, in drawing order. */
	shown: string[];
}

/** The kind of shown fields a type of view keeps; a calendar keeps a list's. */
export const fieldsKind = (type: ViewType): FieldsKind =>
	type === 'board' ? 'board' : type === 'table' ? 'table' : 'list';

const SORT_BY = new Set<string>([...SORT_KEYS.map((k) => k.by), 'field']);
const GROUP_BY = new Set<string>([...Object.keys(GROUP_LABELS), 'field']);

/** Whether a sort rule can be used in a view, as it stands. */
export function sortRuleValid(rule: unknown, look: DisplayLookups): rule is SortRule {
	if (typeof rule !== 'object' || rule === null) return false;
	const r = rule as Partial<SortRule>;
	if (typeof r.by !== 'string' || !SORT_BY.has(r.by)) return false;
	if (r.dir !== 'asc' && r.dir !== 'desc') return false;
	if (r.by === 'project' && look.projectId) return false;
	if (r.by === 'field') {
		const f = look.fields.find((x) => x.id === r.field);
		return !!f && SORTABLE.has(f.type);
	}
	return true;
}

/** Whether a grouping can be used in a list view, as it stands. */
export function groupValid(group: unknown, look: DisplayLookups): group is Grouping {
	if (typeof group !== 'object' || group === null) return false;
	const g = group as Partial<Grouping>;
	if (typeof g.by !== 'string' || !GROUP_BY.has(g.by)) return false;
	if (g.by === 'project' && look.projectId) return false;
	if (g.by === 'field') return groupableField(g.field, look);
	return true;
}

/** Whether columns can be used on a board, as they stand. */
export function columnsValid(columns: unknown, look: DisplayLookups): columns is Columns {
	if (typeof columns !== 'object' || columns === null) return false;
	const c = columns as Partial<Columns>;
	// Statuses are a project's own, and so is a field of one project: across
	// projects, a card can only move into what every task has.
	if (c.by === 'status') return !!look.projectId;
	if (c.by === 'priority' || c.by === 'due') return true;
	return c.by === 'field' && columnFields(look).some((f) => f.id === c.field);
}

/** The fields a board can make columns of: across projects, only the workspace's. */
export function columnFields(look: DisplayLookups): CustomField[] {
	return look.fields.filter(
		(f) => GROUPABLE.has(f.type) && (look.projectId !== null || f.projectId === null)
	);
}

function groupableField(id: unknown, look: DisplayLookups): boolean {
	const f = look.fields.find((x) => x.id === id);
	return !!f && GROUPABLE.has(f.type);
}

/** The fields a list or board offers to show: built-ins, then custom fields. */
export function offeredFields(look: DisplayLookups): { id: string; label: string }[] {
	return [
		...BUILTIN_FIELDS.filter((f) => !(f.id === 'project' && look.projectId)),
		...look.fields.map((f) => ({ id: f.id, label: f.name }))
	];
}

/**
 * The fields a type of view shows: its defaults with the view's own show
 * and hide laid over them, built-ins in drawing order, then custom fields
 * in their own order. References to fields not known are dropped.
 */
export function shownFields(
	type: FieldsKind,
	over: FieldShow | undefined,
	look: DisplayLookups
): string[] {
	const show = Array.isArray(over?.show) ? over.show : [];
	const hide = Array.isArray(over?.hide) ? over.hide : [];
	const across = acrossProjects(look);
	const on = (id: string) =>
		!hide.includes(id) && (shownByDefault(type, id, across) || show.includes(id));
	return [
		...BUILTIN_FIELDS.filter((f) => !(f.id === 'project' && look.projectId) && on(f.id)).map(
			(f) => f.id
		),
		...look.fields.filter((f) => on(f.id)).map((f) => f.id)
	];
}

/** The default grouping of a list: by status in a project, by project across them. */
export const defaultGroup = (look: DisplayLookups): Grouping => ({
	by: look.projectId ? 'status' : 'project'
});

/**
 * A view's effective display: its own choices where they make sense for
 * the view and name things the lookups know, and the defaults (which are
 * how views always looked) everywhere else.
 */
export function resolveDisplay(
	display: ViewDisplay | null | undefined,
	type: ViewType,
	look: DisplayLookups
): Resolved {
	const d = display ?? {};
	const sort: SortRule[] = [];
	for (const rule of Array.isArray(d.sort) ? d.sort : []) {
		if (sort.length === 3 || (rule as SortRule)?.by === 'manual') break;
		if (sortRuleValid(rule, look)) sort.push(rule);
	}
	const kind = fieldsKind(type);
	return {
		sort: type === 'calendar' ? [] : sort,
		group:
			(type === 'list' || type === 'table' || type === 'timeline') && groupValid(d.group, look)
				? d.group
				: defaultGroup(look),
		// Statuses belong to a project; across projects, priorities are what all tasks share.
		columns:
			type === 'board' && columnsValid(d.columns, look)
				? d.columns
				: look.columns
					? look.columns
					: look.projectId
						? { by: 'status' }
						: { by: 'priority' },
		shown: shownFields(kind, d.fields?.[kind], look)
	};
}

/** Whether a view's display differs from the defaults, for a dot on its button. */
export function differs(r: Resolved, type: ViewType, look: DisplayLookups): boolean {
	const base = resolveDisplay({}, type, look);
	return (
		r.sort.length > 0 ||
		((type === 'list' || type === 'table' || type === 'timeline') &&
			(r.group.by !== base.group.by || r.group.field !== base.group.field)) ||
		(type === 'board' &&
			(r.columns.by !== base.columns.by || r.columns.field !== base.columns.field)) ||
		r.shown.join() !== base.shown.join()
	);
}

export { applyPaths, type DisplayPath } from './displayPaths';

/**
 * The chips a choice condition offers: a person field's Me and members, a
 * select's options; then every chosen value with no chip of its own (a
 * deleted option, someone who left, the viewer's own ID saved by someone
 * else), so nothing filters out of sight and each can be taken out.
 */
export function conditionChoices(
	field: CustomField,
	chosen: string[],
	people: {
		me: string;
		members: { userId: string; name: string }[];
		nameOf: (id: string) => string;
	}
): { id: string; name: string }[] {
	const base =
		field.type === 'user'
			? [
					{ id: 'me', name: 'Me' },
					...people.members
						.filter((m) => m.userId !== people.me)
						.map((m) => ({ id: m.userId, name: m.name }))
				]
			: field.options.map((o) => ({ id: o.id, name: o.name }));
	const known = new Set(base.map((c) => c.id));
	const extra = [...new Set(chosen)]
		.filter((id) => !known.has(id))
		.map((id) => ({
			id,
			name:
				field.type !== 'user'
					? 'A deleted choice'
					: id === people.me
						? `${people.nameOf(id)} (you)`
						: people.nameOf(id)
		}));
	return [...base, ...extra];
}

/**
 * A view's sort rules as stored, up to three and up to a manual rule, each
 * kept even when this app cannot use it, so it can be seen and removed.
 */
export function storedSort(display: ViewDisplay | null | undefined): unknown[] {
	const all = Array.isArray(display?.sort) ? (display.sort as unknown[]) : [];
	const manual = all.findIndex((r) => (r as SortRule)?.by === 'manual');
	return all.slice(0, manual < 0 ? 3 : Math.min(manual, 3));
}

/**
 * A type's show and hide with one field turned on or off over the defaults,
 * keeping any other IDs in them; undefined when nothing differs.
 */
export function shownPatch(
	display: ViewDisplay | null | undefined,
	type: FieldsKind,
	id: string,
	on: boolean,
	across = false
): FieldShow | undefined {
	const over = (display?.fields?.[type] ?? {}) as FieldShow;
	const byDefault = shownByDefault(type, id, across);
	const show = (Array.isArray(over.show) ? over.show : []).filter((x) => x !== id);
	const hide = (Array.isArray(over.hide) ? over.hide : []).filter((x) => x !== id);
	if (on && !byDefault) show.push(id);
	if (!on && byDefault) hide.push(id);
	const next: FieldShow = { ...over, show, hide };
	if (!show.length) delete next.show;
	if (!hide.length) delete next.hide;
	return Object.keys(next).length ? next : undefined;
}

// Sorting

/** What sorting needs to know beyond the tasks themselves. */
export interface SortContext {
	category: (task: Task) => StatusCategory;
	status: (task: Task) => Status | undefined;
	/** The projects in sidebar order. */
	projects: Project[];
	times: (taskId: string) => { created: string; updated: string };
	fields: CustomField[];
	values: Map<string, Map<string, FieldValue>>;
	/** A member's name, or undefined for someone not known. */
	memberName: (userId: string) => string | undefined;
	/** The minutes logged on a task itself; none when not given. */
	logged?: (taskId: string) => number;
}

type Key = string | number | null;
/** How one column of keys compares: plainly, or through the collator. */
type Column = { collate: boolean; dir: 1 | -1; key: (t: Task) => Key };

const CATEGORY_INDEX = Object.fromEntries(CATEGORIES.map((c, i) => [c.value, i])) as Record<
	StatusCategory,
	number
>;

function columnsFor(rule: SortRule, ctx: SortContext): Column[] {
	const dir = rule.dir === 'desc' ? -1 : 1;
	const plain = (key: (t: Task) => Key): Column[] => [{ collate: false, dir, key }];
	switch (rule.by) {
		case 'title':
			return [{ collate: true, dir, key: (t) => t.title || null }];
		case 'priority':
			return plain((t) => PRIORITY_RANK[t.priority] ?? null);
		case 'due':
			// A dated task without a time comes after one with a time that day.
			return plain((t) => (t.dueDate ? `${t.dueDate} ${t.dueTime ?? '~'}` : null));
		case 'start':
			return plain((t) => t.startDate);
		case 'completed':
			return plain((t) => t.completedAt);
		case 'created':
			return plain((t) => ctx.times(t.id).created || null);
		case 'updated':
			return plain((t) => ctx.times(t.id).updated || null);
		case 'status':
			return plain(
				(t) => `${CATEGORY_INDEX[ctx.category(t)] ?? 9}|${ctx.status(t)?.sortKey ?? ''}`
			);
		case 'project': {
			const index = new Map(ctx.projects.map((p, i) => [p.id, i]));
			return plain((t) => (t.projectId ? (index.get(t.projectId) ?? null) : -1));
		}
		case 'estimate':
			return plain((t) => t.estimateMinutes);
		case 'logged':
			// Nothing logged sorts with the empty, last.
			return plain((t) => ctx.logged?.(t.id) || null);
		case 'remaining':
			// Past the estimate is less than none left.
			return plain((t) =>
				t.estimateMinutes === null ? null : t.estimateMinutes - (ctx.logged?.(t.id) ?? 0)
			);
		case 'field':
			return fieldColumns(rule, dir, ctx);
	}
	return [];
}

/** A non-empty string value, or null. */
const text = (v: unknown): string | null => (typeof v === 'string' && v ? v : null);

function fieldColumns(rule: SortRule, dir: 1 | -1, ctx: SortContext): Column[] {
	const field = ctx.fields.find((f) => f.id === rule.field);
	const values = ctx.values.get(rule.field ?? '');
	const get = (t: Task) => values?.get(t.id);
	switch (field?.type) {
		case 'number':
			return [
				{
					collate: false,
					dir,
					key: (t) => {
						const v = get(t);
						return typeof v === 'number' && Number.isFinite(v) ? v : null;
					}
				}
			];
		case 'date':
			return [{ collate: false, dir, key: (t) => text(get(t)) }];
		case 'text':
		case 'url':
			return [{ collate: true, dir, key: (t) => text(get(t)) }];
		case 'select': {
			const index = new Map(field.options.map((o, i) => [o.id, i]));
			return [{ collate: false, dir, key: (t) => index.get(get(t) as string) ?? null }];
		}
		case 'checkbox':
			return [{ collate: false, dir, key: (t) => (get(t) === true ? 1 : 0) }];
		case 'user': {
			// Someone no member is sorts after the named, whichever way.
			const name = (t: Task) => {
				const v = get(t);
				return typeof v === 'string' && v ? v : null;
			};
			return [
				{
					collate: false,
					dir: 1,
					key: (t) => {
						const id = name(t);
						return id === null ? null : ctx.memberName(id) === undefined ? 1 : 0;
					}
				},
				{
					collate: true,
					dir,
					key: (t) => {
						const id = name(t);
						return id === null ? null : (ctx.memberName(id) ?? id);
					}
				}
			];
		}
	}
	return [];
}

/**
 * Tasks in a view's order. Each task's keys are worked out once; empty
 * values go last whichever the direction, and ties keep manual order, so
 * no rules at all is manual order exactly.
 */
export function sortTasks(tasks: Task[], rules: SortRule[], ctx: SortContext): Task[] {
	const columns = rules.flatMap((r) => columnsFor(r, ctx));
	if (!columns.length) return [...tasks].sort(bySortKey);
	const decorated = tasks.map((task) => ({ task, keys: columns.map((c) => c.key(task)) }));
	decorated.sort((a, b) => {
		for (let i = 0; i < columns.length; i++) {
			const x = a.keys[i];
			const y = b.keys[i];
			if (x === y) continue;
			if (x === null) return 1;
			if (y === null) return -1;
			const c = columns[i];
			const n = c.collate ? collator.compare(String(x), String(y)) : x < y ? -1 : x > y ? 1 : 0;
			if (n) return n * c.dir;
		}
		return bySortKey(a.task, b.task);
	});
	return decorated.map((d) => d.task);
}

// Grouping

/** A group of rows under a heading, as TaskSections draws them. */
export interface Section {
	id: string;
	title: string;
	tasks: Task[];
	tone?: 'danger';
}

/** What grouping needs to know. */
export interface GroupContext {
	me: string;
	today: string;
	/** The view's project, or null across projects. */
	projectId: string | null;
	projects: Project[];
	/** The view's project's statuses, in order. */
	statuses: Status[];
	category: (task: Task) => StatusCategory;
	labels: Label[];
	taskLabels: (taskId: string) => string[];
	assignees: (taskId: string) => string[];
	fields: CustomField[];
	values: Map<string, Map<string, FieldValue>>;
	/** The workspace's members, for assignee and person groups. */
	members: { userId: string; name: string }[];
	/** Anyone's name, members or not. */
	nameOf: (userId: string) => string;
}

/** Which of Upcoming's and the due grouping's buckets a due date is in. */
export function dueBucket(
	date: string | null,
	today: string
): 'overdue' | 'today' | 'tomorrow' | 'week' | 'later' | 'none' {
	if (!date) return 'none';
	if (date < today) return 'overdue';
	if (date === today) return 'today';
	if (date === addDays(today, 1)) return 'tomorrow';
	if (date <= addDays(today, 7)) return 'week';
	return 'later';
}

const DUE_BUCKETS: { id: ReturnType<typeof dueBucket>; title: string }[] = [
	{ id: 'overdue', title: 'Overdue' },
	{ id: 'today', title: 'Today' },
	{ id: 'tomorrow', title: 'Tomorrow' },
	{ id: 'week', title: 'This week' },
	{ id: 'later', title: 'Later' },
	{ id: 'none', title: 'No date' }
];

/** People in order: me first, then the rest by name. */
export function peopleOrder(
	ids: Iterable<string>,
	ctx: { me: string; nameOf: (userId: string) => string }
): { id: string; title: string }[] {
	const others = [...new Set(ids)]
		.filter((id) => id !== ctx.me)
		.map((id) => ({ id, title: ctx.nameOf(id) }))
		.sort((a, b) => collator.compare(a.title, b.title) || (a.id < b.id ? -1 : 1));
	return [{ id: ctx.me, title: 'Me' }, ...others];
}

/**
 * Tasks in the sections of a grouping, in one pass, keeping their order
 * within each. A task with several labels or assignees is in each of
 * their groups. Empty sections are left in; TaskSections hides them.
 */
export function groupTasks(tasks: Task[], grouping: Grouping, ctx: GroupContext): Section[] {
	const sections: Section[] = [];
	const byId = new Map<string, Section>();
	const add = (id: string, title: string, tone?: 'danger') => {
		const s: Section = { id, title, tasks: [], ...(tone ? { tone } : {}) };
		sections.push(s);
		byId.set(id, s);
	};
	const put = (id: string, task: Task) => byId.get(id)?.tasks.push(task);

	switch (grouping.by) {
		case 'project': {
			add('p:inbox', 'Inbox');
			for (const p of ctx.projects) add(`p:${p.id}`, p.name);
			for (const t of tasks) put(`p:${t.projectId ?? 'inbox'}`, t);
			break;
		}
		case 'status': {
			if (ctx.projectId) {
				for (const s of ctx.statuses) add(`s:${s.id}`, s.name);
				const first = ctx.statuses[0];
				for (const t of tasks) {
					const id = t.statusId && byId.has(`s:${t.statusId}`) ? t.statusId : first?.id;
					if (id) put(`s:${id}`, t);
				}
			} else {
				for (const c of CATEGORIES) add(`c:${c.value}`, c.label);
				for (const t of tasks) put(`c:${ctx.category(t)}`, t);
			}
			break;
		}
		case 'priority': {
			for (const p of PRIORITY_ORDER) add(`prio:${p.value}`, p.label);
			for (const t of tasks) put(`prio:${t.priority}`, t);
			break;
		}
		case 'assignee': {
			const on = tasks.map((t) => ctx.assignees(t.id));
			for (const p of peopleOrder([...ctx.members.map((m) => m.userId), ...on.flat()], ctx))
				add(`u:${p.id}`, p.title);
			add('u:none', 'Nobody');
			tasks.forEach((t, i) => {
				if (!on[i].length) put('u:none', t);
				for (const u of on[i]) put(`u:${u}`, t);
			});
			break;
		}
		case 'label': {
			for (const l of ctx.labels) add(`l:${l.id}`, l.name);
			add('l:none', 'No label');
			for (const t of tasks) {
				const on = ctx.taskLabels(t.id).filter((l) => byId.has(`l:${l}`));
				if (!on.length) put('l:none', t);
				for (const l of on) put(`l:${l}`, t);
			}
			break;
		}
		case 'due': {
			for (const b of DUE_BUCKETS)
				add(`due:${b.id}`, b.title, b.id === 'overdue' ? 'danger' : undefined);
			for (const t of tasks) put(`due:${dueBucket(t.dueDate, ctx.today)}`, t);
			break;
		}
		case 'field': {
			const field = ctx.fields.find((f) => f.id === grouping.field);
			const values = ctx.values.get(grouping.field ?? '');
			const choices =
				field?.type === 'user'
					? peopleOrder(
							[
								...ctx.members.map((m) => m.userId),
								...[...(values?.values() ?? [])].filter((v): v is string => typeof v === 'string')
							],
							ctx
						)
					: (field?.options ?? []).map((o) => ({ id: o.id, title: o.name }));
			for (const c of choices) add(`f:${c.id}`, c.title);
			add('f:none', 'None');
			for (const t of tasks) {
				const v = values?.get(t.id);
				put(typeof v === 'string' && byId.has(`f:${v}`) ? `f:${v}` : 'f:none', t);
			}
			break;
		}
		default:
			add('none', 'Tasks');
			sections[0].tasks = [...tasks];
	}
	return sections;
}

// Board columns

/** What moving a card into a column writes, and what a new card there gets. */
export type ColumnWrite =
	| { status: string }
	| { priority: Priority }
	| { due: string | null }
	| { field: string; value: FieldValue };

/** What a column's header shows beside its name. */
export type ColumnIcon =
	| { kind: 'status'; category: StatusCategory }
	| { kind: 'priority'; priority: Priority }
	| { kind: 'color'; color: string }
	| { kind: 'person'; name: string };

export interface BoardColumn {
	id: string;
	title: string;
	icon?: ColumnIcon;
	has: (task: Task) => boolean;
	/** What moving a card here writes; null for a column nothing can move into (Overdue). */
	write: ColumnWrite | null;
}

/** What a board's columns need to know. */
export interface ColumnContext {
	me: string;
	statuses: Status[];
	fields: CustomField[];
	values: Map<string, Map<string, FieldValue>>;
	members: { userId: string; name: string }[];
	nameOf: (userId: string) => string;
	/** For date columns: today, a day's name as the person writes dates, and whether every task is open and dated (Upcoming). */
	today?: string;
	dayName?: (day: string) => string;
	dated?: boolean;
}
