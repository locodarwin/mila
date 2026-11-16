// What the UI can do to a workspace's data. Each call is a local edit that
// syncs in the background; calls that make several edits undo as one.
import {
	addDaysTo,
	firstDue,
	hashId,
	nextDue,
	nextTaskId,
	tryParseRule,
	type RepeatMode
} from '../recurrence';
import { confetti, wantsConfetti } from '../confetti';
import { localMoment, remindAtFor } from '../reminders';
import { session } from '../session.svelte';
import { uuid } from '../uuid';
import {
	pairId,
	STARTER_STATUSES,
	TASK_CHILDREN,
	type FieldOption,
	type FieldType,
	type FieldValue,
	type Priority,
	type SavedView,
	type Page,
	type Project,
	type StatusCategory,
	type Task,
	type ViewDisplay,
	type ViewFilter,
	type ViewType
} from './entities';
import { applyPaths, type DisplayPath } from '../displayPaths';
import type { ColumnWrite } from '../viewDisplay';
import type { GroupWrite } from '../table';
import { engine } from './engine.svelte';
import type { Replica } from './replica.svelte';
import { keyBetween } from './sortkey';
import { wikiMain } from './pages';

const now = () => new Date().toISOString();

/** Who is making edits on this device. */
const me = () => session.user?.id ?? null;

function replicaOf(ws: string) {
	const r = engine.replica(ws);
	if (!r) throw new Error('That workspace is not loaded yet.');
	return r;
}

function keyAfter(items: { sortKey: string }[]): string {
	return keyBetween(items.at(-1)?.sortKey ?? null, null);
}

const DONE: StatusCategory[] = ['done', 'canceled'];

// Projects

export function createProject(ws: string, name: string): Promise<string> {
	replicaOf(ws);
	return engine.group(async () => {
		const id = uuid();
		await engine.mutate(ws, 'project', id, (replica) => ({
			name: name.trim(),
			sort_key: keyAfter(replica.projects())
		}));
		let key: string | null = null;
		for (const [statusName, category] of STARTER_STATUSES) {
			key = keyBetween(key, null);
			await engine.mutate(ws, 'status', uuid(), {
				project_id: id,
				name: statusName,
				category,
				sort_key: key
			});
		}
		return id;
	});
}

export function renameProject(ws: string, project: Project, name: string) {
	return engine.mutate(ws, 'project', project.id, { name: name.trim() });
}

export function setProjectDeleted(ws: string, projectId: string, deleted: boolean) {
	return engine.mutate(ws, 'project', projectId, { deleted_at: deleted ? now() : null });
}

/** A project's icon (an emoji, "lucide:<name>", or "" for none), color and description. */
export function updateProject(
	ws: string,
	projectId: string,
	fields: { icon?: string; color?: string; description?: string }
) {
	return engine.mutate(ws, 'project', projectId, {
		...fields,
		...(fields.description !== undefined ? { description: fields.description.trim() } : {})
	});
}

/** Archiving takes a project out of the sidebar and lists, keeping everything in it. */
export function setProjectArchived(ws: string, projectId: string, archived: boolean) {
	return engine.mutate(ws, 'project', projectId, { archived_at: archived ? now() : null });
}

/**
 * Moves a project up or down one place among those in use. The order is
 * the workspace's, the same for everyone in it.
 */
export function moveProject(ws: string, projectId: string, by: -1 | 1) {
	return engine.mutate(ws, 'project', projectId, (replica) => {
		const list = replica.projects();
		const i = list.findIndex((p) => p.id === projectId);
		const j = i + by;
		if (i < 0 || j < 0 || j >= list.length) return {};
		// Between the neighbors on the far side of the one it passes.
		const [before, after] = by < 0 ? [list[j - 1], list[j]] : [list[j], list[j + 1]];
		const lo = before?.sortKey ?? null;
		const hi = after?.sortKey ?? null;
		return {
			sort_key: lo !== null && hi !== null && lo >= hi ? keyBetween(lo, null) : keyBetween(lo, hi)
		};
	});
}

// Statuses

/** Moves a status one place left or right among its project's. */
export function moveStatus(ws: string, projectId: string, statusId: string, by: -1 | 1) {
	return engine.mutate(ws, 'status', statusId, (replica) => {
		const list = replica.statuses(projectId);
		const i = list.findIndex((s) => s.id === statusId);
		const j = i + by;
		if (i < 0 || j < 0 || j >= list.length) return {};
		const [before, after] = by < 0 ? [list[j - 1], list[j]] : [list[j], list[j + 1]];
		const lo = before?.sortKey ?? null;
		const hi = after?.sortKey ?? null;
		return {
			sort_key: lo !== null && hi !== null && lo >= hi ? keyBetween(lo, null) : keyBetween(lo, hi)
		};
	});
}

