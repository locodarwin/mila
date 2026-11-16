import { describe, expect, it } from 'vitest';
import { parseCsv, sniffSeparator } from './csv';
import {
	ANY_CSV,
	categoryOf,
	guessTargets,
	plan,
	readDate,
	readTime,
	runImport,
	type Context,
	type Target
} from './csvimport';
import { session } from './session.svelte';
import type { CustomField } from './sync/entities';
import { engine } from './sync/engine.svelte';

describe('parseCsv', () => {
	it('reads quoted fields with separators, quotes and line breaks inside', () => {
		const text = '﻿Title,Notes\r\n"Buy milk, eggs","He said ""now""\nthen left"\r\nPlain,\n\n';
		expect(parseCsv(text)).toEqual([
			['Title', 'Notes'],
			['Buy milk, eggs', 'He said "now"\nthen left'],
			['Plain', '']
		]);
	});

	it('takes the separator from the first line', () => {
		expect(sniffSeparator('a;b;c\n1,5;2;3')).toBe(';');
		expect(sniffSeparator('a\tb\n')).toBe('\t');
		expect(sniffSeparator('"a;b",c\n')).toBe(',');
		expect(parseCsv('Title;Due\nTax;2026-04-15')).toEqual([
			['Title', 'Due'],
			['Tax', '2026-04-15']
		]);
	});

	it('keeps a last line with no line end', () => {
		expect(parseCsv('a,b\n1,2')).toEqual([
			['a', 'b'],
			['1', '2']
		]);
	});
});

describe('reading values', () => {
	it('dates', () => {
		expect(readDate('2026-11-03')).toBe('2026-11-03');
		expect(readDate('2026/1/5')).toBe('2026-01-05');
		expect(readDate('2026-11-03T09:00:00Z')).toBe('2026-11-03');
		expect(readDate('2026-02-30')).toBeNull();
		expect(readDate('11/03/2026')).toBeNull();
	});

	it('times', () => {
		expect(readTime('14:30')).toBe('14:30');
		expect(readTime('9:05')).toBe('09:05');
		expect(readTime('2:30 pm')).toBe('14:30');
		expect(readTime('2pm')).toBe('14:00');
		expect(readTime('14')).toBeNull();
		expect(readTime('12:00 AM')).toBe('00:00');
		expect(readTime('25:00')).toBeNull();
		expect(readTime('13:00 pm')).toBeNull();
	});

	it("statuses' categories", () => {
		expect(categoryOf('Done')).toBe('done');
		expect(categoryOf("Won't do")).toBe('canceled');
		expect(categoryOf('In progress')).toBe('in-progress');
		expect(categoryOf('Someday')).toBe('backlog');
		expect(categoryOf('Waiting')).toBe('todo');
	});
});

const size: CustomField = {
	id: 'f-size',
	projectId: null,
	name: 'Size',
	type: 'select',
	options: [{ id: 'o-l', name: 'Large', color: 'red' }],
	sortKey: 'a0'
};
const points: CustomField = { ...size, id: 'f-pts', name: 'Points', type: 'number', options: [] };

const ctx: Context = {
	projects: [{ id: 'p-home', name: 'Home' }],
	statuses: (id) =>
		id === 'p-home'
			? [
					{ id: 's-todo', name: 'To do', category: 'todo' },
					{ id: 's-done', name: 'Done', category: 'done' }
				]
			: [],
	labels: [{ id: 'l-urgent', name: 'urgent' }],
	fields: [size, points],
	members: [
		{ userId: 'u-ann', name: 'Ann', email: 'ann@example.com' },
		{ userId: 'u-gus', name: 'Gus', email: 'gus@example.com' }
	],
	// Gus is a guest with no projects.
	reaches: (userId) => userId !== 'u-gus'
};

describe('assignees', () => {
	it('leaves out someone who cannot reach the task, and says so', () => {
		const rows = parseCsv('Title,Assignees\nPaint,"Ann, Gus"\n');
		const out = plan(rows, ['title', 'assignees'], { kind: 'inbox' }, ctx);
		expect(out.tasks[0].assignees).toEqual(['u-ann']);
		expect(out.warnings.map((w) => w.message)).toContain("Gus cannot reach this task's project.");
	});
});

describe('guessTargets', () => {
	it("matches the usual headings, Mila's own export and custom fields", () => {
		const headers = ['ID', 'Title', 'Status', 'Due date', 'Tags', 'Assignees', 'size', 'Name'];
		expect(guessTargets(headers, ctx.fields)).toEqual<Target[]>([
			'skip',
			'title',
			'status',
			'due',
			'labels',
			'assignees',
			'field:f-size',
			'skip' // a second title column is left for the person to choose
		]);
	});
});

