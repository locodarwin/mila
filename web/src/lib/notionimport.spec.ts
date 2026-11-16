import { describe, expect, it, vi } from 'vitest';
import { IDS, notionExport, notionFiles, notionFlatFiles } from '../../e2e/fixtures/notion';
import { zip } from '../../e2e/fixtures/zip';
import { summaryLines } from './csvimport';
import type { Context } from './importplan';
import {
	NOTION,
	NotionError,
	convertPage,
	openNotion,
	planNotion,
	readNotion,
	readNotionDate,
	runNotion,
	type NotionEntry,
	type Ref
} from './notionimport';
import { session } from './session.svelte';
import { engine } from './sync/engine.svelte';

vi.mock('./uploads', async (actual) => ({
	...(await actual<typeof import('./uploads')>()),
	uploadFile: vi.fn(async (_ws: string, _to: unknown, file: File) => {
		if (file.name === 'checklist.pdf') throw new Error('no connection');
		return `att-${file.name}`;
	})
}));

// A made-up export of the same shape as Notion writes (e2e/fixtures/notion.ts;
// every name in it is invented): a Part zip inside the zip, pages under
// pages, links, an image and a file, a callout, a toggle, and two databases.
const EXPORT = await readNotion(await openNotion(notionExport()));
const HANDBOOK = `Team handbook ${IDS.handbook}`;
const ONBOARDING = `${HANDBOOK}/Onboarding ${IDS.onboarding}`;
const READING = `${HANDBOOK}/Reading list ${IDS.reading}`;
const TASKS = `Tasks ${IDS.tasks}`;

const ctx: Context = {
	projects: [],
	statuses: () => [],
	labels: [],
	fields: [],
	members: [{ userId: 'u-ada', name: 'Ada Park', email: 'ada@example.com' }],
	reaches: () => true
};
const refOf = (body: string, n = 0) => EXPORT.refs[+[...body.matchAll(/mila-notion:(\d+)/g)][n][1]];

