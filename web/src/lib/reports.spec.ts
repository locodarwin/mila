import { describe, expect, it } from 'vitest';
import { parseCsv } from './csv';
import {
	bucketNames,
	dailyFlow,
	dayOf,
	flowReport,
	foldShares,
	openBy,
	overdueAges,
	priorityShares,
	ranked,
	statusShares,
	inScope,
	median,
	percentile,
	periodOf,
	sayDays,
	statusReport,
	tablesCsv,
	weeksOf,
	workloadReport,
	type ReportSource
} from './reports';
import type { StatusCategory, Task } from './sync/entities';

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

/** A moment on a day where the device is, so days come out the same in any time zone. */
const at = (day: string, hour = 12) => {
	const [y, m, d] = day.split('-').map(Number);
	return new Date(y, m - 1, d, hour).toISOString();
};

// Thursday, November 5, 2026; weeks start on Sunday unless a test says.
const TODAY = '2026-11-05';
const SUNDAY = 0;
const MONDAY = 1;

/** A task's kind from its status ID, as the filter context has it. */
const category = (t: Task): StatusCategory =>
	(t.statusId as StatusCategory | null) ?? (t.completedAt ? 'done' : 'todo');

describe('periods and weeks', () => {
	it('takes the last 6 weeks whole, this week the sixth', () => {
		expect(periodOf('weeks6', TODAY, SUNDAY)).toEqual({ from: '2026-09-27', to: TODAY });
		expect(periodOf('weeks6', TODAY, MONDAY)).toEqual({ from: '2026-09-28', to: TODAY });
		expect(weeksOf(periodOf('weeks6', TODAY, SUNDAY), SUNDAY)).toHaveLength(6);
	});

	it('takes about 3 months in whole weeks, and this year from January 1', () => {
		const three = periodOf('months3', TODAY, SUNDAY);
		expect(three).toEqual({ from: '2026-08-02', to: TODAY });
		expect(weeksOf(three, SUNDAY)[0]).toEqual({ from: '2026-08-02', to: '2026-08-08' });
		expect(periodOf('year', TODAY, SUNDAY)).toEqual({ from: '2026-01-01', to: TODAY });
	});

	it('cuts the first and last weeks to the period', () => {
		const weeks = weeksOf({ from: '2026-01-01', to: TODAY }, SUNDAY);
		expect(weeks[0]).toEqual({ from: '2026-01-01', to: '2026-01-03' });
		expect(weeks.at(-1)).toEqual({ from: '2026-11-01', to: TODAY });
		expect(weeks).toHaveLength(45);
	});

	it('puts picked days in order, and no later than today', () => {
		expect(periodOf('range', TODAY, SUNDAY, { from: '2026-10-20', to: '2026-10-01' })).toEqual({
			from: '2026-10-01',
			to: '2026-10-20'
		});
		expect(periodOf('range', TODAY, SUNDAY, { from: '2026-10-01', to: '2027-01-01' })).toEqual({
			from: '2026-10-01',
			to: TODAY
		});
		expect(periodOf('range', TODAY, SUNDAY, { from: '2027-01-01', to: '2027-02-01' })).toEqual({
			from: TODAY,
			to: TODAY
		});
		// Nothing picked yet: six weeks back.
		expect(periodOf('range', TODAY, SUNDAY, { from: null, to: null }).to).toBe(TODAY);
	});
});

describe('inScope', () => {
	it('leaves out the trash, archived projects, canceled tasks and what the filter drops', () => {
		const tasks = [
			task('open'),
			task('inbox', { projectId: null }),
			task('archived', { projectId: 'p-old' }),
			task('canceled', { statusId: 'canceled', completedAt: at(TODAY) }),
			task('under-trash', { parentId: 'gone' }),
			task('sub', { parentId: 'open' }),
			task('dropped')
		];
		const live = new Map(tasks.map((t) => [t.id, t]));
		const src: ReportSource = {
			tasks,
			task: (id) => live.get(id),
			projectInUse: (id) => id === 'p1',
			category,
			keep: (t) => t.id !== 'dropped'
		};
		expect(inScope(src).map((t) => t.id)).toEqual(['open', 'inbox', 'sub']);
	});
});

