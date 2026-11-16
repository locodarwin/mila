// The Reports page (docs/PLAN.md section 3, Release 3.0), pure: what the
// three tabs count, worked out on the device from the tasks it holds, so
// they work offline and count only what the person can see. Loaded only
// with that page.
//
// The rules every tab shares:
// - The tasks counted are the live ones (subtasks too) in projects in use
//   and the inbox, that the filter keeps; tasks in the trash, under a task
//   in the trash or in an archived project are left out, and so are
//   canceled ones, which were never work done.
// - Done means a status of kind done (or, with no status, completed). A
//   reopened task counts at its last completion: open now, it is open in
//   every week, since only the last completion is kept.
// - Weeks start on the person's first day of the week; days are the
//   device's.
import { writeCsv } from './csv';
import { addDays } from './filter';
import type { StatusCategory, Task } from './sync/entities';
import { weekOf } from './timeReport';

export {
	chartOf,
	PERIOD_KINDS,
	REPORT_CHARTS,
	REPORT_TABS,
	type PeriodKind,
	type ReportChart,
	type ReportTab
} from './reportKinds';
import type { PeriodKind } from './reportKinds';

/** A span of days, both ends counted, as YYYY-MM-DD. */
export interface Period {
	from: string;
	to: string;
}

/** A week of a period, cut to it at either end. */
export interface Week {
	from: string;
	to: string;
}

const DAY_MS = 86_400_000;

