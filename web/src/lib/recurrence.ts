// Repeat rules: a small subset of RFC 5545 RRULE, the same one the server
// accepts (server/internal/recur). FREQ is DAILY, WEEKLY, MONTHLY or
// YEARLY; INTERVAL; BYDAY lists weekdays for WEEKLY, or names one ordinal
// weekday (1MO, -1FR) for MONTHLY; BYMONTHDAY is a day of the month for
// MONTHLY (-1 is the last); UNTIL is the last date it may fall on.
//
// A repeating task comes back only when it is completed: the next task is
// made then, with an ID derived from the completed task and its date, so
// devices that complete it offline make the same one. shared/recurrence-vectors.json
// holds this file and the server to the same answers.
import { formatDate } from './dates';
import { hashId } from './hashid';

export { hashId, sha256 } from './hashid';

export type Freq = 'DAILY' | 'WEEKLY' | 'MONTHLY' | 'YEARLY';
export type RepeatMode = 'schedule' | 'completion';

/** 0 is Monday, 6 is Sunday. */
export type Weekday = 0 | 1 | 2 | 3 | 4 | 5 | 6;

export interface Rule {
	freq: Freq;
	interval: number;
	/** WEEKLY: the days of the week, in order from Monday. */
	weekdays: Weekday[];
	/** MONTHLY: the nth weekday of the month (1 to 5, or -1 to -5 from the end). */
	ordinal?: { n: number; day: Weekday };
	/** MONTHLY: a day of the month, 1 to 31, or -1 for the last day. */
	monthDay?: number;
	/** The last date it may fall on, 2026-12-31. */
	until?: string;
}

export const MAX_INTERVAL = 999;

const CODES = ['MO', 'TU', 'WE', 'TH', 'FR', 'SA', 'SU'];
const FREQS: Freq[] = ['DAILY', 'WEEKLY', 'MONTHLY', 'YEARLY'];

/** Reads a rule; throws an Error saying what is wrong with it. */
export function parseRule(text: string): Rule {
	if (!text) throw new Error('The rule is empty.');
	const rule: Partial<Rule> & { weekdays: Weekday[] } = { interval: 1, weekdays: [] };
	const seen = new Set<string>();
	for (const part of text.split(';')) {
		const eq = part.indexOf('=');
		const key = eq < 0 ? part : part.slice(0, eq);
		const val = eq < 0 ? '' : part.slice(eq + 1);
		if (eq < 0 || !val) throw new Error(`"${part}" is not KEY=VALUE.`);
		if (seen.has(key)) throw new Error(`${key} is given twice.`);
		seen.add(key);
		switch (key) {
			case 'FREQ':
				if (!FREQS.includes(val as Freq))
					throw new Error(`FREQ "${val}" is not DAILY, WEEKLY, MONTHLY or YEARLY.`);
				rule.freq = val as Freq;
				break;
			case 'INTERVAL': {
				const n = Number(val);
				if (!/^[0-9]{1,3}$/.test(val) || n < 1 || n > MAX_INTERVAL)
					throw new Error(`INTERVAL must be a whole number from 1 to ${MAX_INTERVAL}.`);
				rule.interval = n;
				break;
			}
			case 'BYDAY': {
				const m = /^(-?[1-5])(MO|TU|WE|TH|FR|SA|SU)$/.exec(val);
				if (m) {
					rule.ordinal = { n: Number(m[1]), day: CODES.indexOf(m[2]) as Weekday };
					break;
				}
				for (const code of val.split(',')) {
					const d = CODES.indexOf(code);
					if (d < 0) throw new Error(`BYDAY "${code}" is not a weekday like MO.`);
					if (rule.weekdays.includes(d as Weekday)) throw new Error(`BYDAY names ${code} twice.`);
					rule.weekdays.push(d as Weekday);
				}
				rule.weekdays.sort((a, b) => a - b);
				break;
			}
			case 'BYMONTHDAY': {
				const n = Number(val);
				if (!/^-?[0-9]{1,3}$/.test(val) || n === 0 || n < -1 || n > 31)
					throw new Error('BYMONTHDAY must be a day from 1 to 31, or -1 for the last day.');
				rule.monthDay = n;
				break;
			}
			case 'UNTIL': {
				const m = /^(\d{4})(\d{2})(\d{2})$/.exec(val);
				const iso = m ? `${m[1]}-${m[2]}-${m[3]}` : '';
				if (!m || !validDate(iso)) throw new Error('UNTIL must be a date like 20261231.');
				rule.until = iso;
				break;
			}
			default:
				throw new Error(`${key} is not supported.`);
		}
	}
	if (!rule.freq) throw new Error('FREQ is missing.');
	if (rule.weekdays.length && rule.freq !== 'WEEKLY')
		throw new Error('A list of weekdays needs FREQ=WEEKLY.');
	if (rule.ordinal && rule.freq !== 'MONTHLY')
		throw new Error('An ordinal weekday like 1MO needs FREQ=MONTHLY.');
	if (rule.monthDay !== undefined && rule.freq !== 'MONTHLY')
		throw new Error('BYMONTHDAY needs FREQ=MONTHLY.');
	if (rule.monthDay !== undefined && rule.ordinal)
		throw new Error('BYDAY and BYMONTHDAY cannot be combined.');
	return rule as Rule;
}