describe('statusReport', () => {
	const options = {
		period: periodOf('weeks6', TODAY, SUNDAY),
		today: TODAY,
		weekStart: SUNDAY,
		category,
		openBlockers: (id: string) => (id === 'waits' ? 2 : 0),
		logged: (id: string) => (id === 'over' ? 150 : 30)
	};

	it('counts what is open and done now, and what is overdue or due soon', () => {
		const r = statusReport(
			[
				task('late', { dueDate: '2026-11-02' }),
				task('today', { dueDate: TODAY }),
				task('next-week', { dueDate: '2026-11-14', statusId: 'in-progress' }),
				task('after', { dueDate: '2026-11-15', statusId: 'backlog' }),
				task('done', { completedAt: at('2026-11-01') })
			],
			options
		);
		expect([r.open, r.done, r.total, r.overdue, r.dueSoon]).toEqual([4, 1, 5, 1, 2]);
		expect(r.byStatus.map((s) => [s.category, s.count])).toEqual([
			['backlog', 1],
			['todo', 2],
			['in-progress', 1]
		]);
	});

	it('draws the burnup from when tasks were made and done', () => {
		const r = statusReport(
			[
				// Made before the period and done in its second week.
				task('a', { madeAt: at('2026-09-01'), completedAt: at('2026-10-06') }),
				// Made in the fourth week, still open.
				task('b', { madeAt: at('2026-10-19') }),
				// Nobody knows when it was made: there all along.
				task('c'),
				// Reopened: its completion is gone, so it was never done here.
				task('d', { madeAt: at('2026-09-28') }),
				// Done before the day it says it was made (an import): made when done.
				task('e', { madeAt: at('2026-11-04'), completedAt: at('2026-10-27') })
			],
			options
		);
		expect(r.weeks.map((w) => w.from)).toEqual([
			'2026-09-27',
			'2026-10-04',
			'2026-10-11',
			'2026-10-18',
			'2026-10-25',
			'2026-11-01'
		]);
		expect(r.scope).toEqual([3, 3, 3, 4, 5, 5]);
		expect(r.doneBy).toEqual([0, 1, 1, 1, 2, 2]);
	});

	it('lists what needs attention, the latest first', () => {
		const r = statusReport(
			[
				task('fine'),
				task('over', { estimateMinutes: 120 }),
				task('waits', { title: 'Waits' }),
				task('late', { dueDate: '2026-11-03', estimateMinutes: 10 }),
				task('later', { dueDate: '2026-10-29' }),
				task('done late', { dueDate: '2026-10-01', completedAt: at('2026-10-02') })
			],
			options
		);
		expect(r.attention.map((a) => [a.task.id, a.late, a.waiting, a.over])).toEqual([
			['later', 7, 0, null],
			['late', 2, 0, 20],
			['waits', null, 2, null],
			['over', null, 0, 30]
		]);
	});
});

