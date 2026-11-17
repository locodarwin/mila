// The Time page's report, pure: the period's
// days or weeks, the time logged in it by person, project or label, the
// chart of it by week, and estimates against what finished tasks took.
// Loaded only on that page.
import { writeCsv } from './csv';
import { addDays } from './filter';
import type { Task, TimeEntry } from './sync/entities';

export type PeriodKind =
	| 'week'
	| 'last-week'
	| 'month'
	| 'last-month'
	| 'weeks8'
	| 'weeks12'
	| 'quarter'
	| 'last-quarter'
	| 'range';
export type RowsBy = 'person' | 'project' | 'label';

/** A span of days, both ends counted, as YYYY-MM-DD. */
export interface Period {
	from: string;
	to: string;
}

/** A column of the report: one day, or one week (its first day to its last). */
export interface Column {
	from: string;
	to: string;
}

/** What the report reads beyond the entries. */
export interface ReportSource {
	entries: TimeEntry[];
	/** A live task, or undefined for one in the trash. */
	task: (id: string) => Task | undefined;
	/** Whether a project is there, not in the trash, for time logged to it itself. */
	projectLive: (id: string) => boolean;
	projectName: (id: string | null) => string;
	labels: (taskId: string) => { id: string; name: string }[];
	personName: (userId: string) => string;
}

export interface ReportOptions {
	period: Period;
	rows: RowsBy;
	/** One project's time only, "inbox" for tasks in none, or null for all. */
	project: string | null;
	/** Only this person's time (a guest's own), or null for everyone's. */
	onlyUser: string | null;
	/** The first day of the week, 0 for Sunday. */
	weekStart: number;
}

export interface ReportRow {
	key: string;
	name: string;
	cells: number[];
	total: number;
}

export interface Report {
	columns: Column[];
	rows: ReportRow[];
	/** Each column's total, counting every entry once. */
	totals: number[];
	total: number;
	/** The entries counted, oldest day first. */
	entries: TimeEntry[];
}

