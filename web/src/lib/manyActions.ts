// What a selection of tasks can be changed with: the
// bar's lists, the menu of a selected task, and the task keys while anything is
// selected. Each change to many is one undo step, its ops one batch
// (engine.batch). Someone who can't do a thing to every selected task isn't
// offered it. Loaded once something is selected.
import { announce } from './announce.svelte';
import { copyText } from './clipboard';
import type { MenuItem, MenuSpec } from './contextMenu.svelte';
import type { CommandId } from './keys/commands';
import { canOrganize } from './roles';
import { scheduleChoices } from './schedule.svelte';
import { selection } from './selection.svelte';
import { session } from './session.svelte';
import {
	localToday,
	moveTask,
	setAssignee,
	setInMyDay,
	setTaskCompleted,
	setTaskDeleted,
	setTaskFields,
	setTaskLabel,
	setTaskStatus,
	shiftDates
} from './sync/data';
import { engine } from './sync/engine.svelte';
import type { Priority, StatusCategory, Task } from './sync/entities';
import { toasts } from './toasts.svelte';

const sep = 'separator' as const;
const me = () => session.user?.id ?? '';
const role = () => session.workspace?.role;
const may = (check: (r: NonNullable<ReturnType<typeof role>>) => boolean) => {
	const r = role();
	return !r || check(r);
};

/** The selected tasks the page shows, in its order. */
export function selectedTasks(ws: string): Task[] {
	const replica = engine.replica(ws);
	if (!replica || selection.ws !== ws) return [];
	return selection
		.shown()
		.map((id) => replica.task(id))
		.filter((t): t is Task => !!t);
}

/** "5 tasks", or the one task's title in quotes, as a single task's messages have it. */
export function countOf(tasks: Task[]): string {
	return tasks.length === 1 ? `"${tasks[0].title || 'Untitled task'}"` : `${tasks.length} tasks`;
}

/** Makes one change to many, as one undo step, and says what it did with Undo. */
async function change(ws: string, said: string, fn: () => Promise<unknown>) {
	const { done, undo } = engine.batch(ws, fn);
	try {
		await done;
	} catch (err) {
		toasts.error(err instanceof Error ? err.message : 'Could not make that change.');
		return;
	}
	toasts.show(said, { onundo: () => void undo() });
}

async function each(tasks: Task[], fn: (t: Task) => Promise<unknown> | unknown) {
	for (const t of tasks) await fn(t);
}

export function completeMany(ws: string, tasks: Task[]) {
	const reopen = tasks.every((t) => t.completedAt !== null);
	const which = reopen ? tasks : tasks.filter((t) => t.completedAt === null);
	return change(ws, `${reopen ? 'Reopened' : 'Completed'} ${countOf(which)}`, () =>
		each(which, (t) => setTaskCompleted(ws, t, !reopen))
	);
}

export function myDayMany(ws: string, tasks: Task[]) {
	const replica = engine.replica(ws);
	const today = localToday();
	const inDay = (t: Task) => !!replica?.inMyDay(me(), t.id, today);
	const out = tasks.every(inDay);
	const which = out ? tasks : tasks.filter((t) => !inDay(t));
	return change(
		ws,
		out ? `Took ${countOf(which)} out of My Day` : `Added ${countOf(which)} to My Day`,
		() => each(which, (t) => setInMyDay(ws, me(), t.id, !out))
	);
}

const PRIORITY_NAMES: Record<Priority, string> = {
	urgent: 'urgent',
	high: 'high',
	medium: 'medium',
	low: 'low',
	none: 'none'
};

export function priorityMany(ws: string, tasks: Task[], priority: Priority) {
	return change(ws, `Priority ${PRIORITY_NAMES[priority]} on ${countOf(tasks)}`, () =>
		each(tasks, (t) => setTaskFields(ws, t.id, { priority }))
	);
}

