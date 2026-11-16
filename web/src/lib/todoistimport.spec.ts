import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { parseCsv } from './csv';
import { guessTargets, plan, summaryLines, type Target } from './csvimport';
import { runImport, type Context, type Destination } from './importplan';
import { session } from './session.svelte';
import { engine } from './sync/engine.svelte';
import { TODOIST, isTodoist, noteBody, readTodoist, readTodoistDate } from './todoistimport';

// Made-up exports of the same shape as Todoist writes (every name and
// address in them is invented). Home: a comment on the project, labels in
// the content, a subtask with a comment and a file, two sections, a date
// and a deadline, dates in words, repeats, and a date that cannot be read.
// Errands: subtasks three deep.
const fixture = (name: string) =>
	parseCsv(readFileSync(join(import.meta.dirname, '..', '..', 'e2e', 'fixtures', name), 'utf8'));
const HOME = { name: 'Home.csv', rows: fixture('todoist-home.csv') };
const ERRANDS = { name: 'Errands.csv', rows: fixture('todoist-errands.csv') };
const TODAY = new Date(2026, 10, 8);
const day = (at: Date) =>
	[at.getFullYear(), at.getMonth() + 1, at.getDate()]
		.map((n) => String(n).padStart(2, '0'))
		.join('-');

const ctx: Context = {
	projects: [],
	statuses: () => [],
	labels: [],
	fields: [],
	members: [{ userId: 'u-ada', name: 'Ada Park', email: 'ada@example.com' }],
	reaches: () => true
};
const own: Destination = { kind: 'new', name: 'Todoist' };
const table = (files = [HOME]) => readTodoist(files)!;
const dryRun = (files = [HOME]) => {
	const t = table(files);
	const targets = guessTargets(t.rows[0], [], TODOIST);
	const p = plan(t.rows, targets, own, ctx, { dialect: TODOIST, date: day, today: TODAY });
	return { p, targets, t };
};
const byTitle = (title: string, p = dryRun().p) => p.tasks.find((t) => t.title === title)!;

describe("Todoist's file", () => {
	it('is known by its headings', () => {
		expect(isTodoist(HOME.rows[0])).toBe(true);
		expect(isTodoist(['Title', 'Content'])).toBe(false);
		expect(readTodoist([{ name: 'x.csv', rows: [['Title'], ['A']] }])).toBeNull();
	});

	it('becomes a table of a row a task, under its section and parent', () => {
		const t = table();
		expect(t.rows[0]).toEqual([
			'ID',
			'Project',
			'Section',
			'Task',
			'Labels',
			'Description',
			'Priority',
			'Parent',
			'Responsible',
			'Date',
			'Duration',
			'Comments'
		]);
		const tasks = t.rows.slice(1);
		expect(tasks.map((r) => [r[0], r[2], r[3], r[4], r[7]])).toEqual([
			['1-5', '', 'Fix the gate', 'outside, tools', ''],
			['1-6', '', 'Buy hinges', 'shopping', '1-5'],
			['1-10', 'Doing', 'Paint the fence', '', ''],
			['1-11', 'Doing', 'Water the plants', '', ''],
			['1-12', 'Doing', 'Clean the gutters', '', ''],
			['1-15', 'Later', 'Sweep the path', '', ''],
			['1-16', 'Later', 'Mend the shed roof', '', '']
		]);
		expect(new Set(tasks.map((r) => r[1]))).toEqual(new Set(['Home']));
		// Each row is named by its place in the file.
		expect(t.where.slice(2, 4)).toEqual(['Row 5', 'Row 6']);
		// The comment on the project has no task to go on.
		expect(t.left).toEqual([{ line: 9, reason: 'It is a comment on the project, not on a task.' }]);
		expect(t.where[9]).toBe('Row 4');
		const comments = JSON.parse(tasks[1][11]);
		expect(comments).toEqual([
			{
				author: 'Zed Quinn',
				at: '2026-10-02T10:30:00.000Z',
				body: expect.stringContaining('Brass ones, not steel. [[file {')
			}
		]);
	});

	it('takes several files, a project each, named by file', () => {
		const t = table([HOME, ERRANDS]);
		const errands = t.rows.filter((r) => r[1] === 'Errands');
		expect(errands.map((r) => [r[0], r[3], r[7]])).toEqual([
			['2-4', 'Post the parcel', ''],
			['2-5', 'Find some tape', '2-4'],
			['2-6', 'Ask next door for tape', '2-5']
		]);
		expect(t.where.at(-2)).toBe('Errands.csv, row 6');
		expect(t.where.at(-1)).toBe('Home.csv, row 4');
	});
});

