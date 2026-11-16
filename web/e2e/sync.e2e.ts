import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, OWNER, SIGNED_OUT, newPerson } from './helpers';

test.use({ storageState: OWNER });

async function newProject(page: Page, name: string): Promise<string> {
	await page.goto('/projects');
	await page.getByLabel('Name a new project').fill(name);
	await page.getByLabel('Name a new project').press('Enter');
	await expect(page.getByLabel('Project name')).toHaveValue(name);
	return page.url();
}

async function addTask(page: Page, title: string) {
	await page.getByLabel('New task').fill(title);
	await page.getByLabel('New task').press('Enter');
	await expect(taskRow(page, title)).toBeVisible();
}

function taskRow(page: Page, title: string) {
	return page.getByRole('list', { name: 'Tasks' }).getByRole('listitem').filter({ hasText: title });
}

test('tasks persist across a reload', async ({ page }) => {
	await newProject(page, `Garden ${Date.now()}`);
	await addTask(page, 'Plant tomatoes');
	await addTask(page, 'Water the beans');
	await taskRow(page, 'Plant tomatoes')
		.getByRole('button', { name: 'Plant tomatoes', exact: true })
		.click();
	// Wait for the panel's own title, or a slow machine fills the field
	// after the typing and keeps none of it.
	await expect(page.getByLabel('Task title')).toHaveValue('Plant tomatoes');
	await page.getByLabel('Task title').fill('Plant cherry tomatoes');
	await page.getByLabel('Task title').press('Enter');
	await expect(taskRow(page, 'Plant cherry tomatoes')).toBeVisible();
	await expect(page.getByTitle('Synced', { exact: true })).toBeAttached();

	await page.reload();
	await expect(taskRow(page, 'Plant cherry tomatoes')).toBeVisible();
	await expect(taskRow(page, 'Water the beans')).toBeVisible();
});

test('completing holds, then moves the task to Completed, and undo-delete works', async ({
	page
}) => {
	await newProject(page, `Errands ${Date.now()}`);
	await addTask(page, 'Post the letter');
	// The box's label, which waits until the page has finished fading in.
	await taskRow(page, 'Post the letter').locator('label.checkbox').click();
	await expect(taskRow(page, 'Post the letter')).toBeVisible(); // held for a moment
	await expect(taskRow(page, 'Post the letter')).toBeHidden({ timeout: 3000 });
	await page.getByRole('button', { name: /Completed · 1/ }).click();
	await expect(page.getByRole('list', { name: 'Completed tasks' })).toContainText(
		'Post the letter'
	);

	await addTask(page, 'Buy stamps');
	await taskRow(page, 'Buy stamps').getByRole('button', { name: 'Delete Buy stamps' }).click();
	await expect(taskRow(page, 'Buy stamps')).toBeHidden();
	await page.getByRole('button', { name: 'Undo' }).click();
	await expect(taskRow(page, 'Buy stamps')).toBeVisible();
});

test('two devices stay in step, and an offline one catches up', async ({ page, browser }) => {
	const url = await newProject(page, `Shared ${Date.now()}`);
	await addTask(page, 'Seen everywhere');

	// A second device, open on the same project.
	const other = await browser.newContext({ storageState: OWNER });
	const laptop = await other.newPage();
	await laptop.goto(url);
	await expect(taskRow(laptop, 'Seen everywhere')).toBeVisible();

	// Live: an edit here appears there without a reload.
	await addTask(page, 'Added live');
	await expect(taskRow(laptop, 'Added live')).toBeVisible();

	// The laptop goes offline and keeps working.
	await other.setOffline(true);
	await laptop.evaluate(() => window.dispatchEvent(new Event('offline')));
	await addTask(laptop, 'Written offline');
	await laptop.getByRole('button', { name: 'Seen everywhere', exact: true }).click();
	await expect(laptop.getByLabel('Task title')).toHaveValue('Seen everywhere');
	await laptop.getByLabel('Task title').fill('Renamed offline');
	await laptop.getByLabel('Task title').press('Enter');

	// Meanwhile the phone changes something else.
	await taskRow(page, 'Added live')
		.getByRole('button', { name: 'Added live', exact: true })
		.click();
	await expect(page.getByLabel('Task title')).toHaveValue('Added live');
	await page.getByLabel('Task title').fill('Renamed online');
	await page.getByLabel('Task title').press('Enter');
	await expect(taskRow(page, 'Written offline')).toBeHidden();

	// Back online, both devices end up the same.
	await other.setOffline(false);
	await laptop.evaluate(() => window.dispatchEvent(new Event('online')));
	for (const p of [page, laptop]) {
		await expect(taskRow(p, 'Written offline')).toBeVisible({ timeout: 15_000 });
		await expect(taskRow(p, 'Renamed offline')).toBeVisible({ timeout: 15_000 });
		await expect(taskRow(p, 'Renamed online')).toBeVisible({ timeout: 15_000 });
	}
	const titles = (p: Page) =>
		p.getByRole('list', { name: 'Tasks' }).getByRole('listitem').allTextContents();
	expect(await titles(laptop)).toEqual(await titles(page));
	await other.close();
});

