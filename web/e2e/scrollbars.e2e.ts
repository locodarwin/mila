// The app is as tall and as wide as the window and scrolls inside itself, so the page
// itself never overflows: if it did, Windows and other systems with scrollbars that
// take room would draw a second, useless scrollbar at the window's edge.
import { readFileSync } from 'node:fs';
import { expect, request, test, type Page } from '@playwright/test';
import { apiCall, INVITER, newPerson, OWNER, SIGNED_OUT } from './helpers';

test.use({ storageState: OWNER });

test('no screen makes the page itself scroll', async ({ page, isMobile }) => {
	test.skip(isMobile, 'a phone scrolls the page as it pleases');
	await page.goto('/');
	const ws: string = (await apiCall(page, 'POST', '/workspaces', { name: `Edges ${Date.now()}` }))
		.body.id;
	const make = async (entity: string, body: unknown) =>
		(await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, body)).body.id as string;
	const project = await make('projects', { name: 'Garden' });
	for (const n of ['To do', 'Doing', 'Waiting', 'Checking', 'Done'])
		await make('statuses', {
			project_id: project,
			name: n,
			category: n === 'Done' ? 'done' : 'todo'
		});
	const task = await make('tasks', { project_id: project, title: 'Order seeds' });
	for (let i = 0; i < 30; i++) await make('tasks', { project_id: project, title: `Task ${i}` });
	for (const [w, h] of [
		[1280, 800],
		[900, 700]
	]) {
		await page.setViewportSize({ width: w, height: h });
		for (const [path, view] of [
			[`/?ws=${ws}`, ''],
			[`/projects/${project}?ws=${ws}`, 'Board'],
			[`/projects/${project}?ws=${ws}&task=${task}`, 'Table'],
			[`/wiki?ws=${ws}`, ''],
			['/settings/preferences', '']
		]) {
			await page.goto(path);
			if (view)
				await page.getByRole('group', { name: 'View' }).getByRole('button', { name: view }).click();
			await expect(page.locator('main').first()).toBeVisible();
			await expect
				.poll(() =>
					page.evaluate(() => {
						const d = document.documentElement;
						return [d.scrollHeight - d.clientHeight, d.scrollWidth - d.clientWidth];
					})
				)
				.toEqual([0, 0]);
		}
	}
});

// Every frame, not just the last: a page arriving slides in from a little below.
test('moving between pages never makes the page itself scroll', async ({ page, isMobile }) => {
	test.skip(isMobile, 'a phone scrolls the page as it pleases');
	await page.addInitScript(() => {
		const seen: number[] = [];
		(window as unknown as { __overflow: number[] }).__overflow = seen;
		const tick = () => {
			const d = document.documentElement;
			const over = Math.max(d.scrollHeight - d.clientHeight, d.scrollWidth - d.clientWidth);
			if (over > 0) seen.push(over);
			requestAnimationFrame(tick);
		};
		requestAnimationFrame(tick);
	});
	await page.goto('/');
	const nav = page.getByRole('navigation', { name: 'Main' });
	await expect(nav.getByRole('link', { name: 'Upcoming', exact: true })).toBeVisible();
	for (const name of ['My Day', 'Upcoming', 'Time', 'Inbox']) {
		await nav.getByRole('link', { name, exact: true }).click();
		await expect(page).toHaveURL(name === 'Inbox' ? /\/(\?.*)?$/ : /\/(today|upcoming|time)/);
		// Longer than the page takes to arrive.
		await page.waitForTimeout(600);
	}
	expect(
		await page.evaluate(() => (window as unknown as { __overflow: number[] }).__overflow)
	).toEqual([]);
});

