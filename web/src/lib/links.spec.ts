import { describe, expect, it } from 'vitest';
import { linksIn, linkTarget, taskHref } from './links';

const id = '0193a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b';

describe('links', () => {
	it('reads the app’s addresses as the server does', () => {
		// server/internal/syncer/links_test.go holds the same.
		expect(linkTarget(`/wiki?page=${id}`)).toEqual({ kind: 'page', id });
		expect(linkTarget(`/projects/${id}?tab=wiki&page=${id}`)).toEqual({ kind: 'page', id });
		expect(
			linkTarget(`https://mila.example.com/wiki?page=${id}`, 'https://mila.example.com')
		).toEqual({
			kind: 'page',
			id
		});
		expect(linkTarget(taskHref(id))).toEqual({ kind: 'task', id });
		expect(linkTarget(`/projects/${id}?task=${id}`)).toEqual({ kind: 'task', id });
		expect(linkTarget('/wiki?page=not-an-id')).toBeNull();
		expect(linkTarget(`/settings?page=${id}`)).toBeNull();
		expect(linkTarget('https://example.com')).toBeNull();
		expect(linkTarget('mailto:someone@example.com')).toBeNull();
	});

	it('takes no address of another site for one of the app’s, however it is written', () => {
		// A comment's reference shows the page's title: one that led elsewhere
		// would show a real title on the way to anywhere.
		const here = 'https://mila.example.com';
		for (const href of [
			`https://evil.example/wiki?page=${id}`,
			`//evil.example/wiki?page=${id}`,
			`/\\evil.example/?task=${id}`,
			`/\t/evil.example/?task=${id}`,
			`http://mila.example.com.evil.example/wiki?page=${id}`
		]) {
			expect(linkTarget(href, here), href).toBeNull();
		}
	});

	it('finds what a document links to, once each', () => {
		const other = '0193a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c';
		expect(
			linksIn(
				`See [a](/wiki?page=${id}), [b](/wiki?page=${id}) and [c](/?task=${other}), not [d](https://example.com) or ?task=${other} as text.`
			)
		).toEqual([id, other]);
	});
});
