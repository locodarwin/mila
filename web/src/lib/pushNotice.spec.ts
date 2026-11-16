import { describe, expect, it } from 'vitest';
import { appPath, isOpenMessage, noticeOf, openApp, readPush, type AppWindow } from './pushNotice';

describe('appPath', () => {
	it('keeps paths inside the app', () => {
		expect(appPath('/?task=t1&ws=w1')).toBe('/?task=t1&ws=w1');
		expect(appPath('/settings/notifications')).toBe('/settings/notifications');
		expect(appPath('today#top')).toBe('/today#top');
	});
	it('sends anything else to the start', () => {
		for (const bad of [
			'https://example.com/phish',
			'//example.com/x',
			'javascript:alert(1)',
			'',
			42,
			null
		])
			expect(appPath(bad)).toBe('/');
	});
});

describe('readPush', () => {
	it('reads what the server sends', () => {
		expect(
			readPush({
				title: 'Reminder: Water the plants',
				body: 'Due today',
				url: '/?task=t',
				tag: 't'
			})
		).toEqual({
			title: 'Reminder: Water the plants',
			body: 'Due today',
			url: '/?task=t',
			tag: 't'
		});
	});
	it('puts up with anything malformed', () => {
		expect(readPush(null)).toEqual({ title: 'Mila', body: '', url: '/', tag: '' });
		expect(readPush('text')).toEqual({ title: 'Mila', body: '', url: '/', tag: '' });
		expect(readPush({ title: 3, url: 'https://evil.example/' })).toEqual({
			title: 'Mila',
			body: '',
			url: '/',
			tag: ''
		});
	});
});

describe('noticeOf', () => {
	it('tags a notification by its task so a newer one replaces it and still alerts', () => {
		const { title, options } = noticeOf(
			readPush({ title: 'Due', body: 'b', url: '/?task=t', tag: 't' })
		);
		expect(title).toBe('Due');
		expect(options).toEqual({
			body: 'b',
			icon: '/icon-192.png',
			badge: '/badge-96.png',
			tag: 't',
			renotify: true,
			data: { url: '/?task=t' }
		});
	});
	it('leaves renotify out with no tag, where browsers refuse it', () => {
		const { options } = noticeOf(readPush({ title: 'x' }));
		expect(options.tag).toBeUndefined();
		expect(options.renotify).toBeUndefined();
	});
});

describe('openApp', () => {
	const origin = 'https://mila.example.com';
	function win(url: string, extra: Partial<AppWindow> = {}) {
		const w = {
			url,
			focused: false,
			visibilityState: 'hidden',
			got: [] as unknown[],
			focusedNow: false,
			focus: async () => {
				w.focusedNow = true;
			},
			postMessage: (m: unknown) => void w.got.push(m),
			...extra
		};
		return w;
	}
	function windows(list: ReturnType<typeof win>[]) {
		const opened: string[] = [];
		return { opened, list: async () => list, open: async (u: string) => void opened.push(u) };
	}

	it('focuses the focused Mila window and sends it to the link', async () => {
		const other = win('https://elsewhere.example/', { focused: true });
		const hidden = win(origin + '/today');
		const focused = win(origin + '/upcoming', { focused: true });
		const ws = windows([other, hidden, focused]);
		await openApp(ws, origin, '/?task=t&ws=w');
		expect(focused.got).toEqual([{ type: 'open', url: '/?task=t&ws=w' }]);
		expect(focused.focusedNow).toBe(true);
		expect(hidden.got).toEqual([]);
		expect(other.got).toEqual([]);
		expect(ws.opened).toEqual([]);
	});
	it('prefers a visible window, then any', async () => {
		const hidden = win(origin + '/');
		const visible = win(origin + '/', { visibilityState: 'visible' });
		await openApp(windows([hidden, visible]), origin, '/x');
		expect(visible.got).toHaveLength(1);
		await openApp(windows([hidden]), origin, '/y');
		expect(hidden.got).toEqual([{ type: 'open', url: '/y' }]);
	});
	it('opens a window when Mila is not open', async () => {
		const ws = windows([win('https://elsewhere.example/')]);
		await openApp(ws, origin, '/?task=t');
		expect(ws.opened).toEqual(['/?task=t']);
	});
});

describe('isOpenMessage', () => {
	it('accepts only open messages with a URL', () => {
		expect(isOpenMessage({ type: 'open', url: '/' })).toBe(true);
		expect(isOpenMessage({ type: 'open' })).toBe(false);
		expect(isOpenMessage({ type: 'poke', url: '/' })).toBe(false);
		expect(isOpenMessage('open')).toBe(false);
	});
});
