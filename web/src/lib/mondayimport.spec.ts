import { describe, expect, it } from 'vitest';
import { errandsBoard, gardenBoard, xlsx } from '../../e2e/fixtures/monday';
import { guessTargets, plan, summaryLines, type Target } from './csvimport';
import { runImport, type Context, type Destination } from './importplan';
import {
	MONDAY,
	plainText,
	readMonday,
	readMondayFiles,
	updateTime,
	type Workbook
} from './mondayimport';
import { session } from './session.svelte';
import { engine } from './sync/engine.svelte';
import { readXlsx } from './xlsx';

// Made-up boards of the same shape as monday.com writes (e2e/fixtures/monday.ts;
// every name and address in them is invented). Garden Club: two groups,
// subitems, totals, tags, item IDs and a sheet of updates. Errands: shared
// strings, other column names, and a subitem the file leaves out.
const book = async (name: string, data: Uint8Array): Promise<Workbook> => ({
	name,
	sheets: await readXlsx(data)
});
const GARDEN = await book('Garden_Club_1794094712.xlsx', gardenBoard());
const ERRANDS = await book('Errands.xlsx', errandsBoard());

const ctx: Context = {
	projects: [],
	statuses: () => [],
	labels: [],
	fields: [],
	members: [{ userId: 'u-ada', name: 'Ada Park', email: 'ada@example.com' }],
	reaches: () => true
};
const own: Destination = { kind: 'new', name: 'monday.com' };
const table = (books = [GARDEN]) => readMonday(books)!;
const dryRun = (books = [GARDEN]) => {
	const t = table(books);
	const targets = guessTargets(t.rows[0], [], MONDAY);
	const p = plan(t.rows, targets, own, ctx, { dialect: MONDAY, byColumn: true });
	return { p, targets, t };
};
const byTitle = (title: string, p = dryRun().p) => p.tasks.find((t) => t.title === title)!;

