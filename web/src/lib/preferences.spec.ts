import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './api/client';
import { DEFAULT_PREFERENCES, parsePreferences, preferences } from './preferences.svelte';

describe('parsePreferences', () => {
	it('gives the defaults for nothing', () => {
		expect(parsePreferences(undefined)).toEqual(DEFAULT_PREFERENCES);
		expect(parsePreferences('not an object')).toEqual(DEFAULT_PREFERENCES);
	});

	it('keeps valid values and drops the rest, as the server does', () => {
		const p = parsePreferences({
			theme: 'calm',
			scheme: 'sepia',
			accentHue: 400,
			clock: '12',
			showCompleted: 'yes',
			sidebarHidden: ['views', 'favorites'],
			startPage: 'today'
		});
		expect(p).toEqual({ ...DEFAULT_PREFERENCES, theme: 'calm', clock: '12', startPage: 'today' });
	});

	it('completes a sidebar order that leaves items out', () => {
		expect(parsePreferences({ sidebarOrder: ['projects', 'inbox'] }).sidebarOrder).toEqual([
			'projects',
			'inbox',
			'today',
			'upcoming',
			'wiki',
			'time',
			'reports',
			'views'
		]);
		expect(parsePreferences({ sidebarOrder: ['inbox', 'inbox'] }).sidebarOrder).toEqual(
			DEFAULT_PREFERENCES.sidebarOrder
		);
	});

	it('cleans the shortcuts entry by entry', () => {
		const p = parsePreferences({
			shortcuts: {
				'quick-add': ['n', 'Ctrl+n', 'n', 'Alt+n'],
				'Not An Id': ['q'],
				undo: 'Mod+z',
				'go.inbox': ['g n', 'a', 'b', 'c', 'd'],
				'task.complete': [],
				'task.my-day': ['Ctrl+t']
			}
		});
		expect(p.shortcuts).toEqual({
			'quick-add': ['n', 'Alt+n'],
			'go.inbox': ['g n', 'a', 'b', 'c'],
			'task.complete': []
		});
		expect(parsePreferences({ shortcuts: ['q'] }).shortcuts).toEqual({});
		const many = Object.fromEntries(Array.from({ length: 250 }, (_, i) => [`c${i}`, ['F2']]));
		expect(Object.keys(parsePreferences({ shortcuts: many }).shortcuts)).toHaveLength(200);
	});

	it('takes a custom theme only by its lowercase id, as the server and the spec do', () => {
		const id = '0b5c4f6e-8a1d-4c2e-9f3a-1d2e3f4a5b6c';
		expect(parsePreferences({ customTheme: id }).customTheme).toBe(id);
		expect(parsePreferences({ customTheme: id.toUpperCase() }).customTheme).toBeNull();
		expect(parsePreferences({ customTheme: 'night' }).customTheme).toBeNull();
	});

	it('keeps the suggestions answered as a list of up to 100 theme ids', () => {
		const id = (n: number) => `0b5c4f6e-8a1d-4c2e-9f3a-${String(n).padStart(12, '0')}`;
		expect(parsePreferences({}).themeAnswers).toEqual([]);
		expect(parsePreferences({ themeAnswers: [id(1), id(2)] }).themeAnswers).toEqual([id(1), id(2)]);
		expect(parsePreferences({ themeAnswers: [id(1), 'night'] }).themeAnswers).toEqual([]);
		const many = Array.from({ length: 101 }, (_, i) => id(i));
		expect(parsePreferences({ themeAnswers: many }).themeAnswers).toEqual([]);
		expect(parsePreferences({ themeAnswers: many.slice(1) }).themeAnswers).toHaveLength(100);
	});

	it('takes a null accent as the theme default', () => {
		expect(parsePreferences({ accentHue: 120 }).accentHue).toBe(120);
		expect(parsePreferences({ accentHue: null }).accentHue).toBeNull();
	});
});

describe('preferences', () => {
	it('keeps a change made here over the server copy until it is saved', () => {
		preferences.set({ theme: 'calm' });
		preferences.adopt({ ...DEFAULT_PREFERENCES, theme: 'playful', clock: '12' });
		expect(preferences.current.theme).toBe('calm');
		expect(preferences.current.clock).toBe('12');
		// Signing out forgets what was not saved; the server copy wins then.
		preferences.disconnect();
		preferences.adopt({ ...DEFAULT_PREFERENCES, theme: 'playful' });
		expect(preferences.current.theme).toBe('playful');
	});
});

