import { describe, expect, it } from 'vitest';
import vectors from '../../../../shared/sortkey-vectors.json';
import { keyBetween } from './sortkey';

// The server makes keys too (a REST create with no sort_key goes last), from
// its port in server/internal/syncer/sortkey.go, held to the same cases.
describe('sort keys', () => {
	it('agree with the server on the key after another', () => {
		expect(keyBetween(null, null)).toBe(vectors.first);
		for (const [before, after] of vectors.after)
			expect(keyBetween(before, null), before).toBe(after);
	});

	it('agree with the server on what is not a key', () => {
		for (const key of vectors.invalid) expect(() => keyBetween(key, null), key).toThrow();
	});
});
