import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import { preferences } from '../preferences.svelte';
import { blank, type ThemeDoc } from './document';
import { canonical, themes } from './themes.svelte';

const answer = (status: number, body?: unknown) =>
	Promise.resolve({
		data: status < 400 ? body : undefined,
		error: status < 400 ? undefined : (body ?? { message: 'no' }),
		response: new Response(null, { status })
	}) as never;
const offline = () => Promise.reject(new TypeError('Failed to fetch'));

function theme(name = 'Night shift'): ThemeDoc {
	return { ...blank('calm'), name };
}

describe('the themes store', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		preferences.disconnect();
		preferences.adopt({ ...preferences.current, customTheme: null });
		themes.disconnect();
		themes.list = [];
	});

	afterEach(() => {
		themes.disconnect();
		vi.restoreAllMocks();
		vi.useRealTimers();
	});

	it('waits for the picker before loading, with no theme in use or cached', async () => {
		const get = vi.spyOn(api, 'GET').mockImplementation(() => answer(200, { themes: [] }));
		themes.connect('u1');
		await vi.advanceTimersByTimeAsync(100);
		expect(get).not.toHaveBeenCalled();
		await themes.ensure();
		expect(get).toHaveBeenCalledTimes(1);
		expect(themes.loaded).toBe(true);
	});

	it('loads at once when a custom theme is in use', async () => {
		const t = theme();
		const get = vi.spyOn(api, 'GET').mockImplementation(() => answer(200, { themes: [t] }));
		preferences.adopt({ ...preferences.current, customTheme: t.id });
		themes.connect('u1');
		await vi.advanceTimersByTimeAsync(0);
		expect(get).toHaveBeenCalledWith('/me/themes');
		expect(themes.get(t.id)?.name).toBe('Night shift');
	});

	it('tries once offline, then waits for the reconnect', async () => {
		vi.spyOn(api, 'GET').mockImplementation(offline);
		const put = vi.spyOn(api, 'PUT').mockImplementation(offline);
		themes.connect('u1');
		const t = theme();
		themes.put(t);
		await vi.advanceTimersByTimeAsync(10_000);
		expect(put).toHaveBeenCalledTimes(1);
		expect(themes.get(t.id)).toBeDefined();
		expect(themes.unsaved(t.id)).toBe(true);

		put.mockImplementation((_path, init) => answer(200, (init as { body: unknown }).body));
		vi.spyOn(api, 'GET').mockImplementation(() => answer(200, { themes: [t] }));
		themes.reconnected();
		await vi.advanceTimersByTimeAsync(0);
		expect(put).toHaveBeenCalledTimes(2);
		expect(themes.unsaved(t.id)).toBe(false);
	});

	it('keeps a refused theme the server never had, marked with why', async () => {
		const put = vi
			.spyOn(api, 'PUT')
			.mockImplementation(() =>
				answer(400, { error: 'too_many_themes', message: 'You have 20 themes.' })
			);
		const get = vi.spyOn(api, 'GET').mockImplementation(() => answer(404));
		themes.connect('u1');
		const t = theme();
		themes.put(t);
		await vi.advanceTimersByTimeAsync(1000);
		expect(put).toHaveBeenCalledTimes(1);
		expect(get).toHaveBeenCalledWith('/me/themes/{themeId}', {
			params: { path: { themeId: t.id } }
		});
		expect(themes.get(t.id)?.name).toBe('Night shift');
		expect(themes.notSaved[t.id]).toEqual({
			code: 'too_many_themes',
			message: 'You have 20 themes.'
		});
		expect(themes.unsaved(t.id)).toBe(false);
		// Not tried again by itself, and kept through a reload of the list.
		get.mockImplementation(() => answer(200, { themes: [] }));
		await themes.refresh();
		await vi.advanceTimersByTimeAsync(10_000);
		expect(put).toHaveBeenCalledTimes(1);
		expect(themes.get(t.id)).toBeDefined();
		// A change tries again, and a save clears the mark.
		put.mockImplementation((_path, init) => answer(200, (init as { body: unknown }).body));
		themes.put({ ...t, name: 'Fewer themes now' });
		expect(themes.notSaved[t.id]).toBeUndefined();
		await vi.advanceTimersByTimeAsync(1000);
		expect(put).toHaveBeenCalledTimes(2);
		expect(themes.unsaved(t.id)).toBe(false);
	});

	it("takes the server's copy of a refused theme it has", async () => {
		const t = theme();
		vi.spyOn(api, 'PUT').mockImplementation(() => answer(400, { message: 'bad' }));
		vi.spyOn(api, 'GET').mockImplementation(() => answer(200, { ...t, name: 'On the server' }));
		themes.connect('u1');
		themes.put({ ...t, name: 'Refused edit' });
		await vi.advanceTimersByTimeAsync(1000);
		expect(themes.get(t.id)?.name).toBe('On the server');
		expect(themes.notSaved[t.id]).toBeUndefined();
	});

	it('fetches a theme put in use on another device', async () => {
		const t = theme();
		const get = vi.spyOn(api, 'GET').mockImplementation(() => answer(200, t));
		themes.connect('u1');
		themes.follow(null);
		await vi.advanceTimersByTimeAsync(0);
		expect(get).not.toHaveBeenCalled();
		themes.follow(t.id);
		await vi.advanceTimersByTimeAsync(0);
		expect(themes.get(t.id)?.id).toBe(t.id);
		// Had already: nothing to fetch.
		themes.follow(t.id);
		await vi.advanceTimersByTimeAsync(0);
		expect(get).toHaveBeenCalledTimes(1);
	});

	it("keeps no one else's list after they sign out", async () => {
		vi.spyOn(api, 'GET').mockImplementation(() => answer(200, { themes: [] }));
		themes.connect('ann');
		themes.list = [theme("Ann's")];
		themes.disconnect();
		// The look stays for the sign-in page...
		expect(themes.list.length).toBe(1);
		// ...but Ben never gets Ann's themes, even with nothing unsaved.
		themes.connect('ben');
		expect(themes.list).toEqual([]);
	});

	it('compares themes whatever order their keys come in', async () => {
		const t = theme();
		const sorted = JSON.parse(canonical(t)) as ThemeDoc;
		expect(Object.keys(sorted)).not.toEqual(Object.keys(t));
		const put = vi.spyOn(api, 'PUT').mockImplementation(() => answer(200, sorted));
		themes.connect('u1');
		themes.put(t);
		const before = themes.list;
		await vi.advanceTimersByTimeAsync(1000);
		expect(put).toHaveBeenCalledTimes(1);
		// The server's copy, keys sorted, is the same theme: the list is not swapped.
		expect(themes.list).toBe(before);
	});

	it("never sends someone else's unsaved theme to this user", async () => {
		vi.spyOn(api, 'GET').mockImplementation(offline);
		const put = vi.spyOn(api, 'PUT').mockImplementation(offline);
		themes.connect('ann');
		const t = theme();
		themes.put(t);
		await vi.advanceTimersByTimeAsync(1000);
		expect(put).toHaveBeenCalledTimes(1);
		// Ann's device, Ben signs in.
		vi.spyOn(api, 'GET').mockImplementation(() => answer(200, { themes: [] }));
		themes.connect('ben');
		await vi.advanceTimersByTimeAsync(10_000);
		expect(themes.list).toEqual([]);
		expect(put).toHaveBeenCalledTimes(1);
	});

	it('takes one theme from the themes message, unless it was changed here', async () => {
		vi.spyOn(api, 'PUT').mockImplementation(offline);
		themes.connect('u1');
		const t = theme();
		const changed = { ...t, name: 'Changed elsewhere' };
		const get = vi.spyOn(api, 'GET').mockImplementation(() => answer(200, changed));
		await themes.onMessage(t.id);
		expect(themes.get(t.id)?.name).toBe('Changed elsewhere');

		get.mockImplementation(() => answer(404));
		await themes.onMessage(t.id);
		expect(themes.get(t.id)).toBeUndefined();

		// Changed here and not yet saved: that wins.
		themes.put(t);
		get.mockImplementation(() => answer(200, changed));
		await themes.onMessage(t.id);
		expect(themes.get(t.id)?.name).toBe('Night shift');
	});

	it('deletes with an undo that puts the same theme back', async () => {
		const del = vi.spyOn(api, 'DELETE').mockImplementation(() => answer(204));
		const put = vi
			.spyOn(api, 'PUT')
			.mockImplementation((_path, init) => answer(200, (init as { body: unknown }).body));
		themes.connect('u1');
		const t = theme();
		themes.list = [t];
		const undo = themes.remove(t.id);
		expect(themes.get(t.id)).toBeUndefined();
		await vi.advanceTimersByTimeAsync(0);
		expect(del).toHaveBeenCalledTimes(1);
		undo();
		expect(themes.get(t.id)?.id).toBe(t.id);
		await vi.advanceTimersByTimeAsync(1000);
		expect(put).toHaveBeenCalledTimes(1);
	});

	it('keeps a deletion made offline from coming back with the list', async () => {
		const del = vi.spyOn(api, 'DELETE').mockImplementation(offline);
		themes.connect('u1');
		const t = theme();
		themes.list = [t];
		themes.remove(t.id);
		await vi.advanceTimersByTimeAsync(0);
		expect(del).toHaveBeenCalledTimes(1);
		// The server answers the list but cannot take the deletion yet.
		vi.spyOn(api, 'GET').mockImplementation(() => answer(200, { themes: [t] }));
		del.mockImplementation(() => answer(503));
		await themes.refresh();
		expect(del).toHaveBeenCalledTimes(2);
		expect(themes.get(t.id)).toBeUndefined();
		del.mockImplementation(() => answer(204));
		vi.spyOn(api, 'GET').mockImplementation(() => answer(200, { themes: [] }));
		themes.reconnected();
		await vi.advanceTimersByTimeAsync(0);
		expect(del).toHaveBeenCalledTimes(3);
		expect(themes.get(t.id)).toBeUndefined();
	});
});
