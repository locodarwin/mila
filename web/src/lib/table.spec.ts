import { describe, expect, it } from 'vitest';
import type { CustomField, FieldValue, Project, Task, ViewDisplay } from './sync/entities';
import {
	MAX_WIDTH,
	MIN_WIDTH,
	cellValue,
	type TableColumn,
	columnSlots,
	columnSort,
	defaultSummary,
	groupRows,
	groupTasksDeep,
	groupWrite,
	headingSort,
	landing,
	moveColumn,
	placeColumn,
	readTable,
	summarize,
	summaryChoices,
	summaryOf,
	tableColumns,
	withOrder,
	withSummary,
	withWidth,
	withoutLayout,
	type SummaryContext
} from './table';
import { resolveDisplay, shownPatch, type DisplayLookups } from './viewDisplay';

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

const field = (
	id: string,
	type: CustomField['type'],
	over: Partial<CustomField> = {}
): CustomField => ({
	id,
	projectId: null,
	name: id,
	type,
	options: [],
	sortKey: id,
	...over
});

const hours = field('f-hours', 'number', { name: 'Hours' });
const room = field('f-room', 'select', {
	name: 'Room',
	options: [
		{ id: 'o-yard', name: 'Yard', color: 'green' },
		{ id: 'o-shop', name: 'Shop', color: 'orange' }
	]
});
const notes = field('f-notes', 'multi_select', { name: 'Tags' });
const fields = [hours, room, notes];
const inProject: DisplayLookups = { projectId: 'p1', fields };
const across: DisplayLookups = { projectId: null, fields };

describe('the columns a row draws', () => {
	const cols = (n: number) =>
		Array.from({ length: n }, (_, i) => ({ id: `c${i}`, width: 100 }) as TableColumn);
	const said = (slots: ReturnType<typeof columnSlots>) =>
		slots.map((s) => ('gap' in s ? `gap ${s.gap}` : s.at)).join(' ');

	it('draws them all when they fit', () => {
		expect(said(columnSlots(cols(4), 0, 1000, []))).toBe('0 1 2 3');
	});

	it('draws a screenful either side, and one wide cell for each stretch left out', () => {
		expect(said(columnSlots(cols(30), 1000, 200, []))).toBe('0 gap 7 8 9 10 11 12 13 gap 16');
	});

	it('keeps the title and the column with focus', () => {
		expect(said(columnSlots(cols(10), 0, 100, [8]))).toBe('0 1 gap 6 8 gap 1');
	});

	it('names each slot for as long as it stands for the same thing', () => {
		const a = columnSlots(cols(30), 1000, 200, []).map((s) => s.key);
		expect(new Set(a).size).toBe(a.length);
		expect(a).toContain('gap:1');
	});
});

describe('a table view resolved', () => {
	it('shows status, priority, assignees, due, time and every custom field by default', () => {
		expect(resolveDisplay({}, 'table', inProject).shown).toEqual([
			'status',
			'priority',
			'assignees',
			'due',
			'estimate',
			'logged',
			'f-hours',
			'f-room',
			'f-notes'
		]);
		expect(resolveDisplay({}, 'table', across).shown).toContain('project');
	});

	it('hides a custom field only when told to', () => {
		const d = { fields: { table: shownPatch({}, 'table', 'f-room', false) } };
		expect(d.fields.table).toEqual({ hide: ['f-room'] });
		expect(resolveDisplay(d, 'table', inProject).shown).not.toContain('f-room');
		expect(shownPatch(d, 'table', 'f-room', true)).toBeUndefined();
	});

	it('keeps its grouping, which a board would not', () => {
		const d = { group: { by: 'priority' as const } };
		expect(resolveDisplay(d, 'table', inProject).group).toEqual({ by: 'priority' });
		expect(resolveDisplay(d, 'board', inProject).group).toEqual({ by: 'status' });
	});
});

