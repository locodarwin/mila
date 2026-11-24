import { describe, expect, it } from 'vitest';
import { BLOCKS } from './blocks';
import { folding, IN_PLACE, INSERT_GROUPS, narrowest, widest, type Widths } from './toolbar';

const W: Widths = {
	base: 260,
	blocks: 110,
	pictures: 110,
	lists: 140,
	marks: 110,
	label: 50,
	more: 36
};

describe('the toolbar folding to fit', () => {
	const fold = (room: number) => [...folding(room, W)];

	it('folds nothing when it all fits', () => {
		expect(widest(W)).toBe(730);
		expect(fold(730)).toEqual([]);
		expect(fold(2000)).toEqual([]);
	});

	it('folds the blocks first, then the pictures, the lists, the marks and Insert’s word', () => {
		expect(fold(729)).toEqual(['blocks']);
		expect(fold(620)).toEqual(['blocks']);
		expect(fold(619)).toEqual(['blocks', 'pictures']);
		expect(fold(510)).toEqual(['blocks', 'pictures']);
		// The lists going brings the ⋯ button that holds them.
		expect(fold(509)).toEqual(['blocks', 'pictures', 'lists']);
		expect(fold(406)).toEqual(['blocks', 'pictures', 'lists']);
		expect(fold(405)).toEqual(['blocks', 'pictures', 'lists', 'marks']);
		expect(fold(296)).toEqual(['blocks', 'pictures', 'lists', 'marks']);
		expect(fold(295)).toEqual(['blocks', 'pictures', 'lists', 'marks', 'label']);
	});

	it('never folds what always stays', () => {
		expect(narrowest(W)).toBe(246);
		expect(fold(0)).toEqual(['blocks', 'pictures', 'lists', 'marks', 'label']);
	});
});

describe('the Insert menu', () => {
	it('holds every block the / menu offers, each once', () => {
		const ids = INSERT_GROUPS.flatMap((g) => g.ids);
		expect(new Set(ids).size).toBe(ids.length);
		expect([...ids].sort()).toEqual(
			BLOCKS.filter((b) => !b.typed)
				.map((b) => b.id)
				.sort()
		);
	});

	it('changes the line in place only for blocks that are a kind of line', () => {
		for (const id of IN_PLACE) expect(BLOCKS.some((b) => b.id === id)).toBe(true);
		expect(IN_PLACE.has('table')).toBe(false);
		expect(IN_PLACE.has('diagram')).toBe(false);
	});
});
