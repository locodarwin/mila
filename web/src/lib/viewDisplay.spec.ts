import { describe, expect, it } from 'vitest';
import type { CustomField, FieldValue, Project, Status, Task } from './sync/entities';
import { boardColumns } from './boardColumns';
import {
	columnFields,
	applyPaths,
	conditionChoices,
	differs,
	dueBucket,
	groupTasks,
	resolveDisplay,
	shownPatch,
	sortSaid,
	storedSort,
	sortTasks,
	type ColumnContext,
	type DisplayLookups,
	type GroupContext,
	type SortContext
} from './viewDisplay';

const task = (id: string, over: Partial<Task> = {}): Task => ({
	id,
	projectId: 'p1',
	statusId: 's-todo',
	parentId: null,
	title: id,
	notes: '',
	sortKey: id,
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

const statuses: Status[] = [
	{ id: 's-back', projectId: 'p1', name: 'Backlog', category: 'backlog', sortKey: 'a0' },
	{ id: 's-todo', projectId: 'p1', name: 'To do', category: 'todo', sortKey: 'a1' },
	{ id: 's-done', projectId: 'p1', name: 'Done', category: 'done', sortKey: 'a2' }
];
const projects: Project[] = [
	{
		id: 'p1',
		name: 'Garden',
		icon: '',
		color: 'green',
		description: '',
		sortKey: 'a0',
		archivedAt: null
	},
	{
		id: 'p2',
		name: 'Kitchen',
		icon: '',
		color: 'red',
		description: '',
		sortKey: 'a1',
		archivedAt: null
	}
];
const fields: CustomField[] = [
	{ id: 'f-num', projectId: null, name: 'Estimate', type: 'number', options: [], sortKey: 'a0' },
	{
		id: 'f-size',
		projectId: null,
		name: 'Size',
		type: 'select',
		options: [
			{ id: 'o-s', name: 'Small', color: 'green' },
			{ id: 'o-l', name: 'Large', color: 'red' }
		],
		sortKey: 'a1'
	},
	{ id: 'f-who', projectId: null, name: 'Reviewer', type: 'user', options: [], sortKey: 'a2' },
	{ id: 'f-tags', projectId: null, name: 'Tags', type: 'multi_select', options: [], sortKey: 'a3' },
	{ id: 'f-ok', projectId: null, name: 'Approved', type: 'checkbox', options: [], sortKey: 'a4' },
	{ id: 'f-note', projectId: null, name: 'Note', type: 'text', options: [], sortKey: 'a5' }
];
const values = (byField: Record<string, Record<string, FieldValue>>) =>
	new Map(Object.entries(byField).map(([f, v]) => [f, new Map(Object.entries(v))]));

const members = [
	{ userId: 'u-me', name: 'Zed' },
	{ userId: 'u-ann', name: 'Ann' },
	{ userId: 'u-bob', name: 'bob' }
];

function sortCtx(over: Partial<SortContext> = {}): SortContext {
	return {
		category: (t) => statuses.find((s) => s.id === t.statusId)?.category ?? 'todo',
		status: (t) => statuses.find((s) => s.id === t.statusId),
		projects,
		times: (id) => ({ created: `c-${id}`, updated: `u-${id}` }),
		fields,
		values: new Map(),
		memberName: (id) => members.find((m) => m.userId === id)?.name,
		...over
	};
}

const ids = (tasks: Task[]) => tasks.map((t) => t.id);

describe('sortTasks', () => {
	it('keeps manual order with no rules', () => {
		const tasks = [task('c'), task('a'), task('b')];
		expect(ids(sortTasks(tasks, [], sortCtx()))).toEqual(['a', 'b', 'c']);
	});

	it('sorts titles case-blind with numbers by value', () => {
		const tasks = [
			task('1', { title: 'Item 10' }),
			task('2', { title: 'item 9' }),
			task('3', { title: 'Apple' })
		];
		expect(ids(sortTasks(tasks, [{ by: 'title', dir: 'asc' }], sortCtx()))).toEqual([
			'3',
			'2',
			'1'
		]);
		expect(ids(sortTasks(tasks, [{ by: 'title', dir: 'desc' }], sortCtx()))).toEqual([
			'1',
			'2',
			'3'
		]);
	});

	it('puts empty values last in both directions', () => {
		const tasks = [
			task('a', { dueDate: null }),
			task('b', { dueDate: '2026-11-02' }),
			task('c', { dueDate: '2026-11-01' })
		];
		expect(ids(sortTasks(tasks, [{ by: 'due', dir: 'asc' }], sortCtx()))).toEqual(['c', 'b', 'a']);
		expect(ids(sortTasks(tasks, [{ by: 'due', dir: 'desc' }], sortCtx()))).toEqual(['b', 'c', 'a']);
	});

	it('puts a due time before no time on the same day', () => {
		const tasks = [
			task('a', { dueDate: '2026-11-01' }),
			task('b', { dueDate: '2026-11-01', dueTime: '17:00' }),
			task('c', { dueDate: '2026-11-01', dueTime: '09:00' })
		];
		expect(ids(sortTasks(tasks, [{ by: 'due', dir: 'asc' }], sortCtx()))).toEqual(['c', 'b', 'a']);
	});

	it('sorts by priority, with none empty', () => {
		const tasks = [
			task('a', { priority: 'low' }),
			task('b', { priority: 'none' }),
			task('c', { priority: 'urgent' }),
			task('d', { priority: 'high' })
		];
		expect(ids(sortTasks(tasks, [{ by: 'priority', dir: 'desc' }], sortCtx()))).toEqual([
			'c',
			'd',
			'a',
			'b'
		]);
	});

	it('breaks ties by the next rule, then manual order', () => {
		const tasks = [
			task('d', { priority: 'high', title: 'B' }),
			task('c', { priority: 'high', title: 'A' }),
			task('b', { priority: 'low', title: 'A' }),
			task('a', { priority: 'high', title: 'A' })
		];
		expect(
			ids(
				sortTasks(
					tasks,
					[
						{ by: 'priority', dir: 'desc' },
						{ by: 'title', dir: 'asc' }
					],
					sortCtx()
				)
			)
		).toEqual(['a', 'c', 'd', 'b']);
	});

	it('sorts by status category, then status order', () => {
		const tasks = [task('a', { statusId: 's-done' }), task('b', { statusId: 's-back' }), task('c')];
		expect(ids(sortTasks(tasks, [{ by: 'status', dir: 'asc' }], sortCtx()))).toEqual([
			'b',
			'c',
			'a'
		]);
	});

	it('sorts by project with the inbox first', () => {
		const tasks = [task('a', { projectId: 'p2' }), task('b', { projectId: null }), task('c')];
		expect(ids(sortTasks(tasks, [{ by: 'project', dir: 'asc' }], sortCtx()))).toEqual([
			'b',
			'c',
			'a'
		]);
	});

	it('sorts by created and changed through the task times', () => {
		const tasks = [task('a'), task('c'), task('b')];
		expect(ids(sortTasks(tasks, [{ by: 'created', dir: 'desc' }], sortCtx()))).toEqual([
			'c',
			'b',
			'a'
		]);
		expect(ids(sortTasks(tasks, [{ by: 'updated', dir: 'asc' }], sortCtx()))).toEqual([
			'a',
			'b',
			'c'
		]);
	});

	it('sorts by start and completed dates', () => {
		const tasks = [
			task('a', { startDate: '2026-11-03', completedAt: '2026-10-01T00:00:00Z' }),
			task('b', { startDate: '2026-11-01' }),
			task('c', { completedAt: '2026-10-02T00:00:00Z' })
		];
		expect(ids(sortTasks(tasks, [{ by: 'start', dir: 'asc' }], sortCtx()))).toEqual([
			'b',
			'a',
			'c'
		]);
		expect(ids(sortTasks(tasks, [{ by: 'completed', dir: 'desc' }], sortCtx()))).toEqual([
			'c',
			'a',
			'b'
		]);
	});

	it('sorts by custom fields of each kind', () => {
		const tasks = [task('a'), task('b'), task('c'), task('d')];
		const ctx = sortCtx({
			values: values({
				'f-num': { a: 5, b: 12, c: Infinity },
				'f-size': { a: 'o-l', b: 'o-s', d: 'o-gone' },
				'f-who': { a: 'u-bob', b: 'u-left', c: 'u-ann' },
				'f-ok': { b: true, c: false },
				'f-note': { a: 'beta', b: 'Alpha', c: '' }
			})
		});
		const by = (field: string, dir: 'asc' | 'desc' = 'asc') =>
			ids(sortTasks(tasks, [{ by: 'field', field, dir }], ctx));
		// Infinity counts as no number.
		expect(by('f-num', 'desc')).toEqual(['b', 'a', 'c', 'd']);
		expect(by('f-size')).toEqual(['b', 'a', 'c', 'd']);
		// Someone no member is sorts after named people, either way.
		expect(by('f-who')).toEqual(['c', 'a', 'b', 'd']);
		expect(by('f-who', 'desc')).toEqual(['a', 'c', 'b', 'd']);
		expect(by('f-ok', 'desc')).toEqual(['b', 'a', 'c', 'd']);
		expect(by('f-note')).toEqual(['b', 'a', 'c', 'd']);
	});
});

function groupCtx(over: Partial<GroupContext> = {}): GroupContext {
	return {
		me: 'u-me',
		today: '2026-10-30',
		projectId: null,
		projects,
		statuses,
		category: (t) => statuses.find((s) => s.id === t.statusId)?.category ?? 'todo',
		labels: [
			{ id: 'l1', name: 'Errand', color: 'red', sortKey: 'a0' },
			{ id: 'l2', name: 'Home', color: 'blue', sortKey: 'a1' }
		],
		taskLabels: (id) => ({ a: ['l1', 'l2'], b: ['l2', 'l-gone'] })[id] ?? [],
		assignees: (id) => ({ a: ['u-ann', 'u-me'], c: ['u-left'] })[id] ?? [],
		fields,
		values: new Map(),
		members,
		nameOf: (id) => members.find((m) => m.userId === id)?.name ?? 'Someone',
		...over
	};
}

const shape = (sections: { id: string; tasks: Task[] }[]) =>
	Object.fromEntries(sections.filter((s) => s.tasks.length).map((s) => [s.id, ids(s.tasks)]));

describe('groupTasks', () => {
	const tasks = [
		task('a', { priority: 'high', projectId: null, statusId: null }),
		task('b', { statusId: 's-done' }),
		task('c', { projectId: 'p2', statusId: 's-other' })
	];

	it('makes one section for none', () => {
		expect(groupTasks(tasks, { by: 'none' }, groupCtx())).toEqual([
			{ id: 'none', title: 'Tasks', tasks }
		]);
	});

	it('groups by project, inbox first, as views always did', () => {
		const s = groupTasks(tasks, { by: 'project' }, groupCtx());
		expect(s.map((x) => x.title)).toEqual(['Inbox', 'Garden', 'Kitchen']);
		expect(shape(s)).toEqual({ 'p:inbox': ['a'], 'p:p1': ['b'], 'p:p2': ['c'] });
	});

	it('groups by status inside a project, a lost status in the first', () => {
		const s = groupTasks(tasks, { by: 'status' }, groupCtx({ projectId: 'p1' }));
		expect(s.map((x) => x.title)).toEqual(['Backlog', 'To do', 'Done']);
		expect(shape(s)).toEqual({ 's:s-back': ['a', 'c'], 's:s-done': ['b'] });
	});

	it('groups by status category across projects', () => {
		const s = groupTasks(tasks, { by: 'status' }, groupCtx());
		expect(shape(s)).toEqual({ 'c:todo': ['a', 'c'], 'c:done': ['b'] });
	});

	it('groups by priority, highest first', () => {
		const s = groupTasks(tasks, { by: 'priority' }, groupCtx());
		expect(s.map((x) => x.id)).toEqual([
			'prio:urgent',
			'prio:high',
			'prio:medium',
			'prio:low',
			'prio:none'
		]);
		expect(shape(s)).toEqual({ 'prio:high': ['a'], 'prio:none': ['b', 'c'] });
	});

	it('puts a task in each of its labels and assignees', () => {
		const labels = groupTasks(tasks, { by: 'label' }, groupCtx());
		expect(shape(labels)).toEqual({ 'l:l1': ['a'], 'l:l2': ['a', 'b'], 'l:none': ['c'] });
		const people = groupTasks(tasks, { by: 'assignee' }, groupCtx());
		expect(people.map((x) => x.title)).toEqual(['Me', 'Ann', 'bob', 'Someone', 'Nobody']);
		expect(shape(people)).toEqual({
			'u:u-me': ['a'],
			'u:u-ann': ['a'],
			'u:u-left': ['c'],
			'u:none': ['b']
		});
	});

	it('groups by when tasks are due, across a month end', () => {
		const ctx = groupCtx({ today: '2026-10-30' });
		const due = [
			task('over', { dueDate: '2026-10-29' }),
			task('today', { dueDate: '2026-10-30' }),
			task('tomorrow', { dueDate: '2026-10-31' }),
			task('week', { dueDate: '2026-11-06' }),
			task('later', { dueDate: '2026-11-07' }),
			task('none')
		];
		const s = groupTasks(due, { by: 'due' }, ctx);
		expect(s.map((x) => x.title)).toEqual([
			'Overdue',
			'Today',
			'Tomorrow',
			'This week',
			'Later',
			'No date'
		]);
		expect(s[0].tone).toBe('danger');
		expect(s.map((x) => ids(x.tasks))).toEqual([
			['over'],
			['today'],
			['tomorrow'],
			['week'],
			['later'],
			['none']
		]);
		expect(dueBucket('2026-11-01', '2026-10-30')).toBe('week');
	});

	it('groups by a select field and a person field, None last', () => {
		const ctx = groupCtx({
			values: values({ 'f-size': { a: 'o-l', b: 'o-gone' }, 'f-who': { a: 'u-ann', c: 'u-me' } })
		});
		const size = groupTasks(tasks, { by: 'field', field: 'f-size' }, ctx);
		expect(size.map((x) => x.title)).toEqual(['Small', 'Large', 'None']);
		expect(shape(size)).toEqual({ 'f:o-l': ['a'], 'f:none': ['b', 'c'] });
		const who = groupTasks(tasks, { by: 'field', field: 'f-who' }, ctx);
		expect(shape(who)).toEqual({ 'f:u-me': ['c'], 'f:u-ann': ['a'], 'f:none': ['b'] });
	});
});

const look = (projectId: string | null = 'p1'): DisplayLookups => ({ projectId, fields });

describe('resolveDisplay', () => {
	it('defaults to how views always looked', () => {
		expect(resolveDisplay(null, 'list', look())).toEqual({
			sort: [],
			group: { by: 'status' },
			columns: { by: 'status' },
			shown: [
				'status',
				'priority',
				'labels',
				'subtasks',
				'checklist',
				'comments',
				'repeat',
				'reminders',
				'assignees',
				'due',
				'estimate',
				'logged'
			]
		});
		expect(resolveDisplay(undefined, 'list', look(null)).group).toEqual({ by: 'project' });
		expect(resolveDisplay({}, 'board', look()).shown).not.toContain('status');
		expect(differs(resolveDisplay({}, 'list', look()), 'list', look())).toBe(false);
	});

	it('falls back where a choice makes no sense or names nothing known', () => {
		const r = resolveDisplay(
			{
				sort: [
					{ by: 'field', field: 'f-gone', dir: 'asc' },
					{ by: 'field', field: 'f-tags', dir: 'asc' },
					{ by: 'sideways' as never, dir: 'asc' },
					{ by: 'project', dir: 'asc' },
					{ by: 'due', dir: 'asc' }
				],
				group: { by: 'project' },
				columns: { by: 'field', field: 'f-num' },
				fields: { list: { show: ['f-gone', 'f-num', 'start'], hide: ['due'] } },
				later: { from: 'a newer app' }
			},
			'list',
			look()
		);
		expect(r.sort).toEqual([{ by: 'due', dir: 'asc' }]);
		expect(r.group).toEqual({ by: 'status' });
		expect(r.shown).toContain('f-num');
		expect(r.shown).toContain('start');
		expect(r.shown).not.toContain('f-gone');
		expect(r.shown).not.toContain('due');
		expect(r.shown.indexOf('start')).toBeLessThan(r.shown.indexOf('f-num'));
	});

	it('stops at manual order and at three rules', () => {
		const rule = (by: 'title' | 'due' | 'priority' | 'start') => ({ by, dir: 'asc' as const });
		expect(
			resolveDisplay(
				{ sort: [rule('title'), { by: 'manual', dir: 'asc' }, rule('due')] },
				'list',
				look()
			).sort
		).toEqual([rule('title')]);
		expect(
			resolveDisplay(
				{ sort: [rule('title'), rule('due'), rule('priority'), rule('start')] },
				'list',
				look()
			).sort
		).toHaveLength(3);
	});

	it("takes due-date columns anywhere, and a list's own columns as its default", () => {
		expect(resolveDisplay({ columns: { by: 'due' } }, 'board', look()).columns).toEqual({
			by: 'due'
		});
		const upcoming: DisplayLookups = { ...look(null), columns: { by: 'due' } };
		expect(resolveDisplay({}, 'board', upcoming).columns).toEqual({ by: 'due' });
		expect(resolveDisplay({ columns: { by: 'priority' } }, 'board', upcoming).columns).toEqual({
			by: 'priority'
		});
	});

	it("gives a board across projects priorities, statuses being a project's own", () => {
		expect(resolveDisplay({}, 'board', look(null)).columns).toEqual({ by: 'priority' });
		expect(resolveDisplay({ columns: { by: 'status' } }, 'board', look(null)).columns).toEqual({
			by: 'priority'
		});
	});

	it("takes a workspace field's columns across projects, never one project's field", () => {
		const own: CustomField = { ...fields[1], id: 'f-phase', projectId: 'p1', name: 'Phase' };
		const across: DisplayLookups = { projectId: null, fields: [...fields, own] };
		expect(
			resolveDisplay({ columns: { by: 'field', field: 'f-size' } }, 'board', across).columns
		).toEqual({ by: 'field', field: 'f-size' });
		expect(
			resolveDisplay({ columns: { by: 'field', field: 'f-phase' } }, 'board', across).columns
		).toEqual({ by: 'priority' });
		expect(columnFields(across).map((f) => f.id)).toEqual(['f-size', 'f-who']);
		expect(columnFields({ projectId: 'p1', fields: [...fields, own] }).map((f) => f.id)).toEqual([
			'f-size',
			'f-who',
			'f-phase'
		]);
	});

	it('takes columns only on a board, and only fields that can be columns', () => {
		expect(resolveDisplay({ columns: { by: 'priority' } }, 'board', look()).columns).toEqual({
			by: 'priority'
		});
		expect(
			resolveDisplay({ columns: { by: 'field', field: 'f-size' } }, 'board', look()).columns
		).toEqual({ by: 'field', field: 'f-size' });
		expect(
			resolveDisplay({ columns: { by: 'field', field: 'f-num' } }, 'board', look()).columns
		).toEqual({
			by: 'status'
		});
		expect(
			resolveDisplay({ sort: [{ by: 'title', dir: 'asc' }] }, 'calendar', look()).sort
		).toEqual([]);
	});

	it('says what a sort is', () => {
		expect(sortSaid([{ by: 'due', dir: 'asc' }], fields)).toBe('Sorted by due date, soonest first');
		expect(sortSaid([{ by: 'field', field: 'f-num', dir: 'desc' }], fields)).toBe(
			'Sorted by Estimate, highest first'
		);
		expect(sortSaid([], fields)).toBe('Manual order');
	});
});

describe('applyPaths', () => {
	it('replaces parts and keeps the rest, keys from a newer app too', () => {
		const before = {
			sort: [{ by: 'due' as const, dir: 'asc' as const }],
			fields: { list: { show: ['start'] }, calendar: { show: ['x'] } },
			later: 1
		};
		const after = applyPaths(before, {
			group: { by: 'priority' },
			'fields.board': { hide: ['due'] }
		});
		expect(after).toEqual({
			sort: [{ by: 'due', dir: 'asc' }],
			group: { by: 'priority' },
			fields: { list: { show: ['start'] }, calendar: { show: ['x'] }, board: { hide: ['due'] } },
			later: 1
		});
		expect(before.fields).not.toHaveProperty('board');
	});

	it('takes a part away for undefined', () => {
		expect(
			applyPaths({ sort: [], fields: { list: {} } }, { sort: undefined, 'fields.list': undefined })
		).toEqual({});
		expect(applyPaths(null, { columns: { by: 'priority' } })).toEqual({
			columns: { by: 'priority' }
		});
	});
});

describe('boardColumns', () => {
	const ctx: ColumnContext = {
		me: 'u-me',
		statuses,
		fields,
		values: values({ 'f-size': { a: 'o-l', b: 'o-gone' } }),
		members,
		nameOf: (id) => members.find((m) => m.userId === id)?.name ?? 'Someone'
	};

	it('makes a column per status, a lost status in the first', () => {
		const cols = boardColumns({ by: 'status' }, ctx);
		expect(cols.map((c) => c.write)).toEqual([
			{ status: 's-back' },
			{ status: 's-todo' },
			{ status: 's-done' }
		]);
		expect(cols[0].has(task('x', { statusId: 's-gone' }))).toBe(true);
		expect(cols[1].has(task('x'))).toBe(true);
	});

	it('makes priority columns that set the priority', () => {
		const cols = boardColumns({ by: 'priority' }, ctx);
		expect(cols.map((c) => c.title)).toEqual(['Urgent', 'High', 'Medium', 'Low', 'No priority']);
		expect(cols[1].write).toEqual({ priority: 'high' });
		expect(cols[1].has(task('x', { priority: 'high' }))).toBe(true);
	});

	it('makes field columns that set the value, None clearing it', () => {
		const cols = boardColumns({ by: 'field', field: 'f-size' }, ctx);
		expect(cols.map((c) => c.title)).toEqual(['Small', 'Large', 'None']);
		expect(cols[1].write).toEqual({ field: 'f-size', value: 'o-l' });
		expect(cols[2].write).toEqual({ field: 'f-size', value: null });
		expect(cols[1].has(task('a'))).toBe(true);
		// A value that names no column is under None.
		expect(cols[2].has(task('b'))).toBe(true);
		const people = boardColumns({ by: 'field', field: 'f-who' }, ctx);
		expect(people.map((c) => c.title)).toEqual(['Me', 'Ann', 'bob', 'None']);
		expect(people[0].write).toEqual({ field: 'f-who', value: 'u-me' });
	});

	it('makes date columns: the past, each day of the week, later and no date', () => {
		const days = { ...ctx, today: '2026-11-15', dayName: (d: string) => `day ${d}` };
		const cols = boardColumns({ by: 'due' }, days);
		expect(cols.map((c) => c.title)).toEqual([
			'Earlier',
			'Today',
			'Tomorrow',
			'day 2026-11-17',
			'day 2026-11-18',
			'day 2026-11-19',
			'day 2026-11-20',
			'day 2026-11-21',
			'Later',
			'No date'
		]);
		// Nothing moves into the past; Later is the day after the week; no date clears it.
		expect(cols[0].write).toBeNull();
		expect(cols[2].write).toEqual({ due: '2026-11-16' });
		expect(cols[8].write).toEqual({ due: '2026-11-22' });
		expect(cols[9].write).toEqual({ due: null });
		const at = (dueDate: string | null) =>
			cols.filter((c) => c.has(task('x', { dueDate }))).map((c) => c.title);
		expect(at('2026-11-14')).toEqual(['Earlier']);
		expect(at('2026-11-15')).toEqual(['Today']);
		expect(at('2026-11-21')).toEqual(['day 2026-11-21']);
		expect(at('2026-11-22')).toEqual(['Later']);
		expect(at(null)).toEqual(['No date']);
		// Upcoming's, every task open and dated: Overdue, and no No date.
		const upcoming = boardColumns({ by: 'due' }, { ...days, dated: true });
		expect(upcoming.map((c) => c.title)[0]).toBe('Overdue');
		expect(upcoming.map((c) => c.title)).not.toContain('No date');
	});
});

describe('storedSort', () => {
	it('keeps up to three rules, up to a manual one, usable or not', () => {
		const odd = { by: 'sideways', dir: 'asc' } as never;
		expect(storedSort({ sort: [odd, { by: 'due', dir: 'asc' }] })).toEqual([
			odd,
			{ by: 'due', dir: 'asc' }
		]);
		expect(
			storedSort({ sort: [{ by: 'due', dir: 'asc' }, { by: 'manual', dir: 'asc' }, odd] })
		).toEqual([{ by: 'due', dir: 'asc' }]);
		expect(storedSort({ sort: [1, 2, 3, 4] as never })).toHaveLength(3);
		expect(storedSort(null)).toEqual([]);
		expect(storedSort({ sort: 'due' as never })).toEqual([]);
	});
});

describe('shownPatch', () => {
	it('turns fields on and off over the defaults, keeping other IDs', () => {
		expect(shownPatch({}, 'list', 'f-num', true)).toEqual({ show: ['f-num'] });
		expect(shownPatch({}, 'list', 'due', false)).toEqual({ hide: ['due'] });
		expect(shownPatch({}, 'list', 'due', true)).toBeUndefined();
		const d = { fields: { list: { show: ['f-gone', 'f-num'], later: 1 } } };
		expect(shownPatch(d, 'list', 'f-num', false)).toEqual({ show: ['f-gone'], later: 1 });
		// Each change starts from the display as it stands: two in a row both hold.
		const first = { fields: { list: shownPatch({}, 'list', 'start', true) } };
		expect(shownPatch(first, 'list', 'f-num', true)).toEqual({ show: ['start', 'f-num'] });
		// Status is off by default on a board.
		expect(shownPatch({}, 'board', 'status', true)).toEqual({ show: ['status'] });
		// A board across projects shows the project unless hidden.
		expect(shownPatch({}, 'board', 'project', false, true)).toEqual({ hide: ['project'] });
		expect(shownPatch({}, 'board', 'project', true, true)).toBeUndefined();
		expect(resolveDisplay({}, 'board', look(null)).shown).toContain('project');
		expect(resolveDisplay({}, 'board', look()).shown).not.toContain('project');
		expect(resolveDisplay({}, 'list', look(null)).shown).not.toContain('project');
		const inbox = { ...look(null), inbox: true };
		expect(resolveDisplay({}, 'board', inbox).shown).not.toContain('project');
	});
});

describe('conditionChoices', () => {
	const people = {
		me: 'u-me',
		members: members,
		nameOf: (id: string) => members.find((m) => m.userId === id)?.name ?? 'Deleted user'
	};
	it('offers Me and the other members for a person field', () => {
		expect(conditionChoices(fields[2], [], people).map((c) => c.name)).toEqual([
			'Me',
			'Ann',
			'bob'
		]);
	});

	it('gives every chosen value without a chip one of its own', () => {
		expect(conditionChoices(fields[2], ['me', 'u-me', 'u-left', 'u-ann'], people).slice(3)).toEqual(
			[
				{ id: 'u-me', name: 'Zed (you)' },
				{ id: 'u-left', name: 'Deleted user' }
			]
		);
		expect(conditionChoices(fields[1], ['o-l', 'o-gone'], people)).toEqual([
			{ id: 'o-s', name: 'Small' },
			{ id: 'o-l', name: 'Large' },
			{ id: 'o-gone', name: 'A deleted choice' }
		]);
	});
});