describe("Notion's export", () => {
	it('is read through the Part zip inside it', async () => {
		const files = await openNotion(notionExport());
		expect(files.map((f) => f.path)).toContain(`${ONBOARDING}/desk.png`);
		expect(
			new TextDecoder().decode(await files.find((f) => f.path.endsWith('desk.png'))!.read())
		).toContain('PNG');
		await expect(openNotion(new TextEncoder().encode('not a zip'))).rejects.toThrow(NotionError);
	});

	it('makes a tree of its pages, and its databases with their rows', () => {
		expect([...EXPORT.pages.values()].map((p) => [p.title, p.parent])).toEqual([
			['Team handbook', null],
			['Onboarding', HANDBOOK],
			['The Quiet Garden', READING],
			['Plan the offsite', TASKS],
			['Untitled', TASKS]
		]);
		const reading = EXPORT.databases.get(READING)!;
		// The file of every property, not the view's.
		expect([reading.name, reading.parent, reading.headers]).toEqual([
			'Reading list',
			HANDBOOK,
			['Name', 'Author', 'Read']
		]);
		expect(reading.rows.map((r) => [r.cells[0], r.page])).toEqual([
			['The Quiet Garden', `${READING}/The Quiet Garden ${IDS.garden}`],
			['Rivers of Salt', null]
		]);
		// A page in a database with no row of its own is one more row.
		expect(EXPORT.databases.get(TASKS)!.rows.map((r) => r.cells[0])).toEqual([
			'Plan the offsite',
			'Book the venue',
			'Send invites',
			'Untitled'
		]);
	});

	it('makes the tree from the links in an export without folders for subpages', async () => {
		const flat = await readNotion(await openNotion(zip(notionFlatFiles())));
		const titles = new Map([...flat.pages.values()].map((p) => [p.key, p.title]));
		expect(
			[...flat.pages.values()].map((p) => [p.title, p.parent && titles.get(p.parent)])
		).toEqual([
			['Recipes', null],
			['Soups', 'Recipes'],
			['Breads', 'Recipes'],
			['Stock', 'Soups']
		]);
		const soups = [...flat.pages.values()].find((p) => p.title === 'Soups')!;
		// The picture beside it at the top is still its own.
		const pot = soups.body.match(/!\[Pot\]\(mila-notion:(\d+)\)/)!;
		expect(flat.refs[+pot[1]]).toEqual({ kind: 'file', path: 'pot.png' });
	});

	it("makes each page's Markdown Mila's", () => {
		const handbook = EXPORT.pages.get(HANDBOOK)!;
		expect(handbook.body).toBe(
			[
				'Welcome to the team. Start with [Onboarding](mila-notion:0), and the old wiki is gone.',
				'',
				'> [!NOTE]',
				'> 💡 Ask in the team channel first.',
				'',
				'**Office hours**',
				'',
				'Tuesdays at 10.',
				'',
				'[Reading list](mila-notion:1)',
				'',
				'```html',
				'<aside>kept as it is</aside>',
				'```'
			].join('\n')
		);
		expect([handbook.callouts, handbook.toggles, handbook.missing]).toEqual([1, 1, 1]);
		expect(refOf(handbook.body, 0)).toEqual({ kind: 'page', key: ONBOARDING });
		expect(refOf(handbook.body, 1)).toEqual({ kind: 'db', key: READING });
		const onboarding = EXPORT.pages.get(ONBOARDING)!.body;
		// An image stays one; a file is a link; a link back up resolves too.
		expect(onboarding).toMatch(
			/^!\[Desk\]\(mila-notion:\d+\)\n\nThe \[checklist\]\(mila-notion:\d+\)/
		);
		expect([0, 1, 2].map((n) => refOf(onboarding, n))).toEqual<Ref[]>([
			{ kind: 'file', path: `${ONBOARDING}/desk.png` },
			{ kind: 'file', path: `${ONBOARDING}/checklist.pdf` },
			{ kind: 'page', key: HANDBOOK }
		]);
		// A row's properties come off its page, but only its database's.
		const garden = EXPORT.pages.get(`${READING}/The Quiet Garden ${IDS.garden}`)!;
		expect(garden.props).toEqual([
			['Author', 'Lena Marsh'],
			['Read', 'Yes']
		]);
		expect(garden.body).toBe('Note: a slow book, worth it.');
	});

	it('converts what a page writes the way Notion does', () => {
		const page = (md: string) =>
			convertPage(md, {
				fallbackTitle: 'From the file',
				known: null,
				resolve: (href) => (href === 'a.png' ? { kind: 'file', path: 'a.png' } : null),
				ref: () => 7
			});
		expect(page('No heading here').title).toBe('From the file');
		const p = page(
			'# Title\n\n<p>One<br>two <span style="x">and</span> ![gone](missing.png) [far](https://example.com)</p>\n\n![](a.png)'
		);
		expect(p.title).toBe('Title');
		expect(p.body).toBe('One  two and gone [far](https://example.com)\n\n![](mila-notion:7)');
		expect(p.missing).toBe(1);
	});
});

describe("Notion's dates", () => {
	it('are read in each of the ways Notion writes them', () => {
		expect(
			[
				'April 30, 2025',
				'Apr 30, 2025 3:00 PM',
				'30 April 2025',
				'04/30/2025',
				'30/04/2025',
				'2025/04/30',
				'2025-04-30',
				'April 30, 2025 → May 3, 2025'
			].map(readNotionDate)
		).toEqual(Array(8).fill('2025-04-30'));
		expect(['Smarch 3, 2025', 'February 30, 2025', 'Yesterday', ''].map(readNotionDate)).toEqual([
			null,
			null,
			null,
			null
		]);
	});
});