describe('workloadReport', () => {
	const assigned: Record<string, string[]> = {
		a: ['u-dana'],
		b: ['u-dana', 'u-marcus'],
		c: ['u-marcus'],
		d: [],
		e: ['u-dana']
	};
	const names: Record<string, string> = { 'u-dana': 'Dana', 'u-marcus': 'Marcus' };
	const r = workloadReport(
		[
			task('a', { estimateMinutes: 120, dueDate: '2026-11-02' }),
			task('b', { estimateMinutes: 60, dueDate: TODAY }),
			task('c', { dueDate: '2026-11-09' }),
			task('d', { estimateMinutes: 30, dueDate: '2026-12-20' }),
			task('e', { estimateMinutes: 600, completedAt: at('2026-11-01') })
		],
		{
			today: TODAY,
			weekStart: SUNDAY,
			category,
			assignees: (id) => assigned[id] ?? [],
			nameOf: (id) => names[id]
		}
	);

	it('counts open work by person, a task with two assignees for each, nobody last', () => {
		expect(r.people.map((p) => [p.userId, p.minutes, p.tasks, p.overdue, p.unestimated])).toEqual([
			['u-dana', 180, 2, 1, 0],
			['u-marcus', 60, 2, 0, 1],
			[null, 30, 1, 0, 0]
		]);
		expect([r.open, r.unestimated, r.shared]).toEqual([4, 1, 1]);
	});

	it('counts open tasks by the week they are due, overdue first', () => {
		expect(r.due.map((b) => [b.kind, b.from, b.count])).toEqual([
			['overdue', null, 1],
			['this', '2026-11-01', 1],
			['next', '2026-11-08', 1],
			['week', '2026-11-15', 0],
			['week', '2026-11-22', 0],
			['later', '2026-11-29', 1]
		]);
		expect(r.undated).toBe(0);
	});
});

describe('flowReport', () => {
	const period = { from: '2026-10-18', to: TODAY };
	const r = flowReport(
		[
			task('a', {
				madeAt: at('2026-10-19'),
				startedAt: at('2026-10-20', 9),
				completedAt: at('2026-10-20', 15)
			}),
			task('b', {
				madeAt: at('2026-10-19'),
				startedAt: at('2026-10-21'),
				completedAt: at('2026-10-24')
			}),
			task('c', { madeAt: at('2026-10-26'), startedAt: at('2026-10-26'), completedAt: at(TODAY) }),
			// Done without starting.
			task('d', { madeAt: at('2026-10-01'), completedAt: at('2026-10-27') }),
			// Done before it started again: no time of its own.
			task('e', { startedAt: at('2026-11-02'), completedAt: at('2026-10-28') }),
			// Done before the period.
			task('f', { startedAt: at('2026-09-01'), completedAt: at('2026-09-02') }),
			// Open.
			task('g', { madeAt: at(TODAY) })
		],
		{ period, weekStart: SUNDAY, category }
	);

	it('counts what was made and done each week', () => {
		expect(r.weeks.map((w) => w.from)).toEqual(['2026-10-18', '2026-10-25', '2026-11-01']);
		expect(r.made).toEqual([2, 1, 1]);
		expect(r.done).toEqual([2, 2, 1]);
		expect(r.doneTotal).toBe(5);
		expect(r.perWeek).toBeCloseTo(5 / 3);
	});

	it('times tasks from first in progress to done, leaving out those that never started', () => {
		// Elapsed time: a clock change on the way adds or takes an hour.
		expect(r.cycle.map((d) => Math.round(d * 10) / 10)).toEqual([0.3, 3, 10]);
		expect(r.unstarted).toBe(2);
		expect(r.median).toBe(3);
		expect(r.p80).toBeCloseTo(10, 0);
		expect(r.buckets.map((b) => b.count)).toEqual([1, 0, 1, 0, 1, 0]);
	});

	it('says nothing of a period with nothing done', () => {
		const none = flowReport([task('x')], { period, weekStart: SUNDAY, category });
		expect([none.doneTotal, none.median, none.p80, none.perWeek]).toEqual([0, null, null, 0]);
	});
});

