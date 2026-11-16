import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { OWNER } from './helpers';

test.use({ storageState: OWNER });

const unique = (word: string) => `${word}${Date.now().toString(36)}`;

async function newProject(page: Page, name: string) {
	await page.goto('/projects');
	await page.getByLabel('Name a new project').fill(name);
	await page.getByLabel('Name a new project').press('Enter');
	await expect(page.getByLabel('Project name')).toHaveValue(name);
	return page.url();
}

async function addTask(page: Page, title: string) {
	await page.getByLabel('New task').fill(title);
	await page.getByLabel('New task').press('Enter');
	await expect(row(page, title)).toBeVisible();
}

const row = (page: Page, title: string) =>
	page.getByRole('list', { name: 'Tasks' }).getByRole('listitem').filter({ hasText: title });

const detail = (page: Page) => page.getByRole('complementary', { name: 'Task details' });

async function open(page: Page, title: string) {
	await row(page, title).getByRole('button', { name: title, exact: true }).click();
	await expect(detail(page).getByLabel('Task title')).toHaveValue(title);
}

function ymd(d: Date) {
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function inDays(n: number) {
	const d = new Date();
	d.setDate(d.getDate() + n);
	return d;
}

const short = (d: Date) => d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });

test('a repeating task comes back when it is completed', async ({ page }) => {
	await newProject(page, unique('Ferns'));
	await addTask(page, 'Water the ferns');
	await open(page, 'Water the ferns');
	await detail(page).getByLabel('Repeat', { exact: true }).selectOption({ label: 'Every day' });
	// A task that starts repeating gets its first date.
	await expect(detail(page).getByLabel('Due date')).toHaveValue(ymd(new Date()));
	await expect(detail(page)).toContainText('Every day');
	await page.getByRole('button', { name: 'Close' }).click();
	await expect(
		row(page, 'Water the ferns').getByRole('img', { name: 'Repeats every day' })
	).toBeVisible();

	// The box's label takes the tap; its input is hidden.
	await row(page, 'Water the ferns').locator('label').first().click();
	// The completed one leaves; the next one, due tomorrow, takes its place.
	await expect(row(page, 'Water the ferns')).toContainText(short(inDays(1)));
	await expect(row(page, 'Water the ferns')).toHaveCount(1);
	await page.reload();
	await expect(row(page, 'Water the ferns')).toContainText(short(inDays(1)));
	await open(page, 'Water the ferns');
	await expect(detail(page).getByLabel('Due date')).toHaveValue(ymd(inDays(1)));
	await expect(
		detail(page).getByLabel('Repeat', { exact: true }).locator('option:checked')
	).toHaveText('Every day');
});

test('a task repeating after completion survives being done twice in a day', async ({ page }) => {
	await newProject(page, unique('Pills'));
	await addTask(page, 'Take the pills');
	await open(page, 'Take the pills');
	// On a phone the sheet slides up first; a forced click does not wait for it.
	await expect(detail(page)).toBeInViewport({ ratio: 1 });
	await detail(page).getByLabel('Repeat', { exact: true }).selectOption({ label: 'Every day' });
	await detail(page)
		.getByRole('checkbox', { name: 'Repeat after completion' })
		.check({ force: true });
	await expect(detail(page)).toContainText('after completion');
	await page.getByRole('button', { name: 'Close' }).click();

	await row(page, 'Take the pills').locator('label').first().click();
	await expect(row(page, 'Take the pills')).toContainText(short(inDays(1)));
	// Done again today: the next one is due tomorrow as well, a new task
	// rather than the one just completed, so the series goes on.
	await row(page, 'Take the pills').locator('label').first().click();
	await expect(row(page, 'Take the pills')).toHaveCount(1);
	await expect(row(page, 'Take the pills')).toContainText(short(inDays(1)));
	await expect(row(page, 'Take the pills').getByRole('checkbox')).not.toBeChecked();
	await page.reload();
	await expect(row(page, 'Take the pills')).toHaveCount(1);
});

