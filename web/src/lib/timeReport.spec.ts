import { describe, expect, it } from 'vitest';
import type { Task, TimeEntry } from './sync/entities';
import {
	buildReport,
	columnsOf,
	estimatesAgainst,
	periodOf,
	reportCsv,
	tableCsv,
	underWay,
	weekOf,
	weeklySeries,
	type Report,
	type ReportSource
} from './timeReport';

const task = (id: string, over: Partial<Task> = {}): Task => ({
	id,
	projectId: 'p1',
	statusId: null,
	parentId: null,
	title: id,
	notes: '',
	sortKey: 'a0',
	completedAt: null,
	priority: 'none',
	startDate: null,
	dueDate: null,
	dueTime: null,
	createdBy: null,
	recurrence: null,
	recurrenceMode: 'schedule',
	seriesId: null,
	estimateMinutes: null,
	madeAt: null,
	startedAt: null,
	...over
});
let n = 0;
const entry = (
	taskId: string,
	userId: string,
	day: string,
	minutes: number,
	note = ''
): TimeEntry => ({
	id: `e${++n}`,
	taskId,
	projectId: null,
	userId,
	day,
	minutes,
	note,
	startedAt: null,
	running: false,
	activeAt: null
});

const tasks = [
	task('wire', {
		title: 'Wire the outlets',
		estimateMinutes: 360,
		completedAt: '2026-10-30T15:00:00Z'
	}),
	task('order', {
		title: 'Order the counter',
		estimateMinutes: 120,
		completedAt: '2026-10-29T15:00:00Z'
	}),
	task('beds', { title: 'Weed the beds', projectId: 'p2' }),
	task('inbox', { title: '=SUM(A1)', projectId: null })
];
const labels: Record<string, { id: string; name: string }[]> = {
	wire: [{ id: 'l-el', name: 'Electrical' }],
	order: [
		{ id: 'l-el', name: 'Electrical' },
		{ id: 'l-buy', name: 'Ordering' }
	]
};
const src = (entries: TimeEntry[]): ReportSource => ({
	entries,
	task: (id) => tasks.find((t) => t.id === id),
	projectLive: (id) => id !== 'trashed',
	projectName: (id) => ({ p1: 'Kitchen', p2: 'Garden' })[id ?? ''] ?? 'Inbox',
	labels: (id) => labels[id] ?? [],
	personName: (id) => ({ u1: 'Shawn', u2: 'Sam' })[id] ?? 'Someone'
});

describe('periods', () => {
	it('start weeks on the day chosen and months on the first', () => {
		// 2026-11-01 is a Sunday.
		expect(weekOf('2026-11-01', 1)).toBe('2026-10-26');
		expect(weekOf('2026-11-01', 0)).toBe('2026-11-01');
		expect(periodOf('week', '2026-11-01', 1)).toEqual({ from: '2026-10-26', to: '2026-11-01' });
		expect(periodOf('last-week', '2026-11-01', 1)).toEqual({
			from: '2026-10-19',
			to: '2026-10-25'
		});
		expect(periodOf('month', '2026-11-01', 1)).toEqual({ from: '2026-11-01', to: '2026-11-30' });
		expect(periodOf('last-month', '2026-03-10', 1)).toEqual({
			from: '2026-02-01',
			to: '2026-02-28'
		});
	});

	it('count back whole weeks to this one, and quarters from their first month', () => {
		// Tuesday 2026-11-10, weeks from Monday.
		expect(periodOf('weeks8', '2026-11-10', 1)).toEqual({ from: '2026-09-21', to: '2026-11-15' });
		expect(periodOf('weeks12', '2026-11-10', 1)).toEqual({ from: '2026-08-24', to: '2026-11-15' });
		expect(columnsOf(periodOf('weeks8', '2026-11-10', 1), 1)).toHaveLength(8);
		expect(columnsOf(periodOf('weeks12', '2026-11-10', 0), 0)).toHaveLength(12);
		expect(periodOf('quarter', '2026-11-10', 1)).toEqual({ from: '2026-10-01', to: '2026-12-31' });
		expect(periodOf('last-quarter', '2026-11-10', 1)).toEqual({
			from: '2026-07-01',
			to: '2026-09-30'
		});
		// The year before, for the first quarter.
		expect(periodOf('last-quarter', '2026-02-03', 1)).toEqual({
			from: '2025-10-01',
			to: '2025-12-31'
		});
	});

	it('say which column is still under way', () => {
		const week = { from: '2026-11-09', to: '2026-11-15' };
		expect(underWay(week, '2026-11-10')).toBe(true);
		// Its last day too: the week isn't over till that day is.
		expect(underWay(week, '2026-11-15')).toBe(true);
		expect(underWay(week, '2026-11-16')).toBe(false);
		expect(underWay(week, '2026-11-08')).toBe(false);
		expect(underWay({ from: '2026-11-10', to: '2026-11-10' }, '2026-11-10')).toBe(true);
	});

	it('have a column a day up to two weeks, then a week each, cut to the period', () => {
		expect(columnsOf({ from: '2026-10-26', to: '2026-11-01' }, 1)).toHaveLength(7);
		const month = columnsOf({ from: '2026-10-01', to: '2026-10-31' }, 1);
		expect(month[0]).toEqual({ from: '2026-10-01', to: '2026-10-04' });
		expect(month[1]).toEqual({ from: '2026-10-05', to: '2026-10-11' });
		expect(month.at(-1)).toEqual({ from: '2026-10-26', to: '2026-10-31' });
		expect(columnsOf({ from: '2026-10-05', to: '2026-10-01' }, 1)).toEqual([]);
	});
});