describe('A Todoist dry run', () => {
	it('matches every column', () => {
		const { t, targets } = dryRun();
		expect(Object.fromEntries(t.rows[0].map((h, i) => [h, targets[i]]))).toEqual<
			Record<string, Target>
		>({
			ID: 'key',
			Project: 'project',
			Section: 'status',
			Task: 'title',
			Labels: 'labels',
			Description: 'notes',
			Priority: 'priority',
			Parent: 'parent',
			Responsible: 'assignees',
			Date: 'due',
			Duration: 'estimate',
			Comments: 'comments'
		});
	});

	it('makes each section a status, and reads priorities p1 to p4', () => {
		const { p } = dryRun();
		expect(p.newProjects).toEqual(['Home']);
		expect(p.newStatuses.map((s) => [s.name, s.category])).toEqual([
			['Doing', 'in-progress'],
			['Later', 'backlog']
		]);
		expect(
			['Fix the gate', 'Paint the fence', 'Water the plants', 'Sweep the path'].map(
				(title) => byTitle(title, p).priority
			)
		).toEqual(['urgent', 'high', 'medium', 'none']);
		expect(byTitle('Buy hinges', p).parent).toBe(byTitle('Fix the gate', p).key);
		expect(byTitle('Fix the gate', p).labels).toEqual([
			{ make: 'label:outside' },
			{ make: 'label:tools' }
		]);
	});

	it('reads dates in words, repeats and deadlines', () => {
		const { p } = dryRun();
		const when = (title: string) => {
			const t = byTitle(title, p);
			return [t.due, t.dueTime, t.repeat];
		};
		expect(when('Fix the gate')).toEqual(['2026-11-09', '09:00', null]);
		expect(when('Buy hinges')).toEqual(['2026-11-14', null, null]);
		expect(when('Paint the fence')).toEqual(['2026-11-20', null, null]);
		expect(byTitle('Paint the fence', p).notes).toBe('Deadline in Todoist: 2026-11-30');
		expect(when('Clean the gutters')).toEqual(['2026-12-05', null, null]);
		expect(when('Water the plants')).toEqual([
			'2026-11-08',
			null,
			{ rule: 'FREQ=DAILY;INTERVAL=3', mode: 'completion' }
		]);
		expect(when('Sweep the path')).toEqual([
			'2026-11-09',
			'08:00',
			{ rule: 'FREQ=WEEKLY;BYDAY=MO', mode: 'schedule' }
		]);
		expect(when('Mend the shed roof')).toEqual([null, null, null]);
		expect(p.warnings).toEqual([
			{
				line: 8,
				message:
					'"when it stops raining" is not a date Todoist writes, like tomorrow, in 6 days or every monday.'
			}
		]);
		// Tomorrow and in 6 days count from the day of the export.
		expect(p.fromToday).toEqual([2, 3]);
	});

	it('reads people, estimates and comments with their files', () => {
		const { p } = dryRun();
		const gate = byTitle('Fix the gate', p);
		expect(gate.assignees).toEqual(['u-ada']);
		expect(gate.estimate).toBe(45);
		expect(gate.notes).toBe('The hinges are loose.');
		expect(p.strangers).toEqual([{ name: 'Zed Quinn', lines: [4] }]);
		expect(byTitle('Buy hinges', p).comments).toEqual([
			'Originally by Zed Quinn, 2026-10-02\n\nBrass ones, not steel.\n\n[hinge.png](https://files.example.com/hinge.png)'
		]);
	});

	it('says what it brings and what stays behind', () => {
		const { p, targets } = dryRun();
		expect(summaryLines(p, targets, TODOIST)).toEqual([
			'1 subtask, each under its parent task',
			'1 comment, each starting with who wrote it and when',
			'Estimates on 1 task',
			'2 tasks repeat, from the next day each repeats on',
			'2 due dates are written from the day the file was made, like "in 6 days", so they count from today',
			"Completed tasks and reminders are not in Todoist's file, so they stay in Todoist"
		]);
	});

	it('goes in a project for each file', () => {
		const { p } = dryRun([HOME, ERRANDS]);
		expect(p.newProjects).toEqual(['Home', 'Errands']);
		const tape = byTitle('Ask next door for tape', p);
		expect(tape.parent).toBe(byTitle('Find some tape', p).key);
		expect(tape.project).toEqual({ make: 'project:Errands' });
		expect(tape.comments).toHaveLength(1);
	});
});