/** The first status of a category in a project. */
function statusFor(
	replica: Replica,
	projectId: string | null,
	category: StatusCategory
): string | null {
	if (!projectId) return null;
	const statuses = replica.statuses(projectId);
	return (statuses.find((s) => s.category === category) ?? statuses[0])?.id ?? null;
}

export function createStatus(
	ws: string,
	projectId: string,
	name: string,
	category: StatusCategory
) {
	return engine.mutate(ws, 'status', uuid(), (replica) => ({
		project_id: projectId,
		name: name.trim(),
		category,
		sort_key: keyAfter(replica.statuses(projectId))
	}));
}

export function updateStatus(
	ws: string,
	statusId: string,
	fields: { name?: string; category?: StatusCategory }
) {
	return engine.mutate(ws, 'status', statusId, fields);
}

/** Deletes a status, first moving its tasks to another of the same kind. */
export function deleteStatus(ws: string, projectId: string, statusId: string) {
	const replica = replicaOf(ws);
	return engine.group(async () => {
		const gone = replica.status(statusId);
		const others = replica.statuses(projectId).filter((s) => s.id !== statusId);
		const target = others.find((s) => s.category === gone?.category) ?? others[0];
		if (!target) throw new Error('A project needs at least one status.');
		const all = [...replica.tasks(projectId)];
		for (const t of replica.tasks(projectId)) all.push(...replica.subtasks(t.id));
		for (const t of all.filter((t) => t.statusId === statusId)) {
			await engine.mutate(ws, 'task', t.id, { status_id: target.id });
		}
		await engine.mutate(ws, 'status', statusId, { deleted_at: now() });
	});
}

// Tasks

export async function createTask(
	ws: string,
	projectId: string | null,
	title: string,
	opts: { parentId?: string; statusId?: string | null; fields?: Record<string, unknown> } = {}
): Promise<string> {
	replicaOf(ws);
	const id = uuid();
	await engine.mutate(ws, 'task', id, (replica) => {
		const parent = opts.parentId ? replica.task(opts.parentId) : undefined;
		const siblings = parent ? replica.subtasks(parent.id) : replica.tasks(projectId);
		return {
			title: title.trim(),
			project_id: projectId,
			parent_id: opts.parentId ?? null,
			status_id: opts.statusId ?? statusFor(replica, projectId, 'todo'),
			sort_key: keyAfter(siblings),
			created_by: me(),
			...opts.fields
		};
	});
	return id;
}

export function setTaskTitle(ws: string, task: Task, title: string) {
	return engine.mutate(ws, 'task', task.id, { title: title.trim() });
}

export function setTaskFields(
	ws: string,
	taskId: string,
	fields: Partial<{
		notes: string;
		priority: Priority;
		start_date: string | null;
		due_date: string | null;
		due_time: string | null;
	}>
) {
	if (!('due_date' in fields || 'due_time' in fields) || !relativeReminders(ws, taskId).length)
		return engine.mutate(ws, 'task', taskId, fields);
	return engine.group(async () => {
		await engine.mutate(ws, 'task', taskId, fields);
		await followDue(ws, taskId);
	});
}

/**
 * Completing a project task also moves it to the project's done status.
 * Completing a repeating task makes the next one too. Completing the last
 * open task of a project, subtasks included, celebrates in the Playful
 * theme (confetti.ts).
 */
export async function setTaskCompleted(ws: string, task: Task, completed: boolean) {
	await completeTask(ws, task, completed);
	if (completed && !task.completedAt && task.projectId && wantsConfetti()) {
		if (replicaOf(ws).projectFinished(task.projectId)) confetti();
	}
}

function completeTask(ws: string, task: Task, completed: boolean) {
	const mark = () =>
		engine.mutate(ws, 'task', task.id, (replica) => {
			const fields: Record<string, unknown> = { completed_at: completed ? now() : null };
			if (task.projectId)
				fields.status_id = statusFor(replica, task.projectId, completed ? 'done' : 'todo');
			if (completed && task.recurrence && !task.seriesId) fields.series_id = task.id;
			return fields;
		});
	if (!completed || task.completedAt || !task.recurrence) return mark();
	return engine.group(async () => {
		await mark();
		await repeatTask(ws, task.id);
	});
}