describe("monday.com's board", () => {
	it('becomes a table of a row an item or subitem, under its group and parent', () => {
		const t = table();
		expect(t.rows[0]).toEqual([
			'Item ID',
			'Board',
			'Group and tags',
			'Name',
			'Parent item',
			'Updates',
			'Owner',
			'Status',
			'Due date',
			'Priority',
			'Files',
			'Timeline - Start',
			'Timeline - End',
			'Budget',
			'Notes'
		]);
		expect(t.rows.slice(1).map((r) => [r[0], r[1], r[2], r[3], r[4], r[8]])).toEqual([
			['5001', 'Garden Club', 'This week, spring', 'Order seeds', '', '2026-11-08'],
			// A subitem's Date is its item's Due date.
			['5011', 'Garden Club', 'This week', 'Pick varieties', '5001', '2026-11-07'],
			['5012', 'Garden Club', 'This week', 'Check the budget', '5001', ''],
			['5002', 'Garden Club', 'This week', 'Build a cold frame', '', '2026-11-13'],
			['5003', 'Garden Club', 'This week', 'Water the beds', '', ''],
			['5004', 'Garden Club', 'Later, autumn, bulbs', 'Plant bulbs', '', '2026-12-06'],
			// An item with only a name, and no ID of its own.
			['1-17', 'Garden Club', 'Later', 'Mend the hose', '', ''],
			['5005', 'Garden Club', 'Later', 'Water the beds', '', '']
		]);
		// Each row is named by its place in the sheet, the totals left out.
		expect(t.where.slice(2, 6)).toEqual(['Row 6', 'Row 8', 'Row 9', 'Row 10']);
		expect(t.left).toEqual([
			{ line: 10, reason: 'It is an update on an item that is not in the file.' },
			{
				line: 11,
				reason:
					'It is an update on "Water the beds", a name more than one item has, so it is left out.'
			}
		]);
		expect(t.where.slice(10)).toEqual(['updates, row 6', 'updates, row 7']);
		expect(t.said).toEqual([]);
		// The headings come after a title row; a reply is a comment after its update.
		expect(JSON.parse(t.rows[1][5])).toEqual([
			{
				author: 'Ada Park',
				at: '2026-10-02T10:30:00.000Z',
				body: 'Ordered from Green & Co.\nArrives Friday.'
			},
			{ author: 'Zed Quinn', at: expect.stringMatching(/^2026-10-02T/), body: 'Thanks!' }
		]);
		// An update with no ID goes on the item of its name.
		expect(JSON.parse(t.rows[4][5])).toEqual([
			{ author: 'Zed Quinn', at: '2026-10-03T09:00:00.000Z', body: 'Need hinges.' }
		]);
	});

	it('takes several files, a project each, and says what they leave out', () => {
		const t = table([GARDEN, ERRANDS]);
		const errands = t.rows.filter((r) => r[1] === 'Errands');
		// Person is Owner and Date is Due date, whatever a board calls them.
		expect(errands.map((r) => [r[0], r[2], r[3], r[6], r[7], r[8]])).toEqual([
			['2-5', 'Town', 'Post the parcel', 'Ada Park', '', '2026-11-05'],
			['2-6', 'Town', 'Return the books', '', 'Done', '']
		]);
		expect(t.where[10]).toBe('Errands.xlsx, row 5');
		expect(t.where.at(-1)).toBe('Garden_Club_1794094712.xlsx, updates, row 7');
		expect(t.said).toEqual([
			'1 of the files has no updates. If the board has some, export it again with updates ticked to bring them as comments',
			'1 item has subitems that are not in the file. Export the board again with subitems ticked to bring them as subtasks'
		]);
	});

	it('is named by its file where the sheet gives no name, and is no board without headings', async () => {
		const bare = await book(
			'Spring_Plan_1794094712.xlsx',
			xlsx([
				{
					name: 'S',
					rows: [
						['Name', 'Status'],
						['Dig', 'Done']
					]
				}
			])
		);
		expect(table([bare]).rows[1].slice(1, 4)).toEqual(['Spring Plan', '', 'Dig']);
		const none = await book('x.xlsx', xlsx([{ name: 'S', rows: [['Title', 'Status']] }]));
		expect(readMonday([none])).toBeNull();
		expect(readMonday([])).toBeNull();
	});

	it('is due when its timeline ends, where it has no date of its own', async () => {
		const board = await book(
			'Plan.xlsx',
			xlsx([
				{
					name: 'Plan',
					rows: [
						['Plan'],
						[],
						['Now'],
						['Name', 'Subitems', 'Due date', 'Timeline - Start', 'Timeline - End'],
						['Dated', 'Undated', { date: 46340 }, { date: 46337 }, { date: 46347 }],
						['Subitems', 'Name', 'Date', 'Timeline - Start', 'Timeline - End'],
						[null, 'Undated', null, { date: 46337 }, { date: 46347 }],
						['Neither', null, null, null, null]
					]
				}
			])
		);
		const t = table([board]);
		const at = (h: string) => t.rows[0].indexOf(h);
		expect(
			t.rows.slice(1).map((r) => [r[3], r[at('Due date')], r[at('Timeline - Start')]])
		).toEqual([
			['Dated', '2026-11-14', '2026-11-11'],
			['Undated', '2026-11-21', '2026-11-11'],
			['Neither', '', '']
		]);
	});

	it('is read from the files chosen', async () => {
		const file = new File([gardenBoard() as BlobPart], 'Garden.xlsx');
		expect((await readMondayFiles([file]))!.rows).toHaveLength(9);
		await expect(readMondayFiles([new File(['Name,Status'], 'a.xlsx')])).rejects.toThrow(
			'could not be read as an Excel workbook'
		);
	});
});

describe('A monday.com dry run', () => {
	it('matches every column it can', () => {
		const { t, targets } = dryRun();
		expect(Object.fromEntries(t.rows[0].map((h, i) => [h, targets[i]]))).toEqual<
			Record<string, Target>
		>({
			'Item ID': 'key',
			Board: 'project',
			'Group and tags': 'labels',
			Name: 'title',
			'Parent item': 'parent',
			Updates: 'comments',
			Owner: 'assignees',
			Status: 'status',
			'Due date': 'due',
			Priority: 'priority',
			Files: 'attachments',
			'Timeline - Start': 'start',
			'Timeline - End': 'skip',
			Budget: 'skip',
			Notes: 'notes'
		});
	});

	it('makes the board a project, its statuses, groups and tags, people and subitems', () => {
		const { p } = dryRun();
		expect(p.newProjects).toEqual(['Garden Club']);
		expect(p.newStatuses.map((s) => [s.name, s.category])).toEqual([
			['Working on it', 'in-progress'],
			['Stuck', 'in-progress']
		]);
		expect(p.newLabels).toEqual(['This week', 'spring', 'Later', 'autumn', 'bulbs']);
		expect(
			['Order seeds', 'Build a cold frame', 'Plant bulbs', 'Mend the hose'].map(
				(title) => byTitle(title, p).priority
			)
		).toEqual(['high', 'urgent', 'low', 'none']);
		const seeds = byTitle('Order seeds', p);
		expect([seeds.start, seeds.due, seeds.assignees]).toEqual([
			'2026-11-07',
			'2026-11-08',
			['u-ada']
		]);
		expect(seeds.notes).toBe(
			'From the spring catalog.\n\n**Files**\n\n- [seeds.pdf](https://files.example.com/seeds.pdf)'
		);
		expect(seeds.comments).toEqual([
			expect.stringMatching(/^Originally by Ada Park, Oct 2, 2026\n\nOrdered from Green & Co\./),
			expect.stringMatching(/^Originally by Zed Quinn, Oct 2, 2026\n\nThanks!$/)
		]);
		expect(byTitle('Pick varieties', p).parent).toBe(seeds.key);
		expect(byTitle('Plant bulbs', p).completedAt).not.toBeNull();
		expect(p.strangers).toEqual([{ name: 'Zed Quinn', lines: [4, 5] }]);
		expect(p.warnings).toEqual([]);
	});

	it('says what it brings and what stays behind', () => {
		const { p, targets } = dryRun();
		expect(summaryLines(p, targets, MONDAY)).toEqual([
			'2 subtasks, each under its parent task',
			'3 comments, each starting with who wrote it and when',
			'1 file kept as links at the end of the notes; the files themselves stay in monday.com',
			'2 tasks are done with no date for it in the file, so they come in done as of today',
			"Updates on subitems are not in monday.com's file, so they stay in monday.com"
		]);
	});
});

