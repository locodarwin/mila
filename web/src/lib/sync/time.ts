// Time: estimates, time logged and timers.
// Apart from data.ts so a page's first load does not carry them; the task
// panel, the timer and the palette load them when they are used.
import { MAX_ENTRY, minutesBetween } from '../duration';
import { session } from '../session.svelte';
import { uuid } from '../uuid';
import type { TimeEntry, Values } from './entities';
import { engine } from './engine.svelte';
import { localDay } from './data';

const now = () => new Date().toISOString();
const me = () => session.user?.id ?? null;

/** Which rows an entry is: time on a task, or on a project itself. */
export type TimeKind = 'time_entry' | 'project_time_entry';
export const kindOf = (e: Pick<TimeEntry, 'taskId'>): TimeKind =>
	e.taskId === null ? 'project_time_entry' : 'time_entry';

/** Sets a task's own estimate in minutes, or clears it with null. */
export function setEstimate(ws: string, taskId: string, minutes: number | null) {
	return engine.mutate(ws, 'task', taskId, { estimate_minutes: minutes });
}

/** Logs time typed in on a task, as the person on this device. */
export function logTime(
	ws: string,
	taskId: string,
	entry: { day: string; minutes: number; note?: string }
): Promise<string> {
	return logTo(ws, 'time_entry', { task_id: taskId }, entry);
}

/** Logs time typed in to a project itself, not to one of its tasks. */
export function logProjectTime(
	ws: string,
	projectId: string,
	entry: { day: string; minutes: number; note?: string }
): Promise<string> {
	return logTo(ws, 'project_time_entry', { project_id: projectId }, entry);
}

function logTo(
	ws: string,
	kind: TimeKind,
	on: Values,
	entry: { day: string; minutes: number; note?: string }
): Promise<string> {
	const id = uuid();
	return engine
		.mutate(ws, kind, id, {
			...on,
			user_id: me(),
			day: entry.day,
			minutes: Math.min(MAX_ENTRY, Math.max(0, Math.round(entry.minutes))),
			note: (entry.note ?? '').trim()
		})
		.then(() => id);
}

/** Changes an entry's day, length or note: its person's, or an admin's edit. */
export function editTime(
	ws: string,
	id: string,
	fields: { day?: string; minutes?: number; note?: string },
	kind: TimeKind = 'time_entry'
) {
	const values: Values = { ...fields };
	if (fields.minutes !== undefined)
		values.minutes = Math.min(MAX_ENTRY, Math.max(0, Math.round(fields.minutes)));
	if (fields.note !== undefined) values.note = fields.note.trim();
	return engine.mutate(ws, kind, id, values);
}

export function deleteTime(ws: string, id: string, kind: TimeKind = 'time_entry') {
	return engine.mutate(ws, kind, id, { deleted_at: now() });
}

/**
 * Stops a running timer at a moment (now by default), logging the whole
 * minutes since it started, a day at most, on the day it started. A timer
 * stopped before a minute is dropped. Resolves to the minutes kept.
 */
export async function stopTimer(
	ws: string,
	entry: TimeEntry,
	at: Date = new Date()
): Promise<number> {
	const minutes = Math.min(MAX_ENTRY, minutesBetween(entry.startedAt ?? at, at));
	const kind = kindOf(entry);
	if (minutes < 1) {
		await engine.mutate(ws, kind, entry.id, { running: false, deleted_at: now() });
		return 0;
	}
	await engine.mutate(ws, kind, entry.id, { running: false, minutes, active_at: null });
	return minutes;
}

/**
 * Starts a timer on a task. A person has one running, on a task or a
 * project: any other of theirs, in any workspace on this device, stops
 * first, and one undo takes both back. Resolves to the new entry's ID.
 */
export function startTimer(ws: string, taskId: string): Promise<string> {
	return startOn(ws, 'time_entry', { task_id: taskId });
}

/** Starts a timer on a project itself, as startTimer does on a task. */
export function startProjectTimer(ws: string, projectId: string): Promise<string> {
	return startOn(ws, 'project_time_entry', { project_id: projectId });
}

function startOn(ws: string, kind: TimeKind, on: Values): Promise<string> {
	const user = me();
	const id = uuid();
	return engine.group(async () => {
		for (const [other, replica] of engine.replicas)
			for (const running of user ? replica.runningTimers(user) : [])
				await stopTimer(other, running);
		const at = new Date();
		await engine.mutate(ws, kind, id, {
			...on,
			user_id: user,
			day: localDay(at),
			minutes: 0,
			started_at: at.toISOString(),
			running: true,
			active_at: at.toISOString()
		});
		return id;
	});
}

/** Notes that the person is still about while their timer runs; never undone. */
export function touchTimer(ws: string, entry: Pick<TimeEntry, 'id' | 'taskId'>) {
	return engine.mutate(ws, kindOf(entry), entry.id, { active_at: now() }, { undoable: false });
}
