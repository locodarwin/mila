import { describe, expect, it } from 'vitest';
import vectors from '../../../../shared/sync-vectors.json';
import { Clock, HLC_PATTERN } from './hlc';
import { keyBetween } from './sortkey';

describe('HLC', () => {
	it('agrees with the server on what is valid', () => {
		for (const h of vectors.valid) expect(HLC_PATTERN.test(h), h).toBe(true);
		for (const h of vectors.invalid) expect(HLC_PATTERN.test(h), h).toBe(false);
		expect([...vectors.ordered].sort()).toEqual(vectors.ordered);
	});

	it('only moves forward', () => {
		const c = new Clock('me');
		const a = c.tick(1000);
		const b = c.tick(1000);
		const d = c.tick(999); // the wall clock went backward
		expect(a < b && b < d).toBe(true);
	});

	it('outranks everything it has seen', () => {
		const c = new Clock('me');
		c.tick(1000);
		c.observe('0000000005000-0007-other');
		const next = c.tick(1000);
		expect(next > '0000000005000-0007-other').toBe(true);
		expect(next).toBe('0000000005000-0008-me');
	});
});

describe('keyBetween', () => {
	it('fits a key between any two', () => {
		const a = keyBetween(null, null);
		const b = keyBetween(a, null);
		const mid = keyBetween(a, b);
		const first = keyBetween(null, a);
		expect(first < a && a < mid && mid < b).toBe(true);
	});

	it('keeps order through many inserts in the same gap', () => {
		const keys = [keyBetween(null, null)];
		let lo = keys[0];
		const hi = keyBetween(lo, null);
		for (let i = 0; i < 200; i++) {
			lo = keyBetween(lo, hi);
			keys.push(lo);
		}
		expect([...keys].sort()).toEqual(keys);
		expect(Math.max(...keys.map((k) => k.length))).toBeLessThan(64);
	});

	it('appends forever without growing much', () => {
		let k = keyBetween(null, null);
		for (let i = 0; i < 1000; i++) {
			const next = keyBetween(k, null);
			expect(next > k).toBe(true);
			k = next;
		}
		expect(k.length).toBeLessThan(64);
	});

	it('prepends forever too', () => {
		let k = keyBetween(null, null);
		for (let i = 0; i < 1000; i++) {
			const next = keyBetween(null, k);
			expect(next < k).toBe(true);
			k = next;
		}
		expect(k.length).toBeLessThan(64);
	});

	it('keeps random inserts in order', () => {
		const keys: string[] = [];
		let seed = 7;
		const rand = () => (seed = (seed * 1103515245 + 12345) % 2 ** 31) / 2 ** 31;
		for (let i = 0; i < 500; i++) {
			const at = Math.floor(rand() * (keys.length + 1));
			keys.splice(at, 0, keyBetween(keys[at - 1] ?? null, keys[at] ?? null));
		}
		expect([...keys].sort()).toEqual(keys);
		expect(new Set(keys).size).toBe(keys.length);
	});
});
