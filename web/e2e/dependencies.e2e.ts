import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { OWNER } from './helpers';

test.use({ storageState: OWNER });

// By its title button: a row's Blocked mark names other tasks.
const row = (page: Page, title: string) =>
	page
		.getByRole('list', { name: 'Tasks' })
		.getByRole('listitem')
		.filter({ has: page.getByRole('button', { name: title, exact: true }) });

async function addTask(page: Page, title: string) {
	await page.getByLabel('New task').fill(title);
	await page.getByLabel('New task').press('Enter');
	await expect(row(page, title)).toBeVisible();
}

// Closes the open task first: on a phone its sheet covers the list.
async function closeTask(page: Page) {
	const detail = page.getByRole('complementary', { name: 'Task details' });
	if (await detail.isVisible()) {
		await detail.getByRole('button', { name: 'Close' }).click();
		await expect(detail).toBeHidden();
	}
}

async function open(page: Page, title: string) {
	await closeTask(page);
	await row(page, title).getByRole('button', { name: title, exact: true }).click();
	await expect(page.getByRole('complementary', { name: 'Task details' })).toBeVisible();
	await expect(page.getByLabel('Task title')).toHaveValue(title);
}

test('a task waits for another, and says so until it is done', async ({ page }, info) => {
	// Tasks are searched across the workspace, which every test shares.
	const tag = `${info.project.name} ${Date.now() % 100000}`;
	const drywall = `Drywall ${tag}`;
	const paint = `Paint ${tag}`;
	await page.goto('/projects');
	await page.getByLabel('Name a new project').fill(`Kitchen ${Date.now()}`);
	await page.getByLabel('Name a new project').press('Enter');
	await addTask(page, drywall);
	await addTask(page, paint);
	const detail = page.getByRole('complementary', { name: 'Task details' });

	// Paint waits for the drywall, found by part of its title.
	await open(page, paint);
	await detail.getByRole('button', { name: 'Add a task it waits for' }).click();
	const search = detail.getByRole('combobox', { name: `Find a task ${paint} waits for` });
	await expect(search).toBeFocused();
	await search.fill(drywall);
	await expect(
		detail
			.getByRole('listbox', { name: 'Tasks' })
			.getByRole('option', { name: new RegExp(drywall) })
	).toHaveAttribute('aria-selected', 'true');
	await axe(page);
	await search.press('Enter');
	const blockedBy = detail.getByRole('region', { name: 'Blocked by' });
	await expect(blockedBy.getByRole('button', { name: drywall, exact: true })).toBeVisible();
	await expect(detail.getByRole('button', { name: 'Add a task it waits for' })).toBeFocused();
	await expect(row(page, paint).getByText(`Blocked by ${drywall}`)).toBeAttached();
	await expect(row(page, drywall).getByText(/Blocked by/)).toHaveCount(0);

	// Dates that clash are named.
	await detail.getByLabel('Due date').fill('2026-11-12');
	await open(page, drywall);
	await detail.getByLabel('Due date').fill('2026-11-13');
	await expect(
		detail
			.getByRole('region', { name: 'Blocking' })
			.getByRole('button', { name: paint, exact: true })
	).toBeVisible();

	// The drywall cannot wait for the paint: that would be a loop.
	await detail.getByRole('button', { name: 'Add a task it waits for' }).click();
	const again = detail.getByRole('combobox', { name: `Find a task ${drywall} waits for` });
	await expect(again).toBeFocused();
	await again.fill(paint);
	await expect(
		detail.getByRole('listbox', { name: 'Tasks' }).getByRole('option').first()
	).toContainText(paint);
	await again.press('Enter');
	await expect(
		page.getByText(`That would make a loop: ${paint} already waits for ${drywall}.`).first()
	).toBeVisible();
	await page.keyboard.press('Escape');
	await expect(detail).toBeVisible();

	await open(page, paint);
	await expect(detail.getByRole('note')).toContainText(
		new RegExp(`Due .*12.*before ${drywall} is due`)
	);

	// Done, the drywall no longer holds the paint up.
	// The box's label takes the click; its input is hidden.
	await closeTask(page);
	await row(page, drywall).locator('label').first().click();
	await expect(row(page, paint).getByText(/Blocked by/)).toHaveCount(0);
	await open(page, paint);
	await expect(detail.getByRole('note')).toHaveCount(0);

	// Stopping the wait undoes from its toast.
	await blockedBy.getByRole('button', { name: `Stop waiting for ${drywall}` }).click();
	await expect(blockedBy.getByText('Nothing. It can start any time.')).toBeVisible();
	await page.getByRole('button', { name: 'Undo' }).click();
	await expect(blockedBy.getByRole('button', { name: drywall, exact: true })).toBeVisible();
});