/** Time logged to a project itself. */
const own = (projectId: string, userId: string, day: string, minutes: number): TimeEntry => ({
	...entry('', userId, day, minutes),
	taskId: null,
	projectId
});

describe('time on a project itself', () => {
	const entries = [
		entry('wire', 'u1', '2026-10-26', 90),
		own('p1', 'u2', '2026-10-27', 60),
		own('p2', 'u1', '2026-10-28', 30),
		own('trashed', 'u1', '2026-10-28', 500),
		own('p1', 'u1', '2026-10-20', 999)
	];
	const base = {
		period: { from: '2026-10-26', to: '2026-11-01' },
		project: null,
		onlyUser: null,
		weekStart: 1
	};

	it('counts with its project and its person, and under Not on a task by label', () => {
		const byProject = buildReport(src(entries), { ...base, rows: 'project' });
		expect(byProject.total).toBe(180);
		expect(byProject.rows.map((x) => [x.name, x.total])).toEqual([
			['Kitchen', 150],
			['Garden', 30]
		]);
		const byPerson = buildReport(src(entries), { ...base, rows: 'person' });
		expect(byPerson.rows.map((x) => [x.name, x.total])).toEqual([
			['Shawn', 120],
			['Sam', 60]
		]);
		const byLabel = buildReport(src(entries), { ...base, rows: 'label' });
		expect(byLabel.rows.map((x) => [x.name, x.total])).toEqual([
			['Electrical', 90],
			['Not on a task', 90]
		]);
		expect(buildReport(src(entries), { ...base, rows: 'project', project: 'p1' }).total).toBe(150);
		expect(buildReport(src(entries), { ...base, rows: 'project', project: 'inbox' }).total).toBe(0);
	});

	it('leaves estimates alone, and its task empty in the CSV', () => {
		const against = estimatesAgainst(tasks, src(entries), base, (iso) => iso.slice(0, 10));
		expect(against.find((a) => a.key === 'p1')?.logged).toBe(90);
		const report = buildReport(src(entries), { ...base, rows: 'project' });
		const lines = reportCsv(report, src(entries)).trim().split(/\r?\n/);
		expect(lines).toContain('2026-10-27,Sam,60,,Kitchen,,');
	});
});

