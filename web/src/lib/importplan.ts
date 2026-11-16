// What every import shares, whatever file it reads: the dry run's shape
// (the tasks it would make and what it would make for them, what it leaves
// out and why), the checks each task goes through (names cut to the
// server's limits, projects, statuses and labels found by name or made,
// people matched to members who can reach the task), and the import
// itself, made of ordinary edits so it works offline. A reader for one
// app's file (csvimport.ts with clickupimport.ts, asanaimport.ts or
// todoistimport.ts, githubimport.ts) turns
// its rows into planned tasks through a `planner`; `runImport` makes them.
import { MAX_ESTIMATE } from './duration';
import type { RepeatMode } from './recurrence';
import {
	STARTER_STATUSES,
	pairId,
	type CustomField,
	type FieldValue,
	type Priority,
	type StatusCategory
} from './sync/entities';
import { session } from './session.svelte';
import { engine } from './sync/engine.svelte';
import { keyBetween } from './sync/sortkey';
import { uuid } from './uuid';

/** The most tasks one import makes. */
export const MAX_ROWS = 5000;

/** The server's limits, in characters (sync/entities.go). */
export const LIMITS = {
	title: 500,
	notes: 100_000,
	comment: 20_000,
	project: 200,
	status: 60,
	label: 60
} as const;

/** The most comments and new labels one import makes. */
export const MAX_ALL_COMMENTS = 50_000;
export const MAX_NEW_LABELS = 1000;
/** The most checklist items one task brings, and how long each may be (sync/entities.go). */
export const MAX_CHECKLIST = 200;
export const CHECKLIST_TEXT = 500;
/** How deep subtasks may go under one another (the server's walk up a task's parents). */
export const MAX_DEPTH = 50;

// Control characters other than tabs and line ends never come in: Postgres
// refuses a NUL in text, and the rest only hide or garble what is written.
// eslint-disable-next-line no-control-regex
const CONTROLS = /[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g;
/** A mention as a comment stores one: kept as words, so an import tells nobody. */
const MENTION = /@\[([^\]\n]{1,100})\]\(user:[A-Za-z0-9-]{1,64}\)/g;

export const isObject = (v: unknown): v is Record<string, unknown> =>
	typeof v === 'object' && v !== null && !Array.isArray(v);

/** An entry's own value, never one it inherits. */
export const own = (o: Record<string, unknown>, key: string): unknown =>
	Object.hasOwn(o, key) ? o[key] : undefined;

/**
 * Mila's mentions turned to words, until none is left: one pass can make a
 * new one, as `@[a@[X](user:junk)](user:ID)` leaves `@a@[X](user:ID)`.
 * Each pass shortens the text, so it ends.
 */
export function stripMentions(s: string): string {
	for (let was = ''; was !== s;) {
		was = s;
		s = s.replace(MENTION, '@$1');
	}
	return s;
}

/**
 * Text from an untrusted file as it may come in: no control characters,
 * Unix line ends, Mila's mentions turned to words, at most `max` long.
 */
export function cleanText(v: unknown, max: number): string {
	if (typeof v !== 'string') return '';
	return stripMentions(
		v
			.slice(0, max * 2)
			.replace(/\r\n?/g, '\n')
			.replace(CONTROLS, '')
	).slice(0, max);
}

/**
 * A web address fit to link to from notes: https only, with brackets and
 * parentheses (which ClickUp's file links have) written as %28 and the
 * like, so none ends a Markdown link early. Null for anything else.
 */
