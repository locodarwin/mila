// The tasks a saved view shows, in its order, for its page (routes/(app)/views/[id])
// and a view in a wiki page or task's notes (components/doc/ViewEmbed.svelte), so
// the two never disagree.
import { matches, type FilterContext } from './filter';
import { session } from './session.svelte';
import { localToday } from './sync/data';
import type { FieldType, SavedView, Status, Task } from './sync/entities';
import type { Replica } from './sync/replica.svelte';
import { resolveDisplay, sortTasks, type DisplayLookups } from './viewDisplay';

/** Each status by its ID, across the workspace's projects. */
export function statusesById(replica: Replica): Map<string, Status> {
	const out = new Map<string, Status>();
	for (const p of replica.projects()) for (const s of replica.statuses(p.id)) out.set(s.id, s);
	return out;
}

/** What a view's filter needs to know, for the person looking. */
export function filterContextOf(
	replica: Replica,
	me: string,
	statusById: Map<string, Status> = statusesById(replica),
	values = replica.fieldValuesByField()
): FilterContext {
	const types = new Map<string, FieldType>(replica.fields().map((f) => [f.id, f.type]));
	const today = localToday();
	let myDay: Set<string> | undefined;
	return {
		me,
		today,
		category: (t) =>
			(t.statusId && statusById.get(t.statusId)?.category) || (t.completedAt ? 'done' : 'todo'),
		labels: (id) => replica.taskLabels(id).map((l) => l.id),
		assignees: (id) => replica.assignees(id),
		blocked: (id) => replica.isBlocked(id),
		blocking: (id) => replica.isBlocking(id),
		entries: (id) => replica.timeEntries(id),
		inMyDay: (id) => (myDay ??= new Set(replica.myDay(me, today).map((e) => e.task.id))).has(id),
		fieldType: (id) => types.get(id),
		value: (task, field) => values.get(field)?.get(task)
	};
}

/** The tasks a view's filter leaves: in its project, or across the live ones and the inbox. */
export function tasksOfView(replica: Replica, view: SavedView, ctx: FilterContext): Task[] {
	const live = new Set(replica.projects().map((p) => p.id));
	const scope = view.projectId
		? replica.tasks(view.projectId)
		: replica.allTasks().filter((t) => !t.parentId && (!t.projectId || live.has(t.projectId)));
	return scope.filter((t) => matches(t, view.filter, ctx));
}

/** A view's tasks in its own order, as its page lists them. */
export function viewList(ws: string, replica: Replica, view: SavedView, me: string): Task[] {
	const statusById = statusesById(replica);
	const values = replica.fieldValuesByField();
	const ctx = filterContextOf(replica, me, statusById, values);
	const look: DisplayLookups = {
		projectId: view.projectId,
		fields: view.projectId ? replica.fields(view.projectId) : replica.fields()
	};
	const resolved = resolveDisplay(view.display, view.type, look);
	return sortTasks(tasksOfView(replica, view, ctx), resolved.sort, {
		category: ctx.category,
		status: (t) => (t.statusId ? statusById.get(t.statusId) : undefined),
		projects: replica.projects(),
		times: (id) => replica.taskTimes(id),
		fields: look.fields,
		values,
		memberName: (id) => session.memberName(ws, id),
		logged: (id) => replica.logged(id)
	});
}
