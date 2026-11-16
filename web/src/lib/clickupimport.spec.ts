import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { CLICKUP, NO_TIME, OTHER_TYPES, isClickUp } from './clickupimport';
import { parseCsv } from './csv';
import { guessTargets, plan, summaryLines, type Target } from './csvimport';
import { linkParents, parentsFirst, runImport, type Context, type PlannedTask } from './importplan';
import { session } from './session.svelte';
import { engine } from './sync/engine.svelte';

// A made-up export of the same shape as ClickUp writes (every name and
// address in it is invented): tasks in three lists over two spaces, a list
// name in both spaces, a subtask, a Lead and an Account, a task twice, and
// subtasks whose parents are left out or missing.
const SAMPLE = readFileSync(
	join(import.meta.dirname, '..', '..', 'e2e', 'fixtures', 'clickup-tasks.csv'),
	'utf8'
);
const rows = parseCsv(SAMPLE);
const targets = guessTargets(rows[0], [], CLICKUP);
const day = (at: Date) =>
	[at.getFullYear(), at.getMonth() + 1, at.getDate()]
		.map((n) => String(n).padStart(2, '0'))
		.join('-');
/** A cell of the sample, by its row's line and its heading. */
const cellOf = (line: number, heading: string) => rows[line - 1][rows[0].indexOf(heading)];
const localDay = (ms: string) => day(new Date(Number(ms)));

const ctx: Context = {
	projects: [{ id: 'p-web', name: 'Website' }],
	statuses: (id) =>
		id === 'p-web'
			? [
					{ id: 's-next', name: 'Next', category: 'todo' },
					{ id: 's-done', name: 'Complete', category: 'done' }
				]
			: [],
	labels: [],
	fields: [],
	members: [
		{ userId: 'u-ada', name: 'Ada Park', email: 'ada@example.com' },
		{ userId: 'u-gus', name: 'Gus', email: 'gus@example.com' }
	],
	reaches: (userId) => userId !== 'u-gus'
};

const own = { kind: 'new', name: 'ClickUp' } as const;
const dryRun = (r = rows, t = targets, opts = {}) =>
	plan(r, t, own, ctx, { dialect: CLICKUP, date: day, ...opts });

/** One row of ClickUp's columns, from the few a test sets. */
function file(...given: Record<string, string>[]): string[][] {
	const head = rows[0];
	return [
		head,
		...given.map((g, n) =>
			head.map((h) => {
				if (Object.hasOwn(g, h)) return g[h];
				if (h === 'Task ID') return `t${n + 1}`;
				if (h === 'Task Name') return `Task ${n + 1}`;
				if (h === 'Task Type') return 'Task';
				if (h === 'List Name') return 'List';
				if (h === 'Space Name') return 'Space';
				if (h === 'Parent ID' || h === 'Priority') return 'null';
				return '';
			})
		)
	];
}

describe("ClickUp's columns", () => {
	it('are matched before the person sees them', () => {
		const at = (h: string) => targets[rows[0].indexOf(h)];
		expect(isClickUp(rows[0])).toBe(true);
		expect(isClickUp(['Title', 'Due'])).toBe(false);
		expect(
			Object.fromEntries(rows[0].map((h, i) => [h, targets[i]]).filter(([, t]) => t !== 'skip'))
		).toEqual<Record<string, Target>>({
			'Task ID': 'key',
			'Task Link': 'link',
			'Task Type': 'type',
			'Task Name': 'title',
			'Task Content': 'notes',
			Status: 'status',
			'Date Created': 'made',
			'Due Date': 'due',
			'Due Date Text': 'dueTime',
			'Start Date': 'start',
			'Parent ID': 'parent',
			Attachments: 'attachments',
			Assignees: 'assignees',
			Tags: 'labels',
			Priority: 'priority',
			'List Name': 'project',
			'Space Name': 'space',
			'Time Estimated': 'estimate',
			Checklists: 'checklist',
			Comments: 'comments'
		});
		// Time spent stays behind; the dry run says so.
		expect(at('Time Spent')).toBe('skip');
		expect(at('Subtasks IDs')).toBe('skip');
	});

	it('take a closing date where the export has one', () => {
		expect(guessTargets(['Task Name', 'Date Closed', 'Date Done'], [], CLICKUP)).toEqual([
			'title',
			'completed',
			'skip'
		]);
	});

	it('leave other types out, as Shawn chose', () => {
		expect(OTHER_TYPES).toBe('skip');
		expect(NO_TIME).toEqual(['00:00', '04:00']);
	});
});

