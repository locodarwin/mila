import type { Component } from 'svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { iconSet } from './iconSet.svelte';

describe('iconSet.get', () => {
	afterEach(() => {
		iconSet.icons = null;
	});

	it('finds the curated icons and nothing inherited', () => {
		const Folder = (() => {}) as unknown as Component;
		iconSet.icons = { folder: Folder };
		expect(iconSet.get('folder')).toBe(Folder);
		for (const name of ['__proto__', 'constructor', 'hasOwnProperty', 'toString', 'nope']) {
			expect(iconSet.get(name), name).toBeUndefined();
		}
	});

	it('finds nothing before the set loads', () => {
		expect(iconSet.get('folder')).toBeUndefined();
	});
});