describe("monday.com's updates", () => {
	it('are dated as monday.com writes their times, in UTC', () => {
		const iso = (s: string) => updateTime(s)?.toISOString() ?? null;
		expect(iso('10/November/2026  03:38:40 PM')).toBe('2026-11-10T15:38:40.000Z');
		expect(iso('1/Jan/2026 12:05:00 AM')).toBe('2026-01-01T00:05:00.000Z');
		expect(iso('1/January/2026 12:05 PM')).toBe('2026-01-01T12:05:00.000Z');
		expect(iso('2026-10-03 09:00:00 UTC')).toBe('2026-10-03T09:00:00.000Z');
		expect(iso('31/February/2026 01:00:00 PM')).toBeNull();
		expect(iso('10/Smarch/2026 01:00:00 PM')).toBeNull();
		expect(iso('10/November/2026 13:00:00 PM')).toBeNull();
		expect(iso('soon')).toBeNull();
	});

	it('are plain text where they were written as HTML', () => {
		expect(plainText('<p>One &amp; two</p><ul><li>a</li><li>b</li></ul><p>x<br>y</p>')).toBe(
			'One & two\n- a\n- b\nx\ny'
		);
		expect(plainText('  5 < 6 and no tags  ')).toBe('5 < 6 and no tags');
		expect(plainText('<p>a < b</p>  \n<b>c</b>  d')).toBe('a < b\n\nc  d');
	});

	it('are read in time in line with their length, however many tags are left open', () => {
		const texts: [string, string][] = [
			['tags never closed', 'x</p>' + '<a'.repeat(100_000)],
			['list items never closed', 'x</p>' + '<li'.repeat(70_000)],
			['a run of spaces', '<b>x</b>' + ' '.repeat(200_000) + 'x']
		];
		for (const [what, text] of texts) {
			const start = performance.now();
			plainText(text);
			expect(performance.now() - start, what).toBeLessThan(1000);
		}
	});
});

describe('A monday.com board a stranger made', () => {
	it('has a group named with a long run of spaces read in time in line with its length', () => {
		// Spaces with no comma after them, which a pattern starting at them tries one by one.
		const start = performance.now();
		const t = readMonday([
			{
				name: 'Board.xlsx',
				sheets: [
					{
						name: 'Board',
						rows: [
							['Board'],
							[],
							[`Group${' '.repeat(200_000)}x, y`],
							['Name', 'Item ID'],
							['Task', '1']
						]
					}
				]
			}
		])!;
		expect(performance.now() - start).toBeLessThan(1000);
		expect(t.rows[1][2]).toBe(`Group${' '.repeat(200_000)}x y`);
	});
});

describe('importing monday.com boards', () => {
	it('makes the projects, subtasks, labels and comments', async () => {
		const ws = `md-${Math.random()}`;
		const replica = engine.sample(ws, []);
		const { p } = dryRun([GARDEN, ERRANDS]);
		const before = session.user;
		session.user = { id: 'u-me' } as typeof session.user;
		const ids = await runImport(ws, p).finally(() => (session.user = before));
		expect(ids).toHaveLength(10);
		const garden = replica.projects().find((x) => x.name === 'Garden Club')!;
		const seeds = replica.tasks(garden.id).find((t) => t.title === 'Order seeds')!;
		expect(replica.subtasks(seeds.id).map((t) => t.title)).toEqual([
			'Pick varieties',
			'Check the budget'
		]);
		expect(replica.projects().map((x) => x.name)).toContain('Errands');
		engine.dropSample(ws);
	});
});
