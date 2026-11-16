// The Timeline view's rules (docs/PLAN.md section 3), pure: zoom and
// scale, where a task's bar or diamond goes, keyboard steps, the rows a
// timeline shows, and the paths of the arrows between tasks. The view is
// components/views/Timeline.svelte.
import type { Task, ViewDisplay } from './sync/entities';

export type Zoom = 'day' | 'week' | 'month' | 'quarter';

export const ZOOMS: { id: Zoom; label: string }[] = [
	{ id: 'day', label: 'Day' },
	{ id: 'week', label: 'Week' },
	{ id: 'month', label: 'Month' },
	{ id: 'quarter', label: 'Quarter' }
];

/** Pixels per day at each zoom. */
export const DAY_PX: Record<Zoom, number> = { day: 40, week: 18, month: 6, quarter: 2.4 };

/** Room around the dates shown, in days, and the least span drawn. */
const PAD: Record<Zoom, number> = { day: 14, week: 35, month: 62, quarter: 120 };
const SPAN: Record<Zoom, number> = { day: 63, week: 126, month: 400, quarter: 800 };

/** A saved view's zoom, Week when it has none or one this app does not know. */
export function zoomOf(display: ViewDisplay | null | undefined): Zoom {
	const t = display?.timeline as { zoom?: unknown } | undefined;
	const z = t && typeof t === 'object' ? t.zoom : undefined;
	return ZOOMS.some((x) => x.id === z) ? (z as Zoom) : 'week';
}

// Days are whole days since 1970-01-01, so a day is a number.

export function dayOf(iso: string): number {
	const [y, m, d] = iso.split('-').map(Number);
	return Math.round(Date.UTC(y, m - 1, d) / 864e5);
}

export function isoOf(day: number): string {
	return new Date(day * 864e5).toISOString().slice(0, 10);
}

/** 0 is Sunday, as Date has it. */
export const weekdayOf = (day: number) => (((day + 4) % 7) + 7) % 7;

function monthStart(day: number): number {
	const d = new Date(day * 864e5);
	return Math.round(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), 1) / 864e5);
}

function addMonths(day: number, n: number): number {
	const d = new Date(day * 864e5);
	const y = d.getUTCFullYear();
	const m = d.getUTCMonth() + n;
	// The same day of the month, or the month's last when it has fewer.
	const last = new Date(Date.UTC(y, m + 1, 0)).getUTCDate();
	return Math.round(Date.UTC(y, m, Math.min(d.getUTCDate(), last)) / 864e5);
}

/**
 * The days a timeline spans: around today and every date shown, with room
 * either side, starting on a week (Day and Week zoom), a month or a
 * quarter, and at least wide enough to fill a screen.
 */
export function rangeOf(
	dates: string[],
	today: string,
	zoom: Zoom,
	weekStart: number
): { from: number; to: number } {
	let lo = dayOf(today);
	let hi = lo;
	for (const iso of dates) {
		const d = dayOf(iso);
		if (d < lo) lo = d;
		if (d > hi) hi = d;
	}
	let from = lo - PAD[zoom];
	if (zoom === 'day' || zoom === 'week') from -= (weekdayOf(from) - weekStart + 7) % 7;
	else {
		from = monthStart(from);
		if (zoom === 'quarter') {
			const m = new Date(from * 864e5).getUTCMonth();
			from = addMonths(from, -(m % 3));
		}
	}
	const to = Math.max(hi + PAD[zoom], from + SPAN[zoom]);
	return { from, to };
}

/** One label on the scale: what it names, from where, for how many days. */
export interface Tick {
	day: number;
	days: number;
	iso: string;
	weekend?: boolean;
}

export interface Scale {
	/** The upper line: months (Day and Week zoom) or years. */
	top: Tick[];
	/** The lower line: days, weeks, months or quarters. */
	bottom: Tick[];
	/** Vertical lines across the rows; strong ones start a top tick. */
	lines: { day: number; strong: boolean }[];
	/** Saturdays and Sundays, shaded at Day and Week zoom. */
	weekends: number[];
}

