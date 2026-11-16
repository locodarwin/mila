import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { emailTo, OWNER, SIGNED_OUT, newPerson } from './helpers';

test.use({ storageState: OWNER });

const unique = (word: string) => `${word}${Date.now().toString(36)}`;

async function newProject(page: Page, name: string) {
	await page.goto('/projects');
	await page.getByLabel('Name a new project').fill(name);
	await page.getByLabel('Name a new project').press('Enter');
	await expect(page.getByLabel('Project name')).toHaveValue(name);
}

const row = (page: Page, title: string) =>
	page.getByRole('list', { name: 'Tasks' }).getByRole('listitem').filter({ hasText: title });

const detail = (page: Page) => page.getByRole('complementary', { name: 'Task details' });

function ymd(d: Date) {
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function inDays(n: number) {
	const d = new Date();
	d.setDate(d.getDate() + n);
	return d;
}

test('reminders are set, listed, follow the due time and come with a repeat', async ({ page }) => {
	await newProject(page, unique('Porch'));
	await page.getByLabel('New task').fill('Paint the porch');
	await page.getByLabel('New task').press('Enter');
	await row(page, 'Paint the porch')
		.getByRole('button', { name: 'Paint the porch', exact: true })
		.click();
	const panel = detail(page);
	const add = panel.getByRole('button', { name: 'Add a reminder' });
	const times = panel.getByRole('list', { name: 'Reminder times' });
	const list = panel.getByRole('list', { name: 'Reminders' });

	// With no due date, only fixed times are offered.
	await add.click();
	// The browser runs in US English, so times read the 12-hour way.
	await expect(times.getByRole('button')).toHaveText([
		'Tomorrow, 9:00 AM',
		'Pick a date and time…'
	]);
	await page.keyboard.press('Escape');
	await expect(times).toBeHidden();
	await expect(panel).toBeVisible();
	await panel.getByLabel('Due date').fill(ymd(inDays(2)));
	await panel.getByLabel('Due time').fill('14:30');
	await add.click();
	await times.getByRole('button', { name: '1 hour before', exact: true }).click();
	await expect(list.getByRole('listitem')).toHaveText(['1 hour before due']);
	await expect(times).toBeHidden();

	// A relative reminder follows the task.
	await panel.getByLabel('Due time').fill('16:00');
	await expect(list.getByRole('listitem')).toHaveText(['1 hour before due']);

	// A date and time of one's own, from the keyboard. The arrow keys only
	// move through the choices; nothing is added until one is pressed.
	await add.focus();
	await page.keyboard.press('Enter');
	await expect(times.getByRole('button').first()).toBeFocused();
	await page.keyboard.press('ArrowDown');
	await page.keyboard.press('ArrowDown');
	await expect(list.getByRole('listitem')).toHaveText(['1 hour before due']);
	await page.keyboard.press('ArrowUp');
	await page.keyboard.press('ArrowUp');
	await page.keyboard.press('ArrowUp');
	await expect(times.getByRole('button', { name: 'Pick a date and time…' })).toBeFocused();
	await page.keyboard.press('Enter');
	const when = panel.getByLabel('Reminder date and time');
	await expect(when).toBeFocused();
	const day = inDays(3);
	await when.fill(`${ymd(day)}T10:00`);
	await axe(page);
	await when.press('Enter');
	await expect(add).toBeFocused();
	const named = day.toLocaleDateString('en-US', {
		weekday: 'short',
		month: 'short',
		day: 'numeric'
	});
	await expect(list.getByRole('listitem')).toHaveText([
		'1 hour before due',
		`${named} at 10:00 AM`
	]);

	// The row carries the bell.
	await panel.getByRole('button', { name: 'Close' }).click();
	await expect(
		row(page, 'Paint the porch').getByRole('img', { name: 'Has 2 reminders' })
	).toBeVisible();

	// Removing one.
	await row(page, 'Paint the porch')
		.getByRole('button', { name: 'Paint the porch', exact: true })
		.click();
	await panel.getByRole('button', { name: `Remove reminder: ${named} at 10:00` }).click();
	await expect(list.getByRole('listitem')).toHaveText(['1 hour before due']);

	// The next task of a repeat keeps the reminder that follows the due time.
	await panel.getByLabel('Repeat', { exact: true }).selectOption({ label: 'Every day' });
	await panel.getByRole('button', { name: 'Close' }).click();
	await row(page, 'Paint the porch').locator('label').first().click();
	const next = ymd(inDays(3));
	// The completed one leaves; the next one takes its place.
	await expect(row(page, 'Paint the porch')).toContainText(
		inDays(3).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
	);
	await expect(row(page, 'Paint the porch')).toHaveCount(1);
	await row(page, 'Paint the porch')
		.getByRole('button', { name: 'Paint the porch', exact: true })
		.click();
	await expect(panel.getByLabel('Due date')).toHaveValue(next);
	await expect(list.getByRole('listitem')).toHaveText(['1 hour before due']);
	await expect(
		row(page, 'Paint the porch').getByRole('img', { name: 'Has a reminder' })
	).toBeVisible();
});

test('completing again after an undo brings the reminder to the next task', async ({
	page,
	isMobile
}) => {
	test.skip(isMobile, 'phones have no keyboard shortcuts');
	await newProject(page, unique('Stretch'));
	await page.getByLabel('New task').fill('Stretch');
	await page.getByLabel('New task').press('Enter');
	const stretch = row(page, 'Stretch');
	await stretch.getByRole('button', { name: 'Stretch', exact: true }).click();
	const panel = detail(page);
	await panel.getByLabel('Due date').fill(ymd(inDays(2)));
	await panel.getByLabel('Due time').fill('14:30');
	await panel.getByRole('button', { name: 'Add a reminder' }).click();
	await panel
		.getByRole('list', { name: 'Reminder times' })
		.getByRole('button', { name: '1 hour before', exact: true })
		.click();
	await panel.getByLabel('Repeat', { exact: true }).selectOption({ label: 'Every day' });
	await panel.getByRole('button', { name: 'Close' }).click();

	const short = (d: Date) => d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
	await stretch.focus();
	await page.keyboard.press('x');
	await expect(stretch).toContainText(short(inDays(3)));
	await page.getByRole('button', { name: 'Undo', exact: true }).click();
	await expect(stretch).toHaveCount(1);
	await expect(stretch).toContainText(short(inDays(2)));
	await stretch.focus();
	await page.keyboard.press('x');
	await expect(stretch).toContainText(short(inDays(3)));
	await expect(stretch).toHaveCount(1);
	await expect(stretch.getByRole('img', { name: 'Has a reminder' })).toBeVisible();
	await stretch.getByRole('button', { name: 'Stretch', exact: true }).click();
	await expect(panel.getByLabel('Due date')).toHaveValue(ymd(inDays(3)));
	await expect(panel.getByRole('list', { name: 'Reminders' }).getByRole('listitem')).toHaveText([
		'1 hour before due'
	]);
});

test('a reminder that comes due reaches its person in the app and by email', async ({
	browser,
	isMobile
}) => {
	// A reminder set for the next whole minute (at least 20 seconds out, so
	// it is still ahead when added) fires at the server's next look after
	// it, which comes every 30 seconds.
	test.setTimeout(180_000);
	const {
		context,
		page: robin,
		email
	} = await newPerson(() => browser.newContext(SIGNED_OUT), 'Robin');

	const soon = await robin.evaluate(() => {
		const d = new Date(Date.now() + 20_000);
		d.setSeconds(60, 0);
		const pad = (n: number) => String(n).padStart(2, '0');
		return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
	});
	const title = `Call the plumber ${Date.now()}`;
	if (isMobile) await robin.getByRole('button', { name: 'Add a task' }).click();
	else await robin.locator('body').press('q');
	const input = robin.getByLabel('Quick add', { exact: true });
	await input.fill(`${title} remind ${soon}`);
	await expect(robin.getByRole('dialog', { name: 'Add a task' })).toContainText(
		`Reminder remind ${soon}`
	);
	await input.press('Enter');
	await expect(row(robin, title).getByRole('img', { name: 'Has a reminder' })).toBeVisible();

	const bell = robin.getByRole('button', { name: 'Notifications, 1 unread' });
	await expect(bell).toBeVisible({ timeout: 150_000 });
	await bell.click();
	const notes = robin.getByRole('dialog', { name: 'Notifications' });
	await expect(notes.getByRole('link', { name: new RegExp(`Reminder: ${title}`) })).toBeVisible();

	const mail = await emailTo(email, new RegExp(`^Reminder: ${title}$`));
	expect(mail).toMatch(/\/\?task=[\w-]+&ws=[\w-]+/);
	await context.close();
});
