import { expect, test, type Browser, type Page } from '@playwright/test';
import { axe } from './axe';
import { OWNER, saved, SIGNED_OUT, newPerson } from './helpers';

// Preferences belong to a person, and every test here changes them, so
// each test signs up someone new rather than touching the owner other
// tests share.
test.use({ storageState: OWNER });

/** Someone new, signed in, on a page of their own. */
async function person(browser: Browser, name: string) {
	const { page: own, context } = await newPerson(() => browser.newContext(SIGNED_OUT), name);
	return { page: own, context };
}

/** The same person on another device: their session, nothing else of the first browser's. */
async function secondDevice(browser: Browser, first: Page) {
	const { cookies } = await first.context().storageState();
	const context = await browser.newContext({ storageState: { cookies, origins: [] } });
	return context.newPage();
}

test('preferences follow the person to another device', async ({ browser }) => {
	const { page: laptop } = await person(browser, 'Rowan');
	await laptop.goto('/settings/appearance');
	await expect(laptop.getByRole('heading', { name: 'Appearance' })).toBeVisible();
	// The picker's choice, not the theme manager's Start from, which names it too.
	const picker = laptop.getByRole('group', { name: 'Theme', exact: true });
	await picker.getByText('Calm', { exact: true }).click();
	await saved(laptop, 'theme', 'calm');
	await laptop.goto('/settings/preferences');
	await laptop.getByLabel('Dates', { exact: true }).selectOption('day-month');
	await laptop.getByLabel('Times', { exact: true }).selectOption('12');
	await laptop.getByLabel('Open Mila on').selectOption('today');
	await saved(laptop, 'startPage', 'today');
	await saved(laptop, 'theme', 'calm');

	// A new device opens on My Day, in Calm, before anyone touches a setting.
	const phone = await secondDevice(browser, laptop);
	await phone.goto('/');
	await expect(phone).toHaveURL(/\/today$/);
	await expect(phone.locator('html')).toHaveAttribute('data-theme', 'calm');
	// A link into the app is followed, not sent to the start page.
	await phone.goto('/upcoming');
	await expect(phone.getByRole('heading', { name: 'Upcoming' })).toBeVisible();

	// A change on one device reaches the other while it is open.
	await laptop.goto('/settings/appearance');
	await picker.getByText('High contrast', { exact: true }).click();
	await expect(phone.locator('html')).toHaveAttribute('data-theme', 'contrast');
	await phone.context().close();
});

test('XL and XXL make text and rows larger, and paint first', async ({ browser }) => {
	const { page: own } = await person(browser, 'Xavier');
	await own.goto('/settings/appearance');
	const heading = own.getByRole('heading', { name: 'Appearance' });
	await expect(heading).toBeVisible();
	const size = () => heading.evaluate((h) => parseFloat(getComputedStyle(h).fontSize));
	const atM = await size();
	const fontSize = own.getByRole('group', { name: 'Font size' });
	await fontSize.getByText('XL', { exact: true }).click();
	await expect(own.locator('html')).toHaveAttribute('data-font-size', 'xl');
	await saved(own, 'fontSize', 'xl');
	await expect.poll(size).toBeCloseTo(atM * 1.25, 0);
	await fontSize.getByText('XXL', { exact: true }).click();
	await saved(own, 'fontSize', 'xxl');
	await expect.poll(size).toBeCloseTo(atM * 1.4, 0);
	// Only the pre-paint script can set it with the app's code blocked.
	await own.route('**/_app/**', (route) => route.abort());
	await own.reload();
	await expect(own.locator('html')).toHaveAttribute('data-font-size', 'xxl');
	await own.context().close();
});

test('the accent is painted before the app starts', async ({ browser }) => {
	const { page: own } = await person(browser, 'Indigo');
	await own.goto('/settings/appearance');
	await own.getByRole('slider', { name: 'Accent hue' }).fill('140');
	await saved(own, 'accentHue', 140);
	const painted = await own.evaluate(() =>
		document.documentElement.style.getPropertyValue('--accent-l')
	);
	// Only the pre-paint script can set it with the app's code blocked.
	await own.route('**/_app/**', (route) => route.abort());
	await own.reload();
	const html = own.locator('html');
	await expect(html).toHaveAttribute('style', /--accent-hue: ?140/);
	expect(
		await own.evaluate(() => document.documentElement.style.getPropertyValue('--accent-l'))
	).toBe(painted);
});

