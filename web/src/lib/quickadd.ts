// Natural-language quick add: "Call Dana tomorrow 3pm #work !high @shawn +phone"
// becomes a task with a date, time, project, priority, assignee and label.
// What is not recognized stays in the title, so nothing typed is lost.
//
//   #project   a project, by the start of its name (hyphens for spaces)
//   !priority  !urgent !high !medium !low, or !1 (urgent) to !4 (low)
//   @person    a member, by the start of their name
//   +label     a label, by the start of its name
//   dates      today, tomorrow, mon..sun (the next one), next week,
//              in 3 days, in 2 weeks, nov 14, 14 nov, 11/14, 2026-11-14
//   times      3pm, 3:30pm, 15:00, noon, optionally after "at"
//   repeats    every day, every weekday, every week, every 2 weeks, every
//              other month, every monday, every mon and thu, daily, weekly,
//              monthly, yearly; with no date given, it is due on the first one
//   reminders  remind 3pm, remind tomorrow 9am, remind me fri at 8:30am (a
//              time alone is on the due date, else today, or tomorrow once
//              it has passed; a date alone is at 9:00), remind 1h before,
//              remind 30 minutes before, remind 2 days before (before the
//              due time; only for a task with a due date)

import { firstDue, formatRule, type Freq, type Rule, type Weekday } from './recurrence';
import { ALL_DAY_AT, localMoment, MAX_OFFSET } from './reminders';
import type { Priority } from './sync/entities';

export interface Named {
	id: string;
	name: string;
}

export interface QuickAddContext {
	today: Date;
	/** The first day of the user's week, 0 for Sunday; "next week" starts there. Monday if not given. */
	weekStart?: number;
	projects: Named[];
	labels: Named[];
	members: Named[];
}

export type TokenKind =
	'date' | 'time' | 'project' | 'priority' | 'person' | 'label' | 'repeat' | 'reminder';

export interface Token {
	kind: TokenKind;
	start: number;
	end: number;
}

export interface Parsed {
	title: string;
	projectId?: string;
	priority?: Priority;
	dueDate?: string;
	dueTime?: string;
	/** A repeat rule, as stored. */
	recurrence?: string;
	/** A reminder: minutes before the due moment, or a local date and time. */
	reminder?: { offset: number } | { date: string; time: string };
	labelIds: string[];
	assigneeIds: string[];
	tokens: Token[];
}

const WEEKDAYS = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday'];
const MONTHS = [
	'january',
	'february',
	'march',
	'april',
	'may',
	'june',
	'july',
	'august',
	'september',
	'october',
	'november',
	'december'
];
const PRIORITY_WORDS: Record<string, Priority> = {
	urgent: 'urgent',
	high: 'high',
	medium: 'medium',
	med: 'medium',
	low: 'low',
	'1': 'urgent',
	'2': 'high',
	'3': 'medium',
	'4': 'low'
};