test('the app opens with no network, from this device', async ({ page, context }) => {
	const url = await newProject(page, `Offline ${Date.now()}`);
	await addTask(page, 'Still here offline');
	await page.evaluate(() => navigator.serviceWorker.ready);
	await page.reload(); // let the service worker take control
	await expect(taskRow(page, 'Still here offline')).toBeVisible();

	await context.setOffline(true);
	await page.goto(url);
	await expect(taskRow(page, 'Still here offline')).toBeVisible();
	await context.setOffline(false);
});

test('an edit survives a reload the instant after it is made', async ({ page }) => {
	await newProject(page, `Quick ${Date.now()}`);
	await page.getByLabel('New task').fill('Saved before shown');
	await page.getByLabel('New task').press('Enter');
	await expect(taskRow(page, 'Saved before shown')).toBeVisible();
	await page.reload();
	await expect(taskRow(page, 'Saved before shown')).toBeVisible();
});

test('an edit the page left before saving comes back from the journal', async ({ page }) => {
	await newProject(page, `Journal ${Date.now()}`);
	const title = `Kept through a reload ${Date.now()}`;
	// This page's writes to IndexedDB fail, as they do when it goes away
	// in the middle of one; the edit still shows.
	await page.evaluate(() => {
		const fail = () => {
			throw new DOMException('Gone', 'AbortError');
		};
		IDBObjectStore.prototype.put = fail;
		IDBObjectStore.prototype.add = fail;
	});
	await page.getByLabel('New task').fill(title);
	await page.getByLabel('New task').press('Enter');
	await expect(taskRow(page, title)).toBeVisible();
	await page.reload();
	await expect(taskRow(page, title)).toBeVisible();
	await expect
		.poll(async () => {
			const list = (await apiCall(page, 'GET', '/workspaces')).body as { id: string }[];
			for (const w of list) {
				const res = await apiCall(page, 'GET', `/workspaces/${w.id}/tasks?limit=500`);
				const items = res.status === 200 ? res.body.items : [];
				if (items.some((t: { title: string }) => t.title === title)) return true;
			}
			return false;
		})
		.toBe(true);
	expect(await page.evaluate(() => localStorage.getItem('mila.journal'))).toBeNull();
});

test('signing out deletes everything this device held', async ({ browser }) => {
	const { context, page: sam } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Sam');
	await sam.goto('/');
	await addTask(sam, 'Private to Sam');
	const rows = () =>
		sam.evaluate(
			() =>
				new Promise<number>((done) => {
					const req = indexedDB.open('mila');
					req.onsuccess = () => {
						const db = req.result;
						if (!db.objectStoreNames.contains('rows')) return done(0);
						const count = db.transaction('rows').objectStore('rows').count();
						count.onsuccess = () => {
							db.close();
							done(count.result);
						};
					};
				})
		);
	expect(await rows()).toBeGreaterThan(0);
	await sam.goto('/settings/security');
	await sam
		.getByRole('list', { name: 'Signed-in devices' })
		.getByRole('listitem')
		.filter({ hasText: 'This device' })
		.getByRole('button', { name: 'Sign out' })
		.click();
	await expect(sam).toHaveURL(/\/login/);
	expect(await rows()).toBe(0);
	expect(await sam.evaluate(() => localStorage.getItem('mila.session'))).toBeNull();
	await context.close();
});