/** Moving to a done or canceled status completes the task, and back. */
export function setTaskStatus(ws: string, task: Task, statusId: string) {
	const replica = replicaOf(ws);
	const category = replica.status(statusId)?.category;
	const done = category !== undefined && DONE.includes(category);
	const mark = () =>
		engine.mutate(ws, 'task', task.id, () => {
			const fields: Record<string, unknown> = { status_id: statusId };
			if (done && !task.completedAt) fields.completed_at = now();
			if (!done && task.completedAt) fields.completed_at = null;
			if (done && task.recurrence && !task.seriesId) fields.series_id = task.id;
			return fields;
		});
	if (!done || task.completedAt || !task.recurrence) return mark();
	return engine.group(async () => {
		await mark();
		await repeatTask(ws, task.id);
	});
}

/**
 * Sets how a task repeats (null: it does not). A task that starts
 * repeating on a schedule with no due date gets the rule's first date.
 */
export function setRecurrence(ws: string, task: Task, rule: string | null, mode: RepeatMode) {
	const parsed = tryParseRule(rule);
	const start = parsed && mode === 'schedule' && !task.dueDate;
	const set = () =>
		engine.mutate(ws, 'task', task.id, {
			recurrence: parsed ? rule : null,
			recurrence_mode: mode,
			...(start ? { due_date: firstDue(parsed, localToday()) } : {})
		});
	if (!start || !relativeReminders(ws, task.id).length) return set();
	return engine.group(async () => {
		await set();
		await followDue(ws, task.id);
	});
}

function daysBetween(from: string, to: string): number {
	return Math.round((Date.parse(to) - Date.parse(from)) / 864e5);
}

/**
 * Makes the task that follows a completed repeating one. Its ID comes
 * from the completed task alone, so another device completing the same
 * task offline (on another day, or in another timezone) makes the same
 * row. A copy that already exists is left alone; one that was deleted (an
 * undone completion) comes back whole.
 */
async function repeatTask(ws: string, taskId: string) {
	const replica = replicaOf(ws);
	const task = replica.task(taskId);
	const rule = tryParseRule(task?.recurrence);
	if (!task || !rule) return;
	const due = nextDue(rule, task.recurrenceMode, task.dueDate, localToday());
	if (!due) return; // the series has ended
	const series = task.seriesId ?? task.id;
	const id = nextTaskId(task.id);
	if (replica.task(id)) return;
	const gap = task.startDate && task.dueDate ? daysBetween(task.startDate, task.dueDate) : null;
	await engine.mutate(ws, 'task', id, (r) => {
		// Just after the task it follows.
		const siblings = task.parentId ? r.subtasks(task.parentId) : r.tasks(task.projectId);
		const next = siblings.find((t) => t.sortKey > task.sortKey);
		return {
			title: task.title,
			notes: task.notes,
			project_id: task.projectId,
			parent_id: task.parentId,
			status_id: openStatus(r, task.projectId),
			priority: task.priority,
			start_date: gap === null ? null : addDaysTo(due, -gap),
			due_date: due,
			due_time: task.dueTime,
			completed_at: null,
			deleted_at: null,
			created_by: me(),
			recurrence: task.recurrence,
			recurrence_mode: task.recurrenceMode,
			series_id: series,
			sort_key: keyBetween(task.sortKey, next?.sortKey ?? null)
		};
	});
	for (const l of replica.taskLabels(task.id)) await setTaskLabel(ws, id, l.id, true);
	for (const u of replica.assignees(task.id))
		if (!session.outOfReach(ws, task.projectId, u)) await setAssignee(ws, id, u, true);
	for (const item of replica.checklist(task.id)) {
		await engine.mutate(ws, 'checklist_item', hashId(`${id}/${item.id}`), {
			task_id: id,
			text: item.text,
			done: false,
			sort_key: item.sortKey,
			deleted_at: null
		});
	}
	for (const [fieldId, value] of Object.entries(replica.fieldValues(task.id))) {
		await setFieldValue(ws, id, fieldId, value);
	}
	// My reminders relative to the due date come along; the server copies
	// other people's the same way, under the same IDs. A copy an undone
	// completion deleted comes back with the task.
	for (const r of relativeReminders(ws, task.id)) {
		const copy = hashId(`${id}/${r.id}`);
		const had = replica.get('reminder', copy);
		if (had && had.values.deleted_at == null) continue;
		await engine.mutate(ws, 'reminder', copy, (rep) => ({
			task_id: id,
			user_id: me(),
			offset_minutes: r.offsetMinutes,
			remind_at: remindAtFor(rep.task(id) ?? task, r.offsetMinutes!) ?? r.remindAt,
			deleted_at: null
		}));
	}
}

/**
 * Where a repeat starts: the project's first to-do status, as for any new
 * task, else its first that is not done or canceled, else its first.
 */