describe('A Notion dry run', () => {
	it('brings each database as tasks by default, a project each, its rest in the notes', () => {
		const p = planNotion(EXPORT, { where: { kind: 'notion' }, as: {} }, ctx);
		expect(p.pages.map((x) => [x.title, x.parent])).toEqual([
			['Team handbook', null],
			['Onboarding', HANDBOOK]
		]);
		expect(p.tasks!.newProjects).toEqual(['Reading list', 'Tasks']);
		const offsite = p.tasks!.tasks.find((t) => t.title === 'Plan the offsite')!;
		expect([offsite.priority, offsite.due, offsite.assignees, offsite.labels]).toEqual([
			'high',
			'2025-04-30',
			['u-ada'],
			[{ make: 'label:events' }, { make: 'label:planning' }]
		]);
		expect(offsite.notes).toMatch(
			/^\| Property \| Value \|\n\| --- \| --- \|\n\| Points \| 3 \|\n\nRead \[Onboarding\]\(mila-notion:0\) first\./
		);
		// A callout whose icon is a picture of Notion's: the icon left out.
		expect(offsite.notes).toMatch(
			/\n\n> \[!NOTE\]\n> \*\*Budget\*\*\n>\n> Keep it under the team's allowance\.$/
		);
		expect(p.tasks!.tasks.find((t) => t.title === 'Book the venue')!.priority).toBe('urgent');
		expect(p.tasks!.newStatuses.map((s) => [s.name, s.category])).toEqual([
			['Not started', 'todo']
		]);
		expect(p.tasks!.strangers).toEqual([{ name: 'Zed Quinn', lines: [5] }]);
		expect([p.links, p.missing, p.callouts, p.toggles, p.images, p.uploads.length]).toEqual([
			4, 1, 2, 1, 2, 3
		]);
		expect(p.uploads.find((u) => u.name === 'map.png')!.owner).toEqual({
			kind: 'task',
			key: IDS.offsite
		});
		expect(summaryLines(p.tasks!, p.targets, NOTION)).toContain(
			"Comments and page history are not in Notion's export, so they stay in Notion"
		);
	});

	it('brings a database as pages when chosen, its rows in order, properties at the top', () => {
		const p = planNotion(EXPORT, { where: { kind: 'top' }, as: { [READING]: 'pages' } }, ctx);
		expect(p.pages.map((x) => [x.title, x.parent])).toEqual([
			['Team handbook', null],
			['Onboarding', HANDBOOK],
			['Reading list', HANDBOOK],
			['The Quiet Garden', `db:${READING}`],
			['Rivers of Salt', `db:${READING}`]
		]);
		expect(p.pages[3].body).toBe(
			'| Property | Value |\n| --- | --- |\n| Author | Lena Marsh |\n| Read | Yes |\n\nNote: a slow book, worth it.'
		);
		expect(p.pages[4].body).toContain('| Author | Tomas Reyes |');
		expect(p.tasks!.newProjects).toEqual(['Tasks']);
	});

	it('leaves out a file over 25 MB, and a page or two over the limit says so', async () => {
		const files = await openNotion(notionExport());
		const big = files.map((f): NotionEntry =>
			f.path.endsWith('checklist.pdf') ? { ...f, size: 26 << 20 } : f
		);
		const p = planNotion(await readNotion(big), { where: { kind: 'notion' }, as: {} }, ctx);
		expect(p.tooLarge).toEqual(['checklist.pdf']);
		expect(p.uploads.map((u) => u.name)).toEqual(['desk.png', 'map.png']);
		const huge = files.map((f): NotionEntry =>
			f.path.endsWith('Onboarding ' + IDS.onboarding + '.md') ? { ...f, size: 21 << 20 } : f
		);
		expect((await readNotion(huge)).problems).toEqual([
			{ at: `Onboarding ${IDS.onboarding}.md`, reason: 'It is larger than an import takes.' }
		]);
	});

	it('finds a database by its folder when the folder has no ID', async () => {
		// Newer exports name the folders without the IDs their files have.
		const files = Object.fromEntries(
			Object.entries(notionFiles()).map(([path, v]) => {
				const parts = path.split('/');
				return [
					[...parts.slice(0, -1).map((d) => d.replace(/ [0-9a-f]{32}$/, '')), parts.at(-1)].join(
						'/'
					),
					v
				];
			})
		);
		const exp = await readNotion(await openNotion(zip(files)));
		const tasks = [...exp.databases.values()].find((d) => d.name === 'Tasks')!;
		expect(tasks.rows.filter((r) => r.page).map((r) => r.cells[0])).toEqual([
			'Plan the offsite',
			'Untitled'
		]);
		expect([...exp.pages.values()].find((p) => p.title === 'Onboarding')!.parent).toBe(HANDBOOK);
	});

	it('takes an export with no Part zip', async () => {
		const direct = await readNotion(await openNotion(zip(notionFiles())));
		expect(direct.pages.size).toBe(5);
	});
});

