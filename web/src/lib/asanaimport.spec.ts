import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { ASANA, asanaLink, isAsana } from './asanaimport';
import { parseCsv } from './csv';
import { ANY_CSV, guessTargets, plan, summaryLines, type Target } from './csvimport';
import { runImport, type Context, type Destination } from './importplan';
import { session } from './session.svelte';
import { engine } from './sync/engine.svelte';

// A made-up export of the same shape as Asana writes (every name and
// address in it is invented): one project's tasks over four sections, one
// of them Asana's untitled one, a task in two projects, a subtask named by
// its parent's title, a parent that isn't in the file, dependencies, one
// of which would make a loop, and a task completed in an open section.
const SAMPLE = readFileSync(
	join(import.meta.dirname, '..', '..', 'e2e', 'fixtures', 'asana-tasks.csv'),
	'utf8'
);
const rows = parseCsv(SAMPLE);
const targets = guessTargets(rows[0], [], ASANA);
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
const own: Destination = { kind: 'new', name: 'Asana' };
const dryRun = (r = rows, t = targets, opts = {}) =>
	plan(r, t, own, ctx, { dialect: ASANA, date: day, byColumn: true, ...opts });
const byTitle = (title: string, p = dryRun()) => p.tasks.find((t) => t.title === title)!;

/** A file of Asana's headings, from the few cells each row sets. */
function file(...given: Record<string, string>[]): string[][] {
	const head = rows[0];
	return [
		head,
		...given.map((g, n) =>
			head.map((h) => {
				if (Object.hasOwn(g, h)) return g[h];
				if (h === 'Task ID') return `${n + 1}`;
				if (h === 'Name') return `Task ${n + 1}`;
				if (h === 'Projects') return 'Plan';
				return '';
			})
		)
	];
}

describe("Asana's columns", () => {
	it('are matched before the person sees them', () => {
		expect(isAsana(rows[0])).toBe(true);
		expect(isAsana(['Task ID', 'Task Name'])).toBe(false);
		expect(
			Object.fromEntries(rows[0].map((h, i) => [h, targets[i]]).filter(([, t]) => t !== 'skip'))
		).toEqual<Record<string, Target>>({
			'Task ID': 'key',
			'Created At': 'made',
			'Completed At': 'completed',
			Name: 'title',
			'Section/Column': 'status',
			'Assignee Email': 'assignees',
			'Start Date': 'start',
			'Due Date': 'due',
			Tags: 'labels',
			Notes: 'notes',
			Projects: 'project',
			'Parent task': 'parent',
			'Blocked By (Dependencies)': 'blockedBy',
			'Blocking (Dependencies)': 'blocking',
			Priority: 'priority'
		});
	});

	it('read a plain CSV file the usual way', () => {
		const plain = guessTargets(['Title', 'Blocked by', 'Blocks'], [], ANY_CSV);
		expect(plain).toEqual(['title', 'blockedBy', 'blocking']);
	});
});

describe("An Asana file's dry run", () => {
	it('brings each task into the project the file is of', () => {
		const p = dryRun();
		expect(p.tasks.map((t) => t.title)).toEqual([
			'Write the brief',
			'Book the venue',
			'Print the flyers',
			'Choose a font',
			'Order samples',
			'Send the invites',
			'Thank the speakers'
		]);
		expect(p.skipped).toEqual([]);
		// Book the venue is in Events too; the rest of the file is Launch plan.
		expect(p.newProjects).toEqual(['Launch plan']);
		expect(new Set(p.tasks.map((t) => JSON.stringify(t.project)))).toEqual(
			new Set([JSON.stringify({ make: 'project:Launch plan' })])
		);
	});

	it('makes each section a status, and keeps completed tasks done', () => {
		const p = dryRun();
		// In progress is one of a new project's own.
		expect(p.newStatuses.map((s) => [s.name, s.category])).toEqual([['Planning', 'todo']]);
		const status = (title: string) => {
			const ref = byTitle(title, p).status;
			return ref && 'make' in ref ? ref.make.split('|')[1] : null;
		};
		expect(status('Write the brief')).toBe('Planning');
		expect(status('Book the venue')).toBe('In progress');
		// Asana's starter section, untitled, is no status of its own.
		expect(status('Order samples')).toBe('To do');
		expect(status('Print the flyers')).toBe('Done');
		expect(byTitle('Print the flyers', p).completedAt).toBe(
			new Date('2026-11-02T12:00:00').toISOString()
		);
		// Completed in an open section: done all the same.
		expect(status('Thank the speakers')).toBe('Done');
		expect(byTitle('Thank the speakers', p).completedAt).not.toBeNull();
		expect(p.completedNow).toEqual([]);
	});

	it('reads dates, tags, priority, people and notes', () => {
		const p = dryRun();
		const brief = byTitle('Write the brief', p);
		expect(brief.due).toBe('2026-12-01');
		expect(brief.priority).toBe('high');
		expect(brief.labels).toEqual([{ make: 'label:copy' }, { make: 'label:web' }]);
		expect(brief.assignees).toEqual(['u-ada']);
		expect(brief.madeAt).toBe(new Date('2026-10-01T12:00:00').toISOString());
		expect(brief.notes).toBe(
			'[Asana task](https://app.asana.com/0/0/4100000000000001), made 2026-10-01\n\nTwo pages at most.\nKeep it plain.'
		);
		const venue = byTitle('Book the venue', p);
		expect([venue.start, venue.due]).toEqual(['2026-11-10', '2026-11-20']);
		expect(venue.assignees).toEqual([]);
		expect(p.strangers).toEqual([{ name: 'zed@example.net', lines: [3] }]);
		expect(p.unassigned).toEqual([3]);
	});

	it('puts a subtask under the parent its title names', () => {
		const p = dryRun();
		const font = byTitle('Choose a font', p);
		expect(font.parent).toBe(byTitle('Write the brief', p).key);
		expect(byTitle('Order samples', p).parent).toBeNull();
		expect(p.warnings).toContainEqual({
			line: 6,
			message: 'Its parent task is not in the file, so it comes in on its own.'
		});
	});

	it('keeps dependencies between tasks in the file, and none that loops', () => {
		const p = dryRun();
		const key = (title: string) => byTitle(title, p).key;
		// Write the brief blocks Book the venue, which says so too: one dependency.
		expect(byTitle('Book the venue', p).blockedBy).toEqual([key('Write the brief')]);
		expect(byTitle('Send the invites', p).blockedBy).toEqual([key('Book the venue')]);
		expect(byTitle('Write the brief', p).blockedBy).toEqual([]);
		expect(p.warnings).toContainEqual({
			line: 7,
			message: 'A dependency in it would make a loop, so it is left out.'
		});
	});

	it('says what it brings and what stays behind', () => {
		const lines = summaryLines(dryRun(), targets, ASANA);
		expect(lines).toEqual([
			'1 subtask, each under its parent task',
			'2 dependencies between the tasks',
			"Each task's notes start with a link back to it in Asana",
			"Comments and attachments are not in Asana's file, so they stay in Asana"
		]);
	});
});