test('the sidebar can be rearranged and trimmed', async ({ browser, isMobile }) => {
	test.skip(isMobile, 'the sidebar is a desktop thing');
	const { page: own } = await person(browser, 'Sage');
	await own.goto('/settings/preferences');
	const items = own.getByRole('list', { name: 'Sidebar items' });
	await expect(items.getByRole('listitem')).toHaveCount(8);
	await own.getByRole('checkbox', { name: 'Show Upcoming' }).uncheck({ force: true });
	// From the keyboard: Alt+Up on a row's control moves it, and focus stays.
	await own.getByRole('button', { name: 'Move Projects up' }).focus();
	await own.keyboard.press('Alt+ArrowUp');
	await own.keyboard.press('Alt+ArrowUp');
	await own.keyboard.press('Alt+ArrowUp');
	await expect(own.getByRole('button', { name: 'Move Projects up' })).toBeFocused();
	await own.keyboard.press('Enter');
	await own.keyboard.press('Enter');
	await own.keyboard.press('Enter');
	await own.keyboard.press('Enter');
	await expect(items.getByRole('listitem').first()).toContainText('Projects');
	await saved(own, 'sidebarOrder', [
		'projects',
		'inbox',
		'today',
		'upcoming',
		'wiki',
		'time',
		'reports',
		'views'
	]);

	const sidebar = own.getByRole('navigation', { name: 'Main' }).filter({ visible: true });
	await expect(sidebar.getByRole('link', { name: 'Upcoming' })).toHaveCount(0);
	const headingY = await sidebar.getByRole('heading', { name: 'Projects' }).boundingBox();
	const inboxY = await sidebar.getByRole('link', { name: 'Inbox' }).boundingBox();
	expect(headingY!.y).toBeLessThan(inboxY!.y);
	await axe(own);

	// Collapsing is remembered on this device.
	await sidebar.getByRole('button', { name: 'Collapse sidebar' }).click();
	await own.reload();
	await expect(sidebar.getByRole('button', { name: 'Expand sidebar' })).toBeVisible();
});

test('completed tasks can open shown, and one-key shortcuts can be turned off', async ({
	browser
}) => {
	const { page: own } = await person(browser, 'Ash');
	await own.goto('/');
	await own.getByLabel('New task').fill('Already done');
	await own.getByLabel('New task').press('Enter');
	await own.getByRole('checkbox', { name: 'Already done' }).check({ force: true });
	const toggle = own.getByRole('button', { name: /Completed · 1/ });
	await expect(toggle).toHaveAttribute('aria-expanded', 'false');

	await own.goto('/settings/preferences');
	await own.getByRole('checkbox', { name: 'Show completed tasks when a list opens' }).check({
		force: true
	});
	await own.goto('/settings/keyboard');
	await own.getByRole('checkbox', { name: 'Single-key shortcuts' }).uncheck({ force: true });
	await saved(own, 'singleKeyShortcuts', false);
	await own.goto('/');
	await expect(toggle).toHaveAttribute('aria-expanded', 'true');
	await expect(own.getByRole('list', { name: 'Completed tasks' })).toContainText('Already done');

	await own.locator('body').press('q');
	await expect(own.getByRole('dialog', { name: 'Add a task' })).toBeHidden();
	await own.keyboard.press('Control+k');
	await expect(own.getByLabel('Search for a task, project or command')).toBeFocused();
	// Nothing offers the keys that are off.
	await expect(own.getByRole('option', { name: /^Add a task/ })).not.toContainText('Q');
	await own.keyboard.press('Escape');
	await own.goto('/today');
	await expect(own.getByText('Press T on any task to bring it here.')).toBeVisible();

	// Help shows those keys as off, and offers none of them.
	await own.goto('/help/shortcuts');
	await expect(own.getByText('Single-key shortcuts are off, so the keys marked Off')).toBeVisible();
	const quickAdd = own.getByRole('row', { name: /^Add a task/ });
	await expect(quickAdd).toContainText('Off');
	await expect(quickAdd.locator('kbd')).toHaveCount(0);
	// Where the defaults come from offers none that is off either.
	const origin = own.getByRole('listitem').filter({ hasText: "Todoist's key for quick add" });
	await expect(origin).toContainText('Off');
	await expect(origin.locator('kbd')).toHaveCount(0);
	// A sentence about a key that is off reads without it.
	await own.goto('/help/quick-add');
	await expect(own.locator('article')).toContainText('round add button on a phone. As you type');
	await own.goto('/help/shortcuts');
	// Ctrl+/ still opens the list, and says so.
	await own.locator('#main').focus();
	await own.keyboard.press('Control+/');
	const sheet = own.getByRole('dialog', { name: 'Keyboard shortcuts' });
	await expect(sheet).toContainText('Ctrl+/ still opens this list.');
	await own.keyboard.press('Escape');
});