describe('importing a Notion export', () => {
	it('makes the tasks, then the pages under a Notion page, with links and files pointed home', async () => {
		const ws = `nt-${Math.random()}`;
		const replica = engine.sample(ws, []);
		const p = planNotion(EXPORT, { where: { kind: 'notion' }, as: { [READING]: 'pages' } }, ctx);
		const before = session.user;
		session.user = { id: 'u-me' } as typeof session.user;
		const made = await runNotion(
			ws,
			EXPORT,
			p,
			{ kind: 'notion' },
			{
				page: (x) => `/wiki?page=${x.id}`,
				project: (id) => `/projects/${id}`,
				task: (id) => `/?task=${id}`
			}
		).finally(() => (session.user = before));
		expect(made.tasks).toHaveLength(4);
		expect(made.pages).toHaveLength(6);
		expect(made.failed).toEqual(['checklist.pdf']);
		const pages = replica.pages(null);
		const byTitle = (t: string) => pages.find((x) => x.title === t)!;
		const root = byTitle('Notion');
		expect(made.top).toEqual([root.id]);
		const handbook = byTitle('Team handbook');
		expect(handbook.parentId).toBe(root.id);
		expect(byTitle('Onboarding').parentId).toBe(handbook.id);
		expect(byTitle('Rivers of Salt').parentId).toBe(byTitle('Reading list').id);
		expect(handbook.body).toContain(`[Onboarding](/wiki?page=${byTitle('Onboarding').id})`);
		expect(handbook.body).toContain(`[Reading list](/wiki?page=${byTitle('Reading list').id})`);
		const onboarding = byTitle('Onboarding').body;
		expect(onboarding).toContain(`![Desk](/api/v1/workspaces/${ws}/attachments/att-desk.png/file)`);
		// The file that could not be uploaded is its text.
		expect(onboarding).toContain('The checklist has the rest.');
		const tasks = replica.tasks(replica.projects().find((x) => x.name === 'Tasks')!.id);
		const offsite = tasks.find((t) => t.title === 'Plan the offsite')!;
		expect(offsite.notes).toContain(`[Onboarding](/wiki?page=${byTitle('Onboarding').id})`);
		expect(offsite.notes).toContain(
			`![Map](/api/v1/workspaces/${ws}/attachments/att-map.png/file)`
		);
		engine.dropSample(ws);
	});
});

