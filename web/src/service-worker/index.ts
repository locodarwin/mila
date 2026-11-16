// Keeps the app itself available offline. Built assets never change under
// a name, so they come from the cache; pages try the network first and fall
// back to the cached app shell. The API is never cached: data lives in
// IndexedDB, managed by the sync engine. Files are the one exception: each
// one fetched is kept, so it opens offline. It also shows web pushes as
// notifications and brings Mila forward when one is clicked.
import { assets, immutable } from '$app/manifest';
import { self } from '$app/service-worker';
import { APP_CACHE as CACHE } from '#lib/appCache.js';
import { FILES_CACHE, isFilePath, overLimit } from '#lib/fileCache.js';
import { appPath, noticeOf, openApp, readPush } from '#lib/pushNotice.js';

const SHELL = '/';
// Help's pictures load when a topic is read, so nobody downloads them all for offline use.
const PRECACHE = [
	SHELL,
	...immutable.map((f) => f.path),
	...assets.map((f) => f.path).filter((path) => !path.startsWith('help-images/'))
];

self.addEventListener('install', (event) => {
	event.waitUntil(
		caches
			.open(CACHE)
			.then((cache) => cache.addAll(PRECACHE))
			.then(() => self.skipWaiting())
	);
});

self.addEventListener('activate', (event) => {
	event.waitUntil(
		caches
			.keys()
			// A new version of the app leaves the person's files where they are.
			.then((keys) =>
				Promise.all(
					keys.filter((k) => k !== CACHE && k !== FILES_CACHE).map((k) => caches.delete(k))
				)
			)
			.then(() => self.clients.claim())
	);
});

self.addEventListener('fetch', (event) => {
	const url = new URL(event.request.url);
	if (event.request.method !== 'GET' || url.origin !== self.location.origin) return;
	if (isFilePath(url.pathname)) {
		event.respondWith(fetchFile(event, url.pathname));
		return;
	}
	// The API reference at /api/docs is a page of the app: offline it gets
	// the shell, which says the reference needs a connection.
	const apiDocs = url.pathname === '/api/docs' && event.request.mode === 'navigate';
	if (
		(url.pathname.startsWith('/api/') && !apiDocs) ||
		url.pathname.startsWith('/ical/') ||
		url.pathname === '/healthz' ||
		url.pathname === '/readyz'
	)
		return;

	if (event.request.mode === 'navigate') {
		event.respondWith(
			fetch(event.request).catch(async () => (await caches.match(SHELL)) ?? Response.error())
		);
		return;
	}
	event.respondWith(caches.match(event.request).then((hit) => hit ?? fetch(event.request)));
});

/**
 * The network first, so someone who has lost a file is told so, and what this
 * device holds when there is no network.
 */
async function fetchFile(event: FetchEvent, key: string): Promise<Response> {
	let response: Response;
	try {
		response = await fetch(event.request);
	} catch {
		const held = await caches
			.open(FILES_CACHE)
			.then((cache) => cache.match(key, { ignoreVary: true }))
			.catch(() => undefined);
		return held ?? notHeld(event.request);
	}
	if (response.status === 200) event.waitUntil(keepFile(key, response.clone()));
	else if ([401, 403, 404, 410].includes(response.status)) {
		event.waitUntil(caches.open(FILES_CACHE).then((cache) => cache.delete(key)));
	}
	return response;
}

async function keepFile(key: string, response: Response) {
	try {
		const cache = await caches.open(FILES_CACHE);
		// Taken out first, so the order of the keys is the order last fetched.
		await cache.delete(key);
		await cache.put(key, response);
		const held = await Promise.all(
			(await cache.keys()).map(async (request) => {
				const file = await cache.match(request, { ignoreVary: true });
				return { request, size: Number(file?.headers.get('Content-Length')) || 0 };
			})
		);
		await Promise.all(overLimit(held).map((file) => cache.delete(file.request)));
	} catch {
		// No room, or storage blocked: the file still opens this time.
	}
}

/** A file this device never fetched: a page saying so if it opened in a tab. */
function notHeld(request: Request): Response {
	if (request.mode !== 'navigate') return Response.error();
	return new Response(
		'<!doctype html><meta charset="utf-8"><title>Mila</title>' +
			'<p>This file has not been opened on this device, so it needs a connection.</p>',
		{ status: 503, headers: { 'Content-Type': 'text/html; charset=utf-8' } }
	);
}

self.addEventListener('push', (event) => {
	let data: unknown = null;
	try {
		data = event.data?.json();
	} catch {
		// Not JSON: show the plain notice rather than nothing.
	}
	const { title, options } = noticeOf(readPush(data));
	event.waitUntil(self.registration.showNotification(title, options));
});

self.addEventListener('notificationclick', (event) => {
	event.notification.close();
	const data: unknown = event.notification.data;
	const path = appPath(
		typeof data === 'object' && data !== null ? (data as { url?: unknown }).url : '/'
	);
	event.waitUntil(
		openApp(
			{
				list: () => self.clients.matchAll({ type: 'window', includeUncontrolled: true }),
				open: (url) => self.clients.openWindow(url)
			},
			self.location.origin,
			path
		)
	);
});

// The browser replaced the subscription (its push service reset it):
// subscribe again with the same key and tell the server, if still signed in.
self.addEventListener('pushsubscriptionchange', (event) => {
	const e = event as PushSubscriptionChangeEvent;
	const key = e.oldSubscription?.options.applicationServerKey;
	if (!key) return;
	e.waitUntil(
		(async () => {
			const sub =
				e.newSubscription ??
				(await self.registration.pushManager.subscribe({
					userVisibleOnly: true,
					applicationServerKey: key
				}));
			await fetch('/api/v1/me/push/subscribe', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(sub.toJSON())
			});
		})().catch(() => {})
	);
});