describe('table columns', () => {
	const shown = resolveDisplay({}, 'table', inProject).shown;

	it('start with the title and follow the default order', () => {
		const cols = tableColumns(shown, {}, fields);
		expect(cols.map((c) => c.id)).toEqual([
			'title',
			'status',
			'priority',
			'assignees',
			'due',
			'estimate',
			'logged',
			'f-hours',
			'f-room',
			'f-notes'
		]);
		expect(cols[0].kind).toBe('title');
		expect(cols.find((c) => c.id === 'f-hours')).toMatchObject({ kind: 'number', end: true });
	});

	it('take the stored order, then any shown column it leaves out', () => {
		const cols = tableColumns(shown, { order: ['f-room', 'title', 'gone', 'due'] }, fields);
		expect(cols.map((c) => c.id)).toEqual([
			'title',
			'f-room',
			'due',
			'status',
			'priority',
			'assignees',
			'estimate',
			'logged',
			'f-hours',
			'f-notes'
		]);
	});

	it('keep stored widths within bounds', () => {
		const cols = tableColumns(
			shown,
			{
				widths: [
					{ col: 'status', px: 10 },
					{ col: 'due', px: 9999 },
					{ col: 'f-room', px: 201.4 }
				]
			},
			fields
		);
		const w = (id: string) => cols.find((c) => c.id === id)!.width;
		expect([w('status'), w('due'), w('f-room')]).toEqual([MIN_WIDTH, MAX_WIDTH, 201]);
	});

	it('sort by what can be sorted', () => {
		const cols = tableColumns([...shown, 'labels'], {}, fields);
		const sort = (id: string) => cols.find((c) => c.id === id)!.sort;
		expect(sort('title')).toEqual({ by: 'title' });
		expect(sort('f-hours')).toEqual({ by: 'field', field: 'f-hours' });
		expect(sort('f-notes')).toBeNull();
		expect(sort('labels')).toBeNull();
		expect(sort('assignees')).toBeNull();
	});
});

describe('a table layout as stored', () => {
	it('leaves out what has the wrong shape', () => {
		// What another app, or a hand-written REST call, might have stored.
		const stored = (table: unknown) => ({ table }) as ViewDisplay;
		expect(readTable(stored('nonsense'))).toEqual({});
		expect(
			readTable(
				stored({
					order: ['a', 3, null],
					widths: [{ col: 'a', px: 100 }, { col: 'b', px: 'wide' }, null],
					summaries: [{ col: 'a', fn: 'sum' }, { col: 7 }]
				})
			)
		).toEqual({
			order: ['a'],
			widths: [{ col: 'a', px: 100 }],
			summaries: [{ col: 'a', fn: 'sum' }]
		});
	});

	it('changes one part at a time and drops what is empty', () => {
		const t = withWidth({}, 'due', 150)!;
		expect(t).toEqual({ widths: [{ col: 'due', px: 150 }] });
		expect(withWidth(t, 'due', 170)).toEqual({ widths: [{ col: 'due', px: 170 }] });
		expect(withOrder(t, ['title', 'due', 'status'])).toEqual({
			widths: [{ col: 'due', px: 150 }],
			order: ['due', 'status']
		});
		expect(withoutLayout({ order: ['due'], widths: t.widths })).toBeUndefined();
		expect(withoutLayout({ order: ['due'], summaries: [{ col: 'due', fn: 'none' }] })).toEqual({
			summaries: [{ col: 'due', fn: 'none' }]
		});
	});

	it('moves a column one place, and not past either end', () => {
		expect(moveColumn(['a', 'b', 'c'], 'b', -1)).toEqual(['b', 'a', 'c']);
		expect(moveColumn(['a', 'b', 'c'], 'c', 1)).toBeNull();
		expect(placeColumn(['a', 'b', 'c'], 'a', 'c')).toEqual(['b', 'a', 'c']);
		expect(placeColumn(['a', 'b', 'c'], 'a', null)).toEqual(['b', 'c', 'a']);
	});
});

