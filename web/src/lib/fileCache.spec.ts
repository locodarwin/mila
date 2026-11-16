import { describe, expect, it } from 'vitest';
import { isFilePath, overLimit } from './fileCache';

describe('isFilePath', () => {
	it("knows a file's address and nothing else in the API", () => {
		expect(isFilePath('/api/v1/workspaces/w/attachments/a/file')).toBe(true);
		expect(isFilePath('/api/v1/workspaces/w/attachments/a')).toBe(false);
		expect(isFilePath('/api/v1/workspaces/w/sync/bootstrap')).toBe(false);
		expect(isFilePath('/api/v1/me/exports/e/file')).toBe(false);
	});
});

describe('overLimit', () => {
	const held = [
		{ id: 'oldest', size: 40 },
		{ id: 'middle', size: 40 },
		{ id: 'newest', size: 40 }
	];
	it('lets go of the oldest until the rest fit', () => {
		expect(overLimit(held, 90).map((f) => f.id)).toEqual(['oldest']);
		expect(overLimit(held, 50).map((f) => f.id)).toEqual(['oldest', 'middle']);
	});
	it('keeps everything that fits', () => {
		expect(overLimit(held, 120)).toEqual([]);
	});
});
