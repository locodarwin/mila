// Taking a template's snapshot from the workspace, and the rows that
// making something from one writes (docs/PLAN.md section 3). Pure, and
// loaded only when a template is saved or used (templates.ts is what every
// page needs).
import type {
	Automation,
	CustomField,
	FieldValue,
	Label,
	Page,
	Project,
	SavedView,
	Status,
	StatusCategory,
	Task
} from './sync/entities';
import { pairId } from './sync/entities';
import { mainPageId } from './sync/pages';
import { keyBetween } from './sync/sortkey';
import type { TemplateData, TemplateTask } from './templates';

// Days are whole days since 1970-01-01.
const dayOf = (iso: string) => {
	const [y, m, d] = iso.split('-').map(Number);
	return Math.round(Date.UTC(y, m - 1, d) / 864e5);
};
const isoOf = (day: number) => new Date(day * 864e5).toISOString().slice(0, 10);

/** What a snapshot needs to read of the workspace. */
export interface Source {
	project(id: string): Project | undefined;
	task(id: string): Task | undefined;
	tasks(projectId: string | null): Task[];
	subtasks(taskId: string): Task[];
	statuses(projectId: string): Status[];
	status(id: string | null): Status | undefined;
	fields(projectId?: string | null): CustomField[];
	fieldValues(taskId: string): Record<string, FieldValue>;
	taskLabels(taskId: string): Label[];
	assignees(taskId: string): string[];
	checklist(taskId: string): { text: string }[];
	blockers(taskId: string): Task[];
	pages(projectId: string | null): Page[];
	views(userId: string): SavedView[];
	automations(): Automation[];
	comments(taskId: string): unknown[];
	attachments(taskId: string): unknown[];
}

/** What saving left behind, for the dialog to say. */
export interface LeftBehind {
	done: number;
	comments: number;
	files: number;
}

export interface Snapshot {
	data: TemplateData;
	/** The day dates count from: the earliest date (project), or today (task). */
	anchor: string | null;
	left: LeftBehind;
}

/** A task and its open subtasks, depth first; done ones and their subtasks counted, not kept. */
function openTree(src: Source, roots: Task[], left: LeftBehind): Task[] {
	const out: Task[] = [];
	const walk = (t: Task, depth: number) => {
		if (t.completedAt) {
			left.done += 1 + countUnder(src, t);
			return;
		}
		out.push(t);
		left.comments += src.comments(t.id).length;
		left.files += src.attachments(t.id).length;
		if (depth < 20) for (const k of src.subtasks(t.id)) walk(k, depth + 1);
	};
	for (const r of roots) walk(r, 0);
	return out;
}

function countUnder(src: Source, t: Task): number {
	let n = 0;
	for (const k of src.subtasks(t.id)) n += 1 + countUnder(src, k);
	return n;
}

function taskOf(
	src: Source,
	t: Task,
	keep: Set<string>,
	anchor: number | null,
	opts: { keepAssignees: boolean; fieldOK: (id: string) => boolean }
): TemplateTask {
	const rel = (iso: string | null) => (iso && anchor !== null ? dayOf(iso) - anchor : null);
	const values: Record<string, FieldValue> = {};
	for (const [id, v] of Object.entries(src.fieldValues(t.id))) if (opts.fieldOK(id)) values[id] = v;
	return {
		id: t.id,
		title: t.title,
		parent: t.parentId && keep.has(t.parentId) ? t.parentId : null,
		notes: t.notes,
		priority: t.priority,
		status: t.statusId,
		category: src.status(t.statusId)?.category ?? null,
		start: rel(t.startDate),
		due: rel(t.dueDate),
		due_time: t.dueTime,
		recurrence: t.recurrence,
		recurrence_mode: t.recurrenceMode,
		estimate: t.estimateMinutes,
		labels: src.taskLabels(t.id).map((l) => ({ id: l.id, name: l.name, color: l.color })),
		values,
		checklist: src.checklist(t.id).map((i) => i.text),
		...(opts.keepAssignees ? { assignees: src.assignees(t.id) } : {}),
		waits: src
			.blockers(t.id)
			.filter((b) => keep.has(b.id))
			.map((b) => b.id)
	};
}