/** Scrolling boxes inside another that scrolls too: two scroll bars side by side. */
function nested(page: Page) {
	return page.evaluate(() => {
		const scrolls = (e: Element) =>
			/(auto|scroll)/.test(getComputedStyle(e).overflowY) && e.scrollHeight > e.clientHeight + 1;
		const name = (e: Element) => `${e.tagName.toLowerCase()}.${e.classList[0] ?? ''}`;
		const found: string[] = [];
		for (const e of document.querySelectorAll('main *')) {
			if (!scrolls(e)) continue;
			for (let a = e.parentElement; a; a = a.parentElement)
				if (scrolls(a)) {
					found.push(`${name(a)} > ${name(e)}`);
					break;
				}
		}
		return found;
	});
}

// A view one screen tall scrolls inside itself, and the page around it never
// does: not because of what sits above it (the phone's bar, a theme the
// workspace suggests), nor because of what is inside it (a table's hidden
// labels once stretched the page behind it as tall as the table).
test('a view that scrolls inside itself leaves the page still', async ({ browser }) => {
	const { page: kim, workspace } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Kim');
	const ws = workspace.id;
	const baseURL = test.info().project.use.baseURL!;
	const owner = await request.newContext({
		baseURL,
		storageState: INVITER,
		extraHTTPHeaders: { Origin: baseURL }
	});
	const make = async (entity: string, data: unknown) => {
		const res = await owner.post(`/api/v1/workspaces/${ws}/${entity}`, { data });
		expect(res.status(), await res.text()).toBe(201);
		return ((await res.json()) as { id: string }).id;
	};
	const project = await make('projects', { name: 'Tall' });
	for (const n of ['To do', 'Done'])
		await make('statuses', {
			project_id: project,
			name: n,
			category: n === 'Done' ? 'done' : 'todo'
		});
	// Due over the coming days, so Upcoming holds them too.
	const day = (i: number) =>
		new Date(Date.now() + (1 + (i % 5)) * 86_400_000).toISOString().slice(0, 10);
	for (let i = 0; i < 40; i++)
		await make('tasks', { project_id: project, title: `Task ${i}`, due_date: day(i) });
	const theme = JSON.parse(
		readFileSync(new URL('./visual/fixtures/sample.mila-theme.json', import.meta.url), 'utf8')
	);
	const made = await owner.post(`/api/v1/workspaces/${ws}/workspace-themes`, {
		data: { name: 'Suggested', body: theme, suggested_at: new Date().toISOString() }
	});
	expect(made.status()).toBe(201);
	await owner.dispose();

	for (const [path, group, view] of [
		[`/projects/${project}?ws=${ws}`, 'View', 'Table'],
		[`/projects/${project}?ws=${ws}`, 'View', 'Board'],
		[`/projects/${project}?ws=${ws}`, 'View', 'Timeline'],
		[`/upcoming?ws=${ws}`, 'Show as', 'Table']
	]) {
		await kim.goto(path);
		await expect(kim.getByRole('region', { name: 'Theme suggestion' })).toBeVisible();
		await kim.getByRole('group', { name: group }).getByRole('button', { name: view }).click();
		await expect(kim.getByText('Task 39').first()).toBeAttached();
		await expect.poll(() => nested(kim), { message: `${path} as ${view}` }).toEqual([]);
	}
});

// The workspace list grows down to the bottom of the window before it scrolls,
// and never runs past it.
test('the workspace list takes the room below it', async ({ page }) => {
	await page.goto('/');
	const menu = page.locator('#workspace-menu');
	for (const height of [1000, 360]) {
		await page.setViewportSize({ width: page.viewportSize()!.width, height });
		await page
			.getByRole('button', { name: /^Switch workspace/ })
			.first()
			.click();
		await expect(menu).toBeVisible();
		const { max, bottom } = await menu.evaluate((m) => ({
			max: parseFloat(getComputedStyle(m).maxHeight),
			bottom: m.getBoundingClientRect().bottom
		}));
		expect(bottom).toBeLessThanOrEqual(height);
		// More than the 420 pixels every other menu stops at, when there is room.
		if (height === 1000) expect(max).toBeGreaterThan(800);
		await page.keyboard.press('Escape');
		await expect(menu).toBeHidden();
	}
});