export function deleteMany(ws: string, tasks: Task[]) {
	return change(ws, `Deleted ${countOf(tasks)}`, () =>
		each(tasks, (t) => setTaskDeleted(ws, t.id, true))
	);
}

/** Gives them all one due date, or none. */
export function scheduleMany(ws: string, tasks: Task[], date: string | null, said: string) {
	return change(
		ws,
		date ? `Scheduled ${countOf(tasks)} for ${said}` : `Took the dates off ${countOf(tasks)}`,
		() => each(tasks, (t) => setTaskFields(ws, t.id, { due_date: date }))
	);
}

/** Moves each one's own dates, keeping the gaps between them. */
export function shiftMany(ws: string, tasks: Task[], days: number) {
	const dated = tasks.filter((t) => t.dueDate || t.startDate);
	return change(
		ws,
		`Moved ${countOf(dated)} ${days === 1 ? 'a day' : days === 7 ? 'a week' : `${days} days`} later`,
		() =>
			shiftDates(
				ws,
				dated.map((t) => t.id),
				days
			)
	);
}

/** The kinds a status across projects is chosen by. */
const KINDS: [StatusCategory, string][] = [
	['todo', 'To do'],
	['in-progress', 'In progress'],
	['done', 'Done']
];

/** Puts each in its project's first status of a kind; a task in the Inbox is completed or reopened. */
export function kindMany(ws: string, tasks: Task[], kind: StatusCategory, name: string) {
	const replica = engine.replica(ws);
	return change(ws, `Moved ${countOf(tasks)} to ${name}`, () =>
		each(tasks, async (t) => {
			const status = t.projectId
				? replica?.statuses(t.projectId).find((s) => s.category === kind)
				: undefined;
			if (status) {
				if (t.statusId !== status.id) await setTaskStatus(ws, t, status.id);
			} else if (!t.projectId && (kind === 'done') !== (t.completedAt !== null)) {
				await setTaskCompleted(ws, t, kind === 'done');
			}
		})
	);
}

export function statusMany(ws: string, tasks: Task[], statusId: string, name: string) {
	return change(ws, `Moved ${countOf(tasks)} to ${name}`, () =>
		each(tasks, (t) => (t.statusId === statusId ? undefined : setTaskStatus(ws, t, statusId)))
	);
}

/** Every one, or some, or none: what a check beside a choice shows. */
function tri(tasks: Task[], has: (t: Task) => boolean): boolean | 'mixed' {
	const n = tasks.filter(has).length;
	return n === tasks.length ? true : n ? 'mixed' : false;
}

export function moveMany(ws: string, tasks: Task[], projectId: string | null, name: string) {
	// A subtask goes with its parent; the parents are what move.
	const ids = new Set(tasks.map((t) => t.id));
	const top = tasks.filter((t) => !t.parentId || !ids.has(t.parentId));
	return change(ws, `Moved ${countOf(top)} to ${name}`, () =>
		each(top, (t) => (t.projectId === projectId ? undefined : moveTask(ws, t, projectId)))
	);
}

/** A Markdown list of the tasks, with their links, for a page or a message. */
export async function copyLinks(tasks: Task[]) {
	const text = tasks
		.map(
			(t) =>
				`- [${(t.title || 'Untitled task').replace(/[[\]]/g, '\\$&')}](${location.origin}/?task=${t.id})`
		)
		.join('\n');
	if (await copyText(text))
		toasts.show(tasks.length === 1 ? 'Link copied' : `Copied ${tasks.length} links, as a list`);
}

const PRIORITIES: [Priority, string, CommandId][] = [
	['urgent', 'Urgent', 'task.priority.urgent'],
	['high', 'High', 'task.priority.high'],
	['medium', 'Medium', 'task.priority.medium'],
	['low', 'Low', 'task.priority.low'],
	['none', 'None', 'task.priority.none']
];
const PRIORITY_OF: Partial<Record<CommandId, Priority>> = Object.fromEntries(
	PRIORITIES.map(([p, , c]) => [c, p])
);