/**
 * A project as a template: its open tasks, dates counted from the
 * earliest of them, and its statuses, own fields, saved views (shared and
 * the saver's) and wiki pages.
 */
export function snapshotProject(
	src: Source,
	projectId: string,
	opts: { keepAssignees: boolean; me: string }
): Snapshot | null {
	const project = src.project(projectId);
	if (!project) return null;
	const left: LeftBehind = { done: 0, comments: 0, files: 0 };
	const tasks = openTree(src, src.tasks(projectId), left);
	const keep = new Set(tasks.map((t) => t.id));
	const dates = tasks.flatMap((t) => [t.startDate, t.dueDate]).filter((d): d is string => !!d);
	const anchor = dates.length ? Math.min(...dates.map(dayOf)) : null;
	const own = src.fields(projectId).filter((f) => f.projectId === projectId);
	const live = new Set(src.fields(projectId).map((f) => f.id));
	return {
		anchor: anchor === null ? null : isoOf(anchor),
		left,
		data: {
			v: 1,
			tasks: tasks.map((t) =>
				taskOf(src, t, keep, anchor, {
					keepAssignees: opts.keepAssignees,
					fieldOK: (id) => live.has(id)
				})
			),
			project: {
				id: project.id,
				name: project.name,
				description: project.description,
				icon: project.icon,
				color: project.color,
				statuses: src
					.statuses(projectId)
					.map((s) => ({ id: s.id, name: s.name, category: s.category })),
				fields: own.map((f) => ({ id: f.id, name: f.name, type: f.type, options: f.options })),
				views: src
					.views(opts.me)
					.filter((v) => v.projectId === projectId)
					.map((v) => ({ name: v.name, type: v.type, filter: v.filter, display: v.display })),
				pages: src
					.pages(projectId)
					.map((p) => ({ id: p.id, parent: p.parentId, title: p.title, body: p.body })),
				automations: src
					.automations()
					.filter((a) => a.projectId === projectId)
					.map((a) => ({ name: a.name, rule: a.rule }))
			}
		}
	};
}

/**
 * A task as a template: it and its open subtasks, dates counted from
 * today, so it is due as many days after it is used as it was due after
 * it was saved (never before the day it is used).
 */
export function snapshotTask(
	src: Source,
	taskId: string,
	opts: { keepAssignees: boolean; today: string }
): Snapshot | null {
	const root = src.task(taskId);
	if (!root) return null;
	const left: LeftBehind = { done: 0, comments: 0, files: 0 };
	const tasks = openTree(src, [{ ...root, completedAt: null }], left);
	const keep = new Set(tasks.map((t) => t.id));
	let anchor = dayOf(opts.today);
	const rootDay = root.dueDate ?? root.startDate;
	if (rootDay && dayOf(rootDay) < anchor) anchor = dayOf(rootDay);
	const live = new Set(src.fields().map((f) => f.id));
	const data: TemplateData = {
		v: 1,
		tasks: tasks.map((t) =>
			taskOf(src, t, keep, anchor, {
				keepAssignees: opts.keepAssignees,
				fieldOK: (id) => live.has(id)
			})
		)
	};
	data.tasks[0].parent = null;
	return { anchor: isoOf(anchor), left, data };
}

/** A row to write: the order matters (what a row names comes first). */
export interface NewRow {
	entity:
		| 'project'
		| 'status'
		| 'label'
		| 'custom_field'
		| 'saved_view'
		| 'automation'
		| 'task'
		| 'task_label'
		| 'task_dependency'
		| 'task_assignee'
		| 'checklist_item'
		| 'custom_field_value'
		| 'page';
	id: string;
	values: Record<string, unknown>;
}