describe('the dry run of a ClickUp file', () => {
	const p = dryRun();
	const byTitle = (title: string) => p.tasks.find((t) => t.title === title)!;

	it('leaves out what is not a task, and a task the file has twice', () => {
		expect(p.tasks.map((t) => t.title)).toEqual([
			'Plant the bulbs',
			'Order compost',
			'Fix the gate latch',
			'Mail the tax forms',
			'Call the bakery about bread',
			'Sweep the porch',
			'Water the ferns'
		]);
		expect(p.skipped).toEqual([
			{ line: 6, reason: 'It is a ClickUp Lead, not a task.' },
			{ line: 7, reason: 'It is a ClickUp Account, not a task.' },
			{ line: 9, reason: 'It is the same task as row 4.' }
		]);
		expect(p.otherTypes).toEqual([
			{ name: 'Lead', count: 1 },
			{ name: 'Account', count: 1 }
		]);
	});

	it('makes a project for each list, naming the space only where two lists share a name', () => {
		expect(p.newProjects).toEqual(['Garden', 'Errands (Home)', 'Errands (Work)', 'Prospects']);
		expect(byTitle('Fix the gate latch').project).toEqual({ make: 'project:Errands (Home)' });
		expect(byTitle('Mail the tax forms').project).toEqual({ make: 'project:Errands (Work)' });
	});

	it("keeps ClickUp's statuses, each in a category by its name", () => {
		expect(p.newStatuses.map((s) => [s.name, s.category])).toEqual([
			['complete', 'done'],
			['stuck', 'in-progress'],
			['Closed', 'done'],
			['upcoming', 'todo'],
			['upcoming', 'todo'],
			['upcoming', 'todo']
		]);
		// "in progress" is a status new projects have already.
		expect(byTitle('Plant the bulbs').status).toEqual({
			make: 'status:make:project:garden|In progress'
		});
		// Done ones are done as of the import, since the file says not when.
		expect(p.completedNow).toEqual([3, 5]);
		expect(byTitle('Mail the tax forms').completedAt).not.toBeNull();
		expect(byTitle('Fix the gate latch').completedAt).toBeNull();
	});

	it('reads a task whole: notes with a link back and files, dates, people, checklist, comments', () => {
		const t = byTitle('Plant the bulbs');
		const made = new Date(Number(cellOf(2, 'Date Created'))).toISOString();
		expect(t).toMatchObject<Partial<PlannedTask>>({
			key: '86aaa0001',
			parent: null,
			priority: 'high',
			due: localDay(cellOf(2, 'Due Date')),
			// ClickUp's dates without a time are at 4 AM; no due time.
			dueTime: null,
			start: localDay(cellOf(2, 'Start Date')),
			madeAt: made,
			assignees: ['u-ada'],
			estimate: 90,
			checklist: [
				{ text: 'Buy bulbs', done: true },
				{ text: 'Dig the beds', done: false }
			],
			comments: [
				`Originally by Ada Park, 2026-10-02\n\nBought forty.`,
				`Originally by zed.quinn, 2026-10-03\n\nThe shop had none left.`
			]
		});
		expect(t.notes).toBe(
			`[ClickUp task](https://app.clickup.com/t/86aaa0001), made ${day(new Date(made))}\n\n` +
				'Tulips along the fence first.\nThen the daffodils by the gate.\n\n' +
				'**Files**\n\n' +
				'- [bed plan.png](https://t9011000.p.clickup-attachments.com/t9011000/aaaa-1111/bed%20plan.png)'
		);
		expect(p.links).toBe(1);
		expect(byTitle('Water the ferns').labels).toEqual([{ make: 'label:weekly' }]);
		expect(p.newLabels).toEqual(['weekly']);
	});

	it("puts a subtask under its parent, and one whose parent isn't coming on its own", () => {
		const sub = byTitle('Order compost');
		expect(sub).toMatchObject({
			parent: '86aaa0001',
			project: { make: 'project:Garden' },
			priority: 'low',
			due: localDay(cellOf(3, 'Due Date')),
			dueTime: '13:00'
		});
		expect(byTitle('Call the bakery about bread').parent).toBeNull();
		expect(byTitle('Sweep the porch').parent).toBeNull();
		expect(p.warnings).toEqual([
			{ line: 8, message: 'Its parent task is left out, so it comes in on its own.' },
			{ line: 10, message: 'Its parent task is not in the file, so it comes in on its own.' }
		]);
	});

	it('lists people who are not here once, with the rows that name them', () => {
		expect(p.strangers).toEqual([{ name: 'Zed Quinn', lines: [3] }]);
		expect(p.unassigned).toEqual([]);
	});

	it('says what else comes in, and what stays behind', () => {
		expect(summaryLines(p, targets, CLICKUP)).toEqual([
			'2 rows are not tasks in ClickUp, so they are left out: 1 of type Lead, 1 of type Account',
			'1 subtask, each under its parent task',
			'2 comments, each starting with who wrote it and when',
			'2 checklist items',
			'Estimates on 1 task',
			'1 file kept as links at the end of the notes; the files themselves stay in ClickUp',
			"Each task's notes start with a link back to it in ClickUp",
			'2 tasks are done with no date for it in the file, so they come in done as of today',
			'Time tracked in ClickUp is left out'
		]);
	});

	it('puts everything in one project, or the inbox, when the person chooses', () => {
		const into = plan(rows, targets, { kind: 'project', id: 'p-web' }, ctx, {
			dialect: CLICKUP,
			byColumn: false
		});
		expect(into.newProjects).toEqual([]);
		expect(new Set(into.tasks.map((t) => JSON.stringify(t.project)))).toEqual(
			new Set([JSON.stringify({ id: 'p-web' })])
		);
		// The project's own Complete is "complete" by name.
		expect(into.tasks[1].status).toEqual({ id: 's-done' });
		expect(into.tasks[1].parent).toBe('86aaa0001');

		const inbox = plan(rows, targets, { kind: 'inbox' }, ctx, {
			dialect: CLICKUP,
			byColumn: false
		});
		expect(inbox.tasks.every((t) => t.project === null && t.status === null)).toBe(true);
		expect(inbox.tasks[1].parent).toBe('86aaa0001');
	});

	it('can bring other types in with a label named after the type instead', () => {
		const labeled = dryRun(rows, targets, { dialect: { ...CLICKUP, otherTypes: 'label' } });
		expect(labeled.skipped).toEqual([{ line: 9, reason: 'It is the same task as row 4.' }]);
		const lead = labeled.tasks.find((t) => t.title === 'Corner bakery')!;
		expect(lead.labels).toEqual([{ make: 'label:Lead' }]);
		expect(labeled.tasks.find((t) => t.title === 'Call the bakery about bread')!.parent).toBe(
			'86aaa0005'
		);
	});
});

