import { expect, test } from '@playwright/test';
import { apiCall, newMember, newPerson, OWNER, SIGNED_OUT } from './helpers';

test.use({ storageState: OWNER });
test.skip(({ isMobile }) => isMobile, 'offline files work the same on a phone');

const file = (name: string, text: string) => ({
	name,
	mimeType: 'text/plain',
	buffer: Buffer.from(text)
});

test('a file opened once still opens offline; another says it needs a connection', async ({
	browser
}) => {
	const {
		page: own,
		context,
		workspace
	} = await newPerson(() => browser.newContext(SIGNED_OUT), 'Faye');
	const ws = workspace.id;
	const task = crypto.randomUUID();
	const me = (await apiCall(own, 'GET', '/me')).body;
	const made = await apiCall(own, 'POST', `/workspaces/${ws}/tasks`, {
		id: task,
		title: 'Filed',
		sort_key: 'a0',
		created_by: me.id
	});
	expect(made.status).toBeLessThan(300);
	await own.goto(`/?task=${made.body.id}&ws=${ws}`);
	await own.evaluate(() => navigator.serviceWorker.ready);
	await own.reload(); // let the service worker take control
	await own.waitForFunction(() => navigator.serviceWorker.controller !== null);

	const add = own.locator('input[type="file"]').first();
	await add.setInputFiles(file('opened.txt', 'kept on this device'));
	await expect(own.getByRole('link', { name: 'opened.txt' })).toBeVisible();
	await add.setInputFiles(file('never.txt', 'not on this device'));
	const never = own.getByRole('link', { name: 'never.txt' });
	await expect(never).toBeVisible();

	const opened = own.getByRole('link', { name: 'opened.txt' });
	const address = (await opened.getAttribute('href'))!;
	const read = (url: string) =>
		own.evaluate(async (url) => {
			const res = await fetch(url, { cache: 'no-store' });
			return { status: res.status, text: await res.text() };
		}, url);
	expect(await read(address)).toEqual({ status: 200, text: 'kept on this device' });
	await expect
		.poll(() =>
			own.evaluate(
				async (url) => Boolean(await caches.match(url)),
				new URL(address, own.url()).href
			)
		)
		.toBe(true);

	await context.setOffline(true);
	await expect(never).toHaveCount(0);
	await expect(own.getByText('Needs a connection')).toHaveCount(1);
	await expect(opened).toBeVisible();
	expect(await read(address)).toEqual({ status: 200, text: 'kept on this device' });

	// Back online, everything can be opened again.
	await context.setOffline(false);
	await expect(never).toBeVisible();
	await expect(own.getByText('Needs a connection')).toHaveCount(0);

	// A file that has gone stops being held.
	const id = address.split('/').at(-2)!;
	const gone = await apiCall(own, 'DELETE', `/workspaces/${ws}/attachments/${id}`);
	expect(gone.status).toBeLessThan(300);
	await expect.poll(async () => (await read(address)).status).toBe(404);
	await expect
		.poll(() =>
			own.evaluate(
				async (url) => Boolean(await caches.match(url)),
				new URL(address, own.url()).href
			)
		)
		.toBe(false);
	await context.close();
});

test('removed from a workspace while closed, a device lets go of it on opening', async ({
	page,
	browser
}) => {
	const {
		page: own,
		context,
		workspace
	} = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Lane');
	const ws = workspace.id;
	await own.evaluate(() => navigator.serviceWorker.ready);
	await own.reload();
	await own.waitForFunction(() => navigator.serviceWorker.controller !== null);
	const url = new URL(`/api/v1/workspaces/${ws}/attachments/held/file`, own.url()).href;
	await own.evaluate(async (url) => {
		const cache = await caches.open('mila-files');
		await cache.put(url, new Response('held'));
	}, url);

	// What this device holds of a workspace, by its sync position.
	const holds = (at: typeof own) =>
		at.evaluate(
			(ws) =>
				new Promise<boolean>((done, fail) => {
					const open = indexedDB.open('mila');
					open.onerror = () => fail(open.error);
					open.onsuccess = () => {
						const read = open.result.transaction('meta').objectStore('meta').count(ws);
						read.onsuccess = () => {
							open.result.close();
							done(read.result > 0);
						};
					};
				}),
			ws
		);
	await expect.poll(() => holds(own)).toBe(true);
	const me = (await apiCall(own, 'GET', '/me')).body;
	const origin = new URL(own.url()).origin;

	// Every page closed, so nothing can hear of it.
	await own.close();
	const removed = await apiCall(page, 'DELETE', `/workspaces/${ws}/members/${me.id}`);
	expect(removed.status).toBeLessThan(300);

	const back = await context.newPage();
	await back.goto(origin + '/');
	await expect(back.getByRole('heading', { level: 1 }).first()).toBeVisible();
	await expect.poll(() => holds(back)).toBe(false);
	await expect
		.poll(() => back.evaluate(async (url) => Boolean(await caches.match(url)), url))
		.toBe(false);
	await context.close();
});