describe('sorting from a heading', () => {
	const due = { by: 'due' as const };
	const title = { by: 'title' as const };

	it('goes ascending, descending, then not sorted', () => {
		const one = headingSort([], due, false);
		expect(one).toEqual([{ by: 'due', dir: 'asc' }]);
		const two = headingSort(one, due, false);
		expect(two).toEqual([{ by: 'due', dir: 'desc' }]);
		expect(headingSort(two, due, false)).toEqual([]);
	});

	it('makes a plain click the only key', () => {
		expect(headingSort([{ by: 'due', dir: 'asc' }], title, false)).toEqual([
			{ by: 'title', dir: 'asc' }
		]);
	});

	it('adds with Shift, up to three, and turns a key in place', () => {
		let rules: unknown[] = [{ by: 'due', dir: 'asc' }];
		rules = headingSort(rules, title, true);
		expect(rules).toEqual([
			{ by: 'due', dir: 'asc' },
			{ by: 'title', dir: 'asc' }
		]);
		rules = headingSort(rules, due, true);
		expect(rules).toEqual([
			{ by: 'due', dir: 'desc' },
			{ by: 'title', dir: 'asc' }
		]);
		rules = headingSort(rules, { by: 'priority' }, true);
		expect(headingSort(rules, { by: 'start' }, true)).toBe(rules);
		expect(headingSort(rules, due, true)).toEqual([
			{ by: 'title', dir: 'asc' },
			{ by: 'priority', dir: 'asc' }
		]);
	});

	it('keeps a rule this app cannot use on a Shift-click', () => {
		const odd = { by: 'mood', dir: 'asc' };
		expect(headingSort([odd], due, true)).toEqual([odd, { by: 'due', dir: 'asc' }]);
	});

	it('says where a column is in the sort', () => {
		const cols = tableColumns(['due', 'f-hours'], {}, fields);
		const rules = [
			{ by: 'title' as const, dir: 'asc' as const },
			{ by: 'field' as const, field: 'f-hours', dir: 'desc' as const }
		];
		expect(columnSort(rules, cols[2])).toEqual({ dir: 'desc', index: 1 });
		expect(columnSort(rules, cols[1])).toBeNull();
	});
});

describe('rows with subtasks', () => {
	const parent = task('a');
	const kid = task('a1', { parentId: 'a' });
	const grandkid = task('a1x', { parentId: 'a1' });
	const other = task('b');
	const kids = (t: Task) =>
		t.id === 'a' ? [kid] : t.id === 'a1' ? [grandkid] : t.id === 'a1x' ? [parent] : [];
	const section = { id: 's:todo', title: 'To do', tasks: [parent, other] };

	it('nest each task over its subtasks, and stop at a loop', () => {
		const rows = groupRows(section, kids, new Set());
		expect(rows.map((r) => [r.task.id, r.depth, r.parent, r.kids])).toEqual([
			['a', 0, null, 1],
			['a1', 1, 'a', 1],
			['a1x', 2, 'a1', 0],
			['b', 0, null, 0]
		]);
		expect(rows[0].key).toBe('s:todo/a');
	});

	it('leave out the subtasks of a folded task', () => {
		const rows = groupRows(section, kids, new Set(['a1']));
		expect(rows.map((r) => r.task.id)).toEqual(['a', 'a1', 'b']);
		expect(rows[1].open).toBe(false);
	});

	it('count every subtask in the summaries, folded or not', () => {
		expect(groupTasksDeep(section, kids).map((t) => t.id)).toEqual(['a', 'a1', 'a1x', 'b']);
	});
});

