// Dependencies' rules, pure: when a task can
// start, when it conflicts with what it waits for, and what a search for a
// task to wait for finds. The rows and the server's loop check are in
// sync/data.ts and server/internal/syncer/dependencies.go.
import type { Task } from './sync/entities';

/** The day a task begins: its start date, or its due date when it has none. */
export function beginOf(t: Pick<Task, 'startDate' | 'dueDate'>): string | null {
	return t.startDate ?? t.dueDate;
}

/** The day a task finishes: its due date, or its start date when it has none. */
export function finishOf(t: Pick<Task, 'startDate' | 'dueDate'>): string | null {
	return t.dueDate ?? t.startDate;
}

type Dated = Pick<Task, 'startDate' | 'dueDate' | 'completedAt'>;

/**
 * Whether a waiting task begins on or before the day its blocker
 * finishes, while both are open. Undated tasks never conflict.
 */
export function conflicts(blocker: Dated, waiting: Dated): boolean {
	if (blocker.completedAt || waiting.completedAt) return false;
	const end = finishOf(blocker);
	const start = beginOf(waiting);
	return end !== null && start !== null && start <= end;
}

/** How well a title answers a query: lower is better, null for not at all. */
export function titleRank(title: string, query: string): number | null {
	const t = title.toLowerCase();
	const q = query.trim().toLowerCase();
	if (!q) return 2;
	if (t.startsWith(q)) return 0;
	if (t.split(/\s+/).some((w) => w.startsWith(q))) return 1;
	return t.includes(q) ? 2 : null;
}

/**
 * The tasks a search for one to wait for finds, best first: open tasks
 * before done ones, at most `limit`, leaving out those in `except`.
 */
export function blockerChoices(
	query: string,
	tasks: Task[],
	except: Set<string>,
	limit = 8
): Task[] {
	return tasks
		.filter((t) => !except.has(t.id))
		.map((t) => ({ t, r: titleRank(t.title, query) }))
		.filter((x) => x.r !== null)
		.sort(
			(a, b) =>
				Number(Boolean(a.t.completedAt)) - Number(Boolean(b.t.completedAt)) ||
				a.r! - b.r! ||
				a.t.title.localeCompare(b.t.title)
		)
		.slice(0, limit)
		.map((x) => x.t);
}
