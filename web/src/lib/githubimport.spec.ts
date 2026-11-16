import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import {
	GITHUB_COMMAND,
	IssueFileError,
	MAX_NEW_LABELS,
	planIssues,
	readIssues,
	repoName
} from './githubimport';
import { MAX_ROWS, runImport, type Context } from './importplan';
import { session } from './session.svelte';
import { engine } from './sync/engine.svelte';

// The real file gh wrote for this repository's own issues: six, all closed.
const SAMPLE = readFileSync(
	join(import.meta.dirname, '..', '..', 'e2e', 'fixtures', 'github-issues.json'),
	'utf8'
);
// A comment's date as the device's calendar has it, as the app writes dates.
const day = (at: Date) =>
	[at.getFullYear(), at.getMonth() + 1, at.getDate()]
		.map((n) => String(n).padStart(2, '0'))
		.join('-');

const ctx: Context = {
	projects: [{ id: 'p-web', name: 'Website' }],
	statuses: (id) =>
		id === 'p-web'
			? [
					{ id: 's-idea', name: 'Ideas', category: 'backlog' },
					{ id: 's-next', name: 'Next', category: 'todo' }
				]
			: [],
	labels: [{ id: 'l-bug', name: 'bug' }],
	fields: [],
	members: [
		{ userId: 'u-ann', name: 'annmaker', email: 'ann@example.com' },
		{ userId: 'u-gus', name: 'Gus', email: 'gus@example.com' }
	],
	// Gus is a guest with no projects.
	reaches: (userId) => userId !== 'u-gus'
};

/** A file of issues as gh writes them, from a few fields each. */
const file = (...issues: object[]) =>
	JSON.stringify(
		issues.map((i, n) => ({
			number: n + 1,
			title: `Issue ${n + 1}`,
			body: '',
			state: 'OPEN',
			closedAt: null,
			createdAt: '2026-10-01T10:00:00Z',
			labels: [],
			assignees: [],
			milestone: null,
			comments: [],
			url: `https://github.com/acme/rockets/issues/${n + 1}`,
			...i
		}))
	);

describe('the command', () => {
	it('asks gh for every field the reader reads', () => {
		const fields = GITHUB_COMMAND.match(/--json (\S+)/)![1].split(',');
		expect(fields.toSorted()).toEqual(
			Object.keys(JSON.parse(SAMPLE)[0])
				.filter((k) => k !== 'id')
				.toSorted()
		);
		expect(GITHUB_COMMAND).toContain('-s all');
		expect(GITHUB_COMMAND).toContain(`-L ${MAX_ROWS}`);
	});
});

