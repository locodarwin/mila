import { afterEach, describe, expect, it, vi } from 'vitest';
import { uuid } from './uuid';

describe('uuid', () => {
	afterEach(() => vi.unstubAllGlobals());

	it('makes version 4 UUIDs without crypto.randomUUID', () => {
		vi.stubGlobal('crypto', {
			getRandomValues: globalThis.crypto.getRandomValues.bind(globalThis.crypto)
		});
		const ids = new Set(Array.from({ length: 100 }, uuid));
		expect(ids.size).toBe(100);
		for (const id of ids) {
			expect(id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
		}
	});
});