describe('the report', () => {
	const entries = [
		entry('wire', 'u1', '2026-10-26', 90),
		entry('wire', 'u2', '2026-10-27', 60),
		entry('order', 'u1', '2026-10-27', 30),
		entry('beds', 'u2', '2026-10-28', 45),
		entry('gone', 'u1', '2026-10-28', 500),
		entry('wire', 'u1', '2026-10-25', 999),
		entry('inbox', 'u1', '2026-11-01', 15, 'Called "them", twice')
	];
	const period = { from: '2026-10-26', to: '2026-11-01' };
	const base = { period, project: null, onlyUser: null, weekStart: 1 };

	it('sums time by person, by day, the most first, leaving out the trash and other days', () => {
		const r = buildReport(src(entries), { ...base, rows: 'person' });
		expect(r.total).toBe(240);
		expect(r.rows.map((x) => [x.name, x.total])).toEqual([
			['Shawn', 135],
			['Sam', 105]
		]);
		expect(r.rows[0].cells).toEqual([90, 30, 0, 0, 0, 0, 15]);
		expect(r.totals).toEqual([90, 90, 45, 0, 0, 0, 15]);
	});

	it('sums by project, the inbox too, and narrows to one project or one person', () => {
		const r = buildReport(src(entries), { ...base, rows: 'project' });
		expect(r.rows.map((x) => [x.name, x.total])).toEqual([
			['Kitchen', 180],
			['Garden', 45],
			['Inbox', 15]
		]);
		expect(buildReport(src(entries), { ...base, rows: 'project', project: 'p2' }).total).toBe(45);
		expect(buildReport(src(entries), { ...base, rows: 'person', onlyUser: 'u2' }).total).toBe(105);
	});

	it('counts a task with two labels under both, and the total once', () => {
		const r = buildReport(src(entries), { ...base, rows: 'label' });
		expect(r.rows.map((x) => [x.name, x.total])).toEqual([
			['Electrical', 180],
			['No label', 60],
			['Ordering', 30]
		]);
		expect(r.total).toBe(240);
	});

	it('writes a line per entry as CSV, quoted and kept from running as a formula', () => {
		const r = buildReport(src(entries), { ...base, rows: 'person' });
		const csv = reportCsv(r, src(entries)).split('\r\n');
		expect(csv[0]).toBe('Day,Person,Minutes,Task,Project,Labels,Note');
		// A ; is quoted, so Excel in a ; locale keeps the labels in one cell.
		expect(csv[3]).toBe('2026-10-27,Shawn,30,Order the counter,Kitchen,"Electrical; Ordering",');
		expect(csv[5]).toBe(`2026-11-01,Shawn,15,'=SUM(A1),Inbox,,"Called ""them"", twice"`);
		expect(csv).toHaveLength(7);
	});
});

describe('estimates against what they took', () => {
	it('counts tasks finished in the period with an estimate, and all their time', () => {
		const entries = [
			entry('wire', 'u1', '2026-10-20', 300),
			entry('wire', 'u2', '2026-10-27', 100),
			entry('order', 'u1', '2026-10-27', 90)
		];
		const day = (iso: string) => iso.slice(0, 10);
		const o = { period: { from: '2026-10-26', to: '2026-11-01' }, project: null, onlyUser: null };
		expect(estimatesAgainst(tasks, src(entries), o, day)).toEqual([
			{ key: 'p1', name: 'Kitchen', tasks: 2, estimate: 480, logged: 490 }
		]);
		expect(estimatesAgainst(tasks, src(entries), { ...o, onlyUser: 'u1' }, day)[0].logged).toBe(
			390
		);
		expect(
			estimatesAgainst(
				tasks,
				src(entries),
				{ ...o, period: { from: '2026-10-30', to: '2026-10-31' } },
				day
			)[0]
		).toMatchObject({ tasks: 1, estimate: 360 });
		expect(estimatesAgainst(tasks, src(entries), { ...o, project: 'p2' }, day)).toEqual([]);
	});
});

describe('the chart and table by week', () => {
	const row = (key: string, total: number) => ({ key, name: key, cells: [total, 0], total });
	const report = (n: number): Report => ({
		columns: [
			{ from: '2026-11-02', to: '2026-11-08' },
			{ from: '2026-11-09', to: '2026-11-15' }
		],
		rows: Array.from({ length: n }, (_, i) => row(`r${i}`, 100 - i)),
		totals: [0, 0],
		total: 0,
		entries: []
	});

	it('shows each row while six fit, and the rest as Others past five', () => {
		expect(weeklySeries(report(6)).map((s) => s.key)).toEqual(['r0', 'r1', 'r2', 'r3', 'r4', 'r5']);
		const many = weeklySeries(report(8));
		expect(many.map((s) => s.name)).toEqual(['r0', 'r1', 'r2', 'r3', 'r4', 'Others (3)']);
		expect(many.at(-1)).toMatchObject({ other: true, values: [95 + 94 + 93, 0] });
		expect(weeklySeries(report(0))).toEqual([]);
	});

	it('writes the table as CSV in hours, a column a week, with totals', () => {
		const entries = [
			entry('wire', 'u1', '2026-11-03', 90),
			entry('wire', 'u2', '2026-11-10', 45),
			entry('order', 'u1', '2026-11-11', 20)
		];
		const r = buildReport(src(entries), {
			period: { from: '2026-11-02', to: '2026-11-22' },
			rows: 'person',
			project: null,
			onlyUser: null,
			weekStart: 1
		});
		expect(tableCsv(r, 'Person').split('\r\n')).toEqual([
			'Person,2026-11-02 to 2026-11-08,2026-11-09 to 2026-11-15,2026-11-16 to 2026-11-22,Total (hours)',
			'Shawn,1.5,0.33,0,1.83',
			'Sam,0,0.75,0,0.75',
			'Total,1.5,1.08,0,2.58',
			''
		]);
	});
});