function iso(d: Date): string {
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function addDays(d: Date, n: number): Date {
	const out = new Date(d.getFullYear(), d.getMonth(), d.getDate());
	out.setDate(out.getDate() + n);
	return out;
}

/** "mon", "mond", "monday" all name Monday; at least three letters. */
function prefixOf(word: string, names: string[]): number {
	if (word.length < 3) return -1;
	return names.findIndex((n) => n.startsWith(word));
}

/** A month and day, this year or next if it has passed. */
function monthDay(today: Date, month: number, day: number): string | undefined {
	if (month < 0 || month > 11 || day < 1 || day > 31) return undefined;
	let d = new Date(today.getFullYear(), month, day);
	if (d.getMonth() !== month) return undefined;
	if (d < new Date(today.getFullYear(), today.getMonth(), today.getDate())) {
		d = new Date(today.getFullYear() + 1, month, day);
	}
	return iso(d);
}

function findNamed(raw: string, items: Named[]): Named | undefined {
	const want = raw.toLowerCase().replaceAll('-', ' ').replaceAll('_', ' ');
	if (!want) return undefined;
	const exact = items.find((i) => i.name.toLowerCase() === want);
	return exact ?? items.find((i) => i.name.toLowerCase().startsWith(want));
}

interface Word {
	text: string;
	start: number;
	end: number;
}

function words(input: string): Word[] {
	const out: Word[] = [];
	for (const m of input.matchAll(/\S+/g))
		out.push({ text: m[0], start: m.index!, end: m.index! + m[0].length });
	return out;
}

/** Parses a time like 3pm, 3:30 pm, 15:00 or noon; returns HH:MM. */
function parseTime(w: string, next?: string): { time: string; used: number } | undefined {
	const lower = w.toLowerCase();
	if (lower === 'noon') return { time: '12:00', used: 1 };
	if (lower === 'midnight') return { time: '00:00', used: 1 };
	let m = /^(\d{1,2})(?::(\d{2}))?(am|pm)$/.exec(lower);
	let used = 1;
	if (!m && next && /^(am|pm)$/i.test(next)) {
		m = /^(\d{1,2})(?::(\d{2}))?()$/.exec(lower);
		if (m) {
			m[3] = next.toLowerCase();
			used = 2;
		}
	}
	if (m) {
		let h = Number(m[1]);
		const min = Number(m[2] ?? 0);
		if (h < 1 || h > 12 || min > 59) return undefined;
		if (m[3] === 'pm' && h !== 12) h += 12;
		if (m[3] === 'am' && h === 12) h = 0;
		return { time: `${String(h).padStart(2, '0')}:${String(min).padStart(2, '0')}`, used };
	}
	const h24 = /^([01]?\d|2[0-3]):([0-5]\d)$/.exec(lower);
	if (h24) return { time: `${h24[1].padStart(2, '0')}:${h24[2]}`, used: 1 };
	return undefined;
}

/** Parses a date starting at words[i]; returns the date and words used. */
function parseDate(
	ws: Word[],
	i: number,
	today: Date,
	weekStart = 1
): { date: string; used: number } | undefined {
	const w = ws[i].text.toLowerCase().replace(/,$/, '');
	const next = ws[i + 1]?.text.toLowerCase().replace(/,$/, '');
	if (w === 'today' || w === 'tod') return { date: iso(today), used: 1 };
	if (w === 'tomorrow' || w === 'tmr' || w === 'tmrw')
		return { date: iso(addDays(today, 1)), used: 1 };
	if (w === 'next' && next === 'week') {
		const days = (weekStart - today.getDay() + 7) % 7 || 7; // the coming first day of a week
		return { date: iso(addDays(today, days)), used: 2 };
	}
	if (w === 'next' && next && prefixOf(next, WEEKDAYS) >= 0) {
		const target = prefixOf(next, WEEKDAYS);
		const days = ((target - today.getDay() + 7) % 7 || 7) + 7;
		return { date: iso(addDays(today, days)), used: 2 };
	}
	if (w === 'in' && next && /^\d+$/.test(next)) {
		const unit = ws[i + 2]?.text.toLowerCase();
		const n = Number(next);
		if (unit && /^days?$/.test(unit)) return { date: iso(addDays(today, n)), used: 3 };
		if (unit && /^weeks?$/.test(unit)) return { date: iso(addDays(today, 7 * n)), used: 3 };
	}
	const day = prefixOf(w, WEEKDAYS);
	if (day >= 0) {
		const days = (day - today.getDay() + 7) % 7 || 7; // the next one, not today
		return { date: iso(addDays(today, days)), used: 1 };
	}
	const month = prefixOf(w, MONTHS);
	if (month >= 0 && next && /^\d{1,2}(st|nd|rd|th)?$/.test(next)) {
		const d = monthDay(today, month, parseInt(next));
		if (d) return { date: d, used: 2 };
	}
	if (/^\d{1,2}(st|nd|rd|th)?$/.test(w) && next && prefixOf(next, MONTHS) >= 0) {
		const d = monthDay(today, prefixOf(next, MONTHS), parseInt(w));
		if (d) return { date: d, used: 2 };
	}
	let m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(w);
	if (m) {
		const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
		if (d.getMonth() === Number(m[2]) - 1) return { date: w, used: 1 };
	}
	m = /^(\d{1,2})\/(\d{1,2})$/.exec(w);
	if (m) {
		const d = monthDay(today, Number(m[1]) - 1, Number(m[2]));
		if (d) return { date: d, used: 1 };
	}
	return undefined;
}

const ADVERBS: Record<string, Freq> = {
	daily: 'DAILY',
	weekly: 'WEEKLY',
	monthly: 'MONTHLY',
	yearly: 'YEARLY'
};
const UNIT_WORDS: [RegExp, Freq][] = [
	[/^days?$/, 'DAILY'],
	[/^weeks?$/, 'WEEKLY'],
	[/^months?$/, 'MONTHLY'],
	[/^years?$/, 'YEARLY']
];

/** A weekday name as the repeat rules number them, Monday first. */
function weekdayOf(word: string): Weekday | undefined {
	const d = prefixOf(word, WEEKDAYS);
	return d < 0 ? undefined : (((d + 6) % 7) as Weekday);
}

/** Parses a repeat starting at words[i]; returns the rule and words used. */
function parseRepeat(ws: Word[], i: number): { rule: Rule; used: number } | undefined {
	const word = (k: number) => ws[k]?.text.toLowerCase().replace(/[.,;]$/, '') ?? '';
	const w = word(i);
	const rule = (freq: Freq, interval = 1, weekdays: Weekday[] = []): Rule => ({
		freq,
		interval,
		weekdays
	});
	if (ADVERBS[w]) return { rule: rule(ADVERBS[w]), used: 1 };
	if (w !== 'every') return undefined;
	const next = word(i + 1);
	if (next === 'weekday' || next === 'weekdays')
		return { rule: rule('WEEKLY', 1, [0, 1, 2, 3, 4]), used: 2 };
	for (const [pattern, freq] of UNIT_WORDS) {
		if (pattern.test(next)) return { rule: rule(freq), used: 2 };
	}
	const n = next === 'other' ? 2 : /^\d{1,3}$/.test(next) ? Number(next) : 0;
	if (n >= 1) {
		const unit = UNIT_WORDS.find(([pattern]) => pattern.test(word(i + 2)));
		if (unit) return { rule: rule(unit[1], n), used: 3 };
		return undefined;
	}
	// every mon, every monday and thursday, every mon, wed, fri
	const days: Weekday[] = [];
	let k = i + 1;
	while (weekdayOf(word(k)) !== undefined) {
		const d = weekdayOf(word(k))!;
		if (!days.includes(d)) days.push(d);
		k++;
		if (word(k) === 'and' && weekdayOf(word(k + 1)) !== undefined) k++;
	}
	if (!days.length) return undefined;
	return {
		rule: rule(
			'WEEKLY',
			1,
			days.sort((a, b) => a - b)
		),
		used: k - i
	};
}

const OFFSET_UNITS: [RegExp, number][] = [
	[/^(m|mins?|minutes?)$/, 1],
	[/^(h|hrs?|hours?)$/, 60],
	[/^(d|days?)$/, 24 * 60]
];

type ParsedReminder =
	{ offset: number; used: number } | { date?: string; time?: string; used: number };

/** Parses a reminder starting at words[i] ("remind"); returns it and the words used. */
function parseReminder(
	ws: Word[],
	i: number,
	today: Date,
	weekStart?: number
): ParsedReminder | undefined {
	const word = (k: number) => ws[k]?.text.toLowerCase().replace(/[.,;]$/, '') ?? '';
	if (word(i) !== 'remind') return undefined;
	let k = i + 1;
	if (word(k) === 'me') k++;
	// 1h before, 90 minutes before, 2 days before
	const joined = /^(\d{1,4})([a-z]+)$/.exec(word(k));
	const [n, unitWord, unitAt] = joined
		? [Number(joined[1]), joined[2], k]
		: /^\d{1,4}$/.test(word(k))
			? [Number(word(k)), word(k + 1), k + 1]
			: [0, '', k];
	const unit = OFFSET_UNITS.find(([pattern]) => pattern.test(unitWord));
	// Over a year before, the server would refuse it; it stays in the title.
	if (unit && word(unitAt + 1) === 'before' && n * unit[1] <= MAX_OFFSET)
		return { offset: n * unit[1], used: unitAt + 2 - i };
	// 3pm, tomorrow 9am, fri at 8:30am, at 15:00 tomorrow
	const out: { date?: string; time?: string } = {};
	for (let tries = 0; tries < 2; tries++) {
		if (!out.time) {
			const at = word(k) === 'at' ? 1 : 0;
			const t = parseTime(ws[k + at]?.text ?? '', ws[k + at + 1]?.text);
			if (t && (at || /[ap]m$|:|noon|midnight/i.test(ws[k].text) || t.used === 2)) {
				out.time = t.time;
				k += at + t.used;
				continue;
			}
		}
		if (!out.date && ws[k]) {
			const d = parseDate(ws, k, today, weekStart);
			if (d) {
				out.date = d.date;
				k += d.used;
			}
		}
	}
	if (!out.time && !out.date) return undefined;
	return { ...out, used: k - i };
}

/** Whether a parsed reminder's moment is still ahead of now. */
function reminderAhead(p: Parsed, now: Date): boolean {
	const r = p.reminder;
	if (!r) return false;
	if ('offset' in r) {
		const due = localMoment(p.dueDate!, p.dueTime ?? ALL_DAY_AT);
		return due.getTime() - r.offset * 60_000 > now.getTime();
	}
	return localMoment(r.date, r.time) > now;
}

export function parseQuickAdd(input: string, ctx: QuickAddContext): Parsed {
	const out: Parsed = { title: '', labelIds: [], assigneeIds: [], tokens: [] };
	let repeat: Rule | undefined;
	let reminder: ParsedReminder | undefined;
	let reminderAt = -1;
	const ws = words(input);
	const consumed = new Set<number>();
	const take = (i: number, used: number, kind: TokenKind) => {
		for (let k = i; k < i + used; k++) consumed.add(k);
		out.tokens.push({ kind, start: ws[i].start, end: ws[i + used - 1].end });
	};

	for (let i = 0; i < ws.length; i++) {
		if (consumed.has(i)) continue;
		const w = ws[i].text;
		const sigil = w[0];
		const rest = w.slice(1).replace(/[.,;]$/, '');
		if (sigil === '#' && rest && !out.projectId) {
			const p = findNamed(rest, ctx.projects);
			if (p) {
				out.projectId = p.id;
				take(i, 1, 'project');
				continue;
			}
		}
		if (sigil === '!' && rest && !out.priority && PRIORITY_WORDS[rest.toLowerCase()]) {
			out.priority = PRIORITY_WORDS[rest.toLowerCase()];
			take(i, 1, 'priority');
			continue;
		}
		if (sigil === '@' && rest) {
			const m = findNamed(rest, ctx.members);
			if (m && !out.assigneeIds.includes(m.id)) {
				out.assigneeIds.push(m.id);
				take(i, 1, 'person');
				continue;
			}
		}
		if (sigil === '+' && rest) {
			const l = findNamed(rest, ctx.labels);
			if (l && !out.labelIds.includes(l.id)) {
				out.labelIds.push(l.id);
				take(i, 1, 'label');
				continue;
			}
		}
		if (!reminder) {
			const r = parseReminder(ws, i, ctx.today, ctx.weekStart);
			if (r) {
				reminder = r;
				reminderAt = i;
				take(i, r.used, 'reminder');
				continue;
			}
		}
		if (!out.recurrence) {
			const r = parseRepeat(ws, i);
			if (r) {
				out.recurrence = formatRule(r.rule);
				repeat = r.rule;
				take(i, r.used, 'repeat');
				continue;
			}
		}
		if (!out.dueTime) {
			const atWord = w.toLowerCase() === 'at' && ws[i + 1] ? 1 : 0;
			const t = parseTime(ws[i + atWord]?.text ?? '', ws[i + atWord + 1]?.text);
			if (t && (atWord || /[ap]m$|:|noon|midnight/i.test(ws[i].text) || t.used === 2)) {
				out.dueTime = t.time;
				take(i, atWord + t.used, 'time');
				continue;
			}
		}
		if (!out.dueDate) {
			const onWord = ['on', 'due', 'by'].includes(w.toLowerCase()) && ws[i + 1] ? 1 : 0;
			const d = parseDate(ws, i + onWord, ctx.today, ctx.weekStart);
			if (d) {
				out.dueDate = d.date;
				take(i, onWord + d.used, 'date');
				continue;
			}
		}
	}
	// A repeat with no date starts on its first day; a time with no date
	// means today.
	if (repeat && !out.dueDate) out.dueDate = firstDue(repeat, iso(ctx.today));
	if (out.dueTime && !out.dueDate) out.dueDate = iso(ctx.today);
	if (reminder && 'offset' in reminder && out.dueDate) {
		out.reminder = { offset: reminder.offset };
	} else if (reminder && !('offset' in reminder)) {
		const time = reminder.time ?? '09:00';
		let date = reminder.date ?? out.dueDate ?? iso(ctx.today);
		const passed = localMoment(date, time) <= ctx.today;
		if (!reminder.date && !out.dueDate && passed) date = iso(addDays(ctx.today, 1));
		out.reminder = { date, time };
	}
	// Before what? With no due date, or a moment already past (which would
	// never fire), it stays in the title.
	if (reminder && !(out.reminder && reminderAhead(out, ctx.today))) {
		delete out.reminder;
		for (let k = reminderAt; k < reminderAt + reminder.used; k++) consumed.delete(k);
		out.tokens = out.tokens.filter((t) => t.kind !== 'reminder');
	}
	out.title = ws
		.filter((_, i) => !consumed.has(i))
		.map((w) => w.text)
		.join(' ')
		.trim();
	out.tokens.sort((a, b) => a.start - b.start);
	return out;
}