function openStatus(replica: Replica, projectId: string | null): string | null {
	if (!projectId) return null;
	const statuses = replica.statuses(projectId);
	const open =
		statuses.find((s) => s.category === 'todo') ??
		statuses.find((s) => !DONE.includes(s.category)) ??
		statuses[0];
	return open?.id ?? null;
}

/**
 * Moves a task, with its subtasks, to another project (or the inbox), each
 * landing in the status of the same kind there.
 */
export function moveTask(ws: string, task: Task, projectId: string | null) {
	const replica = replicaOf(ws);
	return engine.group(async () => {
		for (const t of [task, ...replica.subtasks(task.id)]) {
			const category = replica.status(t.statusId)?.category ?? (t.completedAt ? 'done' : 'todo');
			await engine.mutate(ws, 'task', t.id, (r) => ({
				project_id: projectId,
				status_id: statusFor(r, projectId, category),
				...(t.id === task.id ? { sort_key: keyAfter(r.tasks(projectId)) } : {})
			}));
		}
	});
}

export function setTaskDeleted(ws: string, taskId: string, deleted: boolean) {
	return engine.mutate(ws, 'task', taskId, { deleted_at: deleted ? now() : null });
}

// Labels

export function createLabel(ws: string, name: string, color = 'blue'): Promise<string> {
	const id = uuid();
	return engine
		.mutate(ws, 'label', id, (replica) => ({
			name: name.trim(),
			color,
			sort_key: keyAfter(replica.labels())
		}))
		.then(() => id);
}

export function updateLabel(
	ws: string,
	labelId: string,
	fields: { name?: string; color?: string }
) {
	return engine.mutate(ws, 'label', labelId, fields);
}

export function deleteLabel(ws: string, labelId: string) {
	return engine.mutate(ws, 'label', labelId, { deleted_at: now() });
}

export function setTaskLabel(ws: string, taskId: string, labelId: string, on: boolean) {
	return engine.mutate(ws, 'task_label', pairId(taskId, labelId), {
		task_id: taskId,
		label_id: labelId,
		deleted_at: on ? null : now()
	});
}

export function setAssignee(ws: string, taskId: string, userId: string, on: boolean) {
	return engine.mutate(ws, 'task_assignee', pairId(taskId, userId), {
		task_id: taskId,
		user_id: userId,
		deleted_at: on ? null : now()
	});
}

// Dependencies (docs/PLAN.md section 3)

/**
 * Why taskId cannot wait for blockerId, or null when it can: itself, a
 * dependency it already has, or a loop (the blocker already waits for
 * it, directly or down a chain). The server refuses a loop too.
 */
export function dependencyProblem(
	replica: Replica,
	taskId: string,
	blockerId: string
): string | null {
	if (taskId === blockerId) return 'A task cannot wait for itself.';
	const task = replica.task(taskId);
	const blocker = replica.task(blockerId);
	if (!task || !blocker) return 'That task is gone.';
	if (replica.blockers(taskId).some((t) => t.id === blockerId))
		return `${task.title} already waits for ${blocker.title}.`;
	const seen = new Set([taskId]);
	const queue = [taskId];
	while (queue.length) {
		for (const w of replica.waiting(queue.shift()!)) {
			if (w.id === blockerId)
				return `That would make a loop: ${blocker.title} already waits for ${task.title}.`;
			if (!seen.has(w.id)) {
				seen.add(w.id);
				queue.push(w.id);
			}
		}
	}
	return null;
}

/** Makes taskId wait for blockerId, or stops it waiting. */
export function setDependency(ws: string, taskId: string, blockerId: string, on: boolean) {
	return engine.mutate(ws, 'task_dependency', pairId(taskId, blockerId), {
		task_id: taskId,
		blocker_id: blockerId,
		deleted_at: on ? null : now()
	});
}

/**
 * The unfinished tasks waiting on taskId, directly or down a chain, in
 * the order they are reached.
 */
export function waitingChain(replica: Replica, taskId: string): Task[] {
	const out: Task[] = [];
	const seen = new Set([taskId]);
	const queue = [taskId];
	while (queue.length) {
		for (const w of replica.waiting(queue.shift()!)) {
			if (seen.has(w.id)) continue;
			seen.add(w.id);
			queue.push(w.id);
			if (!w.completedAt) out.push(w);
		}
	}
	return out;
}