describe('changing shortcuts', () => {
	it('keeps the good keys of a map with a bad one', () => {
		preferences.set({ shortcuts: { 'quick-add': ['n', 'Ctrl+n'], search: ['s'] } });
		expect(preferences.current.shortcuts).toEqual({ 'quick-add': ['n'], search: ['s'] });
		preferences.set({ shortcuts: {} });
		preferences.disconnect();
	});
});

describe('saving preferences', () => {
	const answer = (status: number, body: unknown) =>
		Promise.resolve({
			data: status < 400 ? body : undefined,
			error: status < 400 ? undefined : body,
			response: new Response(null, { status })
		});

	beforeEach(() => {
		vi.useFakeTimers();
		preferences.disconnect();
		preferences.adopt(DEFAULT_PREFERENCES);
	});

	afterEach(() => {
		preferences.disconnect();
		vi.restoreAllMocks();
		vi.useRealTimers();
	});

	it('tries once offline, then waits for the reconnect', async () => {
		const patch = vi.spyOn(api, 'PATCH').mockRejectedValue(new TypeError('Failed to fetch'));
		vi.spyOn(api, 'GET').mockRejectedValue(new TypeError('Failed to fetch'));
		preferences.connect('u1', null);
		preferences.set({ theme: 'calm' });
		await vi.advanceTimersByTimeAsync(10_000);
		expect(patch).toHaveBeenCalledTimes(1);
		expect(preferences.current.theme).toBe('calm');

		// Back online: the reconnect sends it.
		patch.mockImplementation(() => answer(200, { ...DEFAULT_PREFERENCES, theme: 'calm' }) as never);
		await preferences.refresh();
		expect(patch).toHaveBeenCalledTimes(2);
		expect(patch.mock.calls[1][1]).toEqual({ body: { theme: 'calm' } });
		await vi.advanceTimersByTimeAsync(10_000);
		expect(patch).toHaveBeenCalledTimes(2);
	});

	it('keeps a change the server could not take for now', async () => {
		const patch = vi
			.spyOn(api, 'PATCH')
			.mockImplementation(() => answer(502, { message: 'Bad gateway' }) as never);
		const get = vi.spyOn(api, 'GET');
		preferences.connect('u1', null);
		preferences.set({ scheme: 'dark', dateFormat: 'day-month' });
		await vi.advanceTimersByTimeAsync(10_000);
		expect(patch).toHaveBeenCalledTimes(1);
		expect(get).not.toHaveBeenCalled();
		// The server's copy, from a GET /me, does not undo it.
		preferences.adopt(DEFAULT_PREFERENCES);
		expect(preferences.current.scheme).toBe('dark');
		expect(preferences.current.dateFormat).toBe('day-month');
	});

	it("drops a change the server refuses for the server's copy", async () => {
		vi.spyOn(api, 'PATCH').mockImplementation(
			() => answer(400, { error: 'invalid', message: 'theme must be one of...' }) as never
		);
		vi.spyOn(api, 'GET').mockImplementation(
			() => answer(200, { preferences: { ...DEFAULT_PREFERENCES, theme: 'playful' } }) as never
		);
		vi.spyOn(console, 'warn').mockImplementation(() => {});
		preferences.connect('u1', null);
		preferences.set({ theme: 'calm' });
		await vi.advanceTimersByTimeAsync(1000);
		expect(preferences.current.theme).toBe('playful');
	});

	it("never sends one person's unsaved changes to someone else's account", async () => {
		const patch = vi.spyOn(api, 'PATCH').mockRejectedValue(new TypeError('Failed to fetch'));
		preferences.connect('ann', null);
		preferences.set({ theme: 'playful', clock: '12' });
		await vi.advanceTimersByTimeAsync(1000);
		expect(patch).toHaveBeenCalledTimes(1);

		// Ann's session ends elsewhere; Ben signs in on this browser.
		preferences.connect('ben', { ...DEFAULT_PREFERENCES, theme: 'calm' });
		await vi.advanceTimersByTimeAsync(1000);
		expect(patch).toHaveBeenCalledTimes(1);
		expect(preferences.current.theme).toBe('calm');
		expect(preferences.current.clock).toBe('auto');
	});
});