describe('a ClickUp file that is odd or hostile', () => {
	it('reads what it can and says what it could not, never failing', () => {
		const p = dryRun(
			file(
				{
					'Task Name': 'Odd\u0000 one\u0007',
					'Task Content': 'Hi @[Ann](user:u-ann)\\nbye\u0000',
					Priority: '7',
					'Due Date': 'tomorrow',
					'Due Date Text': 'soon',
					'Date Created': '99999999999999999',
					'Time Estimated': 'NaN',
					Assignees: '[]',
					Tags: '',
					Checklists: '{"A": "not a list", "B": [1, {"nothing": true}, "Fine"]}',
					Comments: '{"text": "not a list"}',
					Attachments:
						'[{"title": "x", "url": "javascript:alert(1)"}, {"url": "http://a.example/f"}, "s"]',
					'Task Link': 'javascript:alert(1)'
				},
				{
					'Time Estimated': '""',
					'Time Spent': ' ""',
					Comments:
						'[null, {"text": 5}, {"text": "  "}, {"text": "kept", "by": 7, "date": "never"}]',
					Checklists: 'not json',
					Attachments: '[{"title": "a]b[c\\\\d", "url": "https://files.example.com/a(b).png"}]'
				}
			)
		);
		const [odd, two] = p.tasks;
		expect(odd.title).toBe('Odd one');
		expect(odd.notes).toBe('Hi @Ann\nbye');
		expect(odd).toMatchObject({ priority: 'none', due: null, madeAt: null, estimate: null });
		expect(odd.checklist).toEqual([{ text: 'Fine', done: false }]);
		expect(odd.comments).toEqual([]);
		expect(p.warnings.filter((w) => w.line === 2).map((w) => w.message)).toEqual([
			'"7" is not a priority (1 urgent, 2 high, 3 normal, 4 low).',
			'"tomorrow" is not a date.',
			'"soon" is not a time like 14:30.',
			'"99999999999999999" is not a date.',
			'Its link back is not a web address, so it is left out.',
			'3 checklist items could not be read and are left out.',
			'Its comments could not be read.',
			'3 attachments could not be read and are left out.'
		]);
		expect(two.comments).toEqual(['kept']);
		expect(two.checklist).toEqual([]);
		// Parentheses in an address are written so they cannot end the link.
		expect(two.notes).toBe(
			'**Files**\n\n- [a\\]b\\[c\\\\d](https://files.example.com/a%28b%29.png)'
		);
		expect(p.warnings.filter((w) => w.line === 3).map((w) => w.message)).toEqual([
			'Its checklists could not be read.',
			'2 comments could not be read and are left out.'
		]);
		expect(JSON.stringify(p)).not.toMatch(/\\u000[07]|javascript/);
	});

	it('escapes brackets in a file name, so it cannot write its own link', () => {
		const p = dryRun(
			file({
				Attachments:
					'[{"title": "x](https://evil.example)[y", "url": "https://files.example.com/f.png"}]'
			})
		);
		expect(p.tasks[0].notes).toBe(
			'**Files**\n\n- [x\\](https://evil.example)\\[y](https://files.example.com/f.png)'
		);
	});

	it('never lets its keys reach an object prototype', () => {
		const p = dryRun(
			file({
				Comments: '[{"__proto__": {"text": "inherited"}, "constructor": {"prototype": {"x": 1}}}]',
				Checklists: '{"__proto__": [{"name": "inherited"}]}',
				Attachments: '[{"__proto__": {"url": "https://a.example/x"}}]',
				Assignees: '[{"__proto__": {"name": "Ada Park"}}]'
			})
		);
		expect(Object.prototype).not.toHaveProperty('text');
		expect(Object.prototype).not.toHaveProperty('x');
		const [t] = p.tasks;
		expect(t.comments).toEqual([]);
		expect(t.notes).toBe('');
		expect(t.assignees).toEqual([]);
		// A key that is its own: "__proto__" as JSON writes it is the object's own.
		expect(t.checklist).toEqual([{ text: 'inherited', done: false }]);
	});

	it('breaks loops in parents, and a task that is its own parent', () => {
		const p = dryRun(
			file(
				{ 'Task ID': 'a', 'Parent ID': 'b' },
				{ 'Task ID': 'b', 'Parent ID': 'a' },
				{ 'Task ID': 'c', 'Parent ID': 'c' }
			)
		);
		expect(p.tasks.map((t) => [t.key, t.parent])).toEqual([
			['a', 'b'],
			['b', null],
			['c', null]
		]);
		expect(p.warnings.map((w) => w.message)).toEqual([
			'Its parent tasks loop back to it, so it comes in on its own.',
			'It names itself as its parent, so it comes in on its own.'
		]);
	});

	it('cuts text to what the server takes', () => {
		const huge = 'x'.repeat(300_000);
		const p = dryRun(
			file({
				'Task Name': 't'.repeat(900),
				'Task Content': huge,
				'List Name': 'l'.repeat(300),
				Status: 's'.repeat(90),
				Tags: `[${'g'.repeat(90)}]`,
				Checklists: JSON.stringify({ C: ['c'.repeat(900)] }),
				Comments: JSON.stringify([{ text: huge, by: 'a@example.com', date: '1/2/2026' }]),
				'Time Estimated': String(2000 * 3_600_000)
			})
		);
		const [t] = p.tasks;
		expect(t.title).toHaveLength(500);
		expect(t.notes.length).toBeLessThanOrEqual(100_000);
		expect(p.newProjects[0]).toHaveLength(200);
		expect(p.newStatuses[0].name).toHaveLength(60);
		expect(p.newLabels[0]).toHaveLength(60);
		expect(t.checklist[0].text).toHaveLength(500);
		expect(t.comments[0].length).toBeLessThanOrEqual(20_000);
		expect(t.comments[0].startsWith('Originally by a, 2026-01-02\n\nxxx')).toBe(true);
		expect(t.estimate).toBe(999 * 60);
	});
});