const parse = (day: string) => {
	const [y, m, d] = day.split('-').map(Number);
	return new Date(y, m - 1, d);
};
const iso = (d: Date) =>
	`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;

/** The first day of the week a day is in. */
export function weekOf(day: string, weekStart: number): string {
	return addDays(day, -((parse(day).getDay() - weekStart + 7) % 7));
}

/** A named period around today. */
export function periodOf(
	kind: Exclude<PeriodKind, 'range'>,
	today: string,
	weekStart: number
): Period {
	const d = parse(today);
	switch (kind) {
		case 'week': {
			const from = weekOf(today, weekStart);
			return { from, to: addDays(from, 6) };
		}
		case 'last-week': {
			const from = addDays(weekOf(today, weekStart), -7);
			return { from, to: addDays(from, 6) };
		}
		case 'month':
			return {
				from: iso(new Date(d.getFullYear(), d.getMonth(), 1)),
				to: iso(new Date(d.getFullYear(), d.getMonth() + 1, 0))
			};
		case 'last-month':
			return {
				from: iso(new Date(d.getFullYear(), d.getMonth() - 1, 1)),
				to: iso(new Date(d.getFullYear(), d.getMonth(), 0))
			};
		// The weeks up to and including this one.
		case 'weeks8':
		case 'weeks12': {
			const week = weekOf(today, weekStart);
			return { from: addDays(week, (kind === 'weeks8' ? -7 : -11) * 7), to: addDays(week, 6) };
		}
		case 'quarter':
		case 'last-quarter': {
			const first = Math.floor(d.getMonth() / 3) * 3 - (kind === 'last-quarter' ? 3 : 0);
			return {
				from: iso(new Date(d.getFullYear(), first, 1)),
				to: iso(new Date(d.getFullYear(), first + 3, 0))
			};
		}
	}
}

/** How many days a period holds. */
export const lengthOf = (p: Period) =>
	Math.round((parse(p.to).getTime() - parse(p.from).getTime()) / 86_400_000) + 1;

/**
 * The report's columns: a day each for two weeks or less, else a week each
 * (the first and last cut to the period).
 */
export function columnsOf(p: Period, weekStart: number): Column[] {
	const out: Column[] = [];
	if (p.to < p.from) return out;
	if (lengthOf(p) <= 14) {
		for (let day = p.from; day <= p.to; day = addDays(day, 1)) out.push({ from: day, to: day });
		return out;
	}
	for (let start = weekOf(p.from, weekStart); start <= p.to; start = addDays(start, 7)) {
		const end = addDays(start, 6);
		out.push({ from: start < p.from ? p.from : start, to: end > p.to ? p.to : end });
	}
	return out;
}

/** The key and name of the label row for time on a project itself, which has no task to carry labels. */
export const NOT_ON_A_TASK = { key: 'not-on-a-task', name: 'Not on a task' };

/**
 * The entries a report counts: in the period, on live tasks or on projects
 * themselves that are not in the trash, as the options narrow them.
 */
function counted(src: ReportSource, o: ReportOptions): TimeEntry[] {
	return src.entries
		.filter((e) => {
			if (e.day < o.period.from || e.day > o.period.to || e.minutes <= 0) return false;
			if (o.onlyUser && e.userId !== o.onlyUser) return false;
			if (e.taskId === null)
				return (
					!!e.projectId &&
					src.projectLive(e.projectId) &&
					(o.project === null || e.projectId === o.project)
				);
			const t = src.task(e.taskId);
			if (!t) return false;
			return o.project === null || (t.projectId ?? 'inbox') === o.project;
		})
		.sort((a, b) => a.day.localeCompare(b.day) || a.id.localeCompare(b.id));
}

/**
 * The time logged in a period, a row per person, project or label (a
 * task with two labels counts under both; one with none under No label,
 * and time on a project itself under Not on a task), the most first, and
 * a column per day or week.
 */
export function buildReport(src: ReportSource, o: ReportOptions): Report {
	const columns = columnsOf(o.period, o.weekStart);
	const entries = counted(src, o);
	const col = (day: string) => columns.findIndex((c) => day >= c.from && day <= c.to);
	const rows = new Map<string, ReportRow>();
	const totals = columns.map(() => 0);
	let total = 0;
	const add = (key: string, name: string, at: number, minutes: number) => {
		let row = rows.get(key);
		if (!row) {
			row = { key, name, cells: columns.map(() => 0), total: 0 };
			rows.set(key, row);
		}
		row.cells[at] += minutes;
		row.total += minutes;
	};
	for (const e of entries) {
		const at = col(e.day);
		if (at < 0) continue;
		totals[at] += e.minutes;
		total += e.minutes;
		if (o.rows === 'person') {
			add(e.userId, src.personName(e.userId), at, e.minutes);
			continue;
		}
		if (e.taskId === null) {
			// counted() kept only those with a project.
			if (o.rows === 'project') add(e.projectId!, src.projectName(e.projectId), at, e.minutes);
			else add(NOT_ON_A_TASK.key, NOT_ON_A_TASK.name, at, e.minutes);
			continue;
		}
		const t = src.task(e.taskId)!;
		if (o.rows === 'project') {
			const p = t.projectId ?? 'inbox';
			add(p, src.projectName(t.projectId), at, e.minutes);
		} else {
			const labels = src.labels(t.id);
			if (!labels.length) add('', 'No label', at, e.minutes);
			for (const l of labels) add(l.id, l.name, at, e.minutes);
		}
	}
	const sorted = [...rows.values()].sort(
		(a, b) => b.total - a.total || a.name.localeCompare(b.name)
	);
	return { columns, rows: sorted, totals, total, entries };
}

export interface Against {
	key: string;
	name: string;
	/** Finished tasks with an estimate. */
	tasks: number;
	estimate: number;
	/** All the time logged on those tasks, whenever it was. */
	logged: number;
}

/**
 * Estimates against what they took: the tasks finished in the period that
 * had an estimate, by project, with all the time logged on them. A guest's
 * report reads only their own time.
 */
export function estimatesAgainst(
	tasks: Task[],
	src: Pick<ReportSource, 'entries' | 'projectName'>,
	o: Pick<ReportOptions, 'period' | 'project' | 'onlyUser'>,
	localDay: (iso: string) => string
): Against[] {
	const logged = new Map<string, number>();
	// Time on a project itself is on no task, so it has no estimate to meet.
	for (const e of src.entries)
		if (e.taskId !== null && (!o.onlyUser || e.userId === o.onlyUser))
			logged.set(e.taskId, (logged.get(e.taskId) ?? 0) + e.minutes);
	const out = new Map<string, Against>();
	for (const t of tasks) {
		if (t.estimateMinutes === null || !t.completedAt) continue;
		const day = localDay(t.completedAt);
		if (day < o.period.from || day > o.period.to) continue;
		const key = t.projectId ?? 'inbox';
		if (o.project !== null && key !== o.project) continue;
		let row = out.get(key);
		if (!row) {
			row = { key, name: src.projectName(t.projectId), tasks: 0, estimate: 0, logged: 0 };
			out.set(key, row);
		}
		row.tasks++;
		row.estimate += t.estimateMinutes;
		row.logged += logged.get(t.id) ?? 0;
	}
	return [...out.values()].sort((a, b) => b.estimate - a.estimate || a.name.localeCompare(b.name));
}

/** One series of the chart by week: a row's time each week, in minutes. */
export interface WeekSeries {
	key: string;
	name: string;
	values: number[];
	/** The rows past the first few, added together. */
	other?: boolean;
}

/**
 * The chart over a report's columns: the rows with the most time, each its
 * own series, and the rest added together as Others, so that no more than
 * `most` series show. In the report's order, Others last.
 */
export function weeklySeries(report: Report, most = 6): WeekSeries[] {
	const shown = report.rows.length <= most ? report.rows : report.rows.slice(0, most - 1);
	const out: WeekSeries[] = shown.map((r) => ({ key: r.key, name: r.name, values: r.cells }));
	const rest = report.rows.slice(shown.length);
	if (rest.length)
		out.push({
			key: 'others',
			name: `Others (${rest.length})`,
			values: report.columns.map((_, i) => rest.reduce((n, r) => n + r.cells[i], 0)),
			other: true
		});
	return out;
}

/**
 * Whether a column is still under way: today is in it, its last day
 * included, since that day isn't over either.
 */
export const underWay = (c: Column, today: string) => c.from <= today && today <= c.to;

/** Minutes as hours, to two places, for a spreadsheet. */
const hours = (minutes: number) => Math.round((minutes / 60) * 100) / 100;

/**
 * The report's table as CSV, as the page shows it: a line a row, a column a
 * day or week (named by its first day, or both its days), then the total,
 * in hours; and a last line of totals.
 */
export function tableCsv(report: Report, rowsHeading: string): string {
	const name = (c: Column) => (c.from === c.to ? c.from : `${c.from} to ${c.to}`);
	const rows: (string | number)[][] = [
		[rowsHeading, ...report.columns.map(name), 'Total (hours)'],
		...report.rows.map((r) => [r.name, ...r.cells.map(hours), hours(r.total)]),
		['Total', ...report.totals.map(hours), hours(report.total)]
	];
	return writeCsv(rows);
}

/** The report's entries as CSV, a line per entry. */
export function reportCsv(report: Report, src: ReportSource): string {
	const rows: (string | number)[][] = [
		['Day', 'Person', 'Minutes', 'Task', 'Project', 'Labels', 'Note']
	];
	for (const e of report.entries) {
		// Time on a project itself leaves the task and its labels empty.
		const t = e.taskId === null ? undefined : src.task(e.taskId);
		rows.push([
			e.day,
			src.personName(e.userId),
			e.minutes,
			t?.title ?? '',
			src.projectName(e.projectId ?? t?.projectId ?? null),
			e.taskId === null
				? ''
				: src
						.labels(e.taskId)
						.map((l) => l.name)
						.join('; '),
			e.note
		]);
	}
	return writeCsv(rows);
}