describe('words and numbers', () => {
	it('finds the median and the nearest-rank share', () => {
		expect(median([])).toBeNull();
		expect(median([1, 2, 3, 10])).toBe(2.5);
		expect(percentile([1, 2, 3, 4, 5, 6, 7, 8, 9, 10], 0.8)).toBe(8);
		expect(percentile([4], 0.8)).toBe(4);
	});

	it('says lengths of time in hours or days', () => {
		expect(sayDays(0.01)).toBe('under an hour');
		expect(sayDays(1 / 24)).toBe('1 hour');
		expect(sayDays(0.25)).toBe('6 hours');
		expect(sayDays(1)).toBe('1 day');
		expect(sayDays(3.44)).toBe('3.4 days');
		expect(sayDays(12.6)).toBe('13 days');
	});

	it('names the cycle time buckets', () => {
		expect(bucketNames(0)).toEqual({ short: '<1 d', long: 'Under a day' });
		expect(bucketNames(5).long).toBe('2 weeks or more');
	});

	it('reads a moment as the device day it falls on', () => {
		expect(dayOf(at('2026-11-05', 23))).toBe('2026-11-05');
	});

	it('writes a tab as CSV a spreadsheet reads, its tables under their titles', () => {
		const csv = tablesCsv([
			{ title: 'Burnup', head: ['Week of', 'Done'], rows: [['2026-11-01', 3]] },
			{ title: 'Needs attention', head: ['Task'], rows: [['=SUM(A1)'], ['Say "hi", then go']] }
		]);
		expect(csv.endsWith('\r\n')).toBe(true);
		expect(parseCsv(csv)).toEqual([
			['Burnup'],
			['Week of', 'Done'],
			['2026-11-01', '3'],
			['Needs attention'],
			['Task'],
			["'=SUM(A1)"],
			['Say "hi", then go']
		]);
	});
});

