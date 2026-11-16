import { expect, test } from '@playwright/test';
import { apiCall, OWNER } from './helpers';
import { SYNC_SCHEMA } from '../src/lib/sync/entities';
import { keyBetween } from '../src/lib/sync/sortkey';

test.use({ storageState: OWNER });

// A long list draws a screenful at a time (growing.svelte.ts). On a screen
// tall enough that the end of the list stays in view, a row added past
// what is drawn still has to appear: nothing scrolls, so nothing but the
// list's own growth can tell it to draw more.
test('a row added to a full screen of tasks is drawn', async ({ page, isMobile }) => {
	test.skip(isMobile, 'a tall desktop window is the case');
	await page.setViewportSize({ width: 1280, height: 1600 });
	await page.goto('/projects');
	const name = `Sixty ${Date.now()}`;
	await page.getByLabel('Name a new project').fill(name);
	await page.getByLabel('Name a new project').press('Enter');
	await expect(page.getByLabel('Project name')).toHaveValue(name);
	const project = new URL(page.url()).pathname.split('/').pop()!;

	const stored = await page.evaluate(() => localStorage.getItem('mila.workspace'));
	const ws = stored ?? (await apiCall(page, 'GET', '/workspaces')).body[0].id;
	const me = (await apiCall(page, 'GET', '/me')).body;
	let key: string | null = null;
	const ops = Array.from({ length: 60 }, (_, i) => {
		key = keyBetween(key, null);
		return {
			opId: crypto.randomUUID(),
			entity: 'task',
			id: crypto.randomUUID(),
			hlc: `${String(Date.now()).padStart(13, '0')}-${i.toString(16).padStart(4, '0')}-e2e`,
			fields: { title: `Row ${i + 1}`, sort_key: key, project_id: project, created_by: me.id }
		};
	});
	// Pushed once the new project has reached the server; a refused op is
	// not recorded, so the same ops can be sent again until then. The
	// project can arrive part way through one push, so an op applied by an
	// earlier try answers "duplicate".
	await expect
		.poll(async () => {
			const pushed = await apiCall(page, 'POST', `/workspaces/${ws}/sync/push`, {
				schema: SYNC_SCHEMA,
				ops
			});
			return pushed.body.results.every(
				(r: { status: string }) => r.status === 'applied' || r.status === 'duplicate'
			);
		})
		.toBe(true);

	const rows = page.locator('#main [data-task-id]');
	await expect(rows).toHaveCount(60);
	await page.getByLabel('New task').fill('Row 61');
	await page.getByLabel('New task').press('Enter');
	await expect(rows).toHaveCount(61);
	await expect(
		page.getByRole('list', { name: 'Tasks' }).getByRole('listitem').filter({ hasText: 'Row 61' })
	).toBeVisible();
});
