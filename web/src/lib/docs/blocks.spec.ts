import { describe, expect, it } from 'vitest';
import { BLOCKS, findBlocks } from './blocks';

describe('the / menu', () => {
	const ids = (q: string) => findBlocks(q).map((b) => b.id);

	it('offers every block before anything is typed', () => {
		expect(ids('')).toEqual(BLOCKS.map((b) => b.id));
	});

	it('narrows by the start of a name or another word for it', () => {
		expect(ids('check')).toEqual(['checklist']);
		expect(ids('tab')).toEqual(['table']);
		expect(ids('list')).toEqual(['bullets', 'numbers']);
		expect(ids('box')).toEqual(['note', 'tip', 'warning']);
		expect(ids('PIC')).toEqual(['image']);
		expect(ids('hr')).toEqual(['rule']);
	});

	it('finds nothing for what no block is called', () => {
		expect(ids('zebra')).toEqual([]);
	});
});