/** The scale above a timeline, in days; the view names them. */
export function scaleOf(from: number, to: number, zoom: Zoom, weekStart: number): Scale {
	const out: Scale = { top: [], bottom: [], lines: [], weekends: [] };
	const fine = zoom === 'day' || zoom === 'week';
	const top = (day: number, next: number) =>
		out.top.push({ day, days: Math.min(next, to + 1) - day, iso: isoOf(day) });
	// The upper line: months, or years.
	for (let d = from; d <= to;) {
		const next = fine
			? addMonths(monthStart(d), 1)
			: Math.round(Date.UTC(new Date(d * 864e5).getUTCFullYear() + 1, 0, 1) / 864e5);
		top(d, next);
		out.lines.push({ day: d, strong: true });
		d = next;
	}
	const strong = new Set(out.lines.map((l) => l.day));
	const line = (day: number) => {
		if (!strong.has(day)) out.lines.push({ day, strong: false });
	};
	if (zoom === 'day') {
		for (let d = from; d <= to; d++) {
			const wd = weekdayOf(d);
			out.bottom.push({ day: d, days: 1, iso: isoOf(d), weekend: wd === 0 || wd === 6 });
			if (wd === weekStart) line(d);
		}
	} else if (zoom === 'week') {
		let d = from + ((weekStart - weekdayOf(from) + 7) % 7);
		if (d > from) out.bottom.push({ day: from, days: d - from, iso: isoOf(from) });
		for (; d <= to; d += 7) {
			out.bottom.push({ day: d, days: Math.min(7, to + 1 - d), iso: isoOf(d) });
			line(d);
		}
	} else {
		const months = zoom === 'month' ? 1 : 3;
		for (let d = monthStart(from); d <= to; d = addMonths(d, months)) {
			const start = Math.max(d, from);
			out.bottom.push({
				day: start,
				days: Math.min(addMonths(d, months), to + 1) - start,
				iso: isoOf(start)
			});
			line(start);
		}
	}
	if (fine)
		for (let d = from; d <= to; d++) {
			const wd = weekdayOf(d);
			if (wd === 0 || wd === 6) out.weekends.push(d);
		}
	out.lines.sort((a, b) => a.day - b.day);
	return out;
}

type Dated = Pick<Task, 'startDate' | 'dueDate'>;

/** Where a task is drawn, in pixels from the timeline's first day. */
export type Shape =
	| { kind: 'bar'; left: number; right: number }
	| { kind: 'mark'; center: number; left: number; right: number; startOnly: boolean };

/** A diamond's half width. */
export const MARK = 8;

export function shapeOf(t: Dated, from: number, px: number): Shape | null {
	if (t.startDate && t.dueDate) {
		const a = dayOf(t.startDate);
		const b = dayOf(t.dueDate);
		return {
			kind: 'bar',
			left: (Math.min(a, b) - from) * px,
			right: (Math.max(a, b) + 1 - from) * px
		};
	}
	const one = t.dueDate ?? t.startDate;
	if (!one) return null;
	const center = (dayOf(one) - from + 0.5) * px;
	return { kind: 'mark', center, left: center - MARK, right: center + MARK, startOnly: !t.dueDate };
}

/** A day moved by k steps of the zoom: days, weeks, or months. */
export function step(iso: string, zoom: Zoom, k: number): string {
	const d = dayOf(iso);
	if (zoom === 'day') return isoOf(d + k);
	if (zoom === 'week') return isoOf(d + 7 * k);
	return isoOf(addMonths(d, k));
}

/**
 * A task's dates after a keyboard move: both by one step (keeping its
 * length), or only the due date, which never goes before the start.
 * Null when there is nothing to move or the move would cross the start.
 */
export function stepped(
	t: Dated,
	zoom: Zoom,
	k: number,
	which: 'both' | 'due'
): { startDate: string | null; dueDate: string | null } | null {
	const anchor = t.startDate ?? t.dueDate;
	if (!anchor) return null;
	if (which === 'due') {
		const due = step(t.dueDate ?? t.startDate!, zoom, k);
		if (t.startDate && due < t.startDate) return null;
		return { startDate: t.startDate, dueDate: due };
	}
	const days = dayOf(step(anchor, zoom, k)) - dayOf(anchor);
	return shifted(t, days);
}

/** A task's dates moved by a number of days. */
export function shifted(
	t: Dated,
	days: number
): { startDate: string | null; dueDate: string | null } {
	return {
		startDate: t.startDate ? isoOf(dayOf(t.startDate) + days) : null,
		dueDate: t.dueDate ? isoOf(dayOf(t.dueDate) + days) : null
	};
}

