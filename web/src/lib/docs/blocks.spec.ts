import { describe, expect, it } from 'vitest';
import { BLOCKS, findBlocks } from './blocks';

describe('the / menu', () => {
	const ids = (q: string) => findBlocks(q).map((b) => b.id);

	it('offers every block before anything is typed, but the kinds of diagram', () => {
		expect(ids('')).toEqual(BLOCKS.filter((b) => !b.typed).map((b) => b.id));
		expect(ids('')).toContain('diagram');
		expect(ids('')).not.toContain('diagram-flowchart');
	});

	it('finds a kind of diagram by typing for it', () => {
		expect(ids('flow')).toEqual(['diagram', 'diagram-flowchart']);
		expect(ids('sequence')).toEqual(['diagram-sequence']);
		expect(ids('mermaid')[0]).toBe('diagram');
		expect(ids('diagram')).toContain('diagram-pie');
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