export function httpsUrl(v: unknown): string | null {
	if (typeof v !== 'string') return null;
	const s = v.trim().replace(/[()[\]]/g, (c) => `%${c.charCodeAt(0).toString(16).toUpperCase()}`);
	if (s.length > 2000 || !/^https:\/\/[^\s<>"'`\\]+$/.test(s)) return null;
	try {
		const u = new URL(s);
		return u.protocol === 'https:' && u.hostname && !u.username && !u.password ? s : null;
	} catch {
		return null;
	}
}

/** Words as a Markdown link's text: one line, brackets and backslashes escaped. */
export const linkText = (s: string, max = 200) => oneLine(s, max).replace(/[\\[\]]/g, '\\$&');

/** Where the tasks go when the file does not say. */
export type Destination =
	{ kind: 'new'; name: string } | { kind: 'project'; id: string } | { kind: 'inbox' };

/** What the dry run needs to know of the workspace. */
export interface Context {
	projects: { id: string; name: string }[];
	statuses: (projectId: string) => { id: string; name: string; category: StatusCategory }[];
	labels: { id: string; name: string }[];
	fields: CustomField[];
	members: { userId: string; name: string; email: string }[];
	/** Whether someone can reach a project that exists, or the inbox (null). */
	reaches: (userId: string, projectId: string | null) => boolean;
}

/** A project, status, label or choice the import uses: one that exists, or one it makes. */
export type Ref = { id: string } | { make: string };

export interface PlannedTask {
	/** Where it is in the file: a CSV row's line, or an entry's place in a list. */
	line: number;
	/** Its ID in the file, which subtasks name as their parent; null for none. */
	key: string | null;
	/** The `key` of the planned task it is a subtask of, in the same project; null for none. */
	parent: string | null;
	title: string;
	notes: string;
	/** null for the inbox. */
	project: Ref | null;
	status: Ref | null;
	priority: Priority;
	start: string | null;
	due: string | null;
	dueTime: string | null;
	completedAt: string | null;
	/** When it was made where it came from, for reports; null for now. */
	madeAt: string | null;
	labels: Ref[];
	assignees: string[];
	values: { fieldId: string; value: FieldValue | Ref | Ref[] }[];
	/** Comments' bodies, oldest first, written as the person importing. */
	comments: string[];
	checklist: { text: string; done: boolean }[];
	/** Minutes, or null for none. */
	estimate: number | null;
	/** The `key`s of the planned tasks it waits for. */
	blockedBy: string[];
	/** How it repeats (a rule as lib/recurrence.ts stores it), from its due date; null for not. */
	repeat: { rule: string; mode: RepeatMode } | null;
}

export interface Plan {
	tasks: PlannedTask[];
	/** Rows left out, and why. */
	skipped: { line: number; reason: string }[];
	/** Values that could not be read; the row comes in without them. */
	warnings: { line: number; message: string }[];
	newProjects: string[];
	/** New statuses, as project key (an ID or a new project's name) to names and categories. */
	newStatuses: { project: Ref; name: string; category: StatusCategory }[];
	newLabels: string[];
	newOptions: { fieldId: string; name: string }[];
	/** People named in the file who are not in this workspace, and where. */
	strangers: { name: string; lines: number[] }[];
	/** Tasks that named people and come in with none of them. */
	unassigned: number[];
	/** Done tasks the file gives no time for, so they come in done as of the import. */
	completedNow: number[];
	/** Files the notes link to, since the files themselves cannot be fetched. */
	links: number;
	/** Rows left out for being of a type other than a task (ClickUp's Lead), and how many. */
	otherTypes: { name: string; count: number }[];
	/** Due dates the file gives in words from the day it was written (Todoist's "in 6 days"), read from today. */
	fromToday: number[];
}

export const same = (a: string, b: string) => a.trim().toLowerCase() === b.trim().toLowerCase();

/** A name cut to the server's limit for it, with no space left at either end. */
export const clip = (s: string, max: number) => s.trim().slice(0, max).trim();

/** A name on one line: runs of white space, line breaks among them, become one space. */
export const oneLine = (s: string, max: number) => clip(s.replace(/\s+/g, ' '), max);

export const refKey = (r: Ref) => ('id' in r ? `id:${r.id}` : `make:${r.make.toLowerCase()}`);

export function emptyPlan(): Plan {
	return {
		tasks: [],
		skipped: [],
		warnings: [],
		newProjects: [],
		newStatuses: [],
		newLabels: [],
		newOptions: [],
		strangers: [],
		unassigned: [],
		completedNow: [],
		links: 0,
		otherTypes: [],
		fromToday: []
	};
}

/** A task with nothing set but its title and place, for a reader to fill in. */
export function blankTask(line: number, title: string, project: Ref | null): PlannedTask {
	return {
		line,
		key: null,
		parent: null,
		title,
		notes: '',
		project,
		status: null,
		priority: 'none',
		start: null,
		due: null,
		dueTime: null,
		completedAt: null,
		madeAt: null,
		labels: [],
		assignees: [],
		values: [],
		comments: [],
		checklist: [],
		estimate: null,
		blockedBy: [],
		repeat: null
	};
}

export type Planner = ReturnType<typeof planner>;

/**
 * The checks a reader plans its tasks through, collecting into one plan
 * what they would make. Nothing in the workspace changes.
 */
export function planner(destination: Destination, ctx: Context) {
	const out = emptyPlan();

	const projectByName = (name: string): Ref => {
		name = clip(name, LIMITS.project);
		const found = ctx.projects.find((p) => same(p.name, name));
		if (found) return { id: found.id };
		const made = out.newProjects.find((p) => same(p, name));
		if (!made) out.newProjects.push(name);
		return { make: `project:${made ?? name}` };
	};
	// Where tasks with no project go, worked out at the first such task, so
	// a new project is only made when a task goes in it.
	let fallback: Ref | null | undefined;
	const fallbackRef = (): Ref | null =>
		(fallback ??=
			destination.kind === 'inbox'
				? null
				: destination.kind === 'project'
					? { id: destination.id }
					: projectByName(destination.name));

	const statusesOf = (project: Ref) =>
		'id' in project
			? ctx.statuses(project.id).map((s) => ({ ref: { id: s.id } as Ref, ...s }))
			: STARTER_STATUSES.map(([name, category]) => ({
					ref: { make: `status:${refKey(project)}|${name}` } as Ref,
					name,
					category
				}));
	const statusRef = (
		project: Ref,
		name: string,
		category?: StatusCategory
	): { ref: Ref; category: StatusCategory } => {
		name = clip(name, LIMITS.status);
		const found = statusesOf(project).find((s) => same(s.name, name));
		if (found) return { ref: found.ref, category: found.category };
		const key = `status:${refKey(project)}|${name}`;
		category ??= categoryOf(name);
		if (!out.newStatuses.some((s) => refKey(s.project) === refKey(project) && same(s.name, name)))
			out.newStatuses.push({ project, name, category });
		return { ref: { make: key }, category };
	};
	// Labels by name as `same` compares them, the first of each name, so a
	// file with thousands of labels is not read against every one.
	const fold = (s: string) => s.trim().toLowerCase();
	const labels = new Map<string, string>();
	for (const l of ctx.labels) if (!labels.has(fold(l.name))) labels.set(fold(l.name), l.id);
	const madeLabels = new Map<string, string>();
	/** A label there is, or one the plan makes already. */
	const knownLabel = (name: string): Ref | null => {
		const found = labels.get(fold(name));
		if (found) return { id: found };
		const made = madeLabels.get(fold(name));
		return made === undefined ? null : { make: `label:${made}` };
	};
	const labelRef = (name: string): Ref => {
		const known = knownLabel(name);
		if (known) return known;
		madeLabels.set(fold(name), name);
		out.newLabels.push(name);
		return { make: `label:${name}` };
	};
	const member = (s: string) =>
		ctx.members.find((m) => same(m.name, s) || same(m.email, s))?.userId ?? null;
	// A project the import makes is shared with no guest yet.
	const reaches = (userId: string, project: Ref | null) =>
		ctx.reaches(userId, !project ? null : 'id' in project ? project.id : '');

	return {
		out,
		ctx,
		warn: (line: number, message: string) => out.warnings.push({ line, message }),
		skip: (line: number, reason: string) => out.skipped.push({ line, reason }),
		projectByName,
		fallbackRef,
		statusesOf,
		statusRef,
		/**
		 * A status of the category wanted: the project's first one, or one
		 * of that name made for it.
		 */
		statusIn(project: Ref, category: StatusCategory, name: string): Ref {
			const found = statusesOf(project).find((s) => s.category === category);
			return found ? found.ref : statusRef(project, name, category).ref;
		},
		knownLabel,
		labelRef,
		member,
		reaches,
		/** Someone named in the file isn't in this workspace; the dry run lists them. */
		stranger(name: string, line: number) {
			const known = out.strangers.find((s) => same(s.name, name));
			if (!known) out.strangers.push({ name, lines: [line] });
			else if (known.lines.at(-1) !== line) known.lines.push(line);
		},
		add: (t: PlannedTask) => out.tasks.push(t)
	};
}

export interface Parentage {
	/** The parent's key, or null when it comes in on its own. */
	parent: string | null;
	/** The line of the task at the top of its chain, whose project it goes in. */
	top: number;
	/** Why the parent the file names is dropped, for the dry run. */
	problem: string | null;
}

/**
 * Which subtasks keep the parent the file names: one whose parent comes in
 * too, with no loop, no deeper than the server takes. The rest come in on
 * their own. `leftOut` holds the keys of rows the dry run leaves out.
 */
export function linkParents(
	entries: { line: number; key: string | null; parent: string | null }[],
	leftOut: Set<string> = new Set()
): Map<number, Parentage> {
	const byKey = new Map<string, (typeof entries)[number]>();
	for (const e of entries) if (e.key !== null && !byKey.has(e.key)) byKey.set(e.key, e);
	const done = new Map<number, Parentage & { depth: number }>();
	const visiting = new Set<number>();
	const alone = (line: number, problem: string | null) => ({
		parent: null,
		top: line,
		problem,
		depth: 0
	});
	const resolve = (e: (typeof entries)[number]): Parentage & { depth: number } => {
		const known = done.get(e.line);
		if (known) return known;
		let r: Parentage & { depth: number };
		const up = e.parent === null ? undefined : byKey.get(e.parent);
		if (e.parent === null) r = alone(e.line, null);
		else if (up === e)
			r = alone(e.line, 'It names itself as its parent, so it comes in on its own.');
		else if (!up)
			r = alone(
				e.line,
				leftOut.has(e.parent)
					? 'Its parent task is left out, so it comes in on its own.'
					: 'Its parent task is not in the file, so it comes in on its own.'
			);
		else if (visiting.has(up.line))
			r = alone(e.line, 'Its parent tasks loop back to it, so it comes in on its own.');
		else {
			visiting.add(e.line);
			const above = resolve(up);
			visiting.delete(e.line);
			r =
				above.depth + 1 > MAX_DEPTH
					? alone(e.line, `It is more than ${MAX_DEPTH} subtasks deep, so it comes in on its own.`)
					: { parent: up.key, top: above.top, problem: null, depth: above.depth + 1 };
		}
		done.set(e.line, r);
		return r;
	};
	for (const e of entries) resolve(e);
	return done;
}

/** The plan's tasks with every parent before its subtasks, otherwise in file order. */
export function parentsFirst(tasks: PlannedTask[]): PlannedTask[] {
	const byKey = new Map<string, PlannedTask>();
	for (const t of tasks) if (t.key !== null && !byKey.has(t.key)) byKey.set(t.key, t);
	const depth = new Map<PlannedTask, number>();
	const depthOf = (t: PlannedTask): number => {
		let d = depth.get(t);
		if (d !== undefined) return d;
		const up = t.parent === null ? undefined : byKey.get(t.parent);
		// Guarded against a plan made by hand with a loop in it.
		depth.set(t, 0);
		d = up && up !== t ? Math.min(depthOf(up) + 1, MAX_DEPTH + 1) : 0;
		depth.set(t, d);
		return d;
	};
	return tasks
		.map((t, i) => ({ t, i, d: depthOf(t) }))
		.sort((a, b) => a.d - b.d || a.i - b.i)
		.map((x) => x.t);
}

const squash = (s: string) => s.toLowerCase().replace(/[^a-z0-9]/g, '');

/** A status's category guessed from its name. */
export function categoryOf(name: string): StatusCategory {
	const s = squash(name);
	if (/^(done|complete|completed|finished|closed|resolved)$/.test(s)) return 'done';
	if (/^(canceled|cancelled|wontdo|wontfix|dropped)$/.test(s)) return 'canceled';
	if (/^(inprogress|doing|started|active|working|workingonit|wip|review|inreview|stuck)$/.test(s))
		return 'in-progress';
	if (/^(backlog|someday|later|icebox|idea|ideas)$/.test(s)) return 'backlog';
	return 'todo';
}

/**
 * Makes what a plan says, in order: projects (with the starter statuses),
 * statuses, labels, field choices, then the tasks, each parent before its
 * subtasks, with their labels, assignees, field values, checklist items and
 * comments, and last the dependencies between them. The edits are not
 * undoable one by one (the undo list holds far fewer); the tasks made are
 * returned, so they can be put in the trash together.
 */
export function runImport(
	ws: string,
	p: Plan,
	onProgress: (done: number, total: number) => void = () => {},
	/** Filled with each planned task's key and the ID of the task made for it. */
	keys?: Map<string, string>
): Promise<string[]> {
	return engine.bulkly(ws, () => importRows(ws, p, onProgress, keys));
}

async function importRows(
	ws: string,
	p: Plan,
	onProgress: (done: number, total: number) => void,
	keys?: Map<string, string>
): Promise<string[]> {
	const once = { undoable: false };
	const ids = new Map<string, string>();
	const resolve = (r: Ref | null): string | null =>
		r === null ? null : 'id' in r ? r.id : (ids.get(r.make.toLowerCase()) ?? null);

	for (const name of p.newProjects) {
		const id = uuid();
		await engine.mutate(
			ws,
			'project',
			id,
			(replica) => ({
				name,
				sort_key: keyBetween(replica.projects().at(-1)?.sortKey ?? null, null)
			}),
			once
		);
		ids.set(`project:${name}`.toLowerCase(), id);
		let key: string | null = null;
		for (const [statusName, category] of STARTER_STATUSES) {
			key = keyBetween(key, null);
			const sid = uuid();
			await engine.mutate(
				ws,
				'status',
				sid,
				{ project_id: id, name: statusName, category, sort_key: key },
				once
			);
			ids.set(`status:make:project:${name}|${statusName}`.toLowerCase(), sid);
		}
	}
	for (const s of p.newStatuses) {
		const project = resolve(s.project);
		if (!project) continue;
		const id = uuid();
		await engine.mutate(
			ws,
			'status',
			id,
			(replica) => ({
				project_id: project,
				name: s.name,
				category: s.category,
				sort_key: keyBetween(replica.statuses(project).at(-1)?.sortKey ?? null, null)
			}),
			once
		);
		ids.set(`status:${refKey(s.project)}|${s.name}`.toLowerCase(), id);
	}
	for (const name of p.newLabels) {
		const id = uuid();
		await engine.mutate(
			ws,
			'label',
			id,
			(replica) => ({
				name,
				color: 'blue',
				sort_key: keyBetween(replica.labels().at(-1)?.sortKey ?? null, null)
			}),
			once
		);
		ids.set(`label:${name}`.toLowerCase(), id);
	}
	for (const o of p.newOptions) {
		const id = uuid();
		ids.set(`option:${o.fieldId}|${o.name}`.toLowerCase(), id);
		await engine.mutate(
			ws,
			'custom_field',
			o.fieldId,
			(replica) => ({
				options: [
					...(replica.fields().find((f) => f.id === o.fieldId)?.options ?? []),
					{ id, name: o.name, color: 'blue' }
				]
			}),
			once
		);
	}

	// Each project's new tasks go after the ones it has, in file order, and
	// each parent's new subtasks after one another. Parents are made first.
	const lastKey = new Map<string, string | null>();
	const made: string[] = [];
	const byKey = new Map<string, { id: string; project: string | null }>();
	const me = session.user?.id ?? null;
	for (const [n, t] of parentsFirst(p.tasks).entries()) {
		const project = resolve(t.project);
		// A subtask lives in its parent's project; the plan keeps them together.
		const above = t.parent === null ? undefined : byKey.get(t.parent);
		const parent = above && above.project === project ? above.id : null;
		const where = parent ? `parent:${parent}` : (project ?? '');
		if (!lastKey.has(where))
			lastKey.set(
				where,
				parent ? null : (engine.replica(ws)?.tasks(project).at(-1)?.sortKey ?? null)
			);
		const sortKey = keyBetween(lastKey.get(where) ?? null, null);
		lastKey.set(where, sortKey);
		const id = uuid();
		if (t.key !== null && !byKey.has(t.key)) {
			byKey.set(t.key, { id, project });
			keys?.set(t.key, id);
		}
		await engine.mutate(
			ws,
			'task',
			id,
			{
				title: t.title,
				notes: t.notes,
				project_id: project,
				parent_id: parent,
				status_id: project ? resolve(t.status) : null,
				priority: t.priority,
				start_date: t.start,
				due_date: t.due,
				due_time: t.dueTime,
				completed_at: t.completedAt,
				sort_key: sortKey,
				created_by: me,
				...(t.estimate !== null && t.estimate > 0
					? { estimate_minutes: Math.min(Math.round(t.estimate), MAX_ESTIMATE) }
					: {}),
				...(t.repeat && t.due ? { recurrence: t.repeat.rule, recurrence_mode: t.repeat.mode } : {}),
				// The server makes it now unless told otherwise.
				...(t.madeAt ? { made_at: t.madeAt } : {})
			},
			once
		);
		for (const l of t.labels) {
			const label = resolve(l);
			if (!label) continue;
			await engine.mutate(
				ws,
				'task_label',
				pairId(id, label),
				{ task_id: id, label_id: label, deleted_at: null },
				once
			);
		}
		for (const user of t.assignees) {
			await engine.mutate(
				ws,
				'task_assignee',
				pairId(id, user),
				{ task_id: id, user_id: user, deleted_at: null },
				once
			);
		}
		for (const v of t.values) {
			const value = Array.isArray(v.value)
				? v.value.map((x) => (typeof x === 'object' && x !== null ? resolve(x) : x))
				: typeof v.value === 'object' && v.value !== null
					? resolve(v.value as Ref)
					: v.value;
			await engine.mutate(
				ws,
				'custom_field_value',
				pairId(id, v.fieldId),
				{ task_id: id, field_id: v.fieldId, value: value as FieldValue, deleted_at: null },
				once
			);
		}
		let itemKey: string | null = null;
		for (const item of t.checklist.slice(0, MAX_CHECKLIST)) {
			const text = oneLine(item.text, CHECKLIST_TEXT);
			if (!text) continue;
			itemKey = keyBetween(itemKey, null);
			await engine.mutate(
				ws,
				'checklist_item',
				uuid(),
				{ task_id: id, text, done: item.done, sort_key: itemKey },
				once
			);
		}
		// A comment is its author's, so it needs someone signed in.
		for (const body of me ? t.comments : []) {
			await engine.mutate(ws, 'comment', uuid(), { task_id: id, author_id: me, body }, once);
		}
		made.push(id);
		onProgress(n + 1, p.tasks.length);
	}
	// Dependencies once every task they join is made.
	for (const t of p.tasks) {
		const waiter = t.key === null ? undefined : byKey.get(t.key);
		for (const k of waiter ? t.blockedBy : []) {
			const blocker = byKey.get(k);
			if (!blocker || blocker.id === waiter!.id) continue;
			await engine.mutate(
				ws,
				'task_dependency',
				pairId(waiter!.id, blocker.id),
				{ task_id: waiter!.id, blocker_id: blocker.id, deleted_at: null },
				once
			);
		}
	}
	return made;
}