const UUID = /[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}/g;

/** Text with the IDs of rows made anew replaced by their new IDs. */
export function remapText(text: string, ids: Map<string, string>): string {
	return text.replace(UUID, (id) => ids.get(id.toLowerCase()) ?? id);
}

/** Any JSON with strings that are IDs of rows made anew replaced. */
export function remapJSON<T>(v: T, ids: Map<string, string>): T {
	if (typeof v === 'string') return (ids.get(v.toLowerCase()) ?? v) as T;
	if (Array.isArray(v)) return v.map((x) => remapJSON(x, ids)) as T;
	if (v && typeof v === 'object')
		return Object.fromEntries(Object.entries(v).map(([k, x]) => [k, remapJSON(x, ids)])) as T;
	return v;
}

/** Keys after one another, for rows made together. */
function keys(n: number, after: string | null = null): string[] {
	const out: string[] = [];
	let k = after;
	for (let i = 0; i < n; i++) out.push((k = keyBetween(k, null)));
	return out;
}

/** What making from a template needs to know of the workspace. */
export interface Target {
	me: string;
	uuid: () => string;
	/** The workspace's live labels. */
	labels: Label[];
	/** Who can reach what is being made, for kept assignees. */
	members: Set<string>;
	/** The workspace's live fields, by ID. */
	fields: Map<string, CustomField>;
}

export interface Plan {
	rows: NewRow[];
	/** The project or task made. */
	id: string;
	/** Kept assignees left out because they cannot reach it. */
	dropped: number;
}

/** The labels a task names, found by ID or by name, or made. */
function labelIds(
	t: TemplateTask,
	target: Target,
	made: Map<string, string>,
	rows: NewRow[],
	labelKeys: () => string
): string[] {
	const out: string[] = [];
	for (const l of t.labels) {
		const live =
			target.labels.find((x) => x.id === l.id) ??
			target.labels.find((x) => x.name.toLowerCase() === l.name.toLowerCase());
		if (live) out.push(live.id);
		else {
			const key = l.name.toLowerCase();
			let id = made.get(key);
			if (!id) {
				id = target.uuid();
				made.set(key, id);
				rows.push({
					entity: 'label',
					id,
					values: { name: l.name, color: l.color, sort_key: labelKeys(), deleted_at: null }
				});
			}
			out.push(id);
		}
	}
	return out;
}

/** The rows of a template's tasks, once their own rows' IDs are known. */
function taskRows(
	data: TemplateData,
	ids: Map<string, string>,
	place: (t: TemplateTask) => {
		projectId: string | null;
		statusId: string | null;
		sortKey: string;
		start: string | null;
		due: string | null;
	},
	fieldId: (id: string) => string | null,
	target: Target,
	rows: NewRow[],
	labelKeys: () => string
): number {
	const made = new Map<string, string>();
	const later: NewRow[] = [];
	let dropped = 0;
	for (const t of data.tasks) {
		const id = ids.get(t.id.toLowerCase())!;
		const p = place(t);
		rows.push({
			entity: 'task',
			id,
			values: {
				project_id: p.projectId,
				status_id: p.statusId,
				parent_id: t.parent ? (ids.get(t.parent.toLowerCase()) ?? null) : null,
				title: t.title,
				notes: remapText(t.notes ?? '', ids),
				priority: t.priority,
				start_date: p.start,
				due_date: p.due,
				due_time: p.due ? t.due_time : null,
				recurrence: t.recurrence,
				recurrence_mode: t.recurrence_mode,
				estimate_minutes: t.estimate ?? null,
				sort_key: p.sortKey,
				created_by: target.me,
				completed_at: null,
				deleted_at: null
			}
		});
		for (const label of labelIds(t, target, made, rows, labelKeys))
			later.push({
				entity: 'task_label',
				id: pairId(id, label),
				values: { task_id: id, label_id: label, deleted_at: null }
			});
		for (const w of t.waits) {
			const blocker = ids.get(w.toLowerCase());
			if (blocker)
				later.push({
					entity: 'task_dependency',
					id: pairId(id, blocker),
					values: { task_id: id, blocker_id: blocker, deleted_at: null }
				});
		}
		for (const u of t.assignees ?? []) {
			if (!target.members.has(u)) {
				dropped++;
				continue;
			}
			later.push({
				entity: 'task_assignee',
				id: pairId(id, u),
				values: { task_id: id, user_id: u, deleted_at: null }
			});
		}
		keys(t.checklist.length).forEach((k, i) =>
			later.push({
				entity: 'checklist_item',
				id: target.uuid(),
				values: { task_id: id, text: t.checklist[i], done: false, sort_key: k, deleted_at: null }
			})
		);
		for (const [f, v] of Object.entries(t.values)) {
			const field = fieldId(f);
			if (field)
				later.push({
					entity: 'custom_field_value',
					id: pairId(id, field),
					values: { task_id: id, field_id: field, value: remapJSON(v, ids), deleted_at: null }
				});
		}
	}
	rows.push(...later);
	return dropped;
}