describe("the project picker's charts", () => {
	const cat = (t: Task): StatusCategory =>
		t.statusId === 'done' || t.completedAt
			? 'done'
			: t.statusId === 'doing'
				? 'in-progress'
				: t.statusId === 'later'
					? 'backlog'
					: 'todo';

	it('fold a whole into six parts at most, the smallest into Other', () => {
		const parts = 'abcdefgh'.split('').map((k, i) => ({ key: k, name: k, value: 8 - i }));
		expect(foldShares(parts).map((p) => [p.name, p.value])).toEqual([
			['a', 8],
			['b', 7],
			['c', 6],
			['d', 5],
			['e', 4],
			['Other', 6]
		]);
		// In their order, and without the empty ones.
		expect(
			foldShares([
				{ key: 'x', name: 'x', value: 1 },
				{ key: 'y', name: 'y', value: 0 },
				{ key: 'z', name: 'z', value: 3 }
			]).map((p) => p.key)
		).toEqual(['x', 'z']);
		expect(ranked(parts, 3).map((p) => p.key)).toEqual(['a', 'b', 'c']);
	});

	it('count statuses as one project has them, or by kind across projects', () => {
		const tasks = [
			task('a', { statusId: 'todo' }),
			task('b', { statusId: 'doing' }),
			task('c', { statusId: 'doing' }),
			task('d', { statusId: 'done' }),
			task('e', { statusId: 'gone' })
		];
		const own = [
			{ id: 'todo', name: 'To do' },
			{ id: 'doing', name: 'Building' },
			{ id: 'review', name: 'Review' },
			{ id: 'done', name: 'Done' }
		];
		expect(
			statusShares(tasks, { category: cat, statuses: own }).map((s) => [s.name, s.value])
		).toEqual([
			['To do', 1],
			['Building', 2],
			['Review', 0],
			['Done', 1],
			['No status', 1]
		]);
		expect(statusShares(tasks, { category: cat }).map((s) => [s.name, s.value])).toEqual([
			['Backlog', 0],
			['To do', 2],
			['In progress', 2],
			['Done', 1]
		]);
	});

	it('count open work by priority, overdue work by how late, and open work by label', () => {
		const tasks = [
			task('a', { priority: 'urgent', dueDate: '2026-11-04' }),
			task('b', { priority: 'urgent', dueDate: '2026-10-29' }),
			task('c', { priority: 'low', dueDate: '2026-09-01' }),
			task('d', { priority: 'high', dueDate: '2026-01-01', statusId: 'done' }),
			task('e', { dueDate: '2026-06-01' }),
			task('f', { dueDate: TODAY })
		];
		expect(priorityShares(tasks, cat).map((p) => p.value)).toEqual([2, 0, 0, 1, 2]);
		// 1 day, 7 days, 65 days and 157 days late; d is done and f is due today.
		expect(overdueAges(tasks, { today: TODAY, category: cat })).toEqual([1, 1, 1, 1]);
		const labels: Record<string, string[]> = { a: ['x', 'y'], b: ['x'], d: ['x'] };
		expect(
			ranked(
				openBy(tasks, {
					category: cat,
					keys: (t) => labels[t.id] ?? [],
					nameOf: (k) => k.toUpperCase()
				})
			).map((p) => [p.name, p.value])
		).toEqual([
			['X', 2],
			['Y', 1]
		]);
	});

	it('follow each task from to do through in progress to done, day by day', () => {
		const period = { from: '2026-11-01', to: '2026-11-05' };
		const tasks = [
			// Made before the period, started on the 2nd, done on the 4th.
			task('a', {
				madeAt: at('2026-10-20'),
				startedAt: at('2026-11-02'),
				statusId: 'done',
				completedAt: at('2026-11-04')
			}),
			// Made on the 3rd, still to do.
			task('b', { madeAt: at('2026-11-03') }),
			// Done on the 2nd without ever starting.
			task('c', { madeAt: at('2026-10-01'), statusId: 'done', completedAt: at('2026-11-02') }),
			// Done by a time nobody kept: done all along.
			task('d', { statusId: 'done', startedAt: at('2026-11-03') }),
			// Made after the period: not there.
			task('e', { madeAt: at('2026-11-06') })
		];
		const f = dailyFlow(tasks, { period, category: cat });
		expect(f.days).toEqual(['2026-11-01', '2026-11-02', '2026-11-03', '2026-11-04', '2026-11-05']);
		expect(f.todo).toEqual([2, 0, 1, 1, 1]);
		expect(f.inProgress).toEqual([0, 1, 1, 0, 0]);
		expect(f.done).toEqual([1, 2, 2, 3, 3]);
		// Each day's three add up to the tasks there that day.
		f.days.forEach((_, i) =>
			expect(f.todo[i] + f.inProgress[i] + f.done[i]).toBeGreaterThanOrEqual(0)
		);
	});

	it('give each finished task its own time for the scatter', () => {
		const r = flowReport(
			[
				task('a', {
					startedAt: at('2026-11-01', 9),
					statusId: 'done',
					completedAt: at('2026-11-03', 9)
				}),
				task('b', { statusId: 'done', completedAt: at('2026-11-02') })
			],
			{ period: { from: '2026-11-01', to: TODAY }, weekStart: 0, category: cat }
		);
		expect(r.dots.map((d) => [d.task.id, d.day, d.days])).toEqual([['a', '2026-11-03', 2]]);
		expect(r.unstarted).toBe(1);
	});
});

describe('a picked period a stranger set', () => {
	it('is at most five years, and a day before 2000 is not read at all', async () => {
		const { MAX_RANGE_DAYS } = await import('./reports');
		const { readSetup } = await import('./reportSetup.svelte');
		const { addDays } = await import('./filter');
		const p = periodOf('range', '2026-11-08', 1, { from: '2000-01-01', to: null });
		expect(p.to).toBe('2026-11-08');
		expect(p.from).toBe(addDays('2026-11-08', -MAX_RANGE_DAYS));
		expect(readSetup(JSON.stringify({ period: 'range', from: '1000-01-10' })).from).toBeNull();
		expect(readSetup(JSON.stringify({ period: 'range', from: '2026-01-05' })).from).toBe(
			'2026-01-05'
		);
		// Years are written with four digits, so they compare as text.
		expect(addDays('0999-12-27', 7)).toBe('1000-01-03');
	});
});