describe("Todoist's dates", () => {
	const read = (s: string) => readTodoistDate(s, TODAY);
	it('are read as quick add reads them, whole or not at all', () => {
		expect(read('2026-11-14')).toEqual({
			due: '2026-11-14',
			time: null,
			repeat: null,
			relative: false
		});
		expect(read('14 Nov 2026 10:30')).toMatchObject({ due: '2026-11-14', time: '10:30' });
		// A year in the past stays, though quick add would pick the next one.
		expect(read('Nov 1 2026')).toMatchObject({ due: '2026-11-01', relative: false });
		expect(read('Nov 1')).toMatchObject({ due: '2027-11-01', relative: false });
		expect(read('next week')).toMatchObject({ relative: true });
		expect(read('every day starting nov 20')).toMatchObject({
			due: '2026-11-20',
			repeat: { rule: 'FREQ=DAILY', mode: 'schedule' }
		});
		expect(read('Feb 30 2027')).toBeNull();
		expect(read('every last day')).toBeNull();
		expect(read('every! tomorrow')).toBeNull();
		expect(read('tomorrow !high')).toBeNull();
	});
});

describe("Todoist's notes", () => {
	it('keep their files as links, and say when one has no address', () => {
		const problems: string[] = [];
		expect(
			noteBody(
				'See [[file {"file_name":"a [1].pdf","file_url":"https://f.example.com/a.pdf"}]] and [[file {"file_name":"b"}]] too',
				problems
			)
		).toBe('See\n\n[a \\[1\\].pdf](https://f.example.com/a.pdf)\n\nand\n\ntoo');
		expect(problems).toEqual(['A file in a comment has no web address, so it is left out.']);
		expect(noteBody('No files here', [])).toBe('No files here');
		expect(noteBody('[[file {"broken"', [])).toBe('[[file {"broken"');
	});
});

describe('importing Todoist files', () => {
	it('makes the subtasks, repeats and comments', async () => {
		const ws = `td-${Math.random()}`;
		const replica = engine.sample(ws, []);
		const { p } = dryRun([HOME, ERRANDS]);
		const before = session.user;
		session.user = { id: 'u-me' } as typeof session.user;
		const ids = await runImport(ws, p).finally(() => (session.user = before));
		expect(ids).toHaveLength(10);
		const home = replica.projects().find((x) => x.name === 'Home')!;
		const tasks = replica.tasks(home.id);
		const water = tasks.find((t) => t.title === 'Water the plants')!;
		expect([water.recurrence, water.recurrenceMode, water.dueDate]).toEqual([
			'FREQ=DAILY;INTERVAL=3',
			'completion',
			'2026-11-08'
		]);
		const gate = tasks.find((t) => t.title === 'Fix the gate')!;
		expect(replica.subtasks(gate.id).map((t) => t.title)).toEqual(['Buy hinges']);
		expect(gate.recurrence).toBeNull();
		const errands = replica.projects().find((x) => x.name === 'Errands')!;
		const parcel = replica.tasks(errands.id).find((t) => t.title === 'Post the parcel')!;
		const tape = replica.subtasks(parcel.id)[0];
		expect(replica.subtasks(tape.id).map((t) => t.title)).toEqual(['Ask next door for tape']);
		engine.dropSample(ws);
	});
});

describe('mentions in a file', () => {
	it('stay words, in comments and in who wrote them, however they are nested', () => {
		const LIVE = /@\[[^\]\n]{1,100}\]\(user:[A-Za-z0-9-]{1,64}\)/;
		const rows = HOME.rows.map((row) =>
			row[0] === 'note'
				? row.map((cell, i) =>
						i === 1
							? 'nested @[a@[Dana](user:zz)](user:dana-id) and @[b@[c@[Dana](user:y)](user:x)](user:dana-id)'
							: cell.startsWith('Zed Quinn')
								? '@[Dana](user:dana-id) (1002)'
								: cell
					)
				: row
		);
		const t = readTodoist([{ name: 'Home.csv', rows }])!;
		const targets = guessTargets(t.rows[0], [], TODOIST);
		const p = plan(t.rows, targets, own, ctx, { dialect: TODOIST, date: day, today: TODAY });
		const comments = p.tasks.flatMap((task) => task.comments);
		expect(comments.length).toBeGreaterThan(0);
		for (const c of comments) expect(c).not.toMatch(LIVE);
	});
});

describe('A Todoist file a stranger made', () => {
	it('has its people read in time in line with their length', () => {
		const start = performance.now();
		expect(TODOIST.names(' '.repeat(200_000) + 'x')).toEqual(['x']);
		expect(performance.now() - start).toBeLessThan(1000);
		expect(TODOIST.names('Dana (61395206), Zed Quinn  (1002) ')).toEqual(['Dana', 'Zed Quinn']);
	});
});
