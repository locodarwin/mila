// Lengths of time for estimates and time logged (docs/PLAN.md section 3):
// read from what people type, and shown as "1h 30m". Pure and small, since
// rows and cards show them.

/** The largest estimate, in minutes: 999 hours (MaxEstimate in syncer). */
export const MAX_ESTIMATE = 999 * 60;
/** The most one entry of time logged holds: a day (MaxEntryMinutes). */
export const MAX_ENTRY = 24 * 60;

/**
 * Minutes from "1h 30m", "90m", "1.5h", "1:30" or a bare number of hours
 * ("2"), or null when the text reads as none of them. Case and spaces do
 * not matter; "1 hr 5 min" reads too.
 */
export function parseDuration(text: string): number | null {
	const t = text.trim().toLowerCase();
	if (!t) return null;
	let m = /^(\d+):([0-5]\d)$/.exec(t);
	if (m) return Number(m[1]) * 60 + Number(m[2]);
	m = /^(\d+(?:\.\d+)?|\.\d+)$/.exec(t);
	if (m) return Math.round(Number(m[1]) * 60);
	m = /^(?:(\d+(?:\.\d+)?|\.\d+)\s*h(?:ours?|rs?)?)?\s*(?:(\d+)\s*m(?:in(?:ute)?s?)?)?$/.exec(t);
	if (m && (m[1] !== undefined || m[2] !== undefined))
		return Math.round(Number(m[1] ?? 0) * 60 + Number(m[2] ?? 0));
	return null;
}

/** Minutes as "1h 30m", "2h" or "45m"; negative ones as their size. */
export function formatDuration(minutes: number): string {
	const total = Math.round(Math.abs(minutes));
	const h = Math.floor(total / 60);
	const m = total % 60;
	if (h && m) return `${h}h ${m}m`;
	return h ? `${h}h` : `${m}m`;
}

/** Minutes as words a screen reader says well: "1 hour 30 minutes". */
export function sayDuration(minutes: number): string {
	const total = Math.round(Math.abs(minutes));
	const h = Math.floor(total / 60);
	const m = total % 60;
	const hours = h === 1 ? '1 hour' : `${h} hours`;
	const mins = m === 1 ? '1 minute' : `${m} minutes`;
	if (h && m) return `${hours} ${mins}`;
	return h ? hours : mins;
}

/** A running timer's time so far, as "0:42:15". */
export function stopwatch(seconds: number): string {
	const s = Math.max(0, Math.floor(seconds));
	const h = Math.floor(s / 3600);
	const m = Math.floor((s % 3600) / 60);
	return `${h}:${String(m).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`;
}

/** Whole minutes between two moments, at least 0. */
export function minutesBetween(from: string | Date, to: string | Date): number {
	return Math.max(0, Math.floor((new Date(to).getTime() - new Date(from).getTime()) / 60_000));
}

/**
 * What is left of an estimate after the time logged: the minutes left, or
 * past it the minutes over. Null with no estimate.
 */
export function remaining(
	estimate: number | null,
	logged: number
): { left: number; over: number } | null {
	if (estimate === null) return null;
	return logged > estimate
		? { left: 0, over: logged - estimate }
		: { left: estimate - logged, over: 0 };
}

/**
 * What a row's or card's time badge says: the time logged against the
 * estimate ("3h 20m / 6h"), either alone, and what is left or past it,
 * as the view shows them. Null when it has nothing to say.
 */
export function timeBadge(
	estimate: number | null,
	logged: number,
	show: { estimate: boolean; logged: boolean; remaining: boolean }
): { text: string; said: string; over: boolean } | null {
	const est = show.estimate && estimate !== null ? estimate : null;
	const log = show.logged && logged > 0 ? logged : null;
	const left = show.remaining ? remaining(estimate, logged) : null;
	const text: string[] = [];
	const said: string[] = [];
	if (log !== null && est !== null) {
		text.push(`${formatDuration(log)} / ${formatDuration(est)}`);
		said.push(`${sayDuration(log)} logged of ${sayDuration(est)} estimated`);
	} else if (est !== null) {
		text.push(formatDuration(est));
		said.push(`estimated ${sayDuration(est)}`);
	} else if (log !== null) {
		text.push(formatDuration(log));
		said.push(`${sayDuration(log)} logged`);
	}
	if (left && (left.over || left.left)) {
		text.push(
			left.over ? `${formatDuration(left.over)} over` : `${formatDuration(left.left)} left`
		);
		said.push(
			left.over ? `${sayDuration(left.over)} over the estimate` : `${sayDuration(left.left)} left`
		);
	}
	if (!text.length) return null;
	const over = estimate !== null && logged > estimate && (est !== null || left !== null);
	return { text: text.join(' · '), said: said.join(', '), over };
}
