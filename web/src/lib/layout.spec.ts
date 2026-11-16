import { describe, expect, it } from 'vitest';
import { clampWidth, DETAIL, SIDEBAR } from './layout.svelte';

describe('clampWidth', () => {
	it('keeps widths between their limits, in whole pixels', () => {
		expect(clampWidth(100, SIDEBAR)).toBe(SIDEBAR.min);
		expect(clampWidth(2000, DETAIL)).toBe(DETAIL.max);
		expect(clampWidth(300.4, SIDEBAR)).toBe(300);
	});
});