/** Sort keys for each task among its siblings, in the template's order. */
function siblingKeys(data: TemplateData, firstAfter: string | null): Map<string, string> {
	const out = new Map<string, string>();
	const last = new Map<string, string | null>();
	for (const t of data.tasks) {
		const group = t.parent ?? '';
		const before = last.has(group) ? last.get(group)! : group ? null : firstAfter;
		const k = keyBetween(before, null);
		last.set(group, k);
		out.set(t.id, k);
	}
	return out;
}

/**
 * The rows that make a project from a template: the project, its
 * statuses, fields, views (the maker's own) and wiki, and its tasks with
 * everything under them, dates counted from start.
 */
export function planProject(
	data: TemplateData,
	opts: { name: string; start: string; projectSortKey: string; labelsAfter: string | null },
	target: Target
): Plan {
	const p = data.project;
	const projectId = target.uuid();
	const ids = new Map<string, string>();
	const fresh = (old: string, id = target.uuid()) => (ids.set(old.toLowerCase(), id), id);
	if (p) {
		fresh(p.id, projectId);
		fresh(mainPageId(p.id), mainPageId(projectId));
		for (const s of p.statuses) fresh(s.id);
		for (const f of p.fields) fresh(f.id);
		for (const pg of p.pages) if (!ids.has(pg.id.toLowerCase())) fresh(pg.id);
	}
	for (const t of data.tasks) fresh(t.id);
	const rows: NewRow[] = [
		{
			entity: 'project',
			id: projectId,
			values: {
				name: opts.name.trim(),
				icon: p?.icon ?? '',
				color: p?.color ?? 'blue',
				description: p?.description ?? '',
				sort_key: opts.projectSortKey,
				archived_at: null,
				deleted_at: null
			}
		}
	];
	const statuses = p?.statuses ?? [];
	const statusKeys = keys(statuses.length);
	statuses.forEach((s, i) =>
		rows.push({
			entity: 'status',
			id: ids.get(s.id.toLowerCase())!,
			values: {
				project_id: projectId,
				name: s.name,
				category: s.category,
				sort_key: statusKeys[i],
				deleted_at: null
			}
		})
	);
	const firstTodo = statuses.find((s) => s.category === 'todo') ?? statuses[0];
	const statusOf = (t: TemplateTask): string | null => {
		const own = t.status ? ids.get(t.status.toLowerCase()) : undefined;
		if (own) return own;
		return firstTodo ? (ids.get(firstTodo.id.toLowerCase()) ?? null) : null;
	};
	let labelKey = opts.labelsAfter;
	const labelKeys = () => (labelKey = keyBetween(labelKey, null));
	const fieldKeys = keys(p?.fields.length ?? 0);
	(p?.fields ?? []).forEach((f, i) =>
		rows.push({
			entity: 'custom_field',
			id: ids.get(f.id.toLowerCase())!,
			values: {
				project_id: projectId,
				name: f.name,
				type: f.type,
				options: f.options,
				sort_key: fieldKeys[i],
				deleted_at: null
			}
		})
	);
	// A project from a template doesn't start acting by itself: its rules wait, switched off.
	const ruleKeys = keys(p?.automations?.length ?? 0, 'a0');
	(p?.automations ?? []).forEach((a, i) =>
		rows.push({
			entity: 'automation',
			id: target.uuid(),
			values: {
				project_id: projectId,
				name: a.name,
				enabled: false,
				enabled_at: null,
				rule: remapJSON(a.rule, ids),
				made_by: target.me,
				paused_why: null,
				sort_key: ruleKeys[i],
				deleted_at: null
			}
		})
	);
	const viewKeys = keys(p?.views.length ?? 0, 'a0');
	(p?.views ?? []).forEach((v, i) =>
		rows.push({
			entity: 'saved_view',
			id: target.uuid(),
			values: {
				project_id: projectId,
				owner_id: target.me,
				name: v.name,
				type: v.type,
				config: { filter: remapJSON(v.filter, ids) },
				display: Object.keys(v.display ?? {}).length ? remapJSON(v.display, ids) : null,
				sort_key: viewKeys[i],
				deleted_at: null
			}
		})
	);
	const start = dayOf(opts.start);
	const at = (d: number | null) => (d === null ? null : isoOf(start + d));
	const sortKeys = siblingKeys(data, null);
	const dropped = taskRows(
		data,
		ids,
		(t) => ({
			projectId,
			statusId: statusOf(t),
			sortKey: sortKeys.get(t.id)!,
			start: at(t.start),
			due: at(t.due)
		}),
		(f) => ids.get(f.toLowerCase()) ?? (target.fields.get(f)?.projectId === null ? f : null),
		target,
		rows,
		labelKeys
	);
	// The wiki: the main page first (made even if it was never written,
	// since the others hang off it), then each page after its parent.
	const pages = [...(p?.pages ?? [])];
	if (p && pages.length && !pages.some((pg) => pg.id === mainPageId(p.id)))
		pages.unshift({ id: mainPageId(p.id), parent: null, title: '', body: '' });
	const placed = new Set<string>();
	const pageKeys = new Map<string, string | null>();
	for (let guard = 0; pages.length && guard < 10_000; guard++) {
		const pg = pages.shift()!;
		const parent = pg.parent ? ids.get(pg.parent.toLowerCase()) : null;
		if (
			pg.parent &&
			parent &&
			!placed.has(parent) &&
			pages.some((x) => ids.get(x.id.toLowerCase()) === parent)
		) {
			pages.push(pg);
			continue;
		}
		const id = ids.get(pg.id.toLowerCase())!;
		const main = id === mainPageId(projectId);
		const group = main ? '' : (parent ?? mainPageId(projectId));
		const k = keyBetween(pageKeys.get(group) ?? null, null);
		pageKeys.set(group, k);
		rows.push({
			entity: 'page',
			id,
			values: {
				project_id: projectId,
				parent_id: main ? null : group,
				title: main ? '' : pg.title,
				body: remapText(pg.body, ids),
				sort_key: k,
				deleted_at: null
			}
		});
		placed.add(id);
	}
	return { rows, id: projectId, dropped };
}