describe('dropping a row in a group', () => {
	it('sets what the group stands for', () => {
		expect(groupWrite({ by: 'status' }, 's:s-done')).toEqual({ status: 's-done' });
		expect(groupWrite({ by: 'status' }, 'c:in-progress')).toEqual({ category: 'in-progress' });
		expect(groupWrite({ by: 'priority' }, 'prio:high')).toEqual({ priority: 'high' });
		expect(groupWrite({ by: 'project' }, 'p:inbox')).toEqual({ project: null });
		expect(groupWrite({ by: 'project' }, 'p:p2')).toEqual({ project: 'p2' });
		expect(groupWrite({ by: 'field', field: 'f-room' }, 'f:o-yard')).toEqual({
			field: 'f-room',
			value: 'o-yard'
		});
		expect(groupWrite({ by: 'field', field: 'f-room' }, 'f:none')).toEqual({
			field: 'f-room',
			value: null
		});
	});

	it('sets nothing for groups a task can be in more than once', () => {
		expect(groupWrite({ by: 'label' }, 'l:x')).toBeNull();
		expect(groupWrite({ by: 'assignee' }, 'u:me')).toBeNull();
		expect(groupWrite({ by: 'due' }, 'due:today')).toBeNull();
	});

	it('lands between the neighbors, leaving the task itself out', () => {
		const list = [
			task('a', { sortKey: 'a0' }),
			task('b', { sortKey: 'a1' }),
			task('c', { sortKey: 'a2' })
		];
		expect(landing(list, 'a', 2)).toEqual({ before: 'a2', after: null });
		expect(landing(list, 'c', 0)).toEqual({ before: null, after: 'a0' });
		expect(landing(list, 'x', 1)).toEqual({ before: 'a0', after: 'a1' });
	});
});

describe('summaries', () => {
	const values = new Map<string, Map<string, FieldValue>>([
		[
			'f-hours',
			new Map<string, FieldValue>([
				['a', 2],
				['b', 1.5]
			])
		],
		[
			'f-room',
			new Map<string, FieldValue>([
				['a', 'o-yard'],
				['b', 'o-gone']
			])
		]
	]);
	const projects: Project[] = [];
	const ctx: SummaryContext = {
		today: '2026-10-31',
		category: (t) => (t.completedAt ? 'done' : t.statusId === 's-doing' ? 'in-progress' : 'todo'),
		projects,
		assignees: (id) => (id === 'a' ? ['u1', 'u2'] : id === 'b' ? ['u1'] : []),
		labels: () => [],
		values,
		nameOf: (id) => ({ u1: 'Ann', u2: 'Bo' })[id] ?? 'Someone',
		count: () => 0,
		formatDate: (d) => d,
		formatNumber: (n) => String(n)
	};
	const tasks = [
		task('a', { dueDate: '2026-10-29', statusId: 's-doing', priority: 'high' }),
		task('b', { dueDate: '2026-11-05' }),
		task('c', { completedAt: '2026-10-30T10:00:00Z', dueDate: '2026-10-01' })
	];
	const cols = tableColumns(
		['status', 'priority', 'assignees', 'due', 'labels', 'f-hours', 'f-room'],
		{},
		fields
	);
	const col = (id: string) => cols.find((c) => c.id === id)!;

	it('start as a count, the status bar, sums and the due range', () => {
		expect(cols.map((c) => [c.id, defaultSummary(c)])).toEqual([
			['title', 'count'],
			['status', 'bar'],
			['priority', 'none'],
			['assignees', 'none'],
			['due', 'range'],
			['labels', 'none'],
			['f-hours', 'sum'],
			['f-room', 'none']
		]);
	});

	it('offer what fits the column', () => {
		expect(summaryChoices(col('f-hours'))).toEqual(['none', 'sum', 'avg', 'min', 'max', 'filled']);
		expect(summaryChoices(col('due'))).toContain('overdue');
		expect(summaryChoices(col('f-room'))).toEqual(['none', 'bar', 'filled']);
		expect(summaryChoices(col('labels'))).toEqual(['none', 'filled']);
	});

	it('store only choices that differ from the default, and ignore ones that do not fit', () => {
		const t = withSummary({}, col('f-hours'), 'avg')!;
		expect(summaryOf(t, col('f-hours'))).toBe('avg');
		expect(withSummary(t, col('f-hours'), 'sum')).toBeUndefined();
		expect(summaryOf({ summaries: [{ col: 'labels', fn: 'sum' }] }, col('labels'))).toBe('none');
		expect(summaryOf({ summaries: [{ col: 'due', fn: 'none' }] }, col('due'))).toBe('none');
	});

	it('work out counts, numbers and dates', () => {
		const say = (fn: Parameters<typeof summarize>[0], id: string) => {
			const s = summarize(fn, col(id), tasks, ctx);
			return s?.kind === 'text' ? s.text : s;
		};
		expect(say('count', 'title')).toBe('3 tasks');
		expect(say('sum', 'f-hours')).toBe('3.5');
		expect(say('avg', 'f-hours')).toBe('1.75');
		expect(say('max', 'f-hours')).toBe('2');
		expect(say('filled', 'f-hours')).toBe('2 of 3');
		expect(say('range', 'due')).toBe('2026-10-01 to 2026-11-05');
		expect(say('earliest', 'due')).toBe('2026-10-01');
		// The completed task's past date is not overdue.
		expect(say('overdue', 'due')).toBe('1 overdue');
		expect(say('none', 'due')).toBeNull();
	});

	it('draw a bar in the column order, with the empty and unknown last', () => {
		const status = summarize('bar', col('status'), tasks, ctx);
		expect(status?.kind === 'bar' && status.parts.map((p) => [p.key, p.count])).toEqual([
			['todo', 1],
			['in-progress', 1],
			['done', 1]
		]);
		const room = summarize('bar', col('f-room'), tasks, ctx);
		expect(room?.kind === 'bar' && room.parts.map((p) => [p.label, p.count])).toEqual([
			['Yard', 1],
			['None', 2]
		]);
		const people = summarize('bar', col('assignees'), tasks, ctx);
		expect(people?.kind === 'bar' && people.parts.map((p) => [p.label, p.count])).toEqual([
			['Ann', 2],
			['Bo', 1],
			['Nobody', 1]
		]);
		expect(people?.said).toBe('2 Ann, 1 Bo, 1 Nobody');
	});
});

