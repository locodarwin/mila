import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clear, done, keep, merge, unfinished, type Entry } from './journal';
import type { Row } from './replica.svelte';

const entry = (key: string): Entry => ({
	key,
	ws: 'w1',
	ops: [{ opId: key, entity: 'task', id: 't1', hlc: 'h2', fields: { title: 'B' } }],
	rows: [{ entity: 'task', id: 't1', values: { title: 'B' }, clocks: { title: 'h2' } }],
	schema: 2
});

describe('the journal', () => {
	const stored = new Map<string, string>();
	beforeEach(() => {
		stored.clear();
		vi.stubGlobal('localStorage', {
			getItem: (k: string) => stored.get(k) ?? null,
			setItem: (k: string, v: string) => void stored.set(k, v),
			removeItem: (k: string) => void stored.delete(k)
		});
	});
	afterEach(() => vi.unstubAllGlobals());

	it('keeps edits until IndexedDB has them', () => {
		expect(keep(entry('a'))).toBe(true);
		expect(keep(entry('b'))).toBe(true);
		done('a');
		expect(unfinished().map((e) => e.key)).toEqual(['b']);
		done('b');
		expect(unfinished()).toEqual([]);
		expect(stored.has('mila.journal')).toBe(false);
	});

	it('clears what it was asked to, or everything', () => {
		keep(entry('a'));
		keep(entry('b'));
		clear(['a']);
		expect(unfinished().map((e) => e.key)).toEqual(['b']);
		clear();
		expect(unfinished()).toEqual([]);
	});

	it('says when it could not keep an edit, and reads nothing from junk', () => {
		vi.stubGlobal('localStorage', {
			getItem: () => 'not json',
			setItem: () => {
				throw new Error('quota');
			},
			removeItem: () => {}
		});
		expect(keep(entry('a'))).toBe(false);
		expect(unfinished()).toEqual([]);
	});

	it('merges a kept row so a later write to a field stays', () => {
		const later: Row = {
			entity: 'task',
			id: 't1',
			values: { title: 'C', notes: 'n' },
			clocks: { title: 'h3', notes: 'h1' }
		};
		const kept: Row = {
			entity: 'task',
			id: 't1',
			values: { title: 'B', notes: 'm' },
			clocks: { title: 'h2', notes: 'h2' }
		};
		expect(merge(later, kept).values).toEqual({ title: 'C', notes: 'm' });
		expect(merge(undefined, kept)).toBe(kept);
	});
});