/** Moves each task's dates by a number of days, as one undo step. */
export function shiftDates(ws: string, taskIds: string[], days: number) {
	return engine.group(async () => {
		for (const id of taskIds)
			await engine.mutate(ws, 'task', id, (replica) => {
				const t = replica.task(id);
				return {
					start_date: t?.startDate ? addDaysTo(t.startDate, days) : null,
					due_date: t?.dueDate ? addDaysTo(t.dueDate, days) : null
				};
			});
	});
}

// Checklists

export function addChecklistItem(ws: string, taskId: string, text: string) {
	return engine.mutate(ws, 'checklist_item', uuid(), (replica) => ({
		task_id: taskId,
		text: text.trim(),
		done: false,
		sort_key: keyAfter(replica.checklist(taskId))
	}));
}

export function updateChecklistItem(
	ws: string,
	id: string,
	fields: { text?: string; done?: boolean }
) {
	return engine.mutate(ws, 'checklist_item', id, fields);
}

export function deleteChecklistItem(ws: string, id: string) {
	return engine.mutate(ws, 'checklist_item', id, { deleted_at: now() });
}

// Comments

export function addComment(ws: string, taskId: string, authorId: string, body: string) {
	return engine.mutate(ws, 'comment', uuid(), {
		task_id: taskId,
		author_id: authorId,
		body: body.trim()
	});
}

export function editComment(ws: string, id: string, body: string) {
	return engine.mutate(ws, 'comment', id, { body: body.trim(), edited_at: now() });
}

export function deleteComment(ws: string, id: string) {
	return engine.mutate(ws, 'comment', id, { deleted_at: now() });
}

export function deleteAttachment(ws: string, id: string) {
	return engine.mutate(ws, 'attachment', id, { deleted_at: now() });
}

/** Restores something from the trash, given the server's copy of it. */
export function restore(
	ws: string,
	row: {
		entity: string;
		id: string;
		fields: Record<string, unknown>;
		clocks: Record<string, string>;
	}
) {
	const replica = replicaOf(ws);
	replica.remote(row.entity as 'task' | 'project', row.id, row.fields, row.clocks);
	return engine.mutate(ws, row.entity as 'task' | 'project', row.id, { deleted_at: null });
}

export { TASK_CHILDREN };

/** The date of a moment where the user is, as YYYY-MM-DD. */
export function localDay(at: Date): string {
	return `${at.getFullYear()}-${String(at.getMonth() + 1).padStart(2, '0')}-${String(at.getDate()).padStart(2, '0')}`;
}

/** Today's date where the user is, as YYYY-MM-DD. */
export function localToday(): string {
	return localDay(new Date());
}

/**
 * Puts a task in the user's day, or takes it out. One entry per task and
 * person, so adding it again on another day moves it.
 */
export function setInMyDay(
	ws: string,
	userId: string,
	taskId: string,
	on: boolean,
	day = localToday()
) {
	return engine.mutate(ws, 'my_day_entry', pairId(taskId, userId), (replica) => ({
		task_id: taskId,
		user_id: userId,
		day,
		sort_key: keyAfter(replica.myDay(userId, day).map((e) => ({ sortKey: e.sortKey }))),
		deleted_at: on ? null : now()
	}));
}

// Reminders

/** My reminders on a task that follow its due date. */
function relativeReminders(ws: string, taskId: string) {
	const user = me();
	if (!user) return [];
	return replicaOf(ws)
		.reminders(user, taskId)
		.filter((r) => r.offsetMinutes !== null);
}

/**
 * Keeps my relative reminders on a task at their distance from its due
 * moment, after its due date or time changed. With no due date they keep
 * the time they had.
 */
async function followDue(ws: string, taskId: string) {
	for (const r of relativeReminders(ws, taskId)) {
		await engine.mutate(ws, 'reminder', r.id, (replica) => {
			const task = replica.task(taskId);
			const at = task && remindAtFor(task, r.offsetMinutes!);
			return at ? { remind_at: at } : {};
		});
	}
}

/**
 * Adds a reminder for me on a task: relative to its due moment (minutes
 * before it) or at a fixed time.
 */
export async function addReminder(
	ws: string,
	taskId: string,
	when: { offset: number } | { at: string }
): Promise<string> {
	const user = me();
	if (!user) throw new Error('Sign in to set reminders.');
	const id = uuid();
	await engine.mutate(ws, 'reminder', id, (replica) => {
		const task = replica.task(taskId);
		const relative = 'offset' in when && task?.dueDate;
		return {
			task_id: taskId,
			user_id: user,
			remind_at: relative
				? remindAtFor(task, when.offset)
				: 'at' in when
					? when.at
					: new Date().toISOString(),
			offset_minutes: relative ? when.offset : null,
			deleted_at: null
		};
	});
	return id;
}