describe('plan', () => {
	const rows = [
		['Title', 'Project', 'Status', 'Priority', 'Due', 'Time', 'Labels', 'Owner', 'Size', 'Points'],
		[
			'Fix the sink',
			'Home',
			'done',
			'P1',
			'2026-11-03',
			'2pm',
			'urgent; plumbing',
			'Ann',
			'Large',
			'3'
		],
		[
			'Paint the fence',
			'Garden',
			'Waiting',
			'high',
			'soon',
			'9:00',
			'Plumbing',
			'Bob',
			'Huge',
			'x'
		],
		['', 'Home'],
		['Call mom', '', '', '', '', '', '', '', '', '']
	];
	const targets: Target[] = [
		'title',
		'project',
		'status',
		'priority',
		'due',
		'dueTime',
		'labels',
		'assignees',
		'field:f-size',
		'field:f-pts'
	];

	it('says what would be made, without making anything', () => {
		const p = plan(rows, targets, { kind: 'inbox' }, ctx);
		expect(p.tasks.map((t) => t.title)).toEqual(['Fix the sink', 'Paint the fence', 'Call mom']);
		expect(p.skipped).toEqual([{ line: 4, reason: 'It has no title.' }]);
		expect(p.newProjects).toEqual(['Garden']);
		expect(p.newLabels).toEqual(['plumbing']);
		expect(p.newStatuses).toEqual([
			{ project: { make: 'project:Garden' }, name: 'Waiting', category: 'todo' }
		]);
		expect(p.newOptions).toEqual([{ fieldId: 'f-size', name: 'Huge' }]);

		const [sink, fence, mom] = p.tasks;
		expect(sink).toMatchObject({
			project: { id: 'p-home' },
			status: { id: 's-done' },
			priority: 'urgent',
			due: '2026-11-03',
			dueTime: '14:00',
			labels: [{ id: 'l-urgent' }, { make: 'label:plumbing' }],
			assignees: ['u-ann'],
			values: [
				{ fieldId: 'f-size', value: { id: 'o-l' } },
				{ fieldId: 'f-pts', value: 3 }
			]
		});
		// A done status completes the task.
		expect(sink.completedAt).not.toBeNull();
		expect(fence.project).toEqual({ make: 'project:Garden' });
		expect(fence.labels).toEqual([{ make: 'label:plumbing' }]);
		expect(mom.project).toBeNull();
		expect(mom.status).toBeNull();

		const messages = p.warnings.map((w) => `${w.line}: ${w.message}`);
		expect(messages).toEqual([
			'3: "soon" is not a date like 2026-11-03.',
			'3: A due time needs a due date.',
			'3: Bob is not a member of this workspace.',
			'3: "x" does not fit Points.'
		]);
	});

	it('puts rows with no project where the person chose', () => {
		const one = [['Title'], ['Alone']];
		const into = plan(one, ['title'], { kind: 'project', id: 'p-home' }, ctx).tasks[0];
		expect(into.project).toEqual({ id: 'p-home' });
		expect(into.status).toEqual({ id: 's-todo' });
		const fresh = plan(one, ['title'], { kind: 'new', name: 'Imported' }, ctx);
		expect(fresh.newProjects).toEqual(['Imported']);
		expect(fresh.tasks[0].status).toEqual({ make: 'status:make:project:imported|To do' });
		// A new project is made only when a row goes in it.
		const named = plan(
			[
				['Title', 'Project'],
				['Mow', 'Home']
			],
			['title', 'project'],
			{
				kind: 'new',
				name: 'Imported'
			},
			ctx
		);
		expect(named.newProjects).toEqual([]);
	});

	it('cuts names to what the server takes, so no row is refused', () => {
		const long = (c: string, n: number) => c.repeat(n);
		const rows = [
			['Title', 'Project', 'Status', 'Size'],
			[`${long('t', 499)} tail`, long('p', 250), long('s', 70), `${long('o', 60)}A`],
			['Second', long('p', 250), long('s', 70), `${long('o', 60)}B`]
		];
		const p = plan(rows, ['title', 'project', 'status', 'field:f-size'], { kind: 'inbox' }, ctx);
		expect(p.tasks[0].title).toBe(long('t', 499));
		expect(p.newProjects).toEqual([long('p', 200)]);
		expect(p.newStatuses.map((s) => s.name)).toEqual([long('s', 60)]);
		// Choices are cut the same way they are named, so values find them,
		// and two names alike up to the cut make one choice.
		expect(p.newOptions).toEqual([{ fieldId: 'f-size', name: long('o', 60) }]);
		for (const t of p.tasks)
			expect(t.values).toEqual([
				{ fieldId: 'f-size', value: { make: `option:f-size|${long('o', 60)}` } }
			]);
	});
});

