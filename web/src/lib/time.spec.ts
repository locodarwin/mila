import { describe, expect, it } from 'vitest';
import { relative } from './time';

describe('relative', () => {
	const now = new Date('2026-10-28T12:00:00Z');
	it('says just now for the last minute', () => {
		expect(relative('2026-10-28T11:59:30Z', now)).toBe('just now');
	});
	it('counts back in the largest whole unit', () => {
		expect(relative('2026-10-28T11:55:00Z', now)).toMatch(/5 minutes ago/);
		expect(relative('2026-10-21T12:00:00Z', now)).toMatch(/last week|1 week ago/);
	});
	it('counts forward too', () => {
		expect(relative('2026-11-04T12:00:00Z', now)).toMatch(/next week|in 1 week/);
	});
});