/** The rule, or null if it is not one Mila understands. */
export function tryParseRule(text: string | null | undefined): Rule | null {
	if (!text) return null;
	try {
		return parseRule(text);
	} catch {
		return null;
	}
}

/** Writes a rule in the one form Mila stores. */
export function formatRule(rule: Rule): string {
	const parts = [`FREQ=${rule.freq}`];
	if (rule.interval > 1) parts.push(`INTERVAL=${rule.interval}`);
	if (rule.freq === 'WEEKLY' && rule.weekdays.length)
		parts.push(
			`BYDAY=${[...rule.weekdays]
				.sort((a, b) => a - b)
				.map((d) => CODES[d])
				.join(',')}`
		);
	if (rule.freq === 'MONTHLY' && rule.ordinal)
		parts.push(`BYDAY=${rule.ordinal.n}${CODES[rule.ordinal.day]}`);
	else if (rule.freq === 'MONTHLY' && rule.monthDay !== undefined)
		parts.push(`BYMONTHDAY=${rule.monthDay}`);
	if (rule.until) parts.push(`UNTIL=${rule.until.replaceAll('-', '')}`);
	return parts.join(';');
}

// Dates are YYYY-MM-DD strings, worked on as days since 1970 in UTC, so no
// daylight saving change can move them.

function validDate(iso: string): boolean {
	const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso);
	if (!m) return false;
	const d = new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3])));
	return d.getUTCMonth() === Number(m[2]) - 1 && d.getUTCDate() === Number(m[3]);
}

function toDays(iso: string): number {
	return (
		Date.UTC(Number(iso.slice(0, 4)), Number(iso.slice(5, 7)) - 1, Number(iso.slice(8, 10))) / 864e5
	);
}

function fromDays(days: number): string {
	return new Date(days * 864e5).toISOString().slice(0, 10);
}

function ymd(y: number, m: number, d: number): number {
	return Date.UTC(y, m, d) / 864e5;
}

function daysIn(y: number, m: number): number {
	return new Date(Date.UTC(y, m + 1, 0)).getUTCDate();
}

/** The weekday of a day number, 0 for Monday. */
function weekdayOf(days: number): Weekday {
	return ((((days + 3) % 7) + 7) % 7) as Weekday; // 1970-01-01 was a Thursday
}

/** The weekday of a date, 0 for Monday. */
export function weekdayOfDate(iso: string): Weekday {
	return weekdayOf(toDays(iso));
}

export function addDaysTo(iso: string, n: number): string {
	return fromDays(toDays(iso) + n);
}

/** Months or years tried before deciding a rule never fires again. */
const PERIODS = 200;

/**
 * The rule's first date after both `after` and the anchor (the date the
 * series counts from, like an RRULE's DTSTART), ignoring UNTIL; null if
 * there is none.
 */