test('the palette changes the theme and works on the focused task', async ({ browser }) => {
	const { page: own } = await person(browser, 'Wren');
	await own.goto('/projects');
	for (const name of ['Errands', 'Garden']) {
		await own.getByLabel('Name a new project').fill(name);
		await own.getByLabel('Name a new project').press('Enter');
		await expect(own.getByLabel('Project name')).toHaveValue(name);
		await own.goto('/projects');
	}
	await own
		.getByRole('main')
		.getByRole('link', { name: /Errands/ })
		.click();
	await own.getByLabel('New task').fill('Buy twine');
	await own.getByLabel('New task').press('Enter');

	const palette = own.getByLabel('Search for a task, project or command');
	await own.keyboard.press('Control+k');
	await palette.fill('high contrast');
	await own.keyboard.press('Enter');
	await expect(own.locator('html')).toHaveAttribute('data-theme', 'contrast');

	const row = own.getByRole('list', { name: 'Tasks' }).locator('[data-task-id]').first();
	await row.focus();
	await own.keyboard.press('Control+k');
	await palette.fill('priority urgent');
	await own.keyboard.press('Enter');
	await expect(row.getByRole('img', { name: 'Urgent' })).toBeVisible();

	await row.focus();
	await own.keyboard.press('Control+k');
	await palette.fill('due today');
	await own.keyboard.press('Enter');
	await expect(row.locator('.due')).toBeVisible();

	await row.focus();
	await own.keyboard.press('Control+k');
	await palette.fill('move');
	await own.keyboard.press('Enter');
	await expect(own.getByLabel('Move Buy twine to')).toBeFocused();
	await own.keyboard.type('gard');
	await own.keyboard.press('Enter');
	await expect(own.getByText('Moved to Garden')).toBeVisible();
	await expect(own.getByRole('list', { name: 'Tasks' })).toHaveCount(0);
});

test('a project gets an icon, a color and a description, and can be archived', async ({
	browser
}) => {
	const { page: own } = await person(browser, 'Fern');
	await own.goto('/projects');
	await own.getByLabel('Name a new project').fill('Allotment');
	await own.getByLabel('Name a new project').press('Enter');
	await expect(own.getByLabel('Project name')).toHaveValue('Allotment');

	await own.getByRole('button', { name: 'Icon and color of Allotment' }).click();
	const picker = own.locator('[popover]:popover-open');
	await picker.getByRole('radio', { name: 'green' }).check({ force: true });
	await picker.getByRole('radio', { name: 'sprout' }).check({ force: true });
	await expect(picker.getByRole('radio', { name: 'sprout' })).toBeChecked();
	await axe(own);
	await own.keyboard.press('Escape');
	await own.getByLabel('Project description').fill('Plot 14, by the gate');
	await own.getByLabel('Project description').blur();

	await own.getByRole('button', { name: 'Archive Allotment', exact: true }).click();
	await expect(own).toHaveURL(/\/projects$/);
	const archived = own.getByRole('list', { name: 'Archived projects' });
	await expect(archived).toContainText('Allotment');
	// It was the only one in use.
	await expect(own.getByText('No projects yet')).toBeVisible();
	// The undo toast slides in after the page changes; check once it is in.
	await expect(own.getByRole('status').filter({ hasText: 'Archived Allotment' })).toBeVisible();
	await axe(own);
	await archived.getByRole('link', { name: 'Allotment' }).click();
	await expect(own.getByLabel('Project description')).toHaveValue('Plot 14, by the gate');
	await expect(own.getByText('This project is archived')).toBeVisible();
	await own.getByRole('button', { name: 'Unarchive Allotment' }).click();
	await expect(own.getByRole('button', { name: 'Archive Allotment', exact: true })).toBeVisible();
	await own.goto('/projects');
	await expect(own.getByRole('list', { name: 'Projects', exact: true })).toContainText('Allotment');
	await expect(own.getByRole('list', { name: 'Archived projects' })).toHaveCount(0);
});

test('projects move in the sidebar from the keyboard', async ({ browser, isMobile }) => {
	test.skip(isMobile, 'the sidebar is a desktop thing');
	const { page: own } = await person(browser, 'Moss');
	for (const name of ['First', 'Second']) {
		await own.goto('/projects');
		await own.getByLabel('Name a new project').fill(name);
		await own.getByLabel('Name a new project').press('Enter');
		await expect(own.getByLabel('Project name')).toHaveValue(name);
	}
	const sidebar = own.getByRole('navigation', { name: 'Main' }).filter({ visible: true });
	const second = sidebar.getByRole('link', { name: 'Second' });
	await second.focus();
	await own.keyboard.press('Alt+ArrowUp');
	await expect(second).toBeFocused();
	const links = sidebar.locator('[data-project-nav]');
	await expect(links.first()).toHaveText('Second');
	await own.reload();
	await expect(links.first()).toHaveText('Second');
});