export function removeReminder(ws: string, id: string) {
	return engine.mutate(ws, 'reminder', id, { deleted_at: now() });
}

/** Creates a task from quick add, with everything it understood. */
export function quickAdd(
	ws: string,
	parsed: {
		title: string;
		projectId?: string;
		priority?: Priority;
		dueDate?: string;
		dueTime?: string;
		recurrence?: string;
		reminder?: { offset: number } | { date: string; time: string };
		labelIds: string[];
		assigneeIds: string[];
	},
	defaults: { projectId: string | null; myDayFor?: string }
): Promise<string> {
	return engine.group(async () => {
		const id = await createTask(ws, parsed.projectId ?? defaults.projectId, parsed.title, {
			fields: {
				...(parsed.priority ? { priority: parsed.priority } : {}),
				...(parsed.dueDate ? { due_date: parsed.dueDate } : {}),
				...(parsed.dueTime ? { due_time: parsed.dueTime } : {}),
				...(parsed.recurrence ? { recurrence: parsed.recurrence } : {})
			}
		});
		for (const l of parsed.labelIds) await setTaskLabel(ws, id, l, true);
		for (const u of parsed.assigneeIds) await setAssignee(ws, id, u, true);
		if (defaults.myDayFor) await setInMyDay(ws, defaults.myDayFor, id, true);
		const r = parsed.reminder;
		if (r)
			await addReminder(
				ws,
				id,
				'offset' in r ? r : { at: localMoment(r.date, r.time).toISOString() }
			);
		return id;
	});
}

/** Moves a task to sit between two others in its list. */
export function placeTask(ws: string, taskId: string, before: string | null, after: string | null) {
	// Two devices inserting at once can give neighbors equal keys; then the
	// task goes just after `before`, which is as close as the keys allow.
	const key =
		before !== null && after !== null && before >= after
			? keyBetween(before, null)
			: keyBetween(before, after);
	return engine.mutate(ws, 'task', taskId, { sort_key: key });
}

// Custom fields

export function createField(
	ws: string,
	name: string,
	type: FieldType,
	projectId: string | null
): Promise<string> {
	const id = uuid();
	return engine
		.mutate(ws, 'custom_field', id, (replica) => ({
			project_id: projectId,
			name: name.trim(),
			type,
			options: [],
			sort_key: keyAfter(replica.fields()),
			deleted_at: null
		}))
		.then(() => id);
}

export function updateField(ws: string, id: string, fields: { name: string }) {
	return engine.mutate(ws, 'custom_field', id, { name: fields.name.trim() });
}

/**
 * Changes a select field's choices. The change is worked out in the save
 * queue from the list as it stands then, so quick edits build on each other.
 */
export function editFieldOptions(
	ws: string,
	id: string,
	edit: (options: FieldOption[]) => FieldOption[]
) {
	return engine.mutate(ws, 'custom_field', id, (replica) => ({
		options: edit(replica.fields().find((f) => f.id === id)?.options ?? [])
	}));
}

export function deleteField(ws: string, id: string) {
	return engine.mutate(ws, 'custom_field', id, { deleted_at: now() });
}

/** Sets a task's value for a field; an empty value clears it. One row per task and field. */
export function setFieldValue(ws: string, taskId: string, fieldId: string, value: FieldValue) {
	const empty =
		value === null ||
		value === '' ||
		(Array.isArray(value) && value.length === 0) ||
		value === false;
	return engine.mutate(ws, 'custom_field_value', pairId(taskId, fieldId), {
		task_id: taskId,
		field_id: fieldId,
		...(empty ? { deleted_at: now() } : { value, deleted_at: null })
	});
}

/**
 * Moves a task into a board column: the status, priority or field value the
 * column stands for, when it does not have it already. A task whose status
 * is gone shows in the first status column, and gets that status here.
 */
export function writeColumn(ws: string, task: Task, write: ColumnWrite): Promise<unknown> {
	if ('status' in write)
		return task.statusId === write.status
			? Promise.resolve()
			: setTaskStatus(ws, task, write.status);
	if ('priority' in write)
		return task.priority === write.priority
			? Promise.resolve()
			: setTaskFields(ws, task.id, { priority: write.priority });
	// As the schedule chooser does: no date takes the time with it.
	if ('due' in write)
		return task.dueDate === write.due
			? Promise.resolve()
			: setTaskFields(
					ws,
					task.id,
					write.due ? { due_date: write.due } : { due_date: null, due_time: null }
				);
	const current = replicaOf(ws).fieldValues(task.id)[write.field] ?? null;
	return current === write.value
		? Promise.resolve()
		: setFieldValue(ws, task.id, write.field, write.value);
}

