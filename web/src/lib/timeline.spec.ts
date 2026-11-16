import { describe, expect, it } from 'vitest';
import type { Task } from './sync/entities';
import {
	arrowPath,
	dayOf,
	isoOf,
	rangeOf,
	rowsOf,
	scaleOf,
	shapeOf,
	spanUnder,
	step,
	stepped,
	stretched,
	weekdayOf,
	zoomOf
} from './timeline';

const task = (id: string, over: Partial<Task> = {}): Task => ({
	id,
	projectId: 'p',
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

describe('timeline', () => {
	it('reads days both ways', () => {
		expect(isoOf(dayOf('2026-11-01'))).toBe('2026-11-01');
		expect(weekdayOf(dayOf('2026-11-01'))).toBe(0); // a Sunday
		expect(weekdayOf(dayOf('2026-11-02'))).toBe(1);
	});

	it('takes a saved zoom it knows, else Week', () => {
		expect(zoomOf({ timeline: { zoom: 'month' } })).toBe('month');
		expect(zoomOf({ timeline: { zoom: 'decade' } })).toBe('week');
		expect(zoomOf({ timeline: 'month' } as never)).toBe('week');
		expect(zoomOf(null)).toBe('week');
	});

	it('spans today and every date, from the start of a week, month or quarter', () => {
		const week = rangeOf(['2026-12-20'], '2026-11-01', 'week', 1);
		expect(weekdayOf(week.from)).toBe(1);
		expect(week.from).toBeLessThanOrEqual(dayOf('2026-11-01') - 35);
		expect(week.to).toBeGreaterThanOrEqual(dayOf('2026-12-20') + 35);
		const quarter = rangeOf([], '2026-11-01', 'quarter', 1);
		expect(isoOf(quarter.from)).toBe('2026-07-01');
		expect(quarter.to - quarter.from).toBeGreaterThanOrEqual(800);
		const month = rangeOf([], '2026-11-01', 'month', 0);
		expect(isoOf(month.from)).toBe('2026-08-01');
	});

	it('lays out the scale for each zoom', () => {
		const from = dayOf('2026-10-26'); // a Monday
		const to = dayOf('2026-11-15');
		const week = scaleOf(from, to, 'week', 1);
		expect(week.top.map((t) => t.iso)).toEqual(['2026-10-26', '2026-11-01']);
		expect(week.top[0].days).toBe(6);
		expect(week.bottom.map((t) => t.iso)).toEqual(['2026-10-26', '2026-11-02', '2026-11-09']);
		expect(week.weekends.map(isoOf)).toContain('2026-11-01');
		const day = scaleOf(from, to, 'day', 1);
		expect(day.bottom).toHaveLength(21);
		expect(day.bottom.filter((t) => t.weekend)).toHaveLength(6);
		const month = scaleOf(dayOf('2026-08-01'), dayOf('2027-01-31'), 'month', 1);
		expect(month.top.map((t) => t.iso)).toEqual(['2026-08-01', '2027-01-01']);
		expect(month.bottom.map((t) => t.iso)).toHaveLength(6);
		expect(month.weekends).toHaveLength(0);
		const quarter = scaleOf(dayOf('2026-07-01'), dayOf('2027-06-30'), 'quarter', 1);
		expect(quarter.bottom.map((t) => t.iso)).toEqual([
			'2026-07-01',
			'2026-10-01',
			'2027-01-01',
			'2027-04-01'
		]);
	});

	it('draws a bar for two dates and a diamond for one', () => {
		const from = dayOf('2026-11-01');
		expect(shapeOf({ startDate: '2026-11-02', dueDate: '2026-11-05' }, from, 10)).toEqual({
			kind: 'bar',
			left: 10,
			right: 50
		});
		expect(shapeOf({ startDate: null, dueDate: '2026-11-03' }, from, 10)).toMatchObject({
			kind: 'mark',
			center: 25,
			startOnly: false
		});
		expect(shapeOf({ startDate: '2026-11-03', dueDate: null }, from, 10)).toMatchObject({
			startOnly: true
		});
		expect(shapeOf({ startDate: null, dueDate: null }, from, 10)).toBeNull();
	});

	it('steps by the zoom, keeping a task its length', () => {
		expect(step('2026-11-02', 'day', 1)).toBe('2026-11-03');
		expect(step('2026-11-02', 'week', -1)).toBe('2026-10-26');
		expect(step('2026-01-31', 'month', 1)).toBe('2026-02-28');
		expect(stepped({ startDate: '2026-11-02', dueDate: '2026-11-05' }, 'week', 1, 'both')).toEqual({
			startDate: '2026-11-09',
			dueDate: '2026-11-12'
		});
		expect(stepped({ startDate: '2026-11-02', dueDate: '2026-11-05' }, 'day', 1, 'due')).toEqual({
			startDate: '2026-11-02',
			dueDate: '2026-11-06'
		});
		// The due date never goes before the start.
		expect(
			stepped({ startDate: '2026-11-05', dueDate: '2026-11-05' }, 'day', -1, 'due')
		).toBeNull();
		expect(stepped({ startDate: null, dueDate: null }, 'day', 1, 'both')).toBeNull();
	});

	it('stretches an end without crossing the other', () => {
		const t = { startDate: '2026-11-02', dueDate: '2026-11-05' };
		expect(stretched(t, 'start', -2)).toEqual({ startDate: '2026-10-31', dueDate: '2026-11-05' });
		expect(stretched(t, 'start', 9)).toEqual({ startDate: '2026-11-05', dueDate: '2026-11-05' });
		expect(stretched(t, 'due', -9)).toEqual({ startDate: '2026-11-02', dueDate: '2026-11-02' });
		// A diamond pulled out gets its other date.
		expect(stretched({ startDate: null, dueDate: '2026-11-05' }, 'start', -3)).toEqual({
			startDate: '2026-11-02',
			dueDate: '2026-11-05'
		});
	});

	it('shows dated tasks in rows and leaves the rest for the tray', () => {
		const parent = task('parent');
		const a = task('a', { parentId: 'parent', dueDate: '2026-11-03' });
		const b = task('b', { parentId: 'parent' });
		const loose = task('loose');
		const dated = task('dated', { startDate: '2026-11-02', dueDate: '2026-11-04' });
		const kids = (t: Task) => (t.id === 'parent' ? [a, b] : []);
		const sections = [{ id: 's', title: 'To do', tasks: [parent, loose, dated] }];

		const open = rowsOf(sections, kids, new Set(), true);
		expect(open.rows.map((r) => r.key)).toEqual(['g:s', 'parent', 'a', 'dated']);
		expect(open.rows[0]).toMatchObject({ kind: 'group', count: 2 });
		expect(open.unscheduled.map((t) => t.id)).toEqual(['b', 'loose']);

		const shut = rowsOf(sections, kids, new Set(['parent']), false);
		expect(shut.rows.map((r) => r.key)).toEqual(['parent', 'dated']);
		expect(shut.rows[0]).toMatchObject({ folded: true, kids: 2 });
		expect(shut.unscheduled.map((t) => t.id)).toEqual(['b', 'loose']);
		expect(spanUnder(parent, kids)).toEqual({ from: '2026-11-03', to: '2026-11-03' });
	});

	it('routes arrows straight when there is room, around when not', () => {
		expect(arrowPath(10, 18, 100, 54, 36)).toBe('M10 18 H18 V54 H98');
		expect(arrowPath(100, 18, 90, 54, 36)).toBe('M100 18 H108 V36 H80 V54 H88');
	});
});