/**
 * The rows that make a task from a template in a project (or the inbox),
 * after its last task, due as many days after today as the template says.
 */
export function planTask(
	data: TemplateData,
	opts: {
		projectId: string | null;
		statuses: Status[];
		today: string;
		after: string | null;
		labelsAfter: string | null;
	},
	target: Target
): Plan {
	const ids = new Map<string, string>();
	for (const t of data.tasks) ids.set(t.id.toLowerCase(), target.uuid());
	const today = dayOf(opts.today);
	const at = (d: number | null) => (d === null ? null : isoOf(today + d));
	const statusFor = (c: StatusCategory | null) =>
		opts.projectId
			? ((
					opts.statuses.find((s) => s.category === (c ?? 'todo')) ??
					opts.statuses.find((s) => s.category === 'todo') ??
					opts.statuses[0]
				)?.id ?? null)
			: null;
	const rows: NewRow[] = [];
	let labelKey = opts.labelsAfter;
	const labelKeys = () => (labelKey = keyBetween(labelKey, null));
	const sortKeys = siblingKeys(data, opts.after);
	const dropped = taskRows(
		data,
		ids,
		(t) => ({
			projectId: opts.projectId,
			statusId: statusFor(t.category),
			sortKey: sortKeys.get(t.id)!,
			start: at(t.start),
			due: at(t.due)
		}),
		(f) => {
			const field = target.fields.get(f);
			return field && (field.projectId === null || field.projectId === opts.projectId) ? f : null;
		},
		target,
		rows,
		labelKeys
	);
	return { rows, id: ids.get(data.tasks[0].id.toLowerCase())!, dropped };
}

