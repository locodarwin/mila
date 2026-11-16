import { describe, expect, it } from 'vitest';
import vectors from '../../../shared/filter-vectors.json';
import {
	filterCount,
	filterOf,
	keepsMatches,
	matches,
	statusChoices,
	UNREADABLE,
	type FilterContext
} from './filter';
import type {
	FieldType,
	FieldValue,
	Status,
	StatusCategory,
	Task,
	ViewFilter
} from './sync/entities';

const task = (over: Partial<Task> = {}): Task => ({
	id: 't1',
	projectId: 'p1',
	statusId: 's1',
	parentId: null,
	title: 'Paint the fence',
	notes: 'Use the grey paint',
	sortKey: 'a0',
	completedAt: null,
	priority: 'high',
	startDate: null,
	dueDate: '2026-10-28',
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

const ctx = (over: Partial<FilterContext> = {}): FilterContext => ({
	me: 'u1',
	today: '2026-10-28',
	category: (t) => (t.completedAt ? 'done' : 'todo'),
	labels: () => ['l1'],
	assignees: () => ['u1'],
	...over
});

describe('keepsMatches', () => {
	const t = (id: string) => task({ id });
	const tree: Record<string, Task[]> = { top: [t('mid')], mid: [t('leaf')], loop: [t('loop')] };
	const keeps = (...hits: string[]) =>
		keepsMatches(
			(x) => hits.includes(x.id),
			(id) => tree[id] ?? []
		);

	it('keeps a match, and the tasks above it', () => {
		const k = keeps('leaf');
		expect(['top', 'mid', 'leaf', 'other'].map((id) => k(t(id)))).toEqual([
			true,
			true,
			true,
			false
		]);
	});

	it('leaves out what has no match under it, and ends at a loop', () => {
		expect(keeps('top')(t('mid'))).toBe(false);
		expect(keeps()(t('loop'))).toBe(false);
	});
});

describe('matches', () => {
	it('shows every open task with an empty filter', () => {
		expect(matches(task(), {}, ctx())).toBe(true);
	});

	it('hides finished tasks unless asked', () => {
		const done = task({ completedAt: '2026-10-27T10:00:00Z' });
		expect(matches(done, {}, ctx())).toBe(false);
		expect(matches(done, { showDone: true }, ctx())).toBe(true);
		expect(matches(done, { statuses: ['done'] }, ctx())).toBe(true);
	});

	it('matches any choice within a part', () => {
		expect(matches(task(), { priorities: ['urgent', 'high'] }, ctx())).toBe(true);
		expect(matches(task(), { priorities: ['low'] }, ctx())).toBe(false);
		expect(matches(task(), { labels: ['l2', 'l1'] }, ctx())).toBe(true);
		expect(matches(task(), { labels: ['l2'] }, ctx())).toBe(false);
	});

	it('needs every part', () => {
		expect(matches(task(), { priorities: ['high'], labels: ['l2'] }, ctx())).toBe(false);
	});

	it('reads "me" as whoever is looking', () => {
		expect(matches(task(), { assignees: ['me'] }, ctx())).toBe(true);
		expect(matches(task(), { assignees: ['me'] }, ctx({ me: 'u2' }))).toBe(false);
	});

	it('treats the inbox as a project', () => {
		expect(matches(task({ projectId: null }), { projects: ['inbox'] }, ctx())).toBe(true);
		expect(matches(task(), { projects: ['inbox'] }, ctx())).toBe(false);
	});

	it('filters by when a task is due', () => {
		const at = (dueDate: string | null) => task({ dueDate });
		expect(matches(at('2026-10-27'), { due: 'overdue' }, ctx())).toBe(true);
		expect(matches(at('2026-10-28'), { due: 'overdue' }, ctx())).toBe(false);
		expect(matches(at('2026-10-28'), { due: 'today' }, ctx())).toBe(true);
		expect(matches(at('2026-11-04'), { due: 'week' }, ctx())).toBe(true);
		expect(matches(at('2026-11-05'), { due: 'week' }, ctx())).toBe(false);
		expect(matches(at(null), { due: 'none' }, ctx())).toBe(true);
		expect(matches(at(null), { due: 'week' }, ctx())).toBe(false);
	});

	it('searches the title and notes', () => {
		expect(matches(task(), { text: 'FENCE' }, ctx())).toBe(true);
		expect(matches(task(), { text: 'grey' }, ctx())).toBe(true);
		expect(matches(task(), { text: 'roof' }, ctx())).toBe(false);
		expect(matches(task(), { text: '   ' }, ctx())).toBe(true);
	});
});

describe('field conditions', () => {
	const fields = (over: Partial<FilterContext> = {}) =>
		ctx({
			fieldType: (id) =>
				(({ cost: 'number', due: 'date', size: 'select' }) as Record<string, FieldType>)[id],
			value: (_, id) =>
				(({ cost: 120, due: '2026-10-20', size: 'big' }) as Record<string, FieldValue>)[id],
			...over
		});

	it('compares numbers, dates and choices', () => {
		expect(matches(task(), { fields: [{ field: 'cost', op: 'gt', value: 100 }] }, fields())).toBe(
			true
		);
		expect(matches(task(), { fields: [{ field: 'due', op: 'before', value: 0 }] }, fields())).toBe(
			true
		);
		expect(
			matches(task(), { fields: [{ field: 'size', op: 'none_of', value: ['big'] }] }, fields())
		).toBe(false);
	});

	it('reads a field it does not know as having no value', () => {
		expect(matches(task(), { fields: [{ field: 'gone', op: 'empty' }] }, fields())).toBe(true);
		expect(matches(task(), { fields: [{ field: 'gone', op: 'not_empty' }] }, fields())).toBe(false);
	});

	// Not in the shared vectors: Go cannot read 1e400 into its harness.
	it('never compares a number that is not finite', () => {
		const inf = fields({ value: () => Infinity });
		expect(matches(task(), { fields: [{ field: 'cost', op: 'gt', value: 1 }] }, inf)).toBe(false);
		expect(matches(task(), { fields: [{ field: 'cost', op: 'not_empty' }] }, inf)).toBe(true);
		expect(
			matches(task(), { fields: [{ field: 'cost', op: 'lt', value: Infinity }] }, fields())
		).toBe(false);
	});
});

describe('filterCount', () => {
	it('counts the parts that are set', () => {
		expect(filterCount({})).toBe(0);
		expect(filterCount({ labels: [], text: ' ' })).toBe(0);
		expect(filterCount({ labels: ['l1'], due: 'today', showDone: true })).toBe(3);
	});
});

// The cases the server's copy (server/internal/filter) is held to as well.
describe('shared filter vectors', () => {
	type VectorContext = {
		me: string;
		today: string;
		category: StatusCategory;
		labels: string[];
		assignees: string[];
		fieldTypes?: Record<string, FieldType>;
		values?: Record<string, FieldValue>;
		blocked?: boolean;
		blocking?: boolean;
		inMyDay?: boolean;
		entries?: { user: string; day: string; minutes: number }[];
	};
	const base = vectors.defaults;

	it.each(vectors.matches.map((c) => [c.name, c] as const))('%s', (_, c) => {
		const v = { ...base.context, ...('context' in c ? c.context : {}) } as VectorContext;
		const t = task({ ...base.task, ...('task' in c ? c.task : {}) } as Partial<Task>);
		const context: FilterContext = {
			me: v.me,
			today: v.today,
			category: () => v.category,
			labels: () => v.labels,
			assignees: () => v.assignees,
			fieldType: (id) => v.fieldTypes?.[id],
			value: (_, id) => v.values?.[id],
			blocked: () => Boolean(v.blocked),
			blocking: () => Boolean(v.blocking),
			inMyDay: () => Boolean(v.inMyDay),
			entries: () =>
				(v.entries ?? []).map((e) => ({ userId: e.user, day: e.day, minutes: e.minutes }))
		};
		const f = 'config' in c ? filterOf(c.config) : (c.filter as ViewFilter);
		expect(matches(t, f, context)).toBe(c.want);
	});

	it('counts the parts that are set', () => {
		for (const c of vectors.count) expect(filterCount(c.filter as ViewFilter)).toBe(c.want);
	});
});

describe('filterOf', () => {
	it('reads the filter out of a config, and an unreadable one as matching nothing', () => {
		expect(filterOf({ filter: { labels: ['l1'] } })).toEqual({ labels: ['l1'] });
		expect(filterOf({})).toEqual({});
		expect(filterOf(null)).toEqual({});
		expect(filterOf({ filter: null })).toEqual({});
		for (const odd of ['{}', [], 5, { filter: 'x' }, { filter: [] }])
			expect(filterOf(odd)).toBe(UNREADABLE);
		// Safe to read a part from and to spread.
		expect(UNREADABLE.statuses).toBeUndefined();
		expect('statuses' in UNREADABLE).toBe(false);
		expect({ ...UNREADABLE }).toEqual({});
		expect(filterCount(UNREADABLE)).toBe(1);
	});
});

describe('statusChoices', () => {
	const st = (id: string, projectId: string, name: string, category: StatusCategory): Status => ({
		id,
		projectId,
		name,
		category,
		sortKey: 'a0'
	});

	it('offers each status by name once, across projects, in order', () => {
		const got = statusChoices([
			st('a1', 'p1', 'To do', 'todo'),
			st('a2', 'p1', 'Agent review', 'in-progress'),
			st('a3', 'p1', 'Blocked', 'todo'),
			st('b1', 'p2', 'agent review ', 'in-progress'),
			st('b2', 'p2', 'In review', 'in-progress')
		]);
		expect(got).toEqual([
			{ name: 'Agent review', ids: ['a2', 'b1'] },
			{ name: 'Blocked', ids: ['a3'] },
			{ name: 'In review', ids: ['b2'] }
		]);
	});

	it('leaves out a status named for its own category, but not for another', () => {
		const got = statusChoices([
			st('a1', 'p1', 'Backlog', 'backlog'),
			st('a2', 'p1', ' in progress', 'in-progress'),
			st('a3', 'p1', 'Done', 'canceled'),
			st('a4', 'p1', '  ', 'todo')
		]);
		expect(got).toEqual([{ name: 'Done', ids: ['a3'] }]);
	});
});
