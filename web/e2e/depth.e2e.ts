import { expect, test, type Page } from '@playwright/test';
import { OWNER } from './helpers';

test.use({ storageState: OWNER });

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

async function open(page: Page, title: string) {
	await row(page, title).getByRole('button', { name: title, exact: true }).click();
	await expect(page.getByRole('complementary', { name: 'Task details' })).toBeVisible();
}

async function close(page: Page) {
	await page.getByRole('button', { name: 'Close' }).click();
	await expect(page.getByRole('complementary', { name: 'Task details' })).toBeHidden();
}

async function addTo(page: Page, label: string, text: string) {
	await page.getByLabel(label).fill(text);
	await page.getByLabel(label).press('Enter');
}

test('a real project, managed end to end', async ({ page }) => {
	await newProject(page, `House ${Date.now()}`);
	await addTask(page, 'Repaint the porch');
	await addTask(page, 'Fix the gate');
	await open(page, 'Repaint the porch');
	const detail = page.getByRole('complementary', { name: 'Task details' });

	await detail.getByLabel('Priority', { exact: true }).selectOption('high');
	await detail.getByLabel('Due date').fill('2026-11-14');
	await detail.getByLabel('Due time').fill('09:30');
	await detail.getByRole('button', { name: 'Edit labels' }).click();
	await addTo(page, 'New label', 'Outdoors');
	await page.keyboard.press('Escape');
	await expect(detail.getByText('Outdoors').first()).toBeVisible();
	await detail.getByRole('button', { name: 'Edit assignees' }).click();
	await page.getByRole('checkbox', { name: /Shawn/ }).check();
	await page.keyboard.press('Escape');
	await expect(
		detail
			.getByRole('definition')
			.filter({ has: page.getByRole('button', { name: 'Edit assignees' }) })
	).toContainText('Shawn');

	await detail.getByLabel('Notes', { exact: true }).click();
	await page.keyboard.type('Use the grey paint from the shed.');
	await addTo(page, 'New subtask', 'Sand the boards');
	await addTo(page, 'New subtask', 'Buy brushes');
	await addTo(page, 'New checklist item', 'Primer');
	await addTo(page, 'New checklist item', 'Top coat');
	// Click the box itself, as a person would.
	const checklist = detail.getByRole('list', { name: 'Checklist' });
	await checklist.locator('.box').first().click();
	await expect(checklist.getByRole('checkbox').first()).toBeChecked();
	await detail.getByLabel('New comment').fill('Wait for a dry week.');
	await detail.getByRole('button', { name: 'Comment' }).click();
	await expect(detail.getByRole('list', { name: 'Comments' })).toContainText(
		'Wait for a dry week.'
	);
	await detail.locator('input[type=file]').setInputFiles({
		name: 'swatch.png',
		mimeType: 'image/png',
		buffer: Buffer.concat([Buffer.from('\x89PNG\r\n\x1a\n', 'binary'), Buffer.alloc(64)])
	});
	await expect(detail.getByRole('list', { name: 'Attachments' })).toContainText('swatch.png');
	await detail.getByLabel('Status', { exact: true }).selectOption({ label: 'In progress' });
	// However long the task, the panel scrolls, not the page.
	expect(
		await page.evaluate(() => document.documentElement.scrollHeight <= window.innerHeight + 1)
	).toBe(true);
	await close(page);

	// The list groups by status and sums the task up.
	const porch = row(page, 'Repaint the porch');
	await expect(porch).toHaveCount(1); // the move between groups has finished animating
	await expect(page.getByRole('list', { name: 'Tasks' })).toContainText('In progress');
	await expect(porch.getByRole('img', { name: 'High priority' })).toBeVisible();
	await expect(porch).toContainText('0/2'); // subtasks
	await expect(porch).toContainText('1/2'); // checklist
	await expect(porch).toContainText('Outdoors');

	// All of it survives a reload.
	await page.reload();
	await open(page, 'Repaint the porch');
	await expect(detail.getByLabel('Priority', { exact: true })).toHaveValue('high');
	await expect(detail.getByLabel('Due date')).toHaveValue('2026-11-14');
	await expect(detail.getByLabel('Notes', { exact: true })).toContainText(
		'grey paint from the shed'
	);
	await expect(detail.getByRole('list', { name: 'Subtasks' })).toContainText('Buy brushes');
	await expect(detail.getByRole('list', { name: 'Attachments' })).toContainText('swatch.png');

	// The history tells the story.
	await detail.getByRole('button', { name: 'Show history' }).click();
	const history = detail.getByRole('list', { name: 'History' });
	await expect(history).toContainText('created this task');
	await expect(history).toContainText('set the priority to high');
	await expect(history).toContainText('commented');
	await expect(history).toContainText('added the file swatch.png');
});

test('undo puts the last change back', async ({ page }) => {
	await newProject(page, `Undo ${Date.now()}`);
	await addTask(page, 'Pick a color');
	await open(page, 'Pick a color');
	await page.getByLabel('Priority', { exact: true }).selectOption('urgent');
	await close(page);
	await expect(row(page, 'Pick a color').getByRole('img', { name: 'Urgent' })).toBeVisible();
	await page.keyboard.press('Control+z');
	await expect(page.getByText('Undone')).toBeVisible();
	await expect(row(page, 'Pick a color').getByRole('img', { name: 'Urgent' })).toBeHidden();
});

test('deleted tasks wait in the trash and come back', async ({ page }) => {
	const title = `Lost ${Date.now()}`;
	const url = await newProject(page, `Trash ${Date.now()}`);
	await addTask(page, title);
	await open(page, title);
	await page.getByRole('button', { name: 'Delete task' }).click();
	await expect(row(page, title)).toBeHidden();

	await page.goto('/trash');
	const item = page
		.getByRole('list', { name: 'Deleted items' })
		.getByRole('listitem')
		.filter({ hasText: title });
	await item.getByRole('button', { name: 'Restore' }).click();
	await expect(item).toBeHidden();
	await page.goto(url);
	await expect(row(page, title)).toBeVisible();
});

test('statuses can be renamed and the list follows', async ({ page }) => {
	await newProject(page, `Flow ${Date.now()}`);
	await addTask(page, 'First');
	await open(page, 'First');
	await page.getByLabel('Status', { exact: true }).selectOption({ label: 'In progress' });
	await close(page);
	await addTask(page, 'Second');
	await page.getByRole('button', { name: 'Edit statuses' }).click();
	const editor = page.getByRole('list', { name: 'Statuses' });
	await editor.getByLabel('Status name').nth(1).fill('Next up');
	await editor.getByLabel('Status name').nth(1).press('Enter');
	await page.keyboard.press('Escape');
	await expect(page.getByRole('list', { name: 'Tasks' })).toContainText('Next up');
});

test('a comment reaches the other device live', async ({ page, browser }) => {
	const url = await newProject(page, `Talk ${Date.now()}`);
	await addTask(page, 'Plan the trip');
	const other = await browser.newContext({ storageState: OWNER });
	const laptop = await other.newPage();
	await laptop.goto(url);
	await open(laptop, 'Plan the trip');

	await open(page, 'Plan the trip');
	await page.getByLabel('New comment').fill('Booked the train.');
	await page.getByRole('button', { name: 'Comment' }).click();
	await expect(laptop.getByRole('list', { name: 'Comments' })).toContainText('Booked the train.');
	await other.close();
});
