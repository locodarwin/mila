// One workspace's copy on this device. Every field carries the HLC that
// last set it; a field from elsewhere replaces the local one only if it is
// newer (or, for the few FIRST_WINS fields, older). The same rules are modeled in server/internal/syncer/syncer_test.go,
// whose convergence test is the proof they work.
import { readTemplate, type Template } from '../templates';
import { uuid } from '../uuid';
import { untrack } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import type { Clock } from './hlc';
import { filterOf } from '../viewConfig';
import { linksIn } from '../links';
import { bySortKey } from './sortkey';
import {
	type Automation,
	DEFAULTS,
	FIRST_WINS,
	type Attachment,
	type ChecklistItem,
	type Comment,
	type PageComment,
	type PageThread,
	type CustomField,
	type Entity,
	type FieldOption,
	type FieldType,
	type FieldValue,
	type Label,
	type Page,
	type Priority,
	type Project,
	type Reminder,
	type Status,
	type SavedView,
	type StatusCategory,
	type Task,
	type TimeEntry,
	type Values,
	type ViewDisplay,
	type ViewType,
	type WorkspaceTheme
} from './entities';

function toTask(r: Row): Task {
	const v = r.values;
	return {
		id: r.id,
		projectId: (v.project_id as string | null) ?? null,
		statusId: (v.status_id as string | null) ?? null,
		parentId: (v.parent_id as string | null) ?? null,
		title: v.title as string,
		notes: (v.notes as string) ?? '',
		sortKey: v.sort_key as string,
		completedAt: (v.completed_at as string | null) ?? null,
		priority: ((v.priority as Priority) ?? 'none') as Priority,
		startDate: (v.start_date as string | null) ?? null,
		dueDate: (v.due_date as string | null) ?? null,
		dueTime: (v.due_time as string | null) ?? null,
		createdBy: (v.created_by as string | null) ?? null,
		recurrence: (v.recurrence as string | null) ?? null,
		recurrenceMode: v.recurrence_mode === 'completion' ? 'completion' : 'schedule',
		seriesId: (v.series_id as string | null) ?? null,
		estimateMinutes: typeof v.estimate_minutes === 'number' ? v.estimate_minutes : null,
		madeAt: (v.made_at as string | null) ?? null,
		startedAt: (v.started_at as string | null) ?? null
	};
}

function toPage(r: Row): Page {
	const v = r.values;
	return {
		id: r.id,
		projectId: (v.project_id as string | null | undefined) ?? null,
		parentId: (v.parent_id as string | null | undefined) ?? null,
		title: (v.title as string | undefined) ?? '',
		body: (v.body as string | undefined) ?? '',
		sortKey: (v.sort_key as string | undefined) ?? '',
		deletedAt: (v.deleted_at as string | null | undefined) ?? null
	};
}

const byId = (a: Row, b: Row) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
const byTitle = (a: Task, b: Task) => a.title.localeCompare(b.title) || (a.id < b.id ? -1 : 1);

/** When an HLC was made, as an ISO time. */
function hlcTime(hlc: string | undefined): string {
	return new Date(Number(hlc?.slice(0, 13) ?? 0)).toISOString();
}

export interface Row {
	entity: Entity;
	id: string;
	values: Values;
	clocks: Record<string, string>;
}

export interface Op {
	opId: string;
	entity: Entity;
	id: string;
	hlc: string;
	fields: Values;
	/** One of many rows made at once, which starts no automation. */
	bulk?: boolean;
	/**
	 * One change to many tasks (a selection's): every op of it carries the
	 * same ID, so rules count it once and a person it assigns gets one push.
	 */
	batch?: string;
}

export interface OpResult {
	opId: string;
	status: 'applied' | 'duplicate' | 'rejected';
	message?: string;
	row?: { entity: string; id: string; fields: Values; clocks: Record<string, string> };
}

export const rowKey = (entity: string, id: string) => `${entity}/${id}`;

/** Rows grouped under a key, for the indexes below. */
type Index = Map<string, Map<string, Row>>;

function index(map: Index, key: string, row: Row, add: boolean) {
	let rows = map.get(key);
	if (add) {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- plain: Replica's #version is what reacts
		if (!rows) map.set(key, (rows = new Map()));
		rows.set(row.id, row);
	} else if (rows) {
		rows.delete(row.id);
		if (!rows.size) map.delete(key);
	}
}