/**
 * A task key while anything is selected: it acts on the selection. Returns
 * whether it was one of those keys.
 */
export function runCommand(ws: string, cmd: CommandId): boolean {
	const tasks = selectedTasks(ws);
	if (!tasks.length) return false;
	const priority = PRIORITY_OF[cmd];
	if (cmd === 'task.complete') void completeMany(ws, tasks);
	else if (cmd === 'task.my-day') void myDayMany(ws, tasks);
	else if (cmd === 'task.delete') void deleteMany(ws, tasks);
	else if (priority) void priorityMany(ws, tasks, priority);
	else return false;
	return true;
}

/** Each list the bar and a selected task's menu offer, by its id. */
export type ListId = 'schedule' | 'priority' | 'status' | 'labels' | 'assign' | 'move' | 'more';

/** One list of choices, made from the selection as it is now. */
export function listOf(ws: string, which: ListId): MenuItem[] {
	const tasks = selectedTasks(ws);
	const replica = engine.replica(ws);
	if (!tasks.length || !replica) return [];
	switch (which) {
		case 'schedule': {
			const today = localToday();
			const choices = scheduleChoices(today);
			return [
				...choices.map((c) => ({
					id: c.id,
					label: c.hint ? `${c.label}, ${c.hint}` : c.label,
					checked: tasks.every((t) => (t.dueDate ?? null) === c.date) ? true : undefined,
					run: () => scheduleMany(ws, tasks, c.date, c.label.toLowerCase())
				})),
				{ id: 'pick', label: 'Pick a date', run: () => (selection.askDate = true) },
				sep,
				{ id: 'day-later', label: 'A day later', run: () => shiftMany(ws, tasks, 1) },
				{ id: 'week-later', label: 'A week later', run: () => shiftMany(ws, tasks, 7) }
			].filter((i) => i === sep || i.id !== 'none' || tasks.some((t) => t.dueDate));
		}
		case 'priority':
			return PRIORITIES.map(([p, label, command]) => ({
				id: p,
				label,
				command,
				checked: tasks.every((t) => t.priority === p),
				run: () => priorityMany(ws, tasks, p)
			}));
		case 'status': {
			const projects = new Set(tasks.map((t) => t.projectId));
			const [only] = projects;
			if (projects.size === 1 && only)
				return replica.statuses(only).map((s) => ({
					id: s.id,
					label: s.name,
					checked: tasks.every((t) => t.statusId === s.id),
					run: () => statusMany(ws, tasks, s.id, s.name)
				}));
			return KINDS.map(([kind, label]) => ({
				id: kind,
				label,
				checked: tasks.every((t) => replica.status(t.statusId)?.category === kind),
				run: () => kindMany(ws, tasks, kind, label)
			}));
		}
		case 'labels':
			return replica.labels().map((l) => {
				const on = tri(tasks, (t) => replica.taskLabels(t.id).some((x) => x.id === l.id));
				return {
					id: l.id,
					label: l.name,
					checked: on,
					keep: true,
					run: () =>
						change(
							ws,
							on === true
								? `Took ${l.name} off ${countOf(tasks)}`
								: `Put ${l.name} on ${countOf(tasks)}`,
							() => each(tasks, (t) => setTaskLabel(ws, t.id, l.id, on !== true))
						)
				};
			});
		case 'assign':
			return reachableByAll(ws, tasks).map((p) => {
				const on = tri(tasks, (t) => replica.assignees(t.id).includes(p.userId));
				const name = p.userId === me() ? 'you' : p.name;
				return {
					id: p.userId,
					label: p.userId === me() ? `${p.name} (you)` : p.name,
					checked: on,
					keep: true,
					run: () =>
						change(
							ws,
							on === true
								? `Took ${name} off ${countOf(tasks)}`
								: `Assigned ${countOf(tasks)} to ${name}`,
							() => each(tasks, (t) => setAssignee(ws, t.id, p.userId, on !== true))
						)
				};
			});
		case 'move':
			return [
				{
					id: 'inbox',
					label: 'Inbox',
					checked: tasks.every((t) => t.projectId === null),
					run: () => moveMany(ws, tasks, null, 'the Inbox')
				},
				...replica.projects().map((p) => ({
					id: p.id,
					label: p.name,
					checked: tasks.every((t) => t.projectId === p.id),
					run: () => moveMany(ws, tasks, p.id, p.name)
				}))
			];
		case 'more': {
			const today = localToday();
			const inDay = tasks.every((t) => replica.inMyDay(me(), t.id, today));
			return [
				{
					id: 'my-day',
					label: inDay ? 'Take out of My Day' : 'Add to My Day',
					command: 'task.my-day',
					run: () => myDayMany(ws, tasks)
				},
				{ id: 'links', label: 'Copy links', run: () => copyLinks(tasks) },
				sep,
				{
					id: 'delete',
					label: 'Delete',
					command: 'task.delete',
					danger: true,
					run: () => deleteMany(ws, tasks)
				}
			];
		}
	}
}

