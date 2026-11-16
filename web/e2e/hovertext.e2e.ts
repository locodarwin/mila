import { expect, test, type Page } from '@playwright/test';
import { apiCall, newPerson, OWNER, SIGNED_OUT, WORKSPACE_SETTINGS } from './helpers';

test.use({ storageState: OWNER });
test.skip(({ isMobile }) => isMobile, 'a phone has nothing to hover with');

/** What on the page can be clicked and says nothing when the pointer reaches it. */
function silent(page: Page) {
	return page.evaluate(() => {
		const clickable =
			'a[href], button, summary, select, [role="button"], [role="tab"], [role="menuitem"], ' +
			'[role="menuitemradio"], [role="option"], [role="treeitem"], ' +
			'input[type="checkbox"], input[type="radio"], input[type="file"], input[type="date"], ' +
			'input[type="time"], input[type="range"], input[type="color"], [data-task-id]';
		return [...document.querySelectorAll<HTMLElement>(clickable)]
			.filter((el) => el.checkVisibility() && !el.closest('[inert]'))
			.filter((el) => {
				// A box drawn over its input is hovered at the label that holds it.
				const at = (el.closest('label') as HTMLElement | null) ?? el;
				at.dispatchEvent(new PointerEvent('pointerover', { bubbles: true, pointerType: 'mouse' }));
				return !el.title && !at.title;
			})
			.map((el) => el.outerHTML.slice(0, 160));
	});
}

test('everything that can be clicked says what it is', async ({ page }) => {
	test.slow();
	await page.goto('/');
	const name = `Hover ${Date.now()}`;
	const ws: string = (await apiCall(page, 'POST', '/workspaces', { name })).body.id;
	const make = async (entity: string, body: unknown) =>
		(await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, body)).body.id as string;
	const project = await make('projects', { name: 'Porch' });
	const status = await make('statuses', { project_id: project, name: 'To do', category: 'todo' });
	const task = await make('tasks', {
		title: 'Repaint',
		project_id: project,
		status_id: status,
		sort_key: 'a0'
	});
	await page.goto(`/?ws=${ws}`);
	await expect(page.getByRole('button', { name: 'New task' }).first()).toBeVisible();

	const screens = [
		'/',
		'/today',
		'/upcoming',
		'/projects',
		`/projects/${project}`,
		// The task views are ?view=, the project's tabs ?tab=.
		`/projects/${project}?view=board`,
		`/projects/${project}?view=calendar`,
		`/projects/${project}?view=table`,
		`/projects/${project}?view=timeline`,
		`/projects/${project}?tab=wiki`,
		`/projects/${project}?tab=time`,
		`/projects/${project}?task=${task}`,
		'/wiki',
		'/time',
		'/reports',
		'/trash',
		'/help',
		'/settings',
		'/settings/profile',
		'/settings/appearance',
		'/settings/preferences',
		'/settings/keyboard',
		'/settings/notifications',
		'/settings/security',
		'/settings/tokens',
		'/settings/admin',
		...WORKSPACE_SETTINGS.map((p) => `/settings/w/${ws}/${p}`)
	];
	const found: Record<string, string[]> = {};
	for (const screen of screens) {
		await page.goto(screen);
		await expect(page.getByRole('heading', { level: 1 }).first()).toBeAttached();
		await page.waitForLoadState('networkidle').catch(() => {});
		const quiet = await silent(page);
		if (quiet.length) found[screen] = quiet;
	}

	// Inbox, My Day and Upcoming as boards, with their resize edges.
	for (const path of ['/', '/today', '/upcoming']) {
		await page.goto(path);
		await page
			.getByRole('group', { name: 'Show as' })
			.getByRole('button', { name: 'Board' })
			.click();
		await expect(page.getByRole('group', { name: 'Board' })).toBeVisible();
		const quiet = await silent(page);
		if (quiet.length) found[`${path} as a board`] = quiet;
	}

	// The Time page with Log time open, then by week with its chart.
	await page.goto('/time');
	await page.getByRole('button', { name: 'Log time', exact: true }).click();
	const log = page.getByRole('form', { name: 'Log time to a project' });
	await log.getByLabel('Project').selectOption({ label: 'Porch' });
	await log.getByLabel('Time spent').fill('2h');
	let quiet = await silent(page);
	if (quiet.length) found['/time, logging'] = quiet;
	await log.getByRole('button', { name: 'Log it' }).click();
	await expect(log).toHaveCount(0);
	await page
		.getByRole('combobox', { name: 'Period', exact: true })
		.selectOption({ label: 'Last 8 weeks' });
	await expect(page.getByRole('group', { name: /^Hours logged each week by/ })).toBeVisible();
	quiet = await silent(page);
	if (quiet.length) found['/time by week'] = quiet;
	expect(found).toEqual({});
});

test('everything in the theme editor says what it is', async ({ browser }) => {
	// A theme is a person's own: someone new, so the owner keeps none of it.
	const { page: own, context } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Hova');
	const found: Record<string, string[]> = {};
	const look = async (name: string) => {
		await own.waitForLoadState('networkidle').catch(() => {});
		const quiet = await silent(own);
		if (quiet.length) found[name] = quiet;
	};
	await own.goto('/settings/themes');
	await expect(own.getByRole('heading', { name: 'Your themes' })).toBeVisible();
	await look('manager');
	await own.getByRole('button', { name: 'New theme', exact: true }).click();
	await expect(own.getByLabel('Theme name')).toBeVisible();
	await look('editor');
	// A row open in place, with its color's controls.
	await own.getByRole('button', { name: /^Page:/ }).click();
	await expect(own.getByLabel('Hex', { exact: true })).toBeVisible();
	await look('a color open');
	await own.keyboard.press('Escape');
	// A search across the parts, and the whole app in the preview.
	await own.getByLabel('Find a setting').fill('heading');
	await expect(own.getByRole('button', { name: /^Heading 1:/ })).toBeVisible();
	await look('search');
	await own.getByLabel('Find a setting').fill('');
	await own.getByText('Whole app', { exact: true }).click();
	await look('whole app');
	// The contrast button's problems and goal.
	await own.getByRole('button', { name: /at AA/ }).click();
	await expect(own.getByRole('heading', { name: 'Contrast', exact: true })).toBeVisible();
	await look('contrast');
	expect(found).toEqual({});
	await context.close();
});

test('a name shows on hover, and a title of its own stays', async ({ page }) => {
	await page.goto('/settings/labels');
	const add = page.getByRole('button', { name: 'Add label' });
	// Nothing until hovered, so a screen reader does not hear it twice.
	await expect(add).not.toHaveAttribute('title');
	await add.hover();
	await expect(add).toHaveAttribute('title', 'Add label');

	// Settings' list on a phone, Profile beside the rail elsewhere: the picker is in the rail.
	await page.goto('/settings');
	const picker = page.getByLabel('Workspace', { exact: true });
	await picker.hover();
	await expect(picker).toHaveAttribute('title', 'The workspace whose settings show under it');
});