describe('parents', () => {
	it('are linked only where they come in, no deeper than the server takes', () => {
		const chain = Array.from({ length: 60 }, (_, i) => ({
			line: i + 2,
			key: `k${i}`,
			parent: i ? `k${i - 1}` : null
		}));
		const linked = linkParents(chain);
		expect(linked.get(2)).toEqual(expect.objectContaining({ parent: null, top: 2 }));
		expect(linked.get(52)).toEqual(expect.objectContaining({ parent: 'k49', top: 2 }));
		expect(linked.get(53)!.problem).toBe(
			'It is more than 50 subtasks deep, so it comes in on its own.'
		);
		expect(linked.get(54)).toEqual(expect.objectContaining({ parent: 'k51', top: 53 }));
	});

	it('come before their subtasks when made', () => {
		const t = (key: string, parent: string | null) => ({ key, parent }) as PlannedTask;
		const order = parentsFirst([t('c', 'b'), t('a', null), t('b', 'a'), t('d', null)]);
		expect(order.map((x) => x.key)).toEqual(['a', 'd', 'b', 'c']);
	});
});

describe('importing a ClickUp file', () => {
	it('makes subtasks under their parents, with checklists, estimates and comments', async () => {
		const ws = `cu-${Math.random()}`;
		const replica = engine.sample(ws, []);
		const p = plan(rows, targets, own, { ...ctx, projects: [] }, { dialect: CLICKUP, date: day });
		const before = session.user;
		session.user = { id: 'u-me' } as typeof session.user;
		const ids = await runImport(ws, p).finally(() => (session.user = before));
		expect(ids).toHaveLength(7);
		const garden = replica.projects().find((x) => x.name === 'Garden')!;
		const [plant, water] = replica.tasks(garden.id);
		expect([plant.title, water.title]).toEqual(['Plant the bulbs', 'Water the ferns']);
		expect(plant.estimateMinutes).toBe(90);
		expect(plant.madeAt).toBe(new Date(Number(cellOf(2, 'Date Created'))).toISOString());
		expect(replica.checklist(plant.id).map((i) => [i.text, i.done])).toEqual([
			['Buy bulbs', true],
			['Dig the beds', false]
		]);
		expect(replica.comments(plant.id)).toHaveLength(2);
		const [sub] = replica.subtasks(plant.id);
		expect(sub.title).toBe('Order compost');
		expect(sub.projectId).toBe(garden.id);
		expect(replica.status(sub.statusId)?.name).toBe('complete');
		expect(sub.dueTime).toBe('13:00');
		expect(replica.projects().map((x) => x.name)).toEqual([
			'Garden',
			'Errands (Home)',
			'Errands (Work)',
			'Prospects'
		]);
		expect(engine.canUndo).toBe(false);
		engine.dropSample(ws);
	});
});