/**
 * A task's dates while one end is dragged by a number of days: the start
 * never passes the due date, nor the due date the start. A task with one
 * date gets the other.
 */
export function stretched(
	t: Dated,
	end: 'start' | 'due',
	days: number
): { startDate: string | null; dueDate: string | null } {
	const start = t.startDate ?? t.dueDate!;
	const due = t.dueDate ?? t.startDate!;
	if (end === 'start') {
		const s = Math.min(dayOf(start) + days, dayOf(due));
		return { startDate: isoOf(s), dueDate: due };
	}
	const d = Math.max(dayOf(due) + days, dayOf(start));
	return { startDate: start, dueDate: isoOf(d) };
}

export const dated = (t: Dated) => Boolean(t.startDate || t.dueDate);

/** A row of a timeline: a group's heading, or a task with its depth. */
export type Row =
	| { kind: 'group'; key: string; title: string; count: number }
	| { kind: 'task'; key: string; task: Task; depth: number; kids: number; folded: boolean };

/**
 * The rows a timeline shows: each section's heading (when grouped), then
 * its tasks that have a date or subtasks with one, each followed by those
 * subtasks unless it is folded. What is left, the tasks with no date
 * anywhere under them, waits in the Unscheduled tray.
 */
export function rowsOf(
	sections: { id: string; title: string; tasks: Task[] }[],
	kids: (t: Task) => Task[],
	folded: ReadonlySet<string>,
	grouped: boolean
): { rows: Row[]; unscheduled: Task[] } {
	const rows: Row[] = [];
	const unscheduled: Task[] = [];
	const onChart = new Map<string, boolean>();
	const shows = (t: Task, depth = 0): boolean => {
		let v = onChart.get(t.id);
		if (v === undefined) {
			v = dated(t) || (depth < 20 && kids(t).some((k) => shows(k, depth + 1)));
			onChart.set(t.id, v);
		}
		return v;
	};
	const walk = (t: Task, depth: number, into: Row[]) => {
		if (!shows(t)) {
			unscheduled.push(t);
			return;
		}
		const under = kids(t);
		const isFolded = folded.has(t.id) && under.length > 0;
		into.push({ kind: 'task', key: t.id, task: t, depth, kids: under.length, folded: isFolded });
		if (depth < 20)
			for (const k of under) {
				if (isFolded) {
					if (!shows(k)) unscheduled.push(k);
				} else walk(k, depth + 1, into);
			}
	};
	for (const s of sections) {
		const mine: Row[] = [];
		for (const t of s.tasks) walk(t, 0, mine);
		const count = mine.filter((r) => r.kind === 'task' && r.depth === 0).length;
		if (grouped && mine.length)
			rows.push({ kind: 'group', key: `g:${s.id}`, title: s.title, count });
		rows.push(...mine);
	}
	return { rows, unscheduled };
}

/** The earliest and latest dates under a task, for a folded parent's bracket. */
export function spanUnder(t: Task, kids: (t: Task) => Task[]): { from: string; to: string } | null {
	let from: string | null = null;
	let to: string | null = null;
	const walk = (k: Task, depth: number) => {
		for (const d of [k.startDate, k.dueDate])
			if (d) {
				if (!from || d < from) from = d;
				if (!to || d > to) to = d;
			}
		if (depth < 20) for (const c of kids(k)) walk(c, depth + 1);
	};
	for (const c of kids(t)) walk(c, 1);
	return from && to ? { from, to } : null;
}

/**
 * The path of an arrow from a blocker's end (x1, y1) to the start of the
 * task waiting for it (x2, y2): across, down and across when there is
 * room; otherwise out, along the gap between rows, back and in.
 */
export function arrowPath(x1: number, y1: number, x2: number, y2: number, rowH: number): string {
	const r = (n: number) => Math.round(n * 10) / 10;
	if (x2 - x1 >= 18) return `M${r(x1)} ${r(y1)} H${r(x1 + 8)} V${r(y2)} H${r(x2 - 2)}`;
	const gap = y2 > y1 ? y2 - rowH / 2 : y2 + rowH / 2;
	return `M${r(x1)} ${r(y1)} H${r(x1 + 8)} V${r(gap)} H${r(x2 - 10)} V${r(y2)} H${r(x2 - 2)}`;
}