const parse = (day: string) => {
	const [y, m, d] = day.split('-').map(Number);
	return new Date(y, m - 1, d);
};
const iso = (d: Date) =>
	`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;

/** The device's day a moment falls on. */
export function dayOf(moment: string): string {
	return iso(new Date(moment));
}

/** Whole days from one day to another. */
export function daysBetween(from: string, to: string): number {
	return Math.round((parse(to).getTime() - parse(from).getTime()) / DAY_MS);
}

/**
 * A period ending today: the last 6 weeks or about 3 months in whole
 * weeks, this year from January 1, or two days picked (in either order,
 * and no later than today).
 */
export function periodOf(
	kind: PeriodKind,
	today: string,
	weekStart: number,
	picked?: { from: string | null; to: string | null }
): Period {
	switch (kind) {
		case 'weeks6':
			return { from: addDays(weekOf(today, weekStart), -35), to: today };
		case 'months3': {
			const d = parse(today);
			const back = iso(new Date(d.getFullYear(), d.getMonth() - 3, d.getDate() + 1));
			return { from: weekOf(back, weekStart), to: today };
		}
		case 'year':
			return { from: `${today.slice(0, 4)}-01-01`, to: today };
		case 'range': {
			let from = picked?.from || addDays(today, -41);
			let to = picked?.to || today;
			if (from > to) [from, to] = [to, from];
			if (to > today) to = today;
			if (from > to) from = to;
			// At most MAX_RANGE_DAYS: every chart's work grows with the days.
			const earliest = addDays(to, -MAX_RANGE_DAYS);
			if (from < earliest) from = earliest;
			return { from, to };
		}
	}
}

/** The longest a picked period may be, in days: five years. */
export const MAX_RANGE_DAYS = 5 * 366;

/** The weeks of a period, the first and last cut to it. */
export function weeksOf(p: Period, weekStart: number): Week[] {
	const out: Week[] = [];
	if (p.to < p.from) return out;
	for (let start = weekOf(p.from, weekStart); start <= p.to; start = addDays(start, 7)) {
		const end = addDays(start, 6);
		out.push({ from: start < p.from ? p.from : start, to: end > p.to ? p.to : end });
	}
	return out;
}

/** What the reports read beyond the tasks themselves. */
export interface ReportSource {
	/** Every live task, subtasks too. */
	tasks: Task[];
	/** A live task by its ID, or undefined for one in the trash. */
	task: (id: string) => Task | undefined;
	/** Whether a project is in use (not archived). */
	projectInUse: (id: string) => boolean;
	category: (task: Task) => StatusCategory;
	/** The saved-view filter, with finished tasks shown. */
	keep: (task: Task) => boolean;
}

/** Whether a task's parents, all the way up, are live. */
function parentsLive(t: Task, src: Pick<ReportSource, 'task'>): boolean {
	let parent = t.parentId;
	for (let depth = 0; parent && depth < 50; depth++) {
		const p = src.task(parent);
		if (!p) return false;
		parent = p.parentId;
	}
	return true;
}

/** The tasks the reports count, as the rules above have them. */
export function inScope(src: ReportSource): Task[] {
	return src.tasks.filter(
		(t) =>
			(!t.projectId || src.projectInUse(t.projectId)) &&
			src.category(t) !== 'canceled' &&
			parentsLive(t, src) &&
			src.keep(t)
	);
}

const isDone = (c: StatusCategory) => c === 'done';

/** The day a task came into being, or null if nobody knows (it counts as always there). */
function madeDay(t: Task): string | null {
	const at = t.madeAt && t.completedAt && t.completedAt < t.madeAt ? t.completedAt : t.madeAt;
	return at ? dayOf(at) : null;
}

/** The day a done task was done, or null when it was done at a time nobody kept. */
const doneDay = (t: Task) => (t.completedAt ? dayOf(t.completedAt) : null);

// Project status.

export type OpenCategory = 'backlog' | 'todo' | 'in-progress';
export const OPEN_CATEGORIES: { category: OpenCategory; label: string }[] = [
	{ category: 'backlog', label: 'Backlog' },
	{ category: 'todo', label: 'To do' },
	{ category: 'in-progress', label: 'In progress' }
];

export interface Attention {
	task: Task;
	/** Days past its due date, when overdue. */
	late: number | null;
	/** How many open tasks it waits for. */
	waiting: number;
	/** Minutes logged past its estimate, when over. */
	over: number | null;
}

export interface StatusReport {
	open: number;
	done: number;
	total: number;
	overdue: number;
	/** Open and due from today to the end of next week. */
	dueSoon: number;
	weeks: Week[];
	/** Tasks there by the end of each week. */
	scope: number[];
	/** Tasks done by the end of each week. */
	doneBy: number[];
	/** Open tasks by their status's kind. */
	byStatus: { category: OpenCategory; label: string; count: number }[];
	/** Open tasks that are overdue, waiting on open tasks, or past their estimate. */
	attention: Attention[];
}

export interface StatusOptions {
	period: Period;
	today: string;
	weekStart: number;
	category: (task: Task) => StatusCategory;
	/** How many open tasks a task waits for. */
	openBlockers: (taskId: string) => number;
	/** The minutes logged on a task itself. */
	logged: (taskId: string) => number;
}

export function statusReport(tasks: Task[], o: StatusOptions): StatusReport {
	const weeks = weeksOf(o.period, o.weekStart);
	const soon = addDays(weekOf(o.today, o.weekStart), 13);
	const counts = new Map<OpenCategory, number>();
	const attention: Attention[] = [];
	let open = 0;
	let done = 0;
	let overdue = 0;
	let dueSoon = 0;
	const scope = weeks.map(() => 0);
	const doneBy = weeks.map(() => 0);
	for (const t of tasks) {
		const c = o.category(t);
		const finished = isDone(c);
		const made = madeDay(t);
		const at = finished ? doneDay(t) : null;
		weeks.forEach((w, i) => {
			if (made === null || made <= w.to) scope[i]++;
			if (finished && (at === null || at <= w.to)) doneBy[i]++;
		});
		if (finished) {
			done++;
			continue;
		}
		open++;
		const kind: OpenCategory = c === 'backlog' || c === 'in-progress' ? c : 'todo';
		counts.set(kind, (counts.get(kind) ?? 0) + 1);
		const late = t.dueDate && t.dueDate < o.today ? daysBetween(t.dueDate, o.today) : null;
		if (late !== null) overdue++;
		else if (t.dueDate && t.dueDate <= soon) dueSoon++;
		const waiting = o.openBlockers(t.id);
		const logged = t.estimateMinutes !== null ? o.logged(t.id) : 0;
		const over =
			t.estimateMinutes !== null && logged > t.estimateMinutes ? logged - t.estimateMinutes : null;
		if (late !== null || waiting > 0 || over !== null)
			attention.push({ task: t, late, waiting, over });
	}
	attention.sort(
		(a, b) =>
			(b.late ?? -1) - (a.late ?? -1) ||
			b.waiting - a.waiting ||
			(b.over ?? -1) - (a.over ?? -1) ||
			a.task.title.localeCompare(b.task.title)
	);
	return {
		open,
		done,
		total: open + done,
		overdue,
		dueSoon,
		weeks,
		scope,
		doneBy,
		byStatus: OPEN_CATEGORIES.map((s) => ({ ...s, count: counts.get(s.category) ?? 0 })),
		attention
	};
}

// Workload.

export interface PersonLoad {
	/** The person's ID, or null for tasks nobody is assigned. */
	userId: string | null;
	/** Estimated minutes of their open tasks. */
	minutes: number;
	tasks: number;
	overdue: number;
	/** Their open tasks with no estimate. */
	unestimated: number;
}

export interface DueBucket {
	key: string;
	/** Overdue, this week, next week, two weeks of their own, then later. */
	kind: 'overdue' | 'this' | 'next' | 'week' | 'later';
	/** The week's first day, for a week of its own. */
	from: string | null;
	count: number;
}

export interface WorkloadReport {
	people: PersonLoad[];
	/** Open tasks with no estimate, each counted once. */
	unestimated: number;
	/** Open tasks with more than one assignee. */
	shared: number;
	open: number;
	due: DueBucket[];
	/** Open tasks with no due date, left out of due. */
	undated: number;
}

export interface WorkloadOptions {
	today: string;
	weekStart: number;
	category: (task: Task) => StatusCategory;
	assignees: (taskId: string) => string[];
	/** A person's name, for the order. */
	nameOf: (userId: string) => string;
}

/**
 * Open work by person (a task with several assignees counts for each, and
 * one with no estimate as a task with no hours; unassigned last), and open
 * tasks by the week they fall due, overdue first.
 */
export function workloadReport(tasks: Task[], o: WorkloadOptions): WorkloadReport {
	const people = new Map<string | null, PersonLoad>();
	const thisWeek = weekOf(o.today, o.weekStart);
	const due: DueBucket[] = [
		{ key: 'overdue', kind: 'overdue', from: null, count: 0 },
		{ key: 'this', kind: 'this', from: thisWeek, count: 0 },
		{ key: 'next', kind: 'next', from: addDays(thisWeek, 7), count: 0 },
		{ key: 'w2', kind: 'week', from: addDays(thisWeek, 14), count: 0 },
		{ key: 'w3', kind: 'week', from: addDays(thisWeek, 21), count: 0 },
		{ key: 'later', kind: 'later', from: addDays(thisWeek, 28), count: 0 }
	];
	let unestimated = 0;
	let shared = 0;
	let open = 0;
	let undated = 0;
	for (const t of tasks) {
		if (isDone(o.category(t))) continue;
		open++;
		const late = !!t.dueDate && t.dueDate < o.today;
		if (t.estimateMinutes === null) unestimated++;
		const who = [...new Set(o.assignees(t.id))];
		if (who.length > 1) shared++;
		for (const id of who.length ? who : [null]) {
			let p = people.get(id);
			if (!p) {
				p = { userId: id, minutes: 0, tasks: 0, overdue: 0, unestimated: 0 };
				people.set(id, p);
			}
			p.tasks++;
			p.minutes += t.estimateMinutes ?? 0;
			if (t.estimateMinutes === null) p.unestimated++;
			if (late) p.overdue++;
		}
		if (!t.dueDate) undated++;
		else if (late) due[0].count++;
		else {
			let at = due.length - 1;
			while (at > 1 && due[at].from! > t.dueDate) at--;
			due[at].count++;
		}
	}
	const named = [...people.values()]
		.filter((p) => p.userId !== null)
		.sort(
			(a, b) =>
				b.minutes - a.minutes ||
				b.tasks - a.tasks ||
				o.nameOf(a.userId!).localeCompare(o.nameOf(b.userId!))
		);
	const nobody = people.get(null);
	return {
		people: nobody ? [...named, nobody] : named,
		unestimated,
		shared,
		open,
		due,
		undated
	};
}

// Throughput and cycle time.

export interface CycleBucket {
	key: string;
	/** Days, from (counted) up to (not counted); null for no end. */
	from: number;
	to: number | null;
	count: number;
}

const BUCKETS: [number, number | null][] = [
	[0, 1],
	[1, 2],
	[2, 4],
	[4, 7],
	[7, 14],
	[14, null]
];

export interface FlowReport {
	weeks: Week[];
	made: number[];
	done: number[];
	/** Tasks done in the period. */
	doneTotal: number;
	/** Done each week, on average over the period's weeks. */
	perWeek: number;
	/** Days from first in progress to done, for the tasks done in the period that started. */
	cycle: number[];
	median: number | null;
	/** The time within which 8 in 10 of them were done. */
	p80: number | null;
	buckets: CycleBucket[];
	/** Tasks done in the period that never went in progress, left out of the cycle time. */
	unstarted: number;
	/** Each of the timed ones: the day it was done and how many days it took. */
	dots: CycleDot[];
}

export interface CycleDot {
	task: Task;
	day: string;
	days: number;
}

/** The value a share of a sorted list falls within: the nearest rank. */
export function percentile(sorted: number[], share: number): number | null {
	if (!sorted.length) return null;
	return sorted[Math.max(0, Math.ceil(share * sorted.length) - 1)];
}

export function median(sorted: number[]): number | null {
	if (!sorted.length) return null;
	const mid = sorted.length >> 1;
	return sorted.length % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2;
}

export interface FlowOptions {
	period: Period;
	weekStart: number;
	category: (task: Task) => StatusCategory;
}

/**
 * Tasks made and done each week of the period, and how long the ones done
 * in it took from first going in progress (`startedAt`) to done. A task
 * done without ever starting, or whose last completion came before it
 * started, is left out of the time and counted apart.
 */
export function flowReport(tasks: Task[], o: FlowOptions): FlowReport {
	const weeks = weeksOf(o.period, o.weekStart);
	const made = weeks.map(() => 0);
	const done = weeks.map(() => 0);
	const at = (day: string | null) =>
		day === null ? -1 : weeks.findIndex((w) => day >= w.from && day <= w.to);
	const cycle: number[] = [];
	const dots: CycleDot[] = [];
	let unstarted = 0;
	let doneTotal = 0;
	for (const t of tasks) {
		const m = at(madeDay(t));
		if (m >= 0) made[m]++;
		if (!isDone(o.category(t)) || !t.completedAt) continue;
		const d = at(doneDay(t));
		if (d < 0) continue;
		done[d]++;
		doneTotal++;
		const took = t.startedAt
			? (new Date(t.completedAt).getTime() - new Date(t.startedAt).getTime()) / DAY_MS
			: NaN;
		if (Number.isFinite(took) && took >= 0) {
			cycle.push(took);
			dots.push({ task: t, day: doneDay(t)!, days: took });
		} else unstarted++;
	}
	cycle.sort((a, b) => a - b);
	const buckets = BUCKETS.map(([from, to]) => ({
		key: `${from}`,
		from,
		to,
		count: cycle.filter((c) => c >= from && (to === null || c < to)).length
	}));
	return {
		weeks,
		made,
		done,
		doneTotal,
		perWeek: weeks.length ? doneTotal / weeks.length : 0,
		cycle,
		median: median(cycle),
		p80: percentile(cycle, 0.8),
		buckets,
		unstarted,
		dots: dots.sort((a, b) => a.day.localeCompare(b.day) || a.days - b.days)
	};
}

// The weekly report (docs/PLAN.md section 3, added 2026-11-11). It is an
// email, so the server works it out, in Go (server/internal/reports); this
// is the same arithmetic on the app's own rules, and
// shared/weekly-vectors.json holds the two to the same answers.

/** One week's figures, for the week reported and the one before it. */
export interface WeekFigures {
	done: number;
	made: number;
	/** Open as the week ended, and due on or before its last day. */
	overdue: number;
	/** The median cycle time of the tasks done in the week, in days. */
	median: number | null;
}

/** A project's line: done in the week, and open and overdue as it ended. */
export interface ProjectLine {
	id: string;
	name: string;
	done: number;
	open: number;
	overdue: number;
}

export interface WeeklyReport {
	/** The week reported: the seven days before today. */
	week: Period;
	before: Period;
	last: WeekFigures;
	prior: WeekFigures;
	/** Projects with anything done in the week or open as it ended, most done first. */
	projects: ProjectLine[];
	/** Minutes logged in the week, by the person and by everyone. */
	mine: number;
	everyone: number;
	/** The person's open tasks that are overdue, waiting on open tasks, or past their estimate. */
	needs: Attention[];
	/** Nothing done, made or logged in the week: no email. */
	quiet: boolean;
}

export interface WeeklyOptions {
	/** The day it goes out: the first day of the week after the one reported. */
	today: string;
	category: (task: Task) => StatusCategory;
	openBlockers: (taskId: string) => number;
	/** The minutes logged on a task itself, ever. */
	logged: (taskId: string) => number;
	/** Whether a task is the person's: assigned to them, or made by them and assigned to nobody. */
	mine: (task: Task) => boolean;
	projectName: (projectId: string) => string;
	/** Whether a project is in use (not archived), for time logged to it itself. */
	projectInUse: (projectId: string) => boolean;
	/**
	 * The time entries the person can see: on a task, or with no task, on a
	 * project itself.
	 */
	entries: WeeklyEntry[];
	me: string;
}

/** Time logged on a day, on a task or (taskId null) on a project itself. */
export interface WeeklyEntry {
	taskId: string | null;
	projectId?: string | null;
	userId: string;
	day: string;
	minutes: number;
}

/** Comparing as the server can, by UTF-16 code units, never the locale. */
const byCode = (a: string, b: string) => (a < b ? -1 : a > b ? 1 : 0);

/** Whether a task was made by the end of a day and still open at its end. */
function openAt(t: Task, day: string, finished: boolean): boolean {
	const made = madeDay(t);
	if (made !== null && made > day) return false;
	if (!finished) return true;
	const at = doneDay(t);
	return at !== null && at > day;
}

/**
 * The weekly report of the tasks in scope (`inScope`), for the week before
 * `today`: what was done and made in it and in the week before, what was
 * overdue as each ended, the projects, the time logged and what needs the
 * person now.
 */
export function weeklyReport(tasks: Task[], o: WeeklyOptions): WeeklyReport {
	const week = { from: addDays(o.today, -7), to: addDays(o.today, -1) };
	const before = { from: addDays(o.today, -14), to: addDays(o.today, -8) };
	const weekStart = parse(week.from).getDay();
	const figures = (p: Period): WeekFigures => {
		const flow = flowReport(tasks, { period: p, weekStart, category: o.category });
		let overdue = 0;
		for (const t of tasks) {
			if (t.dueDate && t.dueDate <= p.to && openAt(t, p.to, isDone(o.category(t)))) overdue++;
		}
		return {
			done: flow.doneTotal,
			made: flow.made.reduce((a, b) => a + b, 0),
			overdue,
			median: flow.median
		};
	};
	const last = figures(week);
	const lines = new Map<string, ProjectLine>();
	const needs: Attention[] = [];
	for (const t of tasks) {
		const finished = isDone(o.category(t));
		if (t.projectId) {
			const at = finished ? doneDay(t) : null;
			const doneIn = finished && at !== null && at >= week.from && at <= week.to;
			const open = openAt(t, week.to, finished);
			if (doneIn || open) {
				const line = lines.get(t.projectId) ?? {
					id: t.projectId,
					name: o.projectName(t.projectId),
					done: 0,
					open: 0,
					overdue: 0
				};
				if (doneIn) line.done++;
				if (open) line.open++;
				if (open && t.dueDate && t.dueDate <= week.to) line.overdue++;
				lines.set(t.projectId, line);
			}
		}
		if (finished || !o.mine(t)) continue;
		const late = t.dueDate && t.dueDate < o.today ? daysBetween(t.dueDate, o.today) : null;
		const waiting = o.openBlockers(t.id);
		const logged = t.estimateMinutes !== null ? o.logged(t.id) : 0;
		const over =
			t.estimateMinutes !== null && logged > t.estimateMinutes ? logged - t.estimateMinutes : null;
		if (late !== null || waiting > 0 || over !== null) needs.push({ task: t, late, waiting, over });
	}
	needs.sort(
		(a, b) =>
			(b.late ?? -1) - (a.late ?? -1) ||
			b.waiting - a.waiting ||
			(b.over ?? -1) - (a.over ?? -1) ||
			byCode(a.task.title, b.task.title) ||
			byCode(a.task.id, b.task.id)
	);
	const counted = new Set(tasks.map((t) => t.id));
	let mine = 0;
	let everyone = 0;
	for (const e of o.entries) {
		// Time on a project itself counts while the project is in use.
		const counts = e.taskId
			? counted.has(e.taskId)
			: Boolean(e.projectId && o.projectInUse(e.projectId));
		if (e.day < week.from || e.day > week.to || !counts) continue;
		everyone += e.minutes;
		if (e.userId === o.me) mine += e.minutes;
	}
	return {
		week,
		before,
		last,
		prior: figures(before),
		projects: [...lines.values()].sort(
			(a, b) => b.done - a.done || b.open - a.open || byCode(a.name, b.name) || byCode(a.id, b.id)
		),
		mine,
		everyone,
		needs,
		quiet: last.done === 0 && last.made === 0 && everyone === 0
	};
}

// Shares, ranks and flow by day (added 2026-11-08: the project picker's charts).

/** A part of a whole, or a row of a ranking. */
export interface Share {
	key: string;
	name: string;
	value: number;
	/** The rest, folded into one. */
	other?: boolean;
}

/**
 * At most `max` parts, in their order: the largest `max - 1` kept, the
 * rest folded into Other at the end. Empty parts are left out.
 */
export function foldShares(parts: Share[], max = 6): Share[] {
	const some = parts.filter((p) => p.value > 0);
	if (some.length <= max) return some;
	const keep = new Set(
		[...some]
			.sort((a, b) => b.value - a.value)
			.slice(0, max - 1)
			.map((p) => p.key)
	);
	const rest = some.filter((p) => !keep.has(p.key));
	return [
		...some.filter((p) => keep.has(p.key)),
		{ key: 'other', name: 'Other', value: rest.reduce((n, p) => n + p.value, 0), other: true }
	];
}

/** The largest first, then by name; the first `top`. */
export function ranked(parts: Share[], top = 8): Share[] {
	return parts
		.filter((p) => p.value > 0)
		.sort((a, b) => b.value - a.value || a.name.localeCompare(b.name))
		.slice(0, top);
}

const KINDS: { category: StatusCategory; label: string }[] = [
	{ category: 'backlog', label: 'Backlog' },
	{ category: 'todo', label: 'To do' },
	{ category: 'in-progress', label: 'In progress' },
	{ category: 'done', label: 'Done' }
];

/**
 * Every task by its status, done ones too: one project's own statuses by
 * name and in its order (`statuses`), or across projects by kind, since
 * their statuses differ.
 */
export function statusShares(
	tasks: Task[],
	o: { category: (t: Task) => StatusCategory; statuses?: { id: string; name: string }[] }
): Share[] {
	if (o.statuses) {
		const counts = new Map<string, number>();
		let none = 0;
		for (const t of tasks) {
			if (t.statusId && o.statuses.some((s) => s.id === t.statusId))
				counts.set(t.statusId, (counts.get(t.statusId) ?? 0) + 1);
			else none++;
		}
		const out = o.statuses.map((s) => ({ key: s.id, name: s.name, value: counts.get(s.id) ?? 0 }));
		return none ? [...out, { key: 'none', name: 'No status', value: none }] : out;
	}
	const counts = new Map<StatusCategory, number>();
	for (const t of tasks) {
		const c = o.category(t);
		counts.set(c, (counts.get(c) ?? 0) + 1);
	}
	return KINDS.map((k) => ({ key: k.category, name: k.label, value: counts.get(k.category) ?? 0 }));
}

const PRIORITY_ORDER: { key: Task['priority']; name: string }[] = [
	{ key: 'urgent', name: 'Urgent' },
	{ key: 'high', name: 'High' },
	{ key: 'medium', name: 'Medium' },
	{ key: 'low', name: 'Low' },
	{ key: 'none', name: 'No priority' }
];

/** Open tasks by priority, urgent first. */
export function priorityShares(tasks: Task[], category: (t: Task) => StatusCategory): Share[] {
	const counts = new Map<string, number>();
	for (const t of tasks)
		if (!isDone(category(t))) counts.set(t.priority, (counts.get(t.priority) ?? 0) + 1);
	return PRIORITY_ORDER.map((p) => ({ key: p.key, name: p.name, value: counts.get(p.key) ?? 0 }));
}

/** How late overdue tasks are: from (counted) up to (not counted) days late. */
export const LATENESS: {
	key: string;
	short: string;
	long: string;
	from: number;
	to: number | null;
}[] = [
	{ key: 'week', short: 'Under a week', long: 'Under a week late', from: 1, to: 7 },
	{ key: 'month', short: '1 to 4 weeks', long: '1 to 4 weeks late', from: 7, to: 28 },
	{ key: 'quarter', short: '1 to 3 months', long: '1 to 3 months late', from: 28, to: 91 },
	{ key: 'longer', short: 'Longer', long: 'More than 3 months late', from: 91, to: null }
];

/** Open overdue tasks by how late they are (LATENESS). */
export function overdueAges(
	tasks: Task[],
	o: { today: string; category: (t: Task) => StatusCategory }
): number[] {
	const out = LATENESS.map(() => 0);
	for (const t of tasks) {
		if (isDone(o.category(t)) || !t.dueDate || t.dueDate >= o.today) continue;
		const late = daysBetween(t.dueDate, o.today);
		const i = LATENESS.findIndex((b) => late >= b.from && (b.to === null || late < b.to));
		if (i >= 0) out[i]++;
	}
	return out;
}

/** Open tasks counted under each of a key a task has: its labels, its project. */
export function openBy(
	tasks: Task[],
	o: {
		category: (t: Task) => StatusCategory;
		keys: (t: Task) => string[];
		nameOf: (key: string) => string;
	}
): Share[] {
	const counts = new Map<string, number>();
	for (const t of tasks) {
		if (isDone(o.category(t))) continue;
		for (const k of new Set(o.keys(t))) counts.set(k, (counts.get(k) ?? 0) + 1);
	}
	return [...counts].map(([key, value]) => ({ key, name: o.nameOf(key), value }));
}

export interface DailyFlow {
	days: string[];
	todo: number[];
	inProgress: number[];
	done: number[];
}

/**
 * Tasks to do (backlog too), in progress and done at the end of each day
 * of the period, from when each was made, first went in progress and was
 * done. A task that went back to to do after starting counts as in
 * progress, since only the first start is kept.
 */
export function dailyFlow(
	tasks: Task[],
	o: { period: Period; category: (t: Task) => StatusCategory }
): DailyFlow {
	const days: string[] = [];
	for (let d = o.period.from; d <= o.period.to; d = addDays(d, 1)) days.push(d);
	const index = new Map(days.map((d, i) => [d, i]));
	// Each task adds one from the day it was made, moves from to do to in
	// progress on the day it started, and to done on the day it was done.
	const made = days.map(() => 0);
	const started = days.map(() => 0);
	const finished = days.map(() => 0);
	// The day's place in the period: before it is its first day.
	const at = (day: string) => (day < o.period.from ? 0 : index.get(day)!);
	for (const t of tasks) {
		const m = madeDay(t);
		if (m !== null && m > o.period.to) continue;
		const mi = m === null ? 0 : at(m);
		made[mi]++;
		// Done in the period, or by a time nobody kept (as of when it was made).
		const doneAt = isDone(o.category(t)) ? doneDay(t) : null;
		const done = isDone(o.category(t)) && (doneAt === null || doneAt <= o.period.to);
		const di = done ? Math.max(mi, doneAt === null ? mi : at(doneAt)) : -1;
		const startAt = t.startedAt ? dayOf(t.startedAt) : null;
		let si = startAt === null || startAt > o.period.to ? -1 : Math.max(mi, at(startAt));
		// Never after it was done: a task done with no start, or a later one, leaves to do then.
		if (done && (si < 0 || si > di)) si = di;
		if (si >= 0) started[si]++;
		if (done) finished[di]++;
	}
	const todo: number[] = [];
	const inProgress: number[] = [];
	const doneBy: number[] = [];
	let m = 0,
		s = 0,
		f = 0;
	for (let i = 0; i < days.length; i++) {
		m += made[i];
		s += started[i];
		f += finished[i];
		todo.push(m - s);
		inProgress.push(s - f);
		doneBy.push(f);
	}
	return { days, todo, inProgress, done: doneBy };
}

/** A length of time in days as words: "5 hours", "3.4 days", "12 days". */
export function sayDays(days: number): string {
	if (days < 1) {
		const hours = Math.round(days * 24);
		if (hours < 1) return 'under an hour';
		return hours === 1 ? '1 hour' : `${hours} hours`;
	}
	const n = days < 10 ? Math.round(days * 10) / 10 : Math.round(days);
	return n === 1 ? '1 day' : `${n} days`;
}

const BUCKET_NAMES = [
	{ short: '<1 d', long: 'Under a day' },
	{ short: '1-2 d', long: '1 to 2 days' },
	{ short: '2-4 d', long: '2 to 4 days' },
	{ short: '4-7 d', long: '4 to 7 days' },
	{ short: '1-2 wk', long: '1 to 2 weeks' },
	{ short: '2+ wk', long: '2 weeks or more' }
];

/** A bucket's short name for an axis, and its words for a tooltip or table. */
export function bucketNames(i: number): { short: string; long: string } {
	return BUCKET_NAMES[i];
}

// CSV.

export interface CsvTable {
	title: string;
	head: string[];
	rows: (string | number)[][];
}

/** A tab's tables as one CSV file: each under its title, a blank line between. */
export function tablesCsv(tables: CsvTable[]): string {
	const out: (string | number)[][] = [];
	tables.forEach((t, i) => {
		if (i) out.push([]);
		out.push([t.title], t.head, ...t.rows);
	});
	return writeCsv(out);
}
