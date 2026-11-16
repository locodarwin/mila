import { describe, expect, it } from 'vitest';
import { isConfluence, planConfluence, readPage } from './confluenceimport';

// Made up, in the shape Confluence's Export to Markdown writes: the title as
// the first heading, runs of blank lines where pictures and macros were,
// tables with bold headings, and backslash escapes.
const garden = [
	'﻿# Garden plan',
	'',
	'',
	'',
	'What goes where this year.',
	'',
	'',
	'Attached photo:',
	'',
	'',
	'## Beds',
	'',
	'| **Bed** | **Crop** | **Notes** |',
	'| --- | --- | --- |',
	'| North | Beans | Sow after the frost \\- mid May |',
	'| South | Tomatoes | See [the suppliers](https://example.atlassian.net/wiki/spaces/GDN/pages/123/Seed+suppliers) |',
	''
].join('\r\n');

describe('a Confluence page', () => {
	it('takes its first heading as the title, and the rest as its body', () => {
		const p = readPage({ name: 'Garden plan.md', text: garden });
		expect(p.title).toBe('Garden plan');
		expect(p.body.startsWith('What goes where this year.')).toBe(true);
		expect(p.body).not.toContain('# Garden plan');
		expect(p.body).not.toMatch(/\n{3,}/);
		expect(p.body).not.toContain('\r');
		// What Confluence left behind stays, as written.
		expect(p.body).toContain('Attached photo:');
		expect(p.body).toContain('Sow after the frost \\- mid May');
		expect(p.tables).toBe(1);
		expect(p.confluenceLinks).toBe(1);
		expect(p.words).toBeGreaterThan(15);
		expect(p.cut).toBe(false);
	});

	it("takes the file's name as the title when there is no heading", () => {
		const p = readPage({ name: 'Notes from Tuesday.md', text: 'Just text.\n\n## A section\n' });
		expect(p.title).toBe('Notes from Tuesday');
		expect(p.body).toBe('Just text.\n\n## A section');
	});

	it('cuts a page longer than the server holds', () => {
		const p = readPage({ name: 'Long.md', text: '# Long\n\n' + 'word '.repeat(300_000) });
		expect(p.cut).toBe(true);
		expect(p.body.length).toBe(1_000_000);
	});

	it('knows a link into Confluence from one out of it', () => {
		expect(isConfluence('https://acme.atlassian.net/wiki/spaces/X/pages/1/Y')).toBe(true);
		expect(isConfluence('https://wiki.example.com/wiki/spaces/X/pages/1')).toBe(true);
		expect(isConfluence('https://example.com/docs?id=4')).toBe(false);
		expect(isConfluence('not a link')).toBe(false);
	});
});

describe('planning the import', () => {
	it('puts the pages in title order, and says which repeat a title', () => {
		const plan = planConfluence(
			[
				{ name: 'b.md', text: '# Seed suppliers\n\nWho.' },
				{ name: 'a.md', text: garden },
				{ name: 'c.md', text: '# planting calendar\n\nWhen.' },
				{ name: 'd.md', text: '# Seed Suppliers\n\nAgain.' }
			],
			['Planting calendar'],
			[{ file: 'photo.png', reason: 'not a Markdown file' }]
		);
		expect(plan.pages.map((p) => p.title)).toEqual([
			'Garden plan',
			'planting calendar',
			'Seed suppliers',
			'Seed Suppliers'
		]);
		// One already in the wiki, and one twice among the files.
		expect([...plan.repeats].sort()).toEqual(['c.md', 'd.md']);
		expect(plan.skipped).toHaveLength(1);
		expect(plan.over).toBe(false);
	});
});

describe('A Confluence page a stranger made', () => {
	it('is read in time in line with its length, and without control characters', () => {
		const start = performance.now();
		readPage({ name: 'Long.md', text: '['.repeat(200_000) + ']('.repeat(50_000) });
		expect(performance.now() - start).toBeLessThan(1000);
		const texts: [string, string][] = [
			['a heading of #s', '# T' + '#'.repeat(200_000) + 'x'],
			['a heading of spaces', '# T' + ' '.repeat(200_000) + 'x'],
			['links never closed', '[](a'.repeat(50_000)]
		];
		for (const [what, text] of texts) {
			const start = performance.now();
			readPage({ name: 'Long.md', text });
			expect(performance.now() - start, what).toBeLessThan(1000);
		}
		expect(readPage({ name: 'a.md', text: '# Title # #\n\nx' }).title).toBe('Title');
		const wiki = 'https://example.atlassian.net/wiki/spaces/A/pages/1/Foo_(bar)';
		expect(readPage({ name: 'a.md', text: `[Foo](${wiki})` }).confluenceLinks).toBe(1);
		const p = readPage({ name: 'Bell\u0007.md', text: 'Ring \u0007the\u001b[2K bell.' });
		expect(p.title).toBe('Bell');
		expect(p.body).toBe('Ring the[2K bell.');
	});
});