describe('Parents and dependencies by title', () => {
	it('match only a title one task has', () => {
		const p = dryRun(
			file(
				{ Name: 'Paint' },
				{ Name: 'Paint' },
				{ Name: 'Sand', 'Parent task': 'Paint', 'Blocked By (Dependencies)': 'Paint' },
				{ Name: 'Fill', 'Blocked By (Dependencies)': 'Sand, 99' }
			)
		);
		expect(p.tasks.map((t) => [t.title, t.parent, t.blockedBy])).toEqual([
			['Paint', null, []],
			['Paint', null, []],
			['Sand', null, []],
			['Fill', null, ['3']]
		]);
		expect(p.warnings).toEqual([
			{
				line: 4,
				message: "More than one task has its parent task's name, so it comes in on its own."
			},
			{
				line: 4,
				message: 'More than one task has the name it gives, so that dependency is left out.'
			},
			{
				line: 5,
				message: 'It waits for a task that is not coming in, so that dependency is left out.'
			}
		]);
	});

	it('match a title that holds a comma, and leave out a task waiting for itself', () => {
		const p = dryRun(
			file(
				{ Name: 'Pack, then ship' },
				{
					Name: 'Wave',
					'Blocked By (Dependencies)': 'Pack, then ship',
					'Blocking (Dependencies)': '2'
				}
			)
		);
		expect(p.tasks[1].blockedBy).toEqual(['1']);
		expect(p.warnings).toEqual([{ line: 3, message: 'It waits for itself, so that is left out.' }]);
	});

	it('work for rows with no ID', () => {
		const t: Target[] = ['title', 'parent', 'blockedBy'];
		const p = plan(
			[
				['Title', 'Parent', 'Blocked by'],
				['Roof', '', ''],
				['Tiles', 'Roof', 'Roof']
			],
			t,
			own,
			ctx
		);
		expect(p.tasks[1].parent).toBe(p.tasks[0].key);
		expect(p.tasks[1].blockedBy).toEqual([p.tasks[0].key]);
		expect(p.tasks[0].key).not.toBe('Roof');
	});

	it("leave a link back out when the ID is not one of Asana's", () => {
		expect(asanaLink('4100000000000001')).toBe('https://app.asana.com/0/0/4100000000000001');
		expect(asanaLink('x/../y')).toBeNull();
		expect(byTitle('Task 1', dryRun(file({ 'Task ID': 'abc' }))).notes).toBe('');
	});
});

describe('importing an Asana file', () => {
	it('makes subtasks under their parents and the dependencies between tasks', async () => {
		const ws = `as-${Math.random()}`;
		const replica = engine.sample(ws, []);
		const p = dryRun();
		const before = session.user;
		session.user = { id: 'u-me' } as typeof session.user;
		const ids = await runImport(ws, p).finally(() => (session.user = before));
		expect(ids).toHaveLength(7);
		const project = replica.projects().find((x) => x.name === 'Launch plan')!;
		const titleOf = (id: string) => replica.task(id)?.title;
		const tasks = replica.tasks(project.id);
		const brief = tasks.find((t) => t.title === 'Write the brief')!;
		expect(replica.subtasks(brief.id).map((t) => t.title)).toEqual(['Choose a font']);
		expect(
			replica
				.dependencies()
				.map(([blocker, task]) => [titleOf(task), titleOf(blocker)])
				.sort()
		).toEqual([
			['Book the venue', 'Write the brief'],
			['Send the invites', 'Book the venue']
		]);
		expect(replica.statuses(project.id).map((s) => s.name)).toEqual(
			expect.arrayContaining(['Planning', 'In progress'])
		);
		engine.dropSample(ws);
	});
});
