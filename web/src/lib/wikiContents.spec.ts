import { describe, expect, it } from 'vitest';
import { currentHeading, indents } from './wikiContents';

const heading = (level: number) => ({ level, text: 'x' });

describe('indents', () => {
	it('starts from the top level the page uses', () => {
		expect(indents([2, 3, 2, 4].map(heading))).toEqual([0, 1, 0, 2]);
	});
	it('goes in three steps at most', () => {
		expect(indents([1, 6].map(heading))).toEqual([0, 3]);
	});
	it('takes a page with no headings', () => {
		expect(indents([])).toEqual([]);
	});
});

describe('currentHeading', () => {
	const scrolled = { top: 300, height: 2000, client: 500 };

	it('is the last heading at or above the line', () => {
		expect(currentHeading([-400, 40, 90, 600], 100, scrolled)).toBe(2);
	});
	it('is the first until the reader reaches it', () => {
		expect(currentHeading([200, 600], 100, { ...scrolled, top: 0 })).toBe(0);
	});
	it('is the last at the end of the page', () => {
		expect(currentHeading([-400, 300, 450], 100, { ...scrolled, top: 1500 })).toBe(2);
	});
	it('marks nothing on a page that fits, or has no headings', () => {
		expect(currentHeading([10, 20], 100, { top: 0, height: 500, client: 500 })).toBe(-1);
		expect(currentHeading([], 100, scrolled)).toBe(-1);
	});
});
