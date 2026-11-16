import { describe, expect, it } from 'vitest';
import { upTo } from './growing.svelte';

describe('upTo', () => {
	const groups = [
		{ id: 'a', tasks: [1, 2, 3] },
		{ id: 'b', tasks: [4, 5] },
		{ id: 'c', tasks: [6] }
	];
	const shape = (limit: number) => upTo(groups, limit).map(({ group, shown }) => [group.id, shown]);

	it('draws whole groups while they fit', () => {
		expect(shape(10)).toEqual([
			['a', [1, 2, 3]],
			['b', [4, 5]],
			['c', [6]]
		]);
	});

	it('cuts the group the limit falls in and leaves out the rest', () => {
		expect(shape(4)).toEqual([
			['a', [1, 2, 3]],
			['b', [4]]
		]);
		expect(shape(3)).toEqual([['a', [1, 2, 3]]]);
		expect(shape(0)).toEqual([]);
	});

	it('keeps each group whole, for its count', () => {
		expect(upTo(groups, 4)[1].group.tasks).toEqual([4, 5]);
	});
});
