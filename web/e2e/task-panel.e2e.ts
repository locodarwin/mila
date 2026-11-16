import { expect, test, type Browser, type Page } from '@playwright/test';
import { axe } from './axe';
import { OWNER, saved, SIGNED_OUT, newPerson } from './helpers';

// The task panel's two layouts (the taskPanel preference). Preferences belong
// to a person, so each test signs up someone new.
test.use({ storageState: OWNER });

async function person(browser: Browser, name: string) {
	const { page: own } = await newPerson(() => browser.newContext(SIGNED_OUT), name);
	return own;
}

/** A new task in the Inbox, open in the panel. */
async function openNewTask(page: Page, title: string) {
	await page.goto('/');
	const input = page.getByRole('textbox', { name: 'New task' });
	await input.fill(title);
	await input.press('Enter');
	await page.getByRole('button', { name: title, exact: true }).first().click();
	const panel = page.getByRole('complementary', { name: 'Task details' });
	await expect(panel.getByLabel('Task title')).toHaveValue(title);
	return panel;
}

async function chooseSummary(page: Page) {
	await page.goto('/settings/preferences');
	await page.getByRole('radio', { name: /Summary lines/ }).check();
	await saved(page, 'taskPanel', 'summary');
}

test('labeled boxes show everything, with bare question marks after the names', async ({
	browser
}) => {
	const own = await person(browser, 'Avery');
	const panel = await openNewTask(own, 'Plan the shelves');

	for (const name of ['Details', 'Schedule', 'Checklist', 'Dependencies', 'Time', 'Files']) {
		await expect(panel.getByRole('region', { name, exact: true })).toBeVisible();
	}
	await expect(panel.getByRole('heading', { name: 'Notes', exact: true })).toBeVisible();
	// No priority: the menu says so, with no mark beside it.
	await expect(panel.getByRole('img', { name: 'No priority' })).toHaveCount(0);

	// The question mark sits inside the name and has no words of its own.
	const repeat = panel.locator('dt', { hasText: 'Repeat' });
	await expect(repeat.getByRole('link', { name: 'How repeats work' })).toHaveText('');
	const reminders = panel.locator('dt', { hasText: 'Reminders' });
	await expect(reminders.getByRole('link', { name: 'How reminders work' })).toHaveText('');

	// Every section name is the same size, Blocked by and Blocking no bigger.
	const size = (name: string) =>
		panel.getByRole('heading', { name, exact: true }).evaluate((h) => getComputedStyle(h).fontSize);
	const notes = await size('Notes');
	expect(await size('Blocked by')).toBe(notes);
	expect(await size('Blocking')).toBe(notes);
	expect(await size('Time')).toBe(notes);

	await axe(own);

	// Scrolled, the panel's sticky header stays over the buttons beside Notes.
	const tools = panel.getByRole('button', { name: 'Open the notes wide' });
	const covered = await tools.evaluate((button) => {
		const scroller = button.closest('.detail') as HTMLElement;
		const header = scroller.querySelector('header') as HTMLElement;
		scroller.scrollTop += button.getBoundingClientRect().top - header.getBoundingClientRect().top;
		const { left, top, width, height } = button.getBoundingClientRect();
		return header.contains(document.elementFromPoint(left + width / 2, top + height / 2));
	});
	expect(covered).toBe(true);
});

test('the full-page task keeps its sections in a column that reads well', async ({ browser }) => {
	test.skip(test.info().project.name === 'phone', 'A phone has no choice of width.');
	const own = await person(browser, 'Emery');
	const panel = await openNewTask(own, 'Read the lease');
	await panel.getByRole('button', { name: 'Expand to the full page' }).click();
	await expect(panel.getByRole('button', { name: 'Show beside the list' })).toBeVisible();
	// The panel fills the page, and the section keeps to the column inside it.
	await expect.poll(async () => (await panel.boundingBox())?.width).toBeGreaterThan(800);
	const box = await panel.getByRole('region', { name: 'Schedule' }).boundingBox();
	expect(Math.round(box?.width ?? 0)).toBe(760);
});

test('summary lines open in place, and this device remembers them', async ({ browser }) => {
	const own = await person(browser, 'Blair');
	await chooseSummary(own);
	const panel = await openNewTask(own, 'Sort the receipts');

	// Schedule and Notes start open; the rest are one line each.
	await expect(panel.getByLabel('Due date')).toBeVisible();
	const checklist = panel.getByRole('button', { name: 'Checklist', exact: true });
	await expect(checklist).toHaveAttribute('aria-expanded', 'false');
	await expect(panel.getByLabel('New checklist item')).toHaveCount(0);
	await expect(panel.getByRole('region', { name: 'Checklist' })).toContainText('None yet');
	await expect(panel.getByRole('region', { name: 'Dependencies' })).toContainText('Not blocked');
	await expect(panel.getByRole('region', { name: 'Time' })).toContainText('No estimate');

	// Opening a line shows what it holds, and its summary follows the work.
	await checklist.click();
	await panel.getByLabel('New checklist item').fill('Scan them');
	await panel.getByLabel('New checklist item').press('Enter');
	await expect(panel.getByText('Scan them')).toBeVisible();
	await panel.getByRole('button', { name: 'Schedule', exact: true }).click();
	await expect(panel.getByLabel('Due date')).toHaveCount(0);
	await expect(panel.getByRole('region', { name: 'Schedule' })).toContainText('Nothing set');

	await axe(own);

	// The question mark is its own link, not the line's button.
	const help = panel
		.getByRole('region', { name: 'Dependencies' })
		.getByRole('link', { name: 'How dependencies work' });
	await help.click();
	await expect(own).toHaveURL(/\/help\/timeline-and-dependencies/);

	// A reload keeps what was opened and closed.
	await own.goBack();
	await own.reload();
	const again = own.getByRole('complementary', { name: 'Task details' });
	await expect(again.getByLabel('New checklist item')).toBeVisible();
	await expect(again.getByLabel('Due date')).toHaveCount(0);
});

test('commands that need a closed section open it', async ({ browser }) => {
	const own = await person(browser, 'Casey');
	await chooseSummary(own);
	const panel = await openNewTask(own, 'Count the stock');
	await expect(panel.getByRole('button', { name: 'Time', exact: true })).toHaveAttribute(
		'aria-expanded',
		'false'
	);

	await own.locator('body').press('Control+k');
	const palette = own.getByRole('dialog', { name: 'Command palette' });
	await palette.getByRole('combobox').fill('log time');
	await palette.getByRole('option', { name: /Log time on the task/ }).click();
	await expect(panel.getByRole('button', { name: 'Time', exact: true })).toHaveAttribute(
		'aria-expanded',
		'true'
	);
	await expect(own.getByRole('form', { name: 'Log time' }).getByLabel('Time spent')).toBeFocused();
});

test('the wide notes work in either layout', async ({ browser }) => {
	const own = await person(browser, 'Devon');
	await chooseSummary(own);
	const panel = await openNewTask(own, 'Draft the letter');
	await panel.getByRole('button', { name: 'Open the notes wide' }).click();
	await expect(panel.getByRole('button', { name: 'Checklist', exact: true })).toBeHidden();
	await expect(panel.getByLabel('Status')).toBeHidden();
	await panel.getByRole('button', { name: 'Show the notes with the rest' }).click();
	await expect(panel.getByRole('button', { name: 'Checklist', exact: true })).toBeVisible();
});
