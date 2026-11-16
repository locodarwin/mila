import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { announce, announcer } from './announce.svelte';
import { TOAST_MS, toasts } from './toasts.svelte';

describe('toasts', () => {
	beforeEach(() => vi.useFakeTimers());
	afterEach(() => {
		toasts.resume();
		for (const t of toasts.items) toasts.dismiss(t.id);
		vi.useRealTimers();
	});

	it('go away on their own, an undo and an error later than a plain one', () => {
		toasts.show('Saved');
		toasts.show('Deleted', { onundo: () => {} });
		toasts.error('Failed');
		vi.advanceTimersByTime(TOAST_MS.info);
		expect(toasts.items.map((t) => t.message)).toEqual(['Deleted', 'Failed']);
		vi.advanceTimersByTime(TOAST_MS.undo - TOAST_MS.info);
		expect(toasts.items.map((t) => t.message)).toEqual(['Failed']);
		vi.advanceTimersByTime(TOAST_MS.error);
		expect(toasts.items).toEqual([]);
	});

	it('wait while paused and then take the time they had left', () => {
		toasts.show('Deleted', { onundo: () => {} });
		vi.advanceTimersByTime(TOAST_MS.undo - 2000);
		toasts.pause();
		vi.advanceTimersByTime(60_000);
		expect(toasts.items).toHaveLength(1);
		// A toast shown while paused waits too.
		toasts.show('Saved');
		vi.advanceTimersByTime(60_000);
		expect(toasts.items).toHaveLength(2);
		toasts.resume();
		vi.advanceTimersByTime(1999);
		expect(toasts.items.map((t) => t.message)).toEqual(['Deleted', 'Saved']);
		vi.advanceTimersByTime(1);
		expect(toasts.items.map((t) => t.message)).toEqual(['Saved']);
	});
	it('keep a sticky one until it is dismissed, and give an action as long as an undo', () => {
		const ran = vi.fn();
		toasts.show('Update ready', { sticky: true, action: { label: 'Reload', run: ran } });
		toasts.show('Moved', { action: { label: 'Open', run: ran } });
		vi.advanceTimersByTime(TOAST_MS.undo);
		expect(toasts.items.map((t) => t.message)).toEqual(['Update ready']);
		vi.advanceTimersByTime(60 * 60_000);
		expect(toasts.items).toHaveLength(1);
		toasts.items[0].action?.run();
		expect(ran).toHaveBeenCalledOnce();
	});
});

describe('announce', () => {
	it('empties the region first, so the same words are read again', () => {
		vi.useFakeTimers();
		announce('Moved to Done');
		vi.advanceTimersByTime(100);
		expect(announcer.message).toBe('Moved to Done');
		announce('Moved to Done');
		expect(announcer.message).toBe('');
		vi.advanceTimersByTime(100);
		expect(announcer.message).toBe('Moved to Done');
		vi.useRealTimers();
	});
});
