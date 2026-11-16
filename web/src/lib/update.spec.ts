import { afterEach, describe, expect, it, vi } from 'vitest';
import { toasts } from './toasts.svelte';
import { CHECK_MS, UPDATE_MESSAGES, Updates, watchUpdates, type UpdateEnv } from './update.svelte';

afterEach(() => {
	for (const t of toasts.items) toasts.dismiss(t.id);
});

describe('the update offer', () => {
	it('stays until acted on, and only the one about stopped sync cannot be closed', () => {
		vi.useFakeTimers();
		const u = new Updates();
		u.reload = vi.fn();
		u.offer('deployed');
		u.offer('deployed');
		expect(toasts.items).toHaveLength(1);
		expect(toasts.items[0]).toMatchObject({
			message: UPDATE_MESSAGES.deployed,
			sticky: true,
			closable: true
		});
		vi.advanceTimersByTime(10 * 60_000);
		expect(toasts.items).toHaveLength(1);

		u.offer('required');
		expect(toasts.items).toHaveLength(1);
		expect(toasts.items[0]).toMatchObject({ message: UPDATE_MESSAGES.required, closable: false });
		u.offer('deployed');
		expect(toasts.items[0].message).toBe(UPDATE_MESSAGES.required);

		toasts.items[0].action?.run();
		expect(u.reload).toHaveBeenCalledOnce();
		vi.useRealTimers();
	});
});

function fakeEnv(controller: unknown) {
	const update = vi.fn(async () => {});
	const sw = Object.assign(new EventTarget(), {
		controller,
		getRegistration: async () => ({ update })
	});
	const win = new EventTarget();
	let tick = () => {};
	let stopped = false;
	const every = vi.fn((fn: () => void) => {
		tick = fn;
		return () => (stopped = true);
	});
	const env: UpdateEnv = { serviceWorker: sw, window: win, every };
	return { env, sw, win, update, every, tick: () => tick(), stopped: () => stopped };
}

describe('watchUpdates', () => {
	it('offers a reload when a new service worker takes over, not when the first one does', () => {
		const u = new Updates();
		const f = fakeEnv(null);
		const stop = watchUpdates(f.env, u);
		f.sw.dispatchEvent(new Event('controllerchange'));
		expect(u.reason).toBeNull();
		f.sw.dispatchEvent(new Event('controllerchange'));
		expect(u.reason).toBe('deployed');
		stop();
		expect(f.stopped()).toBe(true);
	});

	it('offers one when a part of the app no longer loads', () => {
		const u = new Updates();
		const f = fakeEnv({});
		const stop = watchUpdates(f.env, u);
		f.win.dispatchEvent(new Event('vite:preloadError'));
		expect(u.reason).toBe('deployed');
		stop();
	});

	it('looks for a new build every half hour and on reconnecting', async () => {
		const u = new Updates();
		const f = fakeEnv({});
		const stop = watchUpdates(f.env, u);
		expect(f.every).toHaveBeenCalledWith(expect.any(Function), CHECK_MS);
		f.tick();
		f.win.dispatchEvent(new Event('online'));
		await vi.waitFor(() => expect(f.update).toHaveBeenCalledTimes(2));
		stop();
		f.win.dispatchEvent(new Event('online'));
		f.sw.dispatchEvent(new Event('controllerchange'));
		await Promise.resolve();
		expect(f.update).toHaveBeenCalledTimes(2);
		expect(u.reason).toBeNull();
	});
});