function afterDay(rule: Rule, anchor: number, after: number): number | null {
	after = Math.max(after, anchor);
	const n = rule.interval;
	switch (rule.freq) {
		case 'DAILY':
			return anchor + (Math.floor((after - anchor) / n) + 1) * n;
		case 'WEEKLY': {
			const days = rule.weekdays.length ? rule.weekdays : [weekdayOf(anchor)];
			const start = anchor - weekdayOf(anchor);
			for (let d = after + 1; d - after <= 14 * n; d++) {
				const week = (d - weekdayOf(d) - start) / 7;
				if (week % n === 0 && days.includes(weekdayOf(d))) return d;
			}
			return null;
		}
		case 'MONTHLY': {
			const a = new Date(anchor * 864e5);
			const x = new Date(after * 864e5);
			const first = a.getUTCFullYear() * 12 + a.getUTCMonth();
			let cur = x.getUTCFullYear() * 12 + x.getUTCMonth();
			const off = (cur - first) % n;
			if (off) cur += n - off;
			for (let i = 0; i < PERIODS; i++, cur += n) {
				const d = inMonth(rule, a.getUTCDate(), Math.floor(cur / 12), cur % 12);
				if (d !== null && d > after) return d;
			}
			return null;
		}
		case 'YEARLY': {
			const a = new Date(anchor * 864e5);
			let y = new Date(after * 864e5).getUTCFullYear();
			const off = (y - a.getUTCFullYear()) % n;
			if (off) y += n - off;
			for (let i = 0; i < PERIODS; i++, y += n) {
				if (a.getUTCDate() > daysIn(y, a.getUTCMonth())) continue;
				const d = ymd(y, a.getUTCMonth(), a.getUTCDate());
				if (d > after) return d;
			}
			return null;
		}
	}
}

/** The day a monthly rule falls on in a month; a month too short is skipped. */
function inMonth(rule: Rule, anchorDay: number, y: number, m: number): number | null {
	const len = daysIn(y, m);
	if (rule.monthDay === -1) return ymd(y, m, len);
	if (rule.ordinal) {
		const { n, day } = rule.ordinal;
		if (n > 0) {
			const first = (day - weekdayOf(ymd(y, m, 1)) + 7) % 7;
			const d = 1 + first + (n - 1) * 7;
			return d <= len ? ymd(y, m, d) : null;
		}
		const last = (weekdayOf(ymd(y, m, len)) - day + 7) % 7;
		const d = len - last + (n + 1) * 7;
		return d >= 1 ? ymd(y, m, d) : null;
	}
	const d = rule.monthDay ?? anchorDay;
	return d <= len ? ymd(y, m, d) : null;
}

/** A date moved on by one interval; a month too short ends on its last day. */
function addInterval(rule: Rule, iso: string): string {
	const n = rule.interval;
	if (rule.freq === 'DAILY') return addDaysTo(iso, n);
	if (rule.freq === 'WEEKLY') return addDaysTo(iso, 7 * n);
	const total =
		Number(iso.slice(0, 4)) * 12 +
		Number(iso.slice(5, 7)) -
		1 +
		(rule.freq === 'YEARLY' ? 12 * n : n);
	const y = Math.floor(total / 12);
	const m = total % 12;
	return fromDays(ymd(y, m, Math.min(Number(iso.slice(8, 10)), daysIn(y, m))));
}

/**
 * The due date of the task that follows one completed on `today`, or null
 * when the series has ended. In schedule mode it is the rule's next date
 * after the due date (the completion date if there is none), skipping any
 * before today so finishing late leaves no pile of overdue copies. In
 * completion mode it is the completion date plus the interval.
 */
export function nextDue(
	rule: Rule,
	mode: RepeatMode,
	due: string | null,
	today: string
): string | null {
	let next: string;
	if (mode === 'completion') {
		next = addInterval(rule, today);
	} else {
		const base = toDays(due ?? today);
		const d = afterDay(rule, base, Math.max(base, toDays(today) - 1));
		if (d === null) return null;
		next = fromDays(d);
	}
	return rule.until && next > rule.until ? null : next;
}

/**
 * The dates a schedule-mode series falls on after `due`, from `from` to
 * `to` inclusive, at most `limit` of them: the calendar's ghost entries.
 */
export function upcoming(rule: Rule, due: string, from: string, to: string, limit = 62): string[] {
	const out: string[] = [];
	const anchor = toDays(due);
	let d: number | null = Math.max(anchor, toDays(from) - 1);
	const end = toDays(to);
	while (out.length < limit) {
		d = afterDay(rule, anchor, d);
		if (d === null || d > end) break;
		const iso = fromDays(d);
		if (rule.until && iso > rule.until) break;
		out.push(iso);
	}
	return out;
}

// Describing a rule.

const DAY_NAMES = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
const LONG_DAYS = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'];
const ORDINALS = ['first', 'second', 'third', 'fourth', 'fifth'];
const UNITS: Record<Freq, string> = {
	DAILY: 'day',
	WEEKLY: 'week',
	MONTHLY: 'month',
	YEARLY: 'year'
};