describe('the targets any app might have', () => {
	it('puts every row in the place chosen, Project column or not, when asked', () => {
		const rows = [
			['Title', 'Project'],
			['Mow', 'Garden'],
			['Rake', '']
		];
		const all = plan(rows, ['title', 'project'], { kind: 'project', id: 'p-home' }, ctx, {
			byColumn: false
		});
		expect(all.tasks.map((t) => t.project)).toEqual([{ id: 'p-home' }, { id: 'p-home' }]);
		expect(all.newProjects).toEqual([]);
	});

	it('reads subtasks, comments, checklists, estimates, files and a link back', () => {
		const rows = [
			['Title', 'ID', 'Parent', 'Comments', 'Checklist', 'Estimate', 'Files', 'Link', 'Type'],
			['Paint', '1', '', 'Use the blue', '[x] Buy paint; Sand', '1h 30m', '', '', 'Task'],
			[
				'Prime',
				'2',
				'1',
				'',
				'',
				'soon',
				'https://files.example.com/a.png ftp://nope',
				'https://example.com/t/2',
				'Bug'
			]
		];
		const p = plan(
			rows,
			[
				'title',
				'key',
				'parent',
				'comments',
				'checklist',
				'estimate',
				'attachments',
				'link',
				'type'
			],
			{ kind: 'inbox' },
			ctx
		);
		const [paint, prime] = p.tasks;
		expect(paint).toMatchObject({
			key: '1',
			parent: null,
			comments: ['Use the blue'],
			checklist: [
				{ text: 'Buy paint', done: true },
				{ text: 'Sand', done: false }
			],
			estimate: 90,
			labels: []
		});
		expect(prime.parent).toBe('1');
		expect(prime.notes).toBe(
			'[Where it came from](https://example.com/t/2)\n\n**Files**\n\n- [a.png](https://files.example.com/a.png)'
		);
		// Any CSV file's other types come in with a label of their type.
		expect(prime.labels).toEqual([{ make: 'label:Bug' }]);
		expect(p.warnings.map((w) => w.message)).toEqual([
			'"soon" is not a length of time like 1h 30m.',
			'A file in it is not a web address, so it is left out.'
		]);
		expect(p.links).toBe(1);
	});
});

describe('runImport', () => {
	it('makes what the plan says, through ordinary edits', async () => {
		const ws = `csv-${Math.random()}`;
		const replica = engine.sample(ws, []);
		const rows = [
			['Title', 'Status', 'Labels', 'Due'],
			['First', 'Doing', 'red, blue', '2026-11-03'],
			['Second', 'Done', 'red', '']
		];
		const p = plan(
			rows,
			['title', 'status', 'labels', 'due'],
			{ kind: 'new', name: 'Moved' },
			{
				...ctx,
				projects: [],
				labels: [],
				fields: []
			}
		);
		const seen: number[] = [];
		const before = session.user;
		session.user = { id: 'u-me' } as typeof session.user;
		const ids = await runImport(ws, p, (done) => seen.push(done)).finally(
			() => (session.user = before)
		);
		expect(seen).toEqual([1, 2]);
		const project = replica.projects()[0];
		expect(project.name).toBe('Moved');
		const statuses = replica.statuses(project.id).map((s) => s.name);
		expect(statuses).toContain('Doing');
		const tasks = replica.tasks(project.id);
		expect(tasks.map((t) => t.title)).toEqual(['First', 'Second']);
		expect(tasks.map((t) => t.id)).toEqual(ids);
		expect(replica.status(tasks[0].statusId)?.name).toBe('Doing');
		expect(tasks[0].dueDate).toBe('2026-11-03');
		expect(tasks[1].completedAt).not.toBeNull();
		// Made by the person importing, as a task typed in is, so its due
		// notices and the digest find them.
		expect(tasks.map((t) => t.createdBy)).toEqual(['u-me', 'u-me']);
		expect(replica.labels().map((l) => l.name)).toEqual(['red', 'blue']);
		expect(replica.taskLabels(tasks[0].id).map((l) => l.name)).toEqual(['red', 'blue']);
		// Not on the undo list: one undo must not take back a slice of an import.
		expect(engine.canUndo).toBe(false);
		engine.dropSample(ws);
	});
});

describe('A checklist a stranger wrote', () => {
	it('is read in time in line with its length', () => {
		const start = performance.now();
		const items = ANY_CSV.checklist('[x] ' + ' '.repeat(200_000) + 'a\u2028b', []);
		expect(performance.now() - start).toBeLessThan(1000);
		expect(items).toEqual([{ text: 'a\u2028b', done: true }]);
	});
});