const count = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

/** What saving takes along and leaves behind, for the save dialog. */
export function summarize(
	snap: Snapshot,
	kind: 'project' | 'task'
): { comes: string; stays: string } {
	const left = snap.left;
	const stays = `comments, files, history${left.done ? ` and ${count(left.done, kind === 'project' ? 'finished task' : 'finished subtask')}` : ''}`;
	if (kind === 'task') {
		const subs = snap.data.tasks.length - 1;
		return {
			comes: `the task${subs ? ` and ${count(subs, 'open subtask')}` : ''}, with checklists, notes, labels and the dependencies among them`,
			stays
		};
	}
	const p = snap.data.project;
	return {
		// What there is, leaving out what the project has none of.
		comes:
			[
				[p?.statuses.length ?? 0, 'status', 'statuses'] as const,
				[p?.fields.length ?? 0, 'field', 'fields'] as const,
				[snap.data.tasks.length, 'open task', 'open tasks'] as const,
				[p?.pages.length ?? 0, 'wiki page', 'wiki pages'] as const,
				[p?.views.length ?? 0, 'saved view', 'saved views'] as const
			]
				.filter(([n]) => n > 0)
				.map(([n, one, many]) =>
					one === 'open task'
						? `${count(n, one, many)} with their subtasks, checklists, notes, labels and dependencies`
						: count(n, one, many)
				)
				.join(', ') || 'the project itself, with nothing in it yet',
		stays
	};
}

/** An image held in a page's text: its file stays behind, so the image does too. */
const IMAGE =
	/!\[(?:[^\]\\[]|\\.|\[[^\]]*\])*\]\(\/api\/v1\/workspaces\/[^)\s]+\/attachments\/[^)\s]+\/file(?:\s+"[^"]*")?\)[ \t]*/g;

/** A link to a wiki page, in either wiki: its address, up to the page's ID. */
const PAGE_LINK =
	/\/(?:wiki|projects\/[^?\s)]+)\?(?:tab=wiki&)?page=([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})/g;

/** What saving a page reads of the workspace. */
export interface PageSource {
	page(id: string): Page | undefined;
	pages(projectId: string | null): Page[];
	pageAttachments(pageId: string): unknown[];
	pageComments(pageId: string): unknown[];
}

/** A page and, if asked for, those under it, parents before their subpages. */
function pagesUnder(src: PageSource, root: Page, subpages: boolean): Page[] {
	const out = [root];
	if (!subpages) return out;
	const all = src.pages(root.projectId);
	for (let i = 0; i < out.length; i++) {
		out.push(...all.filter((p) => p.parentId === out[i].id));
	}
	return out;
}

