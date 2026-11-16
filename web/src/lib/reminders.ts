// Reminders: when a task falls due, the presets offered for a reminder,
// and how a reminder reads ("Tomorrow at 9:00", "1 hour before due").
// A reminder made relative to the due moment keeps its offset in minutes;
// the server fires it at the due moment less the offset in the person's
// timezone, so it follows the task when anyone reschedules it, and this
// device keeps remind_at in step for its own edits.

import { ALL_DAY_AT, formatClock, formatDate, formatTime } from './dates';
import type { Reminder, Task } from './sync/entities';

export { ALL_DAY_AT };

/** ALL_DAY_AT, the user's way. */
const nine = () => formatTime(ALL_DAY_AT);

/** Minutes in a day. */
export const DAY = 24 * 60;

/** The longest a reminder can come before the due moment: a year, as the server allows. */
export const MAX_OFFSET = 366 * DAY;

/** A local date and time ("2026-10-28", "14:30") as an instant. */
export function localMoment(date: string, time: string): Date {
	const [y, m, d] = date.split('-').map(Number);
	const [h, min] = time.split(':').map(Number);
	return new Date(y, m - 1, d, h, min);
}

/** When a task falls due here: its due time, or 09:00 for an all-day task. */
export function dueMoment(task: Pick<Task, 'dueDate' | 'dueTime'>): Date | null {
	if (!task.dueDate) return null;
	return localMoment(task.dueDate, task.dueTime ?? ALL_DAY_AT);
}

/**
 * The instant a relative reminder works out to, as stored in remind_at;
 * null for a task with no due date. Whole days of the offset count as
 * calendar days, so "the day before, 9:00" stays at 9:00 across a change
 * to or from daylight saving time, as the server counts it.
 */
export function remindAtFor(
	task: Pick<Task, 'dueDate' | 'dueTime'>,
	offsetMinutes: number
): string | null {
	const due = dueMoment(task);
	if (!due) return null;
	const days = Math.trunc(offsetMinutes / DAY);
	due.setDate(due.getDate() - days);
	return new Date(due.getTime() - (offsetMinutes - days * DAY) * 60_000).toISOString();
}

/** Minutes since midnight of a time of day ("14:30"). */
function minutesOf(time: string): number {
	const [h, m] = time.split(':').map(Number);
	return h * 60 + m;
}

/** The offsets the presets for a timed task offer by name. */
const TIMED_OFFSETS = [0, 15, 60, DAY];

/**
 * The offset of "the morning of the due date, 9:00" on a timed task: the
 * minutes from 9:00 to its due time, or null if it is due by then.
 */
function morningOffset(dueTime: string): number | null {
	const offset = minutesOf(dueTime) - minutesOf(ALL_DAY_AT);
	return offset > 0 ? offset : null;
}

/** When a reminder fires: the due moment less its offset, or its fixed time. */
export function firesAt(reminder: Reminder, task: Pick<Task, 'dueDate' | 'dueTime'>): Date {
	if (reminder.offsetMinutes !== null) {
		const at = remindAtFor(task, reminder.offsetMinutes);
		if (at) return new Date(at);
	}
	return new Date(reminder.remindAt);
}

export interface Preset {
	id: string;
	label: string;
	/** Minutes before the due moment, for a relative reminder. */
	offset?: number;
	/** A fixed instant, as ISO. */
	at?: string;
}

function iso(d: Date): string {
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function addDays(d: Date, n: number): Date {
	return new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);
}

/**
 * The reminders offered for a task. A timed task gets the due time and
 * times before it, plus the morning of its due date if that is earlier and
 * not one of those; an all-day task gets the morning of the due date and
 * the morning before. All of these are relative to the due moment, so they
 * follow the task when it moves or repeats. Every task gets tomorrow
 * morning. Times already past are left out.
 */
export function presets(task: Pick<Task, 'dueDate' | 'dueTime'>, now = new Date()): Preset[] {
	const out: Preset[] = [];
	const due = dueMoment(task);
	const future = (p: Preset) => {
		const at = p.at ?? (p.offset !== undefined ? remindAtFor(task, p.offset) : null);
		if (at && new Date(at) > now) out.push(p);
	};
	if (due && task.dueTime) {
		future({ id: 'due', label: 'At the due time', offset: 0 });
		future({ id: '15m', label: '15 minutes before', offset: 15 });
		future({ id: '1h', label: '1 hour before', offset: 60 });
		future({ id: '1d', label: '1 day before', offset: DAY });
		const morning = morningOffset(task.dueTime);
		if (morning !== null && !TIMED_OFFSETS.includes(morning))
			future({ id: 'morning', label: `The morning of the due date, ${nine()}`, offset: morning });
	} else if (due) {
		future({ id: 'morning', label: `The morning of the due date, ${nine()}`, offset: 0 });
		future({ id: '1d', label: `The day before, ${nine()}`, offset: DAY });
	}
	future({
		id: 'tomorrow',
		label: `Tomorrow, ${nine()}`,
		at: localMoment(iso(addDays(now, 1)), ALL_DAY_AT).toISOString()
	});
	return out;
}

/** A moment's clock time, the user's way: "9:00 AM", "14:30". */
export function clockOf(d: Date): string {
	return formatClock(d);
}

/** A day near now in words: "Today", "Tomorrow", "Yesterday", "Mon, Nov 2". */
export function dayName(d: Date, now = new Date()): string {
	const day = iso(d);
	if (day === iso(now)) return 'Today';
	if (day === iso(addDays(now, 1))) return 'Tomorrow';
	if (day === iso(addDays(now, -1))) return 'Yesterday';
	return formatDate(d, { weekday: 'short' }, undefined, now);
}

/** A length of time in words: "15 minutes", "1 hour", "2 days 3 hours". */
export function duration(minutes: number): string {
	const parts: string[] = [];
	const unit = (n: number, one: string) => `${n} ${one}${n === 1 ? '' : 's'}`;
	const days = Math.floor(minutes / DAY);
	const hours = Math.floor((minutes % DAY) / 60);
	const mins = minutes % 60;
	if (days) parts.push(unit(days, 'day'));
	if (hours) parts.push(unit(hours, 'hour'));
	if (mins) parts.push(unit(mins, 'minute'));
	return parts.join(' ');
}

/**
 * How a reminder reads in the task panel: "1 hour before due", "At the due
 * time", "The day before at 9:00", "Tomorrow at 9:00".
 */
export function describeReminder(
	reminder: Reminder,
	task: Pick<Task, 'dueDate' | 'dueTime'>,
	now = new Date()
): string {
	const offset = reminder.offsetMinutes;
	if (offset !== null && task.dueDate) {
		if (task.dueTime) {
			if (offset === 0) return 'At the due time';
			if (offset === morningOffset(task.dueTime) && !TIMED_OFFSETS.includes(offset))
				return `The morning of the due date, ${nine()}`;
			return `${duration(offset)} before due`;
		}
		if (offset === 0) return `The morning of the due date, ${nine()}`;
		const at = firesAt(reminder, task);
		if (offset % DAY === 0)
			return offset === DAY
				? `The day before at ${clockOf(at)}`
				: `${offset / DAY} days before at ${clockOf(at)}`;
		return `${dayName(at, now)} at ${clockOf(at)} (${duration(offset)} before ${nine()} on the due date)`;
	}
	const at = new Date(reminder.remindAt);
	return `${dayName(at, now)} at ${clockOf(at)}`;
}

/** The name of the bell a row or card shows for a task with reminders. */
export function bellLabel(count: number): string {
	return count === 1 ? 'Has a reminder' : `Has ${count} reminders`;
}
