// Help: reaching it from the sidebar, the phone's top bar and the
// palette; searching it; a topic that is not there; and topics offline.
import { expect, test } from '@playwright/test';
import { axe } from './axe';
import { OWNER } from './helpers';

test.use({ storageState: OWNER });

test('the sidebar and the phone bar reach Help', async ({ page, isMobile }) => {
	await page.goto('/');
	await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	if (isMobile) await page.getByRole('link', { name: 'Help', exact: true }).click();
	else
		await page
			.getByRole('navigation', { name: 'Main' })
			.getByRole('link', { name: 'Help', exact: true })
			.click();
	await expect(page).toHaveURL(/\/help$/);
	await expect(page.getByRole('heading', { level: 1, name: 'Help' })).toBeVisible();
	await expect(page.getByRole('link', { name: 'Help', exact: true }).first()).toHaveAttribute(
		'aria-current',
		'page'
	);
	await expect(page.getByRole('link', { name: 'Touch gestures' })).toBeVisible();
});

test('searching help says how many results, and a result lands on its section', async ({
	page
}) => {
	await page.goto('/help');
	const search = page.getByLabel('Search help');
	await search.fill('repeat');
	const results = page.getByRole('list', { name: 'Help results' });
	await expect(results.getByRole('link').first()).toHaveText('Repeating tasks');
	await expect(page.locator('#announcer')).toHaveText(/^\s*\d+ results?\s*$/);
	// The query is kept, so Back comes back to the results.
	await expect(page).toHaveURL(/\?q=repeat$/);

	await results.getByRole('link', { name: 'Repeats', exact: true }).click();
	await expect(page).toHaveURL(/\/help\/quick-add#repeats$/);
	await expect(page.getByRole('heading', { name: 'Repeats', exact: true })).toBeFocused();
	await page.goBack();
	await expect(page.getByLabel('Search help')).toHaveValue('repeat');
});

test('the palette finds help', async ({ page, isMobile }) => {
	await page.goto('/');
	await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	if (isMobile) await page.locator('nav.tabs').getByRole('button', { name: 'Search' }).click();
	else await page.keyboard.press('Control+k');
	const palette = page.getByRole('dialog', { name: 'Command palette' });
	await palette.getByRole('combobox').fill('quick add syntax');
	await expect(palette.getByRole('option').first()).toContainText('Quick add');
	await palette.getByRole('option').first().click();
	await expect(page).toHaveURL(/\/help\/quick-add$/);
	await expect(page.getByRole('heading', { level: 1, name: 'Quick add' })).toBeVisible();
});

test('a topic that is not there says so inside the app', async ({ page }) => {
	await page.goto('/help/no-such-topic');
	await expect(page.getByRole('heading', { name: "This help topic isn't here" })).toBeVisible();
	await expect(page.getByRole('link', { name: 'Go to Help' })).toBeVisible();
	// The app shell is still there around it.
	await expect(page.locator('#main')).toBeVisible();
});

test('topics open offline once the app has loaded', async ({ page, context }) => {
	await page.goto('/help');
	await page.waitForFunction(() => navigator.serviceWorker?.controller !== null);
	await expect(page.getByRole('heading', { level: 1, name: 'Help' })).toBeVisible();
	await context.setOffline(true);
	await page.goto('/help/recurrence');
	await expect(page.getByRole('heading', { level: 1, name: 'Repeating tasks' })).toBeVisible();
	await expect(page.getByRole('heading', { name: 'Ending a repeat' })).toBeVisible();
	await context.setOffline(false);
});

test('the shortcut reference shows keys, and finds them by name', async ({ page }) => {
	await page.goto('/help/shortcuts');
	const list = page.getByRole('table', { name: 'On a task in a list' });
	await expect(list.getByRole('row', { name: /^Complete or reopen/ })).toContainText('X');
	await page.getByLabel('Find a shortcut').fill('priority');
	await expect(list.getByRole('row')).toHaveCount(6);
	await expect(page.getByRole('table', { name: 'Anywhere' })).toBeHidden();
	await expect(page.getByText('Todoist')).toHaveCount(2);
});

test("quick add's syntax opens and closes without losing what was typed", async ({
	page,
	isMobile
}) => {
	await page.goto('/');
	await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	await page
		.getByRole('button', { name: isMobile ? 'Add a task' : 'New task' })
		.filter({ visible: true })
		.click();
	const dialog = page.getByRole('dialog', { name: 'Add a task' });
	await dialog.getByLabel('Quick add').fill('Water the plants');
	const syntax = dialog.locator('summary', { hasText: 'Syntax' });
	await syntax.click();
	await expect(dialog.getByRole('link', { name: 'More about quick add' })).toBeVisible();
	await syntax.click();
	await expect(dialog.getByRole('link', { name: 'More about quick add' })).toBeHidden();
	await expect(dialog.getByLabel('Quick add')).toHaveValue('Water the plants');
	await page.keyboard.press('Escape');
	await expect(dialog).toBeHidden();
});

test('going to Help again starts a new search', async ({ page, isMobile }) => {
	await page.goto('/help');
	await page.getByLabel('Search help').fill('repeat');
	await expect(page).toHaveURL(/\?q=repeat$/);
	const nav = isMobile
		? page.locator('.phone-bar')
		: page.getByRole('navigation', { name: 'Main' });
	await nav.getByRole('link', { name: 'Help', exact: true }).click();
	await expect(page).toHaveURL(/\/help$/);
	await expect(page.getByLabel('Search help')).toHaveValue('');
	await expect(page.getByRole('link', { name: 'Touch gestures' })).toBeVisible();
});

test('a topic reads well with the keys as they are', async ({ page }) => {
	await page.goto('/help/quick-add');
	await expect(page.locator('article')).toContainText(
		/round\s+add button on a phone, or press\s+Q\s*Q\. As you type/
	);
	await page.goto('/help/shortcuts');
	// The longer sentence about a command is a row of its own, not part of its name.
	await expect(page.getByRole('rowheader', { name: 'Add a task', exact: true })).toBeVisible();
});

test('a help link in the task sheet closes the task on a phone', async ({ page, isMobile }) => {
	test.skip(!isMobile, 'the sheet covers the page only on a phone');
	await page.goto('/projects');
	await page.getByLabel('Name a new project').fill(`Help sheet ${Date.now()}`);
	await page.getByLabel('Name a new project').press('Enter');
	await page.getByLabel('New task').fill('Explained');
	await page.getByLabel('New task').press('Enter');
	await page.getByRole('button', { name: 'Explained', exact: true }).click();
	const sheet = page.getByRole('complementary', { name: 'Task details' });
	await expect(sheet).toBeVisible();
	await sheet.getByRole('link', { name: 'How repeats work' }).click();
	await expect(page).toHaveURL(/\/help\/recurrence$/);
	await expect(sheet).toBeHidden();
	await expect(page.getByRole('heading', { level: 1, name: 'Repeating tasks' })).toBeVisible();
});

test('help opens with its topics in groups, and a topic leads on to the next', async ({
	page,
	isMobile
}) => {
	await page.goto('/help');
	await expect(page.getByRole('heading', { level: 2, name: 'Getting work in' })).toBeVisible();
	await expect(page.getByRole('heading', { level: 2, name: 'Beyond the browser' })).toBeVisible();
	await page.getByRole('link', { name: 'Quick add', exact: true }).click();

	// Its group, its title and what it covers, a place to try it, and a tip.
	const title = page.getByRole('heading', { level: 1, name: 'Quick add' });
	await expect(title).toBeVisible();
	await expect(page.getByText('Getting work in')).toBeVisible();
	await expect(page.getByText('Call Dana tomorrow 3pm #work !high')).toBeVisible();
	await expect(page.getByRole('note').first()).toContainText('nothing you type is lost');
	await axe(page);

	// The list of sections follows the reading, beside the topic or folded on a phone.
	if (isMobile) {
		await expect(page.locator('details.toc')).toBeVisible();
	} else {
		const list = page.getByRole('navigation', { name: 'On this page' }).filter({ visible: true });
		await expect(list.getByRole('link', { name: 'Projects' })).toHaveAttribute(
			'aria-current',
			'location'
		);
		await list.getByRole('link', { name: 'Where the task goes' }).click();
		await expect(list.getByRole('link', { name: 'Where the task goes' })).toHaveAttribute(
			'aria-current',
			'location'
		);
		await expect(page.getByRole('heading', { name: 'Where the task goes' })).toBeFocused();
		await page.waitForTimeout(350); // a section just picked holds for a moment
		await page.evaluate(() => document.getElementById('dates')?.scrollIntoView());
		await expect(list.getByRole('link', { name: 'Dates and times' })).toHaveAttribute(
			'aria-current',
			'location'
		);
	}

	const pager = page.getByRole('navigation', { name: 'Previous and next topics' });
	await pager.getByRole('link', { name: /Next.*Email to task/ }).click();
	await expect(page).toHaveURL(/\/help\/email-to-task$/);
	await expect(page.getByRole('heading', { level: 1, name: 'Email to task' })).toBeVisible();
	await pager.getByRole('link', { name: /Previous.*Quick add/ }).click();
	await expect(page).toHaveURL(/\/help\/quick-add$/);
	await pager.getByRole('link', { name: /Previous.*Help/ }).click();
	await expect(page).toHaveURL(/\/help$/);

	// The last topic has nothing after it.
	await page.goto('/help/api');
	await expect(pager.getByRole('link', { name: /Next/ })).toHaveCount(0);
	await expect(page.getByRole('navigation', { name: 'Related topics' })).toBeVisible();
});

test('a topic shows its pictures in the scheme showing, and nobody gets them all for offline', async ({
	page
}) => {
	await page.goto('/help/views-and-filters');
	const picture = page.getByRole('img', { name: /^A project shown as a list/ });
	await expect(picture).toHaveCount(1);
	await picture.scrollIntoViewIfNeeded();
	await expect
		.poll(() => picture.evaluate((img: HTMLImageElement) => img.naturalWidth))
		.toBeGreaterThan(0);
	const scheme = await page.locator('html').getAttribute('data-scheme');
	expect(await picture.getAttribute('src')).toContain(`list-${scheme ?? 'light'}.webp`);

	await page.waitForFunction(() => navigator.serviceWorker?.controller !== null);
	const held = await page.evaluate(async () => {
		const names = await caches.keys();
		const held = await Promise.all(
			names
				.filter((n) => !n.includes('files'))
				.map(async (n) => (await (await caches.open(n)).keys()).map((r) => r.url))
		);
		return held.flat();
	});
	expect(held.length).toBeGreaterThan(5);
	expect(held.filter((url) => url.includes('/help-images/'))).toEqual([]);
});
