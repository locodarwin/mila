import { describe, expect, it } from 'vitest';
import { barPath, columnPath, labelEvery, nearest, niceScale, stepMark } from './charts';
import { readSetup, reportDisplay, viewReport } from './reportSetup.svelte';
import { REPORT_CHARTS, REPORT_TABS, chartOf } from './reports';

describe('niceScale', () => {
	it('rounds the axis up to a round number in a few whole steps', () => {
		expect(niceScale(64)).toEqual({ max: 80, step: 20, ticks: [0, 20, 40, 60, 80] });
		expect(niceScale(3)).toEqual({ max: 3, step: 1, ticks: [0, 1, 2, 3] });
		expect(niceScale(13).max).toBe(15);
		expect(niceScale(10)).toEqual({ max: 10, step: 5, ticks: [0, 5, 10] });
		expect(niceScale(100)).toEqual({ max: 100, step: 25, ticks: [0, 25, 50, 75, 100] });
	});

	it('draws an empty chart with an axis of its own', () => {
		expect(niceScale(0)).toEqual({ max: 4, step: 1, ticks: [0, 1, 2, 3, 4] });
	});
});

describe('shapes', () => {
	it('rounds a column at the top only', () => {
		expect(columnPath(10, 20, 24, 100)).toBe('M10,100V24Q10,20 14,20H30Q34,20 34,24V100Z');
		// A short column rounds no more than it is tall.
		expect(columnPath(0, 98, 24, 100)).toBe('M0,100V100Q0,98 2,98H22Q24,98 24,100V100Z');
		expect(columnPath(0, 100, 24, 100)).toBe('');
	});

	it('rounds a bar at its end only', () => {
		expect(barPath(0, 0, 50, 20)).toBe('M0,0H46Q50,0 50,4V16Q50,20 46,20H0Z');
		expect(barPath(0, 0, 0, 20)).toBe('');
	});
});

describe('stepping through marks', () => {
	it('moves with the arrows, stops at the ends, and jumps with Home and End', () => {
		expect(stepMark('ArrowRight', null, 5)).toBe(0);
		expect(stepMark('ArrowLeft', null, 5)).toBe(4);
		expect(stepMark('ArrowRight', 4, 5)).toBe(4);
		expect(stepMark('ArrowDown', 1, 5)).toBe(2);
		expect(stepMark('ArrowUp', 0, 5)).toBe(0);
		expect(stepMark('Home', 3, 5)).toBe(0);
		expect(stepMark('End', 0, 5)).toBe(4);
		expect(stepMark('a', 0, 5)).toBeNull();
		expect(stepMark('ArrowRight', null, 0)).toBeNull();
	});

	it('finds the nearest point to the pointer', () => {
		expect(nearest(0, 10, 100, 5)).toBe(0);
		expect(nearest(62, 10, 100, 5)).toBe(2);
		expect(nearest(500, 10, 100, 5)).toBe(4);
		expect(nearest(50, 10, 100, 1)).toBe(0);
	});

	it('writes every label that fits, else every nth', () => {
		expect(labelEvery(6, 400, 50)).toBe(1);
		expect(labelEvery(45, 400, 50)).toBe(6);
	});
});

describe('readSetup', () => {
	it('reads a stored setup, and anything else as the page comes', () => {
		expect(readSetup(null)).toEqual({
			tab: 'status',
			project: null,
			period: 'weeks6',
			from: null,
			to: null,
			filter: {}
		});
		expect(
			readSetup(
				JSON.stringify({
					tab: 'flow',
					project: 'p2',
					period: 'range',
					from: '2026-10-01',
					to: 'soon',
					filter: { projects: ['p1'] }
				})
			)
		).toEqual({
			tab: 'flow',
			project: 'p2',
			period: 'range',
			from: '2026-10-01',
			to: null,
			filter: { projects: ['p1'] }
		});
		expect(readSetup('{"tab":"nope","project":7,"period":7,"filter":[]}')).toEqual(readSetup(null));
		expect(readSetup('not json')).toEqual(readSetup(null));
	});
});

describe('a report view', () => {
	const view = (display: Record<string, unknown>, projectId: string | null = null) => ({
		projectId,
		filter: { priorities: ['high' as const] },
		display
	});

	it('is set up from its display, with its own filter', () => {
		const setup = viewReport(
			view({
				report: {
					tab: 'flow',
					period: 'range',
					from: '2026-10-01',
					to: '2026-10-31',
					project: 'p1'
				}
			})
		);
		expect(setup).toEqual({
			tab: 'flow',
			period: 'range',
			from: '2026-10-01',
			to: '2026-10-31',
			project: 'p1',
			filter: { priorities: ['high'] }
		});
	});

	it('reports on its own project alone, whatever its display says', () => {
		expect(viewReport(view({ report: { project: 'other' } }, 'mine')).project).toBe('mine');
	});

	it('takes a display it cannot read as the page comes', () => {
		for (const report of [undefined, null, 'flow', [], { tab: 'nope', period: 3, from: 'x' }]) {
			const setup = viewReport(view({ report }));
			expect(setup).toEqual({ ...readSetup(null), filter: { priorities: ['high'] } });
		}
	});

	it('keeps all but the filter in its display, and reads back the same', () => {
		const setup = {
			...readSetup(null),
			tab: 'workload' as const,
			project: 'p2',
			filter: { text: 'x' }
		};
		const kept = reportDisplay(setup);
		expect(kept).not.toHaveProperty('filter');
		expect(
			viewReport({ projectId: null, filter: { text: 'x' }, display: { report: kept } })
		).toEqual(setup);
	});
});

describe('the charts a page can show', () => {
	it('each has a key of its own, which a marker can hold, and a tab', () => {
		const keys = REPORT_CHARTS.map((c) => c.key);
		expect(new Set(keys).size).toBe(keys.length);
		for (const c of REPORT_CHARTS) {
			expect(c.key).toMatch(/^[a-z][a-z0-9-]{0,39}$/);
			expect(REPORT_TABS).toContain(c.tab);
			expect(chartOf(c.key)).toBe(c);
		}
		expect(new Set(REPORT_CHARTS.map((c) => c.label)).size).toBe(keys.length);
		expect(chartOf('sprint-velocity')).toBeUndefined();
	});
});