test('tasks added in a burst keep the order they were added in', async ({ page }) => {
	await newProject(page, `Burst ${Date.now()}`);
	const input = page.getByLabel('New task');
	const titles = ['One', 'Two', 'Three', 'Four', 'Five', 'Six'];
	for (const t of titles) {
		await input.fill(t);
		await input.press('Enter');
	}
	const list = page.getByRole('list', { name: 'Tasks' }).getByRole('listitem');
	const shown = () => page.getByRole('list', { name: 'Tasks' }).locator('.title').allTextContents();
	await expect(list).toHaveCount(titles.length);
	expect(await shown()).toEqual(titles);
	await page.reload();
	await expect(list).toHaveCount(titles.length);
	expect(await shown()).toEqual(titles);
});

test('an app the server no longer syncs with keeps its edits and offers a reload', async ({
	page,
	browser
}) => {
	const url = await newProject(page, `Gate ${Date.now()}`);
	await addTask(page, 'Before the update');
	// A second device, which sees what reaches the server.
	const other = await browser.newContext({ storageState: OWNER });
	const laptop = await other.newPage();
	await laptop.goto(url);
	await expect(taskRow(laptop, 'Before the update')).toBeVisible();

	// A new server is deployed that wants a newer app.
	const push = '**/api/v1/workspaces/*/sync/push';
	await page.route(push, (route) =>
		route.fulfill({
			status: 426,
			json: { error: 'update_required', message: 'A new version of Mila is available.' }
		})
	);
	await addTask(page, 'Waiting for the update');
	const toast = page.locator('.toaster').filter({ hasText: 'Mila has been updated' });
	await expect(toast).toBeVisible();
	await expect(toast.getByRole('button', { name: 'Reload' })).toBeVisible();
	// Edits are waiting: it cannot be put off.
	await expect(toast.getByRole('button', { name: 'Dismiss' })).toHaveCount(0);
	// The sync indicator does not claim everything is synced.
	await expect(page.locator('.sync.paused')).toContainText(/Update needed · \d+ not sent/);
	await expect(page.locator('.sync.synced')).toHaveCount(0);
	await axe(page);

	// Nothing reached the server.
	await laptop.reload();
	await expect(taskRow(laptop, 'Before the update')).toBeVisible();
	await expect(taskRow(laptop, 'Waiting for the update')).toHaveCount(0);

	// The reload brings the new app, which sends the edit it kept.
	await page.unroute(push);
	await toast.getByRole('button', { name: 'Reload' }).click();
	await expect(taskRow(page, 'Waiting for the update')).toBeVisible();
	await expect(page.locator('.toaster')).not.toContainText('Mila has been updated');
	await expect(page.locator('.sync.paused')).toHaveCount(0);
	await expect(taskRow(laptop, 'Waiting for the update')).toBeVisible();
	await other.close();
});

test('edits the server puts off wait on the device and go when it said', async ({
	page,
	browser
}) => {
	const url = await newProject(page, `Budget ${Date.now()}`);
	await addTask(page, 'Before the limit');
	const other = await browser.newContext({ storageState: OWNER });
	const laptop = await other.newPage();
	await laptop.goto(url);
	await expect(taskRow(laptop, 'Before the limit')).toBeVisible();

	// The server has had too much from this person in the last hour.
	const push = '**/api/v1/workspaces/*/sync/push';
	let refused = 0;
	await page.route(push, (route) => {
		refused++;
		return route.fulfill({
			status: 429,
			headers: { 'Retry-After': '5' },
			json: { error: 'rate_limited', message: 'Too much was written in the last hour.' }
		});
	});
	await addTask(page, 'Over the limit');
	await addTask(page, 'Over it again');
	const toast = page.locator('.toaster').filter({ hasText: 'saved a great deal' });
	await expect(toast).toHaveCount(1);
	await expect.poll(() => refused).toBeGreaterThan(0);
	await laptop.reload();
	await expect(taskRow(laptop, 'Before the limit')).toBeVisible();
	await expect(taskRow(laptop, 'Over the limit')).toHaveCount(0);

	// Once the wait is over the kept edits go, with no new edit to send them.
	await page.unroute(push);
	await expect(async () => {
		await laptop.reload();
		await expect(taskRow(laptop, 'Over it again')).toBeVisible({ timeout: 1000 });
	}).toPass({ timeout: 15_000 });
	await expect(taskRow(laptop, 'Over the limit')).toBeVisible();
	await other.close();
});