/** The people who can reach every task: whom they can all be assigned to. */
function reachableByAll(ws: string, tasks: Task[]) {
	const projects = [...new Set(tasks.map((t) => t.projectId))];
	const [first, ...rest] = projects.map((p) => session.reachable(ws, p));
	return (first ?? []).filter((p) => rest.every((r) => r.some((x) => x.userId === p.userId)));
}

/** Which lists may be offered for this selection. */
export function offered(ws: string, tasks = selectedTasks(ws)): Record<ListId, boolean> {
	const replica = engine.replica(ws);
	const ids = new Set(tasks.map((t) => t.id));
	return {
		schedule: true,
		priority: true,
		status: tasks.some((t) => t.projectId),
		labels: !!replica?.labels().length,
		assign: reachableByAll(ws, tasks).length > 0,
		// As for one task: organizers, and subtasks only with their parents.
		move: may(canOrganize) && tasks.every((t) => !t.parentId || ids.has(t.parentId)),
		more: true
	};
}

const LIST_NAMES: Record<ListId, string> = {
	schedule: 'Schedule',
	priority: 'Priority',
	status: 'Status',
	labels: 'Labels',
	assign: 'Assign',
	move: 'Move to',
	more: 'More'
};

/** One of the bar's lists, as a menu of its own. */
export function listMenu(ws: string, which: ListId): MenuSpec | null {
	const items = listOf(ws, which);
	const n = selectedTasks(ws).length;
	if (!items.length || !n) return null;
	return {
		name: `${LIST_NAMES[which]}, ${n} tasks`,
		kind: 'actions',
		items,
		refresh: () => listOf(ws, which)
	};
}

/** The menu of a selected task: what the bar does, for all of them. */
export function manyMenu(ws: string): MenuSpec | null {
	const tasks = selectedTasks(ws);
	if (!tasks.length) return null;
	const can = offered(ws);
	const reopen = tasks.every((t) => t.completedAt !== null);
	const sub = (which: ListId) => ({
		id: which,
		label: LIST_NAMES[which],
		sub: () => listOf(ws, which)
	});
	const items: MenuItem[] = [
		{
			id: 'complete',
			label: reopen ? 'Reopen' : 'Complete',
			command: 'task.complete',
			run: () => completeMany(ws, tasks)
		},
		sub('schedule'),
		...(['priority', 'status', 'labels', 'assign', 'move'] as const).filter((w) => can[w]).map(sub),
		sep,
		...listOf(ws, 'more'),
		sep,
		{ id: 'clear', label: 'Clear the selection', run: () => clearSelection() }
	];
	return { name: `${tasks.length} tasks`, kind: 'selection actions', items };
}

export function clearSelection() {
	selection.clear();
	announce('Selection cleared');
}