test('a custom repeat is set from the keyboard', async ({ page }) => {
	await newProject(page, unique('Gym'));
	await addTask(page, 'Lift');
	await open(page, 'Lift');
	const panel = detail(page);
	await panel.getByLabel('Repeat', { exact: true }).selectOption('custom');
	const custom = panel.getByRole('group', { name: 'Custom repeat' });
	await custom.getByLabel('Repeat every').fill('2');
	await custom.getByLabel('Repeat every').press('Tab');
	await expect(custom.getByLabel('Repeat unit').locator('option:checked')).toHaveText('weeks');
	// Monday and Thursday on and the rest off, whatever today is.
	const days = custom.getByRole('group', { name: 'Repeat on these days' });
	const names = ['Monday', 'Thursday', 'Tuesday', 'Wednesday', 'Friday', 'Saturday', 'Sunday'];
	for (const day of names) {
		const button = days.getByRole('button', { name: day });
		const want = day === 'Monday' || day === 'Thursday' ? 'true' : 'false';
		if ((await button.getAttribute('aria-pressed')) !== want) {
			await button.focus();
			await page.keyboard.press(want === 'true' ? 'Enter' : 'Space');
			await expect(button).toHaveAttribute('aria-pressed', want);
		}
	}
	await expect(panel).toContainText('Every 2 weeks on Mon and Thu');
	await panel.evaluate((el) => el.scrollTo(0, 0));
	await axe(page);

	await panel.getByRole('checkbox', { name: 'Repeat after completion' }).check({ force: true });
	await expect(panel).toContainText('2 weeks after completion');
	await expect(days).toBeHidden();
	await page.reload();
	await expect(panel).toContainText('2 weeks after completion');
});

test.describe('the keyboard', () => {
	test.skip(({ isMobile }) => isMobile, 'phones have no keyboard shortcuts');

	test('quick add understands a repeat', async ({ page }) => {
		const project = unique('Chores');
		const url = await newProject(page, project);
		await page.locator('body').press('q');
		const input = page.getByLabel('Quick add', { exact: true });
		await input.fill(`Take out the bins every monday #${project}`);
		await expect(page.getByRole('dialog', { name: 'Add a task' })).toContainText(
			'Repeats every monday'
		);
		await input.press('Enter');
		await expect(page.getByText(`Added "Take out the bins" to ${project}`)).toBeVisible();
		await page.goto(url);
		const monday = new Date();
		while (monday.getDay() !== 1) monday.setDate(monday.getDate() + 1);
		const bins = row(page, 'Take out the bins');
		await expect(bins.getByRole('img', { name: 'Repeats weekly on Mon' })).toBeVisible();
		await expect(bins).toContainText(short(monday));
	});

	test('undo takes back a completion and the task it made', async ({ page }) => {
		await newProject(page, unique('Undo'));
		await addTask(page, 'Stretch');
		await open(page, 'Stretch');
		await detail(page).getByLabel('Repeat', { exact: true }).selectOption({ label: 'Every day' });
		await page.getByRole('button', { name: 'Close' }).click();
		await row(page, 'Stretch').focus();
		await page.keyboard.press('x');
		await expect(row(page, 'Stretch')).toContainText(short(inDays(1)));
		await page.getByRole('button', { name: 'Undo', exact: true }).click();
		await expect(row(page, 'Stretch')).toHaveCount(1);
		await expect(row(page, 'Stretch')).toContainText(short(new Date()));
		await expect(row(page, 'Stretch')).not.toContainText(short(inDays(1)));
	});

	test('the calendar shows where a repeating task falls next', async ({ page }) => {
		const url = await newProject(page, unique('Ahead'));
		await addTask(page, 'Feed the cat');
		await open(page, 'Feed the cat');
		await detail(page).getByLabel('Repeat', { exact: true }).selectOption({ label: 'Every day' });
		await page.getByRole('button', { name: 'Close' }).click();
		await page.goto(url + '?view=calendar');
		const tomorrow = inDays(1).toLocaleDateString(undefined, { month: 'long', day: 'numeric' });
		// Exactly: "November 2" is also how "November 20" begins.
		const ghost = page.getByRole('button', {
			name: `Feed the cat, repeats on ${tomorrow}`,
			exact: true
		});
		await expect(ghost).toBeVisible();
		await expect(ghost).not.toHaveAttribute('draggable', 'true');
		await expect(
			page
				.getByRole('button', { name: /^Feed the cat/ })
				.first()
				.getByRole('img', { name: 'Repeats every day' })
		).toBeVisible();
		await axe(page);
		await ghost.focus();
		await page.keyboard.press('Enter');
		await expect(detail(page).getByLabel('Task title')).toHaveValue('Feed the cat');
		await expect(detail(page).getByLabel('Due date')).toHaveValue(ymd(new Date()));
	});
});