export class Replica {
	/** Every row; change it only through #set, #delete and replaceAll. */
	rows = new SvelteMap<string, Row>();

	// Indexes over the rows, so a query reads only the rows it is about
	// rather than the whole workspace: by entity, the rows that hang off a
	// task by entity and task, and tasks by parent and by project. They are
	// plain maps kept in step with every change; #version, bumped by every
	// change, is what makes the queries reactive.
	#version = $state(0);
	/* eslint-disable svelte/prefer-svelte-reactivity -- plain on purpose, as above */
	#byEntity: Index = new Map();
	#byTask: Index = new Map();
	#byParent: Index = new Map();
	#byProject: Index = new Map();
	#byPage: Index = new Map();
	// Dependencies by the task they wait for.
	#byBlocker: Index = new Map();
	// Tasks and pages by what their notes or body link to (links.ts).
	#linksTo: Index = new Map();
	// Timers running, by the person whose they are, on tasks and projects.
	#running: Index = new Map();
	// Time logged to a project itself, by its project.
	#projectTime: Index = new Map();
	/* eslint-enable svelte/prefer-svelte-reactivity */

	#index(row: Row, add: boolean) {
		const v = row.values;
		index(this.#byEntity, row.entity, row, add);
		if (typeof v.task_id === 'string') index(this.#byTask, `${row.entity}/${v.task_id}`, row, add);
		if (typeof v.page_id === 'string') index(this.#byPage, `${row.entity}/${v.page_id}`, row, add);
		if (row.entity === 'task_dependency' && typeof v.blocker_id === 'string')
			index(this.#byBlocker, v.blocker_id, row, add);
		const timed = row.entity === 'time_entry' || row.entity === 'project_time_entry';
		if (timed && v.running === true && typeof v.user_id === 'string')
			index(this.#running, v.user_id, row, add);
		if (row.entity === 'project_time_entry' && typeof v.project_id === 'string')
			index(this.#projectTime, v.project_id, row, add);
		if (row.entity === 'task') {
			index(this.#byParent, (v.parent_id as string | null | undefined) ?? '', row, add);
			index(this.#byProject, (v.project_id as string | null | undefined) ?? '', row, add);
		}
		const text = row.entity === 'task' ? v.notes : row.entity === 'page' ? v.body : null;
		if (typeof text === 'string' && text.includes(']('))
			for (const target of linksIn(text)) index(this.#linksTo, target, row, add);
	}

	#set(row: Row) {
		const key = rowKey(row.entity, row.id);
		const old = this.rows.get(key);
		if (old) this.#index(old, false);
		this.#index(row, true);
		this.rows.set(key, row);
		this.#bump();
	}

	#delete(entity: Entity, id: string) {
		const key = rowKey(entity, id);
		const old = this.rows.get(key);
		if (old) this.#index(old, false);
		this.rows.delete(key);
		this.#bump();
	}

	// Untracked, so an effect that edits the replica does not come to
	// depend on every change to it.
	#bump() {
		this.#version = untrack(() => this.#version) + 1;
	}

	/** The live rows of an index's bucket. */
	#liveIn(map: Index, key: string): Row[] {
		void this.#version;
		const out: Row[] = [];
		for (const r of map.get(key)?.values() ?? []) if (r.values.deleted_at == null) out.push(r);
		return out;
	}

	constructor(
		readonly workspaceId: string,
		readonly clock: Clock
	) {}

	get(entity: Entity, id: string): Row | undefined {
		return this.rows.get(rowKey(entity, id));
	}

	/** Marks what is prepared as one of many rows made at once. */
	bulk = false;
	/** Marks what is prepared as part of one change to many tasks. */
	batch: string | null = null;

	/**
	 * Works out a local edit without applying it: the op that records it and
	 * the row as it will be. The engine saves both, then calls apply.
	 */
	prepare(entity: Entity, id: string, given: Values): { op: Op; row: Row } {
		// A NUL, pasted or imported, is taken out: the server refuses text
		// holding one (Postgres can't store it), and nothing shows it.
		const fields: Values = Object.fromEntries(
			Object.entries(given).map(([k, v]) => [k, typeof v === 'string' ? v.replaceAll('\0', '') : v])
		);
		const hlc = this.clock.tick();
		const old = this.get(entity, id);
		const values = { ...(old?.values ?? DEFAULTS[entity]) };
		const clocks = { ...(old?.clocks ?? {}) };
		const first = FIRST_WINS[entity] ?? [];
		for (const [name, value] of Object.entries(fields)) {
			// A FIRST_WINS field already set stays: the server keeps the older.
			if (first.includes(name) && clocks[name] !== undefined) continue;
			values[name] = value;
			clocks[name] = hlc;
		}
		return {
			op: {
				opId: uuid(),
				entity,
				id,
				hlc,
				fields,
				...(this.bulk ? { bulk: true } : {}),
				...(this.batch ? { batch: this.batch } : {})
			},
			row: { entity, id, values, clocks }
		};
	}

	apply(row: Row) {
		this.#set(row);
	}

	/** Applies a local edit at once and returns its op. */
	local(entity: Entity, id: string, fields: Values): Op {
		const { op, row } = this.prepare(entity, id, fields);
		this.apply(row);
		return op;
	}

	/**
	 * Merges fields from the server, keeping each local field that is newer
	 * (older for a FIRST_WINS field). Returns the row if anything changed.
	 */
	remote(
		entity: Entity,
		id: string,
		fields: Values,
		clocks: Record<string, string>
	): Row | undefined {
		const old = this.get(entity, id);
		const values = { ...(old?.values ?? {}) };
		const merged = { ...(old?.clocks ?? {}) };
		const first = FIRST_WINS[entity] ?? [];
		let changed = false;
		for (const [name, value] of Object.entries(fields)) {
			const hlc = clocks[name];
			this.clock.observe(hlc);
			const mine = merged[name];
			if (first.includes(name) ? mine === undefined || hlc < mine : hlc > (mine ?? '')) {
				values[name] = value;
				merged[name] = hlc;
				changed = true;
			}
		}
		if (!changed) return undefined;
		const row = { entity, id, values, clocks: merged };
		this.#set(row);
		return row;
	}

	/**
	 * Undoes a rejected op: each field it was the last local write to goes
	 * back to the server's copy, or away if the server has none. Returns the
	 * row as it now stands, or null if it is gone.
	 */
	revert(op: Op, server: OpResult['row']): Row | null | undefined {
		const row = this.get(op.entity, op.id);
		if (!row) return undefined;
		const values = { ...row.values };
		const clocks = { ...row.clocks };
		for (const name of Object.keys(op.fields)) {
			if (clocks[name] !== op.hlc) continue;
			if (server) {
				values[name] = server.fields[name];
				clocks[name] = server.clocks[name];
			} else {
				delete values[name];
				delete clocks[name];
			}
		}
		if (!server && Object.keys(clocks).length === 0) {
			this.#delete(op.entity, op.id);
			return null;
		}
		const next = { ...row, values, clocks };
		this.#set(next);
		return next;
	}

	/**
	 * Takes a row out altogether: a row of a document the server merged
	 * into another, which is no use kept.
	 */
	forget(entity: Entity, id: string) {
		this.#delete(entity, id);
	}

	/** Replaces everything, as after a bootstrap. */
	replaceAll(rows: Row[]) {
		this.rows.clear();
		for (const map of [
			this.#byEntity,
			this.#byTask,
			this.#byParent,
			this.#byProject,
			this.#byPage,
			this.#byBlocker,
			this.#running,
			this.#projectTime,
			this.#linksTo
		])
			map.clear();
		for (const r of rows) {
			for (const hlc of Object.values(r.clocks)) this.clock.observe(hlc);
			this.#index(r, true);
			this.rows.set(rowKey(r.entity, r.id), r);
		}
		this.#bump();
	}

	// Typed views of live rows.

	#live(entity: Entity): Row[] {
		return this.#liveIn(this.#byEntity, entity);
	}

	#projects(archived: boolean): Project[] {
		return this.#live('project')
			.filter((r) => (r.values.archived_at != null) === archived)
			.map((r): Project => ({
				id: r.id,
				name: r.values.name as string,
				// A row first seen in a pull holds only the fields sent.
				icon: (r.values.icon as string | undefined) ?? '',
				color: (r.values.color as string | undefined) ?? 'blue',
				description: (r.values.description as string | undefined) ?? '',
				sortKey: r.values.sort_key as string,
				archivedAt: r.values.archived_at as string | null
			}))
			.sort(bySortKey);
	}

	/** The projects in use, in order; archived ones are left out. */
	projects(): Project[] {
		return this.#projects(false);
	}

	archivedProjects(): Project[] {
		return this.#projects(true);
	}

	/** A project, archived or not. */
	project(id: string): Project | undefined {
		return (
			this.projects().find((p) => p.id === id) ?? this.archivedProjects().find((p) => p.id === id)
		);
	}

	statuses(projectId: string): Status[] {
		return this.#live('status')
			.filter((r) => r.values.project_id === projectId)
			.map((r): Status => ({
				id: r.id,
				projectId,
				name: r.values.name as string,
				category: r.values.category as StatusCategory,
				sortKey: r.values.sort_key as string
			}))
			.sort(bySortKey);
	}

	/** Top-level tasks in a project, or in the inbox for null. */
	tasks(projectId: string | null): Task[] {
		return this.#liveIn(this.#byProject, projectId ?? '')
			.filter((r) => !r.values.parent_id)
			.map(toTask)
			.sort(bySortKey);
	}

	task(id: string): Task | undefined {
		const r = this.get('task', id);
		return r && r.values.deleted_at == null ? toTask(r) : undefined;
	}

	/** The rows of a task's live document (docs.ts), oldest first. */
	docUpdates(taskId: string): Row[] {
		return this.#liveIn(this.#byTask, `doc_update/${taskId}`).sort(byId);
	}

	/** The rows of a page's live document, oldest first. */
	pageUpdates(pageId: string): Row[] {
		return this.#liveIn(this.#byPage, `page_update/${pageId}`).sort(byId);
	}

	/** A wiki's pages, deleted ones left out: a project's, or the workspace's for null. */
	pages(projectId: string | null): Page[] {
		return this.#live('page')
			.filter((r) => ((r.values.project_id as string | null | undefined) ?? null) === projectId)
			.map(toPage)
			.sort(bySortKey);
	}

	/** A page, deleted or not. */
	page(id: string): Page | undefined {
		const r = this.get('page', id);
		return r ? toPage(r) : undefined;
	}

	/** The live pages and tasks whose body or notes link to a page or task. */
	linkedFrom(id: string): { pages: Page[]; tasks: Task[] } {
		const rows = this.#liveIn(this.#linksTo, id).filter((r) => r.id !== id);
		return {
			pages: rows.filter((r) => r.entity === 'page').map(toPage),
			tasks: rows.filter((r) => r.entity === 'task').map(toTask)
		};
	}

	/** Every live page the device holds, in every wiki: for search. */
	allPages(): Page[] {
		return this.#live('page').map(toPage);
	}

	subtasks(taskId: string): Task[] {
		return this.#liveIn(this.#byParent, taskId).map(toTask).sort(bySortKey);
	}

	status(id: string | null): Status | undefined {
		if (!id) return undefined;
		const r = this.get('status', id);
		if (!r || r.values.deleted_at != null) return undefined;
		return {
			id: r.id,
			projectId: r.values.project_id as string,
			name: r.values.name as string,
			category: r.values.category as StatusCategory,
			sortKey: r.values.sort_key as string
		};
	}

	labels(): Label[] {
		return this.#live('label')
			.map((r): Label => ({
				id: r.id,
				name: r.values.name as string,
				color: r.values.color as string,
				sortKey: r.values.sort_key as string
			}))
			.sort(bySortKey);
	}

	#children(entity: Entity, taskId: string): Row[] {
		return this.#liveIn(this.#byTask, `${entity}/${taskId}`);
	}

	/** The tasks this one waits for, done ones too. */
	blockers(taskId: string): Task[] {
		const out: Task[] = [];
		for (const r of this.#children('task_dependency', taskId)) {
			const t = this.task(r.values.blocker_id as string);
			if (t) out.push(t);
		}
		return out.sort(byTitle);
	}

	/** The tasks that wait for this one, done ones too. */
	waiting(taskId: string): Task[] {
		const out: Task[] = [];
		for (const r of this.#liveIn(this.#byBlocker, taskId)) {
			const t = this.task(r.values.task_id as string);
			if (t) out.push(t);
		}
		return out.sort(byTitle);
	}

	/** The unfinished tasks this one waits for, by title. */
	openBlockers(taskId: string): string[] {
		const out: string[] = [];
		for (const r of this.#children('task_dependency', taskId)) {
			const t = this.task(r.values.blocker_id as string);
			if (t && !t.completedAt) out.push(t.title);
		}
		return out.sort((a, b) => a.localeCompare(b));
	}

	/** Whether a task waits for one that is not done. */
	isBlocked(taskId: string): boolean {
		for (const r of this.#children('task_dependency', taskId)) {
			const t = this.task(r.values.blocker_id as string);
			if (t && !t.completedAt) return true;
		}
		return false;
	}

	/** Whether an unfinished task holds up another that is not done. */
	isBlocking(taskId: string): boolean {
		const self = this.task(taskId);
		if (!self || self.completedAt) return false;
		for (const r of this.#liveIn(this.#byBlocker, taskId)) {
			const t = this.task(r.values.task_id as string);
			if (t && !t.completedAt) return true;
		}
		return false;
	}

	/** Every live dependency whose two tasks are live: [blocker, waiting]. */
	dependencies(): [string, string][] {
		const out: [string, string][] = [];
		for (const r of this.#liveIn(this.#byEntity, 'task_dependency')) {
			const a = r.values.blocker_id as string;
			const b = r.values.task_id as string;
			if (this.task(a) && this.task(b)) out.push([a, b]);
		}
		return out;
	}

	taskLabels(taskId: string): Label[] {
		const on = this.#children('task_label', taskId).map((r) => r.values.label_id);
		return this.labels().filter((l) => on.includes(l.id));
	}

	assignees(taskId: string): string[] {
		return this.#children('task_assignee', taskId)
			.map((r) => r.values.user_id as string)
			.sort();
	}

	checklist(taskId: string): ChecklistItem[] {
		return this.#children('checklist_item', taskId)
			.map((r): ChecklistItem => ({
				id: r.id,
				taskId,
				text: r.values.text as string,
				done: Boolean(r.values.done),
				sortKey: r.values.sort_key as string
			}))
			.sort(bySortKey);
	}

	comments(taskId: string): Comment[] {
		return this.#children('comment', taskId)
			.map((r): Comment => ({
				id: r.id,
				taskId,
				authorId: r.values.author_id as string,
				body: r.values.body as string,
				editedAt: (r.values.edited_at as string | null) ?? null,
				createdAt: hlcTime(r.clocks.author_id)
			}))
			.sort((a, b) => (a.createdAt < b.createdAt ? -1 : a.createdAt > b.createdAt ? 1 : 0));
	}

	attachments(taskId: string): Attachment[] {
		return this.#children('attachment', taskId).map((r): Attachment => ({
			id: r.id,
			taskId,
			filename: r.values.filename as string,
			mime: r.values.mime as string,
			size: Number(r.values.size),
			uploadedBy: r.values.uploaded_by as string
		}));
	}

	/** A wiki page's files. */
	pageAttachments(pageId: string): Attachment[] {
		return this.#liveIn(this.#byPage, `page_attachment/${pageId}`).map((r): Attachment => ({
			id: r.id,
			taskId: '',
			pageId,
			filename: r.values.filename as string,
			mime: r.values.mime as string,
			size: Number(r.values.size),
			uploadedBy: r.values.uploaded_by as string
		}));
	}

	/** A wiki page's comment threads, oldest first. */
	pageThreads(pageId: string): PageThread[] {
		return this.#liveIn(this.#byPage, `page_thread/${pageId}`)
			.map((r): PageThread => ({
				id: r.id,
				pageId,
				createdBy: r.values.created_by as string,
				quote: r.values.quote as string,
				anchor: (r.values.anchor as string | undefined) ?? '',
				resolvedAt: (r.values.resolved_at as string | null) ?? null,
				createdAt: hlcTime(r.clocks.created_by)
			}))
			.sort((a, b) => (a.createdAt < b.createdAt ? -1 : a.createdAt > b.createdAt ? 1 : 0));
	}

	/** The comments on a wiki page's threads, oldest first. */
	pageComments(pageId: string): PageComment[] {
		return this.#liveIn(this.#byPage, `page_comment/${pageId}`)
			.map((r): PageComment => ({
				id: r.id,
				threadId: r.values.thread_id as string,
				authorId: r.values.author_id as string,
				body: r.values.body as string,
				editedAt: (r.values.edited_at as string | null) ?? null,
				createdAt: hlcTime(r.clocks.author_id)
			}))
			.sort((a, b) => (a.createdAt < b.createdAt ? -1 : a.createdAt > b.createdAt ? 1 : 0));
	}

	/**
	 * Whether a project has tasks and all of them, subtasks too, are
	 * completed. A subtask whose parent was deleted no longer counts.
	 */
	projectFinished(projectId: string): boolean {
		const counts = (r: Row): boolean => {
			const parent = r.values.parent_id as string | null | undefined;
			if (!parent) return true;
			const p = this.get('task', parent);
			return p !== undefined && p.values.deleted_at == null && counts(p);
		};
		const tasks = this.#liveIn(this.#byProject, projectId).filter(counts);
		return tasks.length > 0 && tasks.every((r) => r.values.completed_at != null);
	}

	/** Every live task, top-level or not, in any project or the inbox. */
	allTasks(): Task[] {
		return this.#live('task').map(toTask).sort(bySortKey);
	}

	/** Every task but those in archived projects, for lists across projects. */
	activeTasks(): Task[] {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- built fresh by each call
		const archived = new Set(this.archivedProjects().map((p) => p.id));
		return this.allTasks().filter((t) => !t.projectId || !archived.has(t.projectId));
	}

	/** A user's My Day entries, with their tasks, for days up to and including `day`. */
	myDay(
		userId: string,
		day: string
	): { entryId: string; day: string; sortKey: string; task: Task }[] {
		const out = [];
		for (const r of this.#live('my_day_entry')) {
			if (r.values.user_id !== userId || (r.values.day as string) > day) continue;
			const task = this.task(r.values.task_id as string);
			if (task)
				out.push({
					entryId: r.id,
					day: r.values.day as string,
					sortKey: r.values.sort_key as string,
					task
				});
		}
		return out.sort((a, b) => (a.sortKey < b.sortKey ? -1 : a.sortKey > b.sortKey ? 1 : 0));
	}

	inMyDay(userId: string, taskId: string, day: string): boolean {
		return this.#children('my_day_entry', taskId).some(
			(r) => r.values.user_id === userId && r.values.day === day
		);
	}

	/** A user's reminders on a task, soonest first. */
	reminders(userId: string, taskId: string): Reminder[] {
		return this.#children('reminder', taskId)
			.filter((r) => r.values.user_id === userId)
			.map(toReminder)
			.sort((a, b) => (a.remindAt < b.remindAt ? -1 : a.remindAt > b.remindAt ? 1 : 0));
	}

	/**
	 * The fields a project's tasks have: the workspace's and its own (only
	 * the workspace's for the inbox, null). With no argument, every field.
	 */
	fields(projectId?: string | null): CustomField[] {
		return this.#live('custom_field')
			.filter(
				(r) =>
					projectId === undefined ||
					r.values.project_id === null ||
					r.values.project_id === projectId
			)
			.map((r): CustomField => ({
				id: r.id,
				projectId: (r.values.project_id as string | null) ?? null,
				name: r.values.name as string,
				type: r.values.type as FieldType,
				options: readOptions(r.values.options),
				sortKey: r.values.sort_key as string
			}))
			.sort(bySortKey);
	}

	/** A task's field values by field ID. */
	fieldValues(taskId: string): Record<string, FieldValue> {
		const out: Record<string, FieldValue> = {};
		for (const r of this.#children('custom_field_value', taskId))
			out[r.values.field_id as string] = r.values.value as FieldValue;
		return out;
	}

	/**
	 * Every live field value, by field and then task: one pass over the
	 * value rows, for a saved view to filter, sort and group by.
	 */
	fieldValuesByField(): Map<string, Map<string, FieldValue>> {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- built fresh by each derivation
		const out = new Map<string, Map<string, FieldValue>>();
		for (const r of this.#live('custom_field_value')) {
			const field = r.values.field_id as string;
			let byTask = out.get(field);
			// eslint-disable-next-line svelte/prefer-svelte-reactivity -- as above
			if (!byTask) out.set(field, (byTask = new Map()));
			byTask.set(r.values.task_id as string, r.values.value as FieldValue);
		}
		return out;
	}

	/**
	 * When a task was made and last changed: the oldest and newest clocks of
	 * its fields, as HLC strings (the rule ical.lastChange uses).
	 */
	taskTimes(id: string): { created: string; updated: string } {
		void this.#version;
		let created = '';
		let updated = '';
		for (const c of Object.values(this.get('task', id)?.clocks ?? {})) {
			if (!created || c < created) created = c;
			if (c > updated) updated = c;
		}
		return { created, updated };
	}

	/** The time logged on a task, newest day first. */
	timeEntries(taskId: string): TimeEntry[] {
		return this.#children('time_entry', taskId).map(toEntry).sort(newestFirst);
	}

	/** The time logged to a project itself, not to its tasks, newest day first. */
	projectTimeEntries(projectId: string): TimeEntry[] {
		return this.#liveIn(this.#projectTime, projectId).map(toEntry).sort(newestFirst);
	}

	/**
	 * Every piece of time logged in the workspace: on tasks (in the trash
	 * too) and on projects themselves (taskId null).
	 */
	allTimeEntries(): TimeEntry[] {
		return [...this.#live('time_entry'), ...this.#live('project_time_entry')].map(toEntry);
	}

	/** The minutes logged on a task itself; a timer running counts once it stops. */
	logged(taskId: string): number {
		let sum = 0;
		for (const r of this.#children('time_entry', taskId)) sum += Number(r.values.minutes) || 0;
		return sum;
	}

	/**
	 * A task's estimate and time logged with its subtasks' added in (live
	 * ones, done ones too), and whether any of them has an estimate.
	 */
	withSubtasks(taskId: string): { estimate: number; logged: number; estimated: boolean } {
		const out = { estimate: 0, logged: 0, estimated: false };
		const walk = (id: string, depth: number) => {
			const t = this.task(id);
			if (!t) return;
			if (t.estimateMinutes !== null) {
				out.estimate += t.estimateMinutes;
				out.estimated = true;
			}
			out.logged += this.logged(id);
			if (depth < 20) for (const c of this.#liveIn(this.#byParent, id)) walk(c.id, depth + 1);
		};
		walk(taskId, 0);
		return out;
	}

	/** A person's running timer, the newest when devices started two at once. */
	runningTimer(userId: string): TimeEntry | undefined {
		return this.runningTimers(userId)[0];
	}

	/** Every timer a person has running, newest first: one, unless devices raced. */
	runningTimers(userId: string): TimeEntry[] {
		return this.#liveIn(this.#running, userId)
			.map(toEntry)
			.sort(
				(a, b) => (b.startedAt ?? '').localeCompare(a.startedAt ?? '') || a.id.localeCompare(b.id)
			);
	}

	/** The workspace's templates, those it can read, in their order. */
	templates(): Template[] {
		const out: Template[] = [];
		for (const r of this.#live('template')) {
			const data = readTemplate(r.values.data);
			if (!data) continue;
			out.push({
				id: r.id,
				kind: r.values.kind === 'project' || r.values.kind === 'page' ? r.values.kind : 'task',
				name: (r.values.name as string | undefined) ?? '',
				data,
				savedBy: (r.values.saved_by as string | null | undefined) ?? null,
				savedAt: (r.values.saved_at as string | null | undefined) ?? null,
				sortKey: (r.values.sort_key as string | undefined) ?? ''
			});
		}
		return out.sort(bySortKey);
	}

	/** The themes the workspace offers, in their order. */
	automations(): Automation[] {
		return this.#live('automation').map(toAutomation).sort(bySortKey);
	}

	workspaceThemes(): WorkspaceTheme[] {
		return this.#live('workspace_theme').map(toWorkspaceTheme).sort(bySortKey);
	}

	/** One theme the workspace offers, unless it was unpublished. */
	workspaceTheme(id: string): WorkspaceTheme | undefined {
		const r = this.get('workspace_theme', id);
		return r && !r.values.deleted_at ? toWorkspaceTheme(r) : undefined;
	}

	/** The views a user sees: shared ones and their own. */
	views(userId: string): SavedView[] {
		return this.#live('saved_view')
			.filter((r) => r.values.owner_id === null || r.values.owner_id === userId)
			.map(toView)
			.sort(bySortKey);
	}

	view(id: string): SavedView | undefined {
		const r = this.get('saved_view', id);
		return r && !r.values.deleted_at ? toView(r) : undefined;
	}
}

function toAutomation(r: Row): Automation {
	const v = r.values;
	return {
		id: r.id,
		projectId: (v.project_id as string | null | undefined) ?? null,
		name: (v.name as string | undefined) ?? '',
		enabled: Boolean(v.enabled),
		rule: v.rule,
		madeBy: (v.made_by as string | undefined) ?? '',
		pausedWhy: (v.paused_why as string | null | undefined) ?? null,
		sortKey: (v.sort_key as string | undefined) ?? ''
	};
}

function toWorkspaceTheme(r: Row): WorkspaceTheme {
	const v = r.values;
	return {
		id: r.id,
		name: (v.name as string | undefined) ?? '',
		body: v.body,
		sourceId: (v.source_id as string | null | undefined) ?? null,
		suggestedAt: (v.suggested_at as string | null | undefined) ?? null,
		publishedBy: (v.published_by as string | null | undefined) ?? null,
		publishedAt: (v.published_at as string | null | undefined) ?? null,
		sortKey: (v.sort_key as string | undefined) ?? ''
	};
}

const newestFirst = (a: TimeEntry, b: TimeEntry) =>
	b.day.localeCompare(a.day) ||
	(b.startedAt ?? '').localeCompare(a.startedAt ?? '') ||
	a.id.localeCompare(b.id);

/** A time_entry or project_time_entry row as the app reads it. */
function toEntry(r: Row): TimeEntry {
	const v = r.values;
	const own = r.entity === 'project_time_entry';
	return {
		id: r.id,
		taskId: own ? null : (v.task_id as string),
		projectId: own ? (v.project_id as string) : null,
		userId: v.user_id as string,
		day: (v.day as string | undefined) ?? '',
		minutes: Number(v.minutes) || 0,
		note: (v.note as string | undefined) ?? '',
		startedAt: (v.started_at as string | null | undefined) ?? null,
		running: v.running === true,
		activeAt: (v.active_at as string | null | undefined) ?? null
	};
}

function toReminder(r: Row): Reminder {
	const offset = r.values.offset_minutes;
	return {
		id: r.id,
		taskId: r.values.task_id as string,
		remindAt: r.values.remind_at as string,
		offsetMinutes: offset === null || offset === undefined ? null : Number(offset)
	};
}

const isRecord = (v: unknown): v is Record<string, unknown> =>
	typeof v === 'object' && v !== null && !Array.isArray(v);

/**
 * A field's choices as stored, which a broken app or an old REST write
 * may have left in any shape: only objects with an ID of their own, each
 * once, with a name and a color that are strings.
 */
export function readOptions(v: unknown): FieldOption[] {
	if (!Array.isArray(v)) return [];
	// eslint-disable-next-line svelte/prefer-svelte-reactivity -- local to one read, never shown
	const seen = new Set<string>();
	const out: FieldOption[] = [];
	for (const o of v) {
		if (!isRecord(o) || typeof o.id !== 'string' || o.id === '' || seen.has(o.id)) continue;
		seen.add(o.id);
		// Keys from a newer app are kept, so an edit here keeps them.
		out.push({
			...o,
			id: o.id,
			name: typeof o.name === 'string' ? o.name : '',
			color: typeof o.color === 'string' ? o.color : 'blue'
		});
	}
	return out;
}

function toView(r: Row): SavedView {
	return {
		id: r.id,
		projectId: (r.values.project_id as string | null) ?? null,
		ownerId: (r.values.owner_id as string | null) ?? null,
		name: r.values.name as string,
		type: r.values.type as ViewType,
		filter: filterOf(r.values.config),
		// A row first seen in a pull may not hold the field at all; one that
		// is not an object (from a broken app) shows the defaults.
		display: isRecord(r.values.display) ? (r.values.display as ViewDisplay) : {},
		sortKey: r.values.sort_key as string
	};
}