/**
 * Puts a task in a table group (lib/table.ts, groupWrite): what the group
 * stands for, when it does not have it already. Resolves to false, writing
 * nothing, when the task cannot take it: a status category its project
 * has no status in, or one an inbox task cannot be in (it is only to do
 * or done).
 */
export async function writeGroup(ws: string, task: Task, write: GroupWrite): Promise<boolean> {
	if ('project' in write) {
		if (task.projectId !== write.project) await moveTask(ws, task, write.project);
		return true;
	}
	if ('category' in write) {
		const replica = replicaOf(ws);
		if (!task.projectId) {
			if (write.category !== 'todo' && write.category !== 'done') return false;
			const done = write.category === 'done';
			if (done !== (task.completedAt !== null)) await setTaskCompleted(ws, task, done);
			return true;
		}
		if (replica.status(task.statusId)?.category === write.category) return true;
		const status = replica.statuses(task.projectId).find((s) => s.category === write.category);
		if (!status) return false;
		await setTaskStatus(ws, task, status.id);
		return true;
	}
	await writeColumn(ws, task, write);
	return true;
}

/** Makes a task in a board column, with what the column stands for, as one edit. */
export function createTaskIn(
	ws: string,
	projectId: string | null,
	title: string,
	write: ColumnWrite
): Promise<string> {
	if ('status' in write) return createTask(ws, projectId, title, { statusId: write.status });
	if ('priority' in write)
		return createTask(ws, projectId, title, { fields: { priority: write.priority } });
	if ('due' in write) return createTask(ws, projectId, title, { fields: { due_date: write.due } });
	return engine.group(async () => {
		const id = await createTask(ws, projectId, title);
		if (write.value !== null) await setFieldValue(ws, id, write.field, write.value);
		return id;
	});
}

// Saved views

const isRecord = (v: unknown): v is Record<string, unknown> =>
	typeof v === 'object' && v !== null && !Array.isArray(v);

export function createView(
	ws: string,
	view: {
		name: string;
		type: ViewType;
		projectId: string | null;
		ownerId: string | null;
		filter?: ViewFilter;
		display?: ViewDisplay;
	}
): Promise<string> {
	const id = uuid();
	return engine
		.mutate(ws, 'saved_view', id, (replica) => ({
			project_id: view.projectId,
			owner_id: view.ownerId,
			name: view.name.trim(),
			type: view.type,
			// A filter that cannot be read (a copy of a broken view) starts afresh.
			config: { filter: isRecord(view.filter) ? view.filter : {} },
			...(view.display && Object.keys(view.display).length ? { display: view.display } : {}),
			sort_key: keyAfter(replica.views(view.ownerId ?? '')),
			deleted_at: null
		}))
		.then(() => id);
}

/** A change to a part of a view: the new values, or how to work them out from the current ones. */
export type ViewEdit<T, P> = P | ((current: T) => P);

/**
 * Changes a saved view. Only the parts given are written. The filter and
 * display are patched onto the row as it stands when the write runs, so
 * edits made elsewhere to other parts survive, keys from a newer app too;
 * a patch value of undefined takes that part away. A change that rewrites
 * a list (sort rules, conditions, shown fields) is given as a function of
 * the current value, so quick changes build on each other in the save queue
 * rather than each starting from what the page showed.
 */
export function editView(
	ws: string,
	id: string,
	edit: {
		name?: string;
		type?: ViewType;
		filter?: ViewEdit<ViewFilter, Partial<ViewFilter>>;
		display?: ViewEdit<ViewDisplay, Partial<Record<DisplayPath, unknown>>>;
	}
) {
	return engine.mutate(ws, 'saved_view', id, (replica) => {
		const row = replica.get('saved_view', id)?.values ?? {};
		const fields: Record<string, unknown> = {};
		if (edit.name !== undefined) fields.name = edit.name.trim();
		if (edit.type !== undefined) fields.type = edit.type;
		// A config, filter or display that is not an object (from a broken
		// app) is started afresh: the edit then makes it one again.
		if (edit.filter) {
			const config = isRecord(row.config) ? row.config : {};
			const filter: Record<string, unknown> = isRecord(config.filter) ? { ...config.filter } : {};
			const patch =
				typeof edit.filter === 'function' ? edit.filter(filter as ViewFilter) : edit.filter;
			for (const [k, v] of Object.entries(patch)) {
				if (v === undefined) delete filter[k];
				else filter[k] = v;
			}
			fields.config = { ...config, filter };
		}
		if (edit.display) {
			const display = isRecord(row.display) ? (row.display as ViewDisplay) : {};
			const patch = typeof edit.display === 'function' ? edit.display(display) : edit.display;
			fields.display = applyPaths(display, patch);
		}
		return fields;
	});
}