describe('time in a table', () => {
	const cols = tableColumns(['estimate', 'logged', 'remaining'], {}, fields);
	const logged: Record<string, number> = { a: 200, b: 210, c: 0 };
	const ctx = {
		today: '2026-11-01',
		category: () => 'todo' as const,
		projects: [],
		assignees: () => [],
		labels: () => [],
		values: new Map(),
		nameOf: (id: string) => id,
		count: () => 0,
		formatDate: (d: string) => d,
		formatNumber: (n: number) => String(n),
		logged: (id: string) => logged[id] ?? 0
	};
	const tasks = [
		task('a', { estimateMinutes: 360 }),
		task('b', { estimateMinutes: 120 }),
		task('c', { estimateMinutes: null })
	];

	it('are columns of durations that sort and line up on the right', () => {
		expect(cols.map((c) => [c.id, c.kind, c.end, c.sort?.by])).toEqual([
			['title', 'title', false, 'title'],
			['estimate', 'duration', true, 'estimate'],
			['logged', 'duration', true, 'logged'],
			['remaining', 'duration', true, 'remaining']
		]);
	});

	it('sum up as lengths of time, what is left counting none past an estimate', () => {
		const [, estimate, spent, left] = cols;
		expect(summarize('sum', estimate, tasks, ctx)).toMatchObject({
			text: '8h',
			said: 'Sum 8 hours'
		});
		expect(summarize('sum', spent, tasks, ctx)).toMatchObject({ text: '6h 50m' });
		expect(summarize('sum', left, tasks, ctx)).toMatchObject({ text: '2h 40m left' });
		expect(summarize('filled', estimate, tasks, ctx)).toMatchObject({ text: '2 of 3' });
		expect(cellValue(left, tasks[1], ctx)).toBe(0);
		expect(cellValue(left, tasks[2], ctx)).toBeNull();
		expect(cellValue(spent, tasks[2], ctx)).toBeNull();
	});
});