/**
 * A snapshot of a page, with its subpages if kept. Its images stay behind with
 * its other files, so they are taken out of the text.
 */
export function snapshotPage(
	src: PageSource,
	pageId: string,
	opts: { subpages: boolean; title: string; body?: string }
): { data: TemplateData } | null {
	const root = src.page(pageId);
	if (!root) return null;
	return {
		data: {
			v: 1,
			tasks: [],
			pages: pagesUnder(src, root, opts.subpages).map((p) => ({
				id: p.id,
				parent: p.id === root.id ? null : p.parentId,
				title: p.id === root.id ? opts.title : p.title,
				body: (p.id === root.id ? (opts.body ?? p.body) : p.body).replace(IMAGE, '')
			}))
		}
	};
}

/** What saving a page takes along and leaves behind, for the save dialog. */
export function summarizePage(
	src: PageSource,
	pageId: string,
	subpages: boolean
): { comes: string; stays: string; under: number } {
	const root = src.page(pageId);
	if (!root) return { comes: '', stays: '', under: 0 };
	const under = pagesUnder(src, root, true).length - 1;
	const kept = pagesUnder(src, root, subpages);
	let comments = 0;
	let files = 0;
	for (const p of kept) {
		comments += src.pageComments(p.id).length;
		files += src.pageAttachments(p.id).length;
	}
	const stays = [
		...(under && !subpages ? [count(under, 'subpage')] : []),
		...(comments ? [count(comments, 'comment')] : []),
		...(files ? [count(files, 'file')] : []),
		'its history'
	];
	return {
		comes: `the title and text${subpages && under ? ` and ${count(under, 'subpage')}` : ''}`,
		stays: stays.join(', ').replace(/, ([^,]*)$/, ' and $1'),
		under
	};
}

/** Text with today's date where the template says {{today}}. */
export function fillToday(text: string, today: string): string {
	return text.replace(/\{\{\s*today\s*\}\}/gi, today);
}

/**
 * What using a page template writes: the title and text for the page it fills,
 * and rows for the subpages made under it. Links between the template's pages
 * lead to the new copies, in the wiki they land in.
 */
export function planPage(
	data: TemplateData,
	target: {
		pageId: string;
		projectId: string | null;
		/** Today's date as the person writes it, for {{today}}. */
		today: string;
		uuid: () => string;
		href: (page: { id: string; projectId: string | null }) => string;
	}
): { title: string; body: string; rows: NewRow[]; subpages: string[] } {
	const pages = data.pages ?? [];
	const ids = new Map<string, string>();
	pages.forEach((p, i) => ids.set(p.id.toLowerCase(), i === 0 ? target.pageId : target.uuid()));
	const text = (body: string) =>
		fillToday(
			body.replace(PAGE_LINK, (link, id: string) => {
				const made = ids.get(id.toLowerCase());
				return made ? target.href({ id: made, projectId: target.projectId }) : link;
			}),
			target.today
		);
	const keys = new Map<string, string>();
	const rows: NewRow[] = [];
	const subpages: string[] = [];
	for (const p of pages.slice(1)) {
		const id = ids.get(p.id.toLowerCase())!;
		const parent = (p.parent && ids.get(p.parent.toLowerCase())) || target.pageId;
		const key = keyBetween(keys.get(parent) ?? null, null);
		keys.set(parent, key);
		subpages.push(id);
		rows.push({
			entity: 'page',
			id,
			values: {
				project_id: target.projectId,
				parent_id: parent,
				title: fillToday(p.title, target.today),
				body: text(p.body),
				sort_key: key,
				deleted_at: null
			}
		});
	}
	return {
		title: fillToday(pages[0]?.title ?? '', target.today),
		body: text(pages[0]?.body ?? ''),
		rows,
		subpages
	};
}
