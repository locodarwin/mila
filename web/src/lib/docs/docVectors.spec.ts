import { describe, expect, it } from 'vitest';
import vectors from '../../../../shared/doc-vectors.json';
import { buildVectors, canonical, parseMarkdown } from './vectors';

// The server's reading and writing of live documents is held to the
// editor's by shared/doc-vectors.json; server/internal/ydoc checks the Go
// half. Upgrading Tiptap or Yjs can change these: then run
// `npx tsx scripts/doc-vectors.ts` and `go test ./internal/ydoc -update`.

// The server's Markdown that the editor reads back differently, and why.
const READ_BACK_DIFFERENTLY: Record<string, string> = {
	'break in heading': 'a heading is one line, so its break is a space',
	'spaces in bold': 'Markdown has no spaces just inside bold',
	'empty paragraph last': "the editor's empty last paragraph is left out, as its own Markdown does",
	'read from written by the editor: empty paragraph last':
		"the editor's empty last paragraph is left out, as its own Markdown does",
	'only an empty paragraph': 'an empty document is written as nothing',
	'table with pipes': 'a table cell is one line, so its break is a space',

	'table cell with two paragraphs': 'a table cell is one line, so its paragraphs run together',
	'table without a header row': "a Markdown table's first row is always its header",
	'image between bold text': 'Markdown has no spaces just inside bold',
	'read from image in bold': 'Markdown has no spaces just inside bold'
};

describe('the live-document vectors', () => {
	const fresh = buildVectors();

	it('still hold what the editor reads, writes and merges', () => {
		expect(fresh.schema).toEqual(vectors.schema);
		expect(fresh.parse).toEqual(vectors.parse);
		expect(fresh.yjs).toEqual(vectors.yjs);
		expect(fresh.render.map((c) => c.json)).toEqual(vectors.render.map((c) => c.json));
	});

	it("are read back by the editor from the server's Markdown", () => {
		const wrong: string[] = [];
		for (const c of vectors.render as { name: string; json: object; md?: string }[]) {
			expect(c.md, `${c.name} has the server's Markdown`).toBeTypeOf('string');
			const same = canonical(parseMarkdown(c.md!)) === canonical(c.json);
			const known = c.name in READ_BACK_DIFFERENTLY;
			if (same === known)
				wrong.push(
					`${c.name}: ${JSON.stringify(c.md)} ${same ? 'now reads back' : 'reads back differently'}`
				);
		}
		expect(wrong).toEqual([]);
	});
});