describe('A Notion export a stranger made', () => {
	it('is read in time in line with its length, however many [ a line holds', () => {
		const start = performance.now();
		convertPage('# T\n\n' + '['.repeat(200_000), {
			fallbackTitle: 'x',
			known: null,
			resolve: () => null,
			ref: () => 0
		});
		expect(performance.now() - start).toBeLessThan(1000);
	});

	it('keeps a marker of its own as words, which the plan reads as nothing', async () => {
		const files = { ...notionFiles() };
		const page = Object.keys(files).find((p) => p.endsWith(`Onboarding ${IDS.onboarding}.md`))!;
		files[page] = `# Onboarding\n\nSee [this](mila-notion:99999) and [that](mila-notion:0).\n`;
		const exp = await readNotion(await openNotion(zip(files)));
		const body = [...exp.pages.values()].find((p) => p.title === 'Onboarding')!.body;
		expect(body).not.toMatch(/mila-notion/);
		expect(() => planNotion(exp, { where: { kind: 'notion' }, as: {} }, ctx)).not.toThrow();
	});

	it('is held to the limit a page may inflate to, whatever size its zip claims', async () => {
		const files = { ...notionFiles() };
		const page = Object.keys(files).find((p) => p.endsWith(`Onboarding ${IDS.onboarding}.md`))!;
		files[page] = '# Onboarding\n\n' + 'a'.repeat(21 << 20);
		// Deflated, or stored as it is, its directory saying it is 10 bytes.
		for (const stored of [false, true]) {
			const exp = await readNotion(await openNotion(claiming(zip(files, { stored }), page, 10)));
			expect(exp.problems, stored ? 'stored' : 'deflated').toEqual([
				{ at: `Onboarding ${IDS.onboarding}.md`, reason: 'It is larger than an import takes.' }
			]);
		}
	});

	it('is held to sixteen times its size inflated, at least 64 MB', async () => {
		// Four pages each just under the limit a page may hold, in a zip of
		// some 80 KB.
		const page = '# Page\n\n' + 'a'.repeat((20 << 20) - 64);
		const files: Record<string, string> = {};
		for (let k = 0; k < 4; k++) files[`Page ${k} ${'0'.repeat(31)}${k}.md`] = page;
		const b = zip(files);
		expect(b.length).toBeLessThan(1 << 20);
		await expect(readNotion(await openNotion(b))).rejects.toThrow(
			new NotionError('That export is larger than an import takes.')
		);
		// Three fit.
		delete files[`Page 3 ${'0'.repeat(31)}3.md`];
		expect((await readNotion(await openNotion(zip(files)))).pages.size).toBe(3);
	});

	it('is read in time in line with its length, however many tags a line leaves open', () => {
		const o = { fallbackTitle: 'x', known: null, resolve: () => null, ref: () => 0 };
		const pages: [string, string][] = [
			['tags never closed', '# T\n\n' + '<b'.repeat(100_000)],
			['toggles never closed', '# T\n\n' + '<details'.repeat(50_000)],
			['toggle titles never closed', '# T\n\n' + '<summary>'.repeat(50_000)],
			['a heading of #s', '# T' + '#'.repeat(200_000) + 'x'],
			['a heading of spaces', '# T' + ' '.repeat(100_000) + 'x']
		];
		for (const [what, md] of pages) {
			const start = performance.now();
			convertPage(md, o);
			expect(performance.now() - start, what).toBeLessThan(1000);
		}
		const start = performance.now();
		readNotionDate(' '.repeat(100_000) + 'x→ y');
		expect(performance.now() - start, 'a date of spaces').toBeLessThan(1000);
		// What the patterns are for still holds.
		const p = convertPage(
			'# Title ##\n\n<details><summary>Open <b>me</b></summary>Inside</details>',
			o
		);
		expect([p.title, p.body, p.toggles]).toEqual(['Title', '**Open me**\nInside', 1]);
	});

	it('without folders for subpages, is placed in time in line with its pages', async () => {
		// A chain of pages, each the subpage of the one before, and the last
		// linking back to the first, which stays at the top.
		const n = 20_000;
		const id = (k: number) => k.toString(16).padStart(32, '0');
		const name = (k: number) => `P${k} ${id(k)}`;
		const bytes = new TextEncoder();
		const entries: NotionEntry[] = [];
		for (let k = 0; k < n; k++) {
			const b = bytes.encode(`# P${k}\n\n[next](${encodeURIComponent(name((k + 1) % n))}.md)\n`);
			entries.push({ path: `${name(k)}.md`, size: b.length, read: async () => b });
		}
		const start = performance.now();
		const exp = await readNotion(entries);
		expect(performance.now() - start).toBeLessThan(3000);
		expect(exp.pages.get(name(0))!.parent).toBeNull();
		for (const k of [1, 2, n / 2, n - 1]) expect(exp.pages.get(name(k))!.parent).toBe(name(k - 1));
	}, 120_000);
});

/** The zip with the size the directory claims for one file changed. */
function claiming(b: Uint8Array, path: string, size: number): Uint8Array {
	const view = new DataView(b.buffer, b.byteOffset, b.byteLength);
	const name = new TextEncoder().encode(path);
	for (let i = 0; i + 46 <= b.length; i++)
		if (view.getUint32(i, true) === 0x02014b50 && view.getUint16(i + 28, true) === name.length) {
			const here = b.subarray(i + 46, i + 46 + name.length);
			if (here.every((c, k) => c === name[k])) view.setUint32(i + 24, size, true);
		}
	return b;
}