export function deleteView(ws: string, id: string) {
	return engine.mutate(ws, 'saved_view', id, { deleted_at: now() });
}

/**
 * Shares a personal view with the workspace. A view's owner is fixed, so
 * this makes a shared copy and removes the personal one.
 */
export function shareView(ws: string, view: SavedView): Promise<string> {
	return engine.group(async () => {
		const id = await createView(ws, { ...view, ownerId: null });
		await deleteView(ws, view.id);
		return id;
	});
}

// Wiki pages (docs/PLAN.md section 3)

/**
 * Makes a wiki's main page if this device has not seen it: as far as
 * anyone can tell it exists from the start, and whoever first writes in a
 * new wiki makes the same row any other device would (pages.ts).
 */
export async function ensureMainPage(ws: string, projectId: string | null): Promise<string> {
	const id = wikiMain(ws, projectId);
	if (engine.replica(ws)?.get('page', id)) return id;
	await engine.mutate(
		ws,
		'page',
		id,
		{
			project_id: projectId,
			parent_id: null,
			title: '',
			body: '',
			sort_key: 'a0',
			deleted_at: null
		},
		{ undoable: false }
	);
	return id;
}

/** Makes a page under another, last among its siblings. */
export async function createPage(
	ws: string,
	projectId: string | null,
	parentId: string,
	title: string
): Promise<string> {
	if (parentId === wikiMain(ws, projectId)) await ensureMainPage(ws, projectId);
	const id = uuid();
	await engine.mutate(ws, 'page', id, (replica) => ({
		project_id: projectId,
		parent_id: parentId,
		title: title.trim(),
		body: '',
		sort_key: keyAfter(replica.pages(projectId).filter((p) => p.parentId === parentId)),
		deleted_at: null
	}));
	return id;
}

export function setPageTitle(ws: string, pageId: string, title: string) {
	return engine.mutate(ws, 'page', pageId, { title: title.trim() });
}

/**
 * Puts a page under a parent, between two siblings' sort keys (null for an
 * end). The server refuses a parent inside the page's own subpages.
 */
export function movePage(
	ws: string,
	pageId: string,
	parentId: string,
	before: string | null,
	after: string | null
) {
	const key =
		before !== null && after !== null && before >= after
			? keyBetween(before, null)
			: keyBetween(before, after);
	return engine.mutate(ws, 'page', pageId, { parent_id: parentId, sort_key: key });
}

/** A page's subpages, live ones, at every depth. */
export function subpagesOf(pages: Page[], pageId: string): Page[] {
	const out: Page[] = [];
	const walk = (id: string) => {
		for (const p of pages)
			if (p.parentId === id && !out.includes(p)) {
				out.push(p);
				walk(p.id);
			}
	};
	walk(pageId);
	return out;
}

/** Deletes a page and its subpages, as one edit to undo. */
export function deletePage(ws: string, pageId: string) {
	const replica = replicaOf(ws);
	const page = replica.page(pageId);
	if (!page) return Promise.resolve();
	const gone = [page, ...subpagesOf(replica.pages(page.projectId), pageId)];
	const at = now();
	return engine.group(async () => {
		for (const p of gone) await engine.mutate(ws, 'page', p.id, { deleted_at: at });
	});
}

/**
 * Writes a page's body whole, as a restore from its history does: the
 * server makes it an edit of the page's live document, and keeps it as a
 * version of its own.
 */
export function restorePageBody(ws: string, pageId: string, body: string) {
	return engine.mutate(ws, 'page', pageId, { body });
}

/**
 * Brings a page back from the trash: with the subpages deleted with it,
 * and the pages above it, so it has somewhere to be.
 */
export function restorePage(ws: string, pageId: string) {
	const replica = replicaOf(ws);
	const page = replica.page(pageId);
	if (!page?.deletedAt) return Promise.resolve();
	const all = [...replica.rows.values()]
		.filter((r) => r.entity === 'page')
		.map((r) => replica.page(r.id)!)
		.filter((p) => p.projectId === page.projectId);
	const with_ = [page, ...subpagesOf(all, pageId).filter((p) => p.deletedAt === page.deletedAt)];
	for (
		let up = all.find((p) => p.id === page.parentId);
		up?.deletedAt;
		up = all.find((p) => p.id === up!.parentId)
	)
		with_.push(up);
	return engine.group(async () => {
		for (const p of with_) await engine.mutate(ws, 'page', p.id, { deleted_at: null });
	});
}
