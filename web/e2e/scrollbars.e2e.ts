// The app is as tall and as wide as the window and scrolls inside itself, so the page
// itself never overflows: if it did, Windows and other systems with scrollbars that
// take room would draw a second, useless scrollbar at the window's edge.
import { expect, test } from '@playwright/test';
import { apiCall, OWNER } from './helpers';

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