describe('the real sample', () => {
	const read = readIssues(SAMPLE);

	it('reads every issue', () => {
		expect(read.issues).toHaveLength(6);
		expect(read.repo).toBe('locodarwin/mila');
		expect(repoName(read.repo)).toBe('mila');
		expect(read.names.get(1)).toBe('Issue #6');
		expect(read.issues.every((i) => i.problems.length === 0)).toBe(true);
	});

	it('plans a task for each, done when the issue closed, with its comments', () => {
		const p = planIssues(read, { kind: 'new', name: 'mila' }, ctx, { date: day });
		expect(p.tasks).toHaveLength(6);
		expect(p.skipped).toEqual([]);
		expect(p.warnings).toEqual([]);
		expect(p.newProjects).toEqual(['mila']);
		expect(p.newStatuses).toEqual([]);
		expect(p.newLabels).toEqual([]);
		const [six] = p.tasks;
		expect(six).toMatchObject({
			title: 'Comments in task panel need some QOL updates',
			project: { make: 'project:mila' },
			status: { make: 'status:make:project:mila|Done' },
			completedAt: '2026-11-03T04:02:51.000Z',
			madeAt: expect.stringMatching(/^2026-1\d-\d\dT/),
			assignees: []
		});
		expect(p.tasks.every((t) => t.madeAt && t.madeAt <= t.completedAt!)).toBe(true);
		expect(six.notes).toMatch(
			/^\[GitHub issue #6\]\(https:\/\/github\.com\/locodarwin\/mila\/issues\/6\), opened \d{4}-\d{2}-\d{2}\n\nComments in tasks have some issues:/
		);
		expect(six.comments).toEqual([
			`Originally by locodarwin, ${day(new Date('2026-11-03T04:02:50Z'))}\n\nBuilt in d56b7fb and tried on staging.`
		]);
		expect(p.tasks.flatMap((t) => t.comments)).toHaveLength(4);
		expect(p.tasks.every((t) => t.completedAt)).toBe(true);
	});

	it("writes a comment's date in the person's style by default", () => {
		const p = planIssues(read, { kind: 'inbox' }, ctx);
		expect(p.tasks[0].comments[0]).toMatch(/^Originally by locodarwin, \S.*2026\n\n/);
		// The inbox has no statuses.
		expect(p.tasks.every((t) => t.status === null)).toBe(true);
	});
});

describe('planIssues', () => {
	it('matches labels by name, makes milestones labels, and matches people by login', () => {
		const read = readIssues(
			file(
				{
					labels: [{ name: 'Bug' }, { name: 'help wanted' }],
					milestone: { title: 'Launch' },
					assignees: [{ login: 'AnnMaker' }, { login: 'zed' }]
				},
				{ state: 'CLOSED', closedAt: '2026-10-05T09:30:00Z', assignees: [{ login: 'zed' }] },
				{ assignees: [{ login: 'gus' }], labels: [{ name: 'help WANTED' }] }
			)
		);
		const p = planIssues(read, { kind: 'project', id: 'p-web' }, ctx, { date: day });
		const [one, two, three] = p.tasks;
		expect(one.labels).toEqual([
			{ id: 'l-bug' },
			{ make: 'label:help wanted' },
			{ make: 'label:Launch' }
		]);
		expect(p.newLabels).toEqual(['help wanted', 'Launch']);
		expect(three.labels).toEqual([{ make: 'label:help wanted' }]);
		expect(one.assignees).toEqual(['u-ann']);
		// Open goes to the project's first open status, closed to a Done made for it.
		expect(one.status).toEqual({ id: 's-next' });
		expect(one.completedAt).toBeNull();
		expect(two.status).toEqual({ make: 'status:id:p-web|Done' });
		expect(p.newStatuses).toEqual([{ project: { id: 'p-web' }, name: 'Done', category: 'done' }]);
		expect(two.completedAt).toBe('2026-10-05T09:30:00.000Z');
		// Someone not here is listed once, with every issue that named them.
		expect(p.strangers).toEqual([{ name: 'zed', lines: [1, 2] }]);
		expect(p.warnings).toEqual([{ line: 3, message: "gus cannot reach this task's project." }]);
		expect(p.unassigned).toEqual([2, 3]);
	});

	it('leaves out what is not an issue, and issues past the limit', () => {
		const many = JSON.parse(file(...Array.from({ length: MAX_ROWS + 2 }, () => ({}))));
		many[3] = 'a string';
		many[4] = { title: '   ', number: 5 };
		const p = planIssues(readIssues(JSON.stringify(many)), { kind: 'inbox' }, ctx);
		expect(p.tasks).toHaveLength(MAX_ROWS - 1);
		expect(p.skipped.slice(0, 2)).toEqual([
			{ line: 4, reason: 'It is not an issue.' },
			{ line: 5, reason: 'It has no title.' }
		]);
		expect(p.skipped.slice(2)).toEqual([
			{ line: MAX_ROWS + 2, reason: `An import takes at most ${MAX_ROWS} issues.` }
		]);
	});

	it('makes at most so many labels', () => {
		const labels = Array.from({ length: 100 }, (_, i) => ({ name: `tag ${i}` }));
		const issues = Array.from({ length: 11 }, (_, n) => ({
			labels: labels.map((l) => ({ name: `${l.name} of ${n}` }))
		}));
		const p = planIssues(readIssues(file(...issues)), { kind: 'inbox' }, ctx);
		expect(p.newLabels).toHaveLength(MAX_NEW_LABELS);
		expect(p.tasks[10].labels).toEqual([]);
		expect(p.warnings).toHaveLength(100);
	});
});

describe('a file that is not what gh writes', () => {
	it('is refused when it is not a list of issues', () => {
		for (const raw of ['', 'not json', '{"issues": []}', '"text"', '12', 'null'])
			expect(() => readIssues(raw), raw).toThrow(IssueFileError);
		expect(() => readIssues(JSON.stringify(new Array(MAX_ROWS * 4 + 1).fill({})))).toThrow(
			/at most 5000 issues/
		);
	});

	it('takes only values of the right type, and says what it could not read', () => {
		const read = readIssues(file({ number: '7', title: ['not', 'a', 'string'] }));
		expect(read.issues[0].title).toBe('');
		expect(read.issues[0].number).toBeNull();
		expect(read.names.get(1)).toBe('Entry 1');

		const odd = readIssues(
			file({
				title: 'Odd',
				state: 'MERGED',
				closedAt: 'yesterday',
				createdAt: '+275760-09-13T00:00:00.000Z',
				labels: 'bug',
				assignees: [{ login: 'has space' }, { login: 42 }, null, { login: 'ok-name' }],
				milestone: 'v1',
				comments: [
					null,
					{ body: 'no author', author: null, createdAt: 3 },
					{ body: '   ', author: { login: 'x' } },
					{ body: { text: 'object' }, author: { login: 'x' } }
				],
				url: 'javascript:alert(1)//github.com/a/b/issues/1'
			})
		);
		const [issue] = odd.issues;
		expect(issue).toMatchObject({
			title: 'Odd',
			closed: false,
			closedAt: null,
			createdAt: null,
			labels: [],
			milestone: null,
			assignees: ['ok-name'],
			url: null,
			comments: [{ author: null, at: null, body: 'no author' }]
		});
		expect(odd.repo).toBeNull();
		expect(issue.problems).toEqual([
			'Its state could not be read; it comes in open.',
			'Its labels could not be read.'
		]);
		const p = planIssues(odd, { kind: 'new', name: 'Imported' }, ctx, { date: day });
		expect(p.tasks[0].notes).toBe('');
		expect(p.tasks[0].comments).toEqual(['Originally by someone\n\nno author']);
		expect(p.warnings.map((w) => w.message)).toEqual(issue.problems);
	});

	it('never lets its keys reach an object prototype', () => {
		const raw =
			'[{"__proto__": {"polluted": true}, "constructor": {"prototype": {"polluted": true}},' +
			' "title": "Proto", "state": "OPEN", "labels": [{"__proto__": {"name": "x"}}],' +
			' "milestone": {"__proto__": {"title": "inherited"}},' +
			' "comments": [{"__proto__": {"body": "inherited"}}]}]';
		const read = readIssues(raw);
		expect(({} as Record<string, unknown>).polluted).toBeUndefined();
		expect(Object.prototype).not.toHaveProperty('polluted');
		const [issue] = read.issues;
		expect(issue).toMatchObject({ title: 'Proto', labels: [], milestone: null, comments: [] });
		const p = planIssues(read, { kind: 'inbox' }, ctx);
		expect(p.tasks[0].title).toBe('Proto');
	});

	it('cuts text to what the server takes and drops control characters', () => {
		const huge = 'x'.repeat(300_000);
		const read = readIssues(
			file({
				title: `  A\r\nline\u0000 ${'t'.repeat(600)}`,
				body: huge,
				labels: [{ name: `${'l'.repeat(70)}\n` }],
				milestone: { title: 'm'.repeat(90) },
				comments: [
					{
						author: { login: 'octocat' },
						createdAt: '2026-10-02T12:00:00Z',
						body: `Hi @[Ann](user:u-ann) and \u0007bell\r\n${huge}`
					}
				]
			})
		);
		const p = planIssues(read, { kind: 'inbox' }, ctx, { date: day });
		const [t] = p.tasks;
		expect(t.title.startsWith('A line ttt')).toBe(true);
		expect(t.title).toHaveLength(500);
		expect(t.notes.length).toBeLessThanOrEqual(100_000);
		expect(t.notes.startsWith('[GitHub issue #1](https://github.com/acme/rockets/issues/1)')).toBe(
			true
		);
		expect(p.newLabels).toEqual(['l'.repeat(60), 'm'.repeat(60)]);
		expect(t.comments[0].length).toBeLessThanOrEqual(20_000);
		// A mention in the file stays words, so importing it tells nobody.
		expect(
			t.comments[0].startsWith(
				`Originally by octocat, ${day(new Date('2026-10-02T12:00:00Z'))}\n\nHi @Ann and bell\nxxx`
			)
		).toBe(true);
		expect(JSON.stringify(t)).not.toMatch(/\\u000[07]/);
	});
});

describe('importing issues', () => {
	it('makes the tasks with their labels and comments', async () => {
		const ws = `gh-${Math.random()}`;
		const replica = engine.sample(ws, []);
		const read = readIssues(
			file(
				{ title: 'Open one', labels: [{ name: 'bug' }], milestone: { title: 'Launch' } },
				{
					title: 'Closed one',
					state: 'CLOSED',
					createdAt: '2026-10-01T08:00:00Z',
					closedAt: '2026-10-05T09:30:00Z',
					comments: [
						{ author: { login: 'octocat' }, createdAt: '2026-10-02T12:00:00Z', body: 'First' },
						{ author: { login: 'hubot' }, createdAt: '2026-10-03T12:00:00Z', body: 'Second' }
					]
				}
			)
		);
		const p = planIssues(
			read,
			{ kind: 'new', name: repoName(read.repo)! },
			{ ...ctx, projects: [], labels: [] },
			{ date: day }
		);
		const before = session.user;
		session.user = { id: 'u-me' } as typeof session.user;
		const ids = await runImport(ws, p).finally(() => (session.user = before));
		const project = replica.projects()[0];
		expect(project.name).toBe('rockets');
		const tasks = replica.tasks(project.id);
		expect(tasks.map((t) => t.id)).toEqual(ids);
		expect(tasks.map((t) => replica.status(t.statusId)?.name)).toEqual(['To do', 'Done']);
		expect(tasks[1].completedAt).toBe('2026-10-05T09:30:00.000Z');
		// Made when it was opened, not when it was imported.
		expect(tasks[1].madeAt).toBe('2026-10-01T08:00:00.000Z');
		expect(replica.taskLabels(tasks[0].id).map((l) => l.name)).toEqual(['bug', 'Launch']);
		const comments = replica.comments(tasks[1].id);
		expect(comments.map((c) => c.body)).toEqual([
			`Originally by octocat, ${day(new Date('2026-10-02T12:00:00Z'))}\n\nFirst`,
			`Originally by hubot, ${day(new Date('2026-10-03T12:00:00Z'))}\n\nSecond`
		]);
		expect(comments.every((c) => c.authorId === 'u-me')).toBe(true);
		expect(engine.canUndo).toBe(false);
		engine.dropSample(ws);
	});
});
