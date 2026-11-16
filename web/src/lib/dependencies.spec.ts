import { describe, expect, it } from 'vitest';
import { beginOf, blockerChoices, conflicts, finishOf, titleRank } from './dependencies';
import type { Task } from './sync/entities';

const task = (id: string, title: string, over: Partial<Task> = {}): Task => ({
	id,
	projectId: null,
	statusId: null,
	parentId: null,
	title,
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

describe('dependencies', () => {
	it('reads a missing date from the other one', () => {
		expect(beginOf({ startDate: null, dueDate: '2026-11-09' })).toBe('2026-11-09');
		expect(finishOf({ startDate: '2026-11-03', dueDate: null })).toBe('2026-11-03');
		expect(beginOf({ startDate: null, dueDate: null })).toBeNull();
	});

	it('finds a conflict when a task starts on or before its blocker is due', () => {
		const counters = task('c', 'Countertops', { startDate: '2026-11-30', dueDate: '2026-12-01' });
		const tile = (start: string) => task('t', 'Tile', { startDate: start, dueDate: '2026-12-03' });
		expect(conflicts(counters, tile('2026-11-30'))).toBe(true);
		expect(conflicts(counters, tile('2026-12-01'))).toBe(true);
		expect(conflicts(counters, tile('2026-12-02'))).toBe(false);
		// A due date alone stands for both ends.
		expect(conflicts(counters, task('w', 'Walk', { dueDate: '2026-12-01' }))).toBe(true);
		// Done or undated tasks never conflict.
		expect(
			conflicts({ ...counters, completedAt: '2026-11-29T10:00:00Z' }, tile('2026-11-30'))
		).toBe(false);
		expect(conflicts(counters, task('n', 'No dates'))).toBe(false);
	});

	it('ranks titles as the [[ menu does', () => {
		expect(titleRank('Paint', 'pai')).toBe(0);
		expect(titleRank('Pick paint colors', 'pai')).toBe(1);
		expect(titleRank('Repaint', 'pai')).toBe(2);
		expect(titleRank('Drywall', 'pai')).toBeNull();
	});

	it('offers open tasks first, best match first, without those left out', () => {
		const all = [
			task('a', 'Repaint trim'),
			task('b', 'Paint', { completedAt: '2026-11-01T00:00:00Z' }),
			task('c', 'Pick paint colors'),
			task('d', 'Paint touch-ups'),
			task('e', 'Drywall')
		];
		expect(blockerChoices('pai', all, new Set(['d'])).map((t) => t.id)).toEqual(['c', 'a', 'b']);
		expect(blockerChoices('', all, new Set(), 2).map((t) => t.id)).toEqual(['e', 'd']);
	});
});