function list(items: string[]): string {
	return new Intl.ListFormat('en-US', { type: 'conjunction' }).format(items);
}

function ordinalWord(n: number): string {
	if (n === -1) return 'last';
	if (n < 0) return `${ORDINALS[-n - 1]} to last`;
	return ORDINALS[n - 1];
}

function suffix(n: number): string {
	const s = n % 100 >= 11 && n % 100 <= 13 ? 'th' : (['th', 'st', 'nd', 'rd'][n % 10] ?? 'th');
	return `${n}${s}`;
}

function shortDate(iso: string, withYear = false): string {
	return formatDate(iso, { year: withYear });
}

/**
 * The rule in words: "Every 2 weeks on Mon and Thu", "Monthly on the last
 * Friday", "3 days after completion". The anchor (the task's due date)
 * fills in what the rule leaves to it.
 */
export function describe(rule: Rule, mode: RepeatMode, anchor?: string | null): string {
	const n = rule.interval;
	const unit = UNITS[rule.freq];
	let text: string;
	if (mode === 'completion') {
		text = `${n} ${unit}${n === 1 ? '' : 's'} after completion`;
	} else {
		const every = n === 1 ? '' : `Every ${n} ${unit}s`;
		switch (rule.freq) {
			case 'DAILY':
				text = n === 1 ? 'Every day' : every;
				break;
			case 'WEEKLY': {
				const days = rule.weekdays.length ? rule.weekdays : anchor ? [weekdayOfDate(anchor)] : [];
				if (n === 1 && days.join() === '0,1,2,3,4') text = 'Every weekday';
				else {
					const on = days.length ? ` on ${list(days.map((d) => DAY_NAMES[d]))}` : '';
					text = (every || 'Weekly') + on;
				}
				break;
			}
			case 'MONTHLY': {
				let on = '';
				if (rule.ordinal)
					on = ` on the ${ordinalWord(rule.ordinal.n)} ${LONG_DAYS[rule.ordinal.day]}`;
				else if (rule.monthDay === -1) on = ' on the last day';
				else if (rule.monthDay) on = ` on the ${suffix(rule.monthDay)}`;
				else if (anchor) on = ` on the ${suffix(Number(anchor.slice(8, 10)))}`;
				text = (every || 'Monthly') + on;
				break;
			}
			case 'YEARLY':
				text = (every || 'Yearly') + (anchor ? ` on ${shortDate(anchor)}` : '');
				break;
		}
	}
	if (rule.until) text += `, until ${shortDate(rule.until, true)}`;
	return text;
}

// Deterministic IDs.

/**
 * The ID of the task that follows a completed one, the same on every
 * device. It comes from the task completed alone, not the next date, so
 * every completion of one task (on any device, day or timezone) lands on
 * one row. It takes the task rather than the series, so a copy can never
 * get the ID of the task before it.
 */
export function nextTaskId(afterId: string): string {
	return hashId(`${afterId}/next`);
}

/**
 * The first date a newly repeating task is due, from today: today itself
 * for a rule that counts from its start, else the first day from today on
 * that the rule names (the next Monday for "every Monday"). The interval
 * does not push it back: the series counts from that date.
 */
export function firstDue(rule: Rule, today: string): string {
	const fixed = rule.weekdays.length > 0 || rule.ordinal || rule.monthDay !== undefined;
	if (!fixed) return today;
	const d = afterDay({ ...rule, interval: 1 }, toDays(today) - 1, toDays(today) - 1);
	return d === null ? today : fromDays(d);
}

/**
 * A rule described to follow other words: "weekly on Mon and Thu", with
 * only the first letter lowered, so day and month names keep theirs.
 */
export function describeAfter(rule: Rule, mode: RepeatMode, anchor: string | null = null): string {
	const text = describe(rule, mode, anchor);
	return `${text[0].toLowerCase()}${text.slice(1)}`;
}

/** "Repeats every day": a task's rule as the name of its repeat icon; null if none. */
export function repeatLabel(
	rule: string | null,
	mode: RepeatMode,
	anchor: string | null
): string | null {
	const parsed = tryParseRule(rule);
	return parsed ? `Repeats ${describeAfter(parsed, mode, anchor)}` : null;
}
