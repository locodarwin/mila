import { describe, expect, it } from 'vitest';
import { fillToday, planPage, snapshotPage, summarizePage, type PageSource } from './templatePlan';
import { readTemplate } from './templates';
import { contents, tooBig } from './templateInfo';
import type { Page } from './sync/entities';

const A = '00000000-0000-4000-8000-00000000000a';
const B = '00000000-0000-4000-8000-00000000000b';
const C = '00000000-0000-4000-8000-00000000000c';
const OUTSIDE = '00000000-0000-4000-8000-00000000000d';
const page = (id: string, parentId: string | null, title: string, body: string): Page =>
	({ id, parentId, title, body, projectId: 'old', sortKey: 'a0' }) as Page;
const image = '![plan.png](/api/v1/workspaces/w/attachments/f/file)';
const pages = [
	page(A, null, 'Onboarding', `Start. ${image} Then [Access](/projects/old?tab=wiki&page=${B}).`),
	page(
		B,
		A,
		'Access',
		`Back to [Onboarding](/projects/old?tab=wiki&page=${A}), or [Other](/wiki?page=${OUTSIDE}).`
	),
	page(C, B, 'Keys, {{today}}', 'Handed over on {{ Today }}.')
];
const src: PageSource = {
	page: (id) => pages.find((p) => p.id === id),
	pages: () => pages,
	pageAttachments: (id) => (id === A ? [{}] : []),
	pageComments: (id) => (id === B ? [{}, {}] : [])
};
let n = 0;
const target = {
	pageId: 'new-root',
	projectId: 'p2',
	today: 'Nov 3, 2026',
	uuid: () => `made-${++n}`,
	href: (p: { id: string; projectId: string | null }) =>
		`/projects/${p.projectId}?tab=wiki&page=${p.id}`
};

describe('page templates', () => {
	it('takes the page with its subpages, leaving images behind', () => {
		const data = snapshotPage(src, A, { subpages: true, title: 'Onboarding' })!.data;
		expect(data.pages?.map((p) => p.id)).toEqual([A, B, C]);
		expect(data.pages?.[0].parent).toBeNull();
		expect(data.pages?.[0].body).toBe(`Start. Then [Access](/projects/old?tab=wiki&page=${B}).`);
		expect(contents({ kind: 'page', data })).toBe('with 2 subpages');
		expect(tooBig(data)).toBeNull();
	});

	it('takes only the page when subpages are not kept', () => {
		const data = snapshotPage(src, A, { subpages: false, title: 'Onboarding' })!.data;
		expect(data.pages?.map((p) => p.id)).toEqual([A]);
		expect(contents({ kind: 'page', data })).toBe('one page');
	});

	it('leaves images behind whatever they are called', () => {
		const url = '/api/v1/workspaces/w/attachments/f/file';
		for (const shot of [
			`![Shot [2].png](${url})`,
			`![Shot \\] 2.png](${url})`,
			`![a](${url} "A title")`,
			`![](${url})`
		]) {
			const source: PageSource = { ...src, page: () => page(A, null, 'P', `Before ${shot}after.`) };
			const data = snapshotPage(source, A, { subpages: false, title: 'P' })!.data;
			expect(data.pages?.[0].body, shot).toBe('Before after.');
		}
		// Anything else that looks a little like one stays.
		const kept = `![photo](https://example.com/a.png) and [file](${url})`;
		const source: PageSource = { ...src, page: () => page(A, null, 'P', kept) };
		expect(snapshotPage(source, A, { subpages: false, title: 'P' })!.data.pages?.[0].body).toBe(
			kept
		);
	});

	it('says what stays behind', () => {
		expect(summarizePage(src, A, true)).toEqual({
			comes: 'the title and text and 2 subpages',
			stays: '2 comments, 1 file and its history',
			under: 2
		});
		expect(summarizePage(src, A, false).stays).toBe('2 subpages, 1 file and its history');
	});

	it('refuses more pages than a template holds', () => {
		const many = Array.from({ length: 101 }, (_, i) => ({
			id: `${i}`,
			parent: null,
			title: '',
			body: ''
		}));
		expect(tooBig({ v: 1, tasks: [], pages: many })).toContain('holds 100');
	});

	it('makes copies whose links lead to each other, in the wiki they land in', () => {
		n = 0;
		const data = snapshotPage(src, A, { subpages: true, title: 'Onboarding' })!.data;
		const plan = planPage(data, target);
		expect(plan.body).toBe('Start. Then [Access](/projects/p2?tab=wiki&page=made-1).');
		expect(plan.subpages).toEqual(['made-1', 'made-2']);
		const access = plan.rows[0];
		expect(access.values.parent_id).toBe('new-root');
		expect(access.values.project_id).toBe('p2');
		// A link to the root leads to the page filled; one out of the template stays.
		expect(access.values.body).toBe(
			`Back to [Onboarding](/projects/p2?tab=wiki&page=new-root), or [Other](/wiki?page=${OUTSIDE}).`
		);
		expect(plan.rows[1].values.parent_id).toBe('made-1');
	});

	it('fills in today, in titles and text', () => {
		n = 0;
		const plan = planPage(
			snapshotPage(src, A, { subpages: true, title: 'Notes, {{today}}' })!.data,
			target
		);
		expect(plan.title).toBe('Notes, Nov 3, 2026');
		expect(plan.rows[1].values.title).toBe('Keys, Nov 3, 2026');
		expect(plan.rows[1].values.body).toBe('Handed over on Nov 3, 2026.');
		expect(fillToday('{{tomorrow}} and {today}', 'x')).toBe('{{tomorrow}} and {today}');
	});

	it('reads what it stored', () => {
		const data = snapshotPage(src, A, { subpages: true, title: 'Onboarding' })!.data;
		expect(readTemplate(JSON.parse(JSON.stringify(data)))?.pages).toEqual(data.pages);
		expect(readTemplate({ v: 1, tasks: [], pages: [null, { id: 'x' }] })?.pages).toEqual([
			{ id: 'x', parent: null, title: '', body: '' }
		]);
	});
});
