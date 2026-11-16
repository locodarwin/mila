// Changing keyboard shortcuts in Settings, Keyboard: the recorder, the
// conflicts it finds, and the changed keys working everywhere a key is
// shown or pressed. Shortcuts are a preference, so each test signs up
// someone new. Desktop only: shortcuts need a keyboard.
import { expect, test, type Browser, type Page } from '@playwright/test';
import { apiCall, OWNER, saved, SIGNED_OUT, newPerson } from './helpers';

test.use({ storageState: OWNER });
test.skip(({ isMobile }) => isMobile, 'shortcuts need a keyboard');

async function person(browser: Browser, name: string) {
	return newPerson(() => browser.newContext(SIGNED_OUT), name);
}

const quickAdd = (page: Page) => page.getByRole('dialog', { name: 'Add a task' });
const row = (page: Page, title: string) =>
	page.locator('#main [data-task-id]').filter({ hasText: title });

async function addTask(page: Page, title: string) {
	await page.getByLabel('New task').fill(title);
	await page.getByLabel('New task').press('Enter');
	await expect(row(page, title)).toBeVisible();
}

/** Records keys for a command, from the button of the key it replaces or from Add a key. */
async function record(page: Page, button: string, ...keys: string[]) {
	await page.getByRole('button', { name: button }).click();
	await expect(page.getByLabel(/^Press the keys for /)).toBeFocused();
	for (const k of keys) await page.keyboard.press(k);
}

test('a changed key works, and every hint shows it', async ({ browser }) => {
	const { page: own } = await person(browser, 'Juniper');
	await own.goto('/settings/keyboard');
	// A plain key for a command that works anywhere waits for Enter, or a second key.
	await record(own, 'Q, shortcut for Add a task, change', 'n');
	// The live region says the same words 50 ms later, so name the first.
	await expect(own.getByText('Press Enter to use N alone').first()).toBeVisible();
	await own.keyboard.press('Enter');
	await saved(own, 'shortcuts', { 'quick-add': ['n'] });
	await expect(
		own.getByRole('button', { name: 'N, shortcut for Add a task, change' })
	).toBeFocused();

	await own.goto('/');
	await expect(own.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	await own.locator('body').press('q');
	await expect(quickAdd(own)).toBeHidden();
	await own.locator('body').press('n');
	await expect(quickAdd(own)).toBeVisible();
	await own.keyboard.press('Escape');
	await expect(quickAdd(own)).toBeHidden();

	const newTask = own.getByRole('button', { name: 'New task' });
	await expect(newTask).toContainText('N');
	await expect(newTask).toHaveAttribute('aria-keyshortcuts', 'N');

	// From the page itself, not wherever focus went back to after quick add.
	await own.locator('#main').focus();
	await own.keyboard.press('?');
	const sheet = own.getByRole('dialog', { name: 'Keyboard shortcuts' });
	await expect(sheet).toBeVisible();
	await expect(sheet.getByRole('row', { name: /^Add a task/ })).toContainText('N');
	await expect(sheet.getByRole('row', { name: /^Add a task/ })).toContainText('Changed');
	await own.keyboard.press('Escape');

	await own.keyboard.press('Control+k');
	await expect(own.getByRole('option', { name: /^Add a task/ })).toContainText('N');
	await own.keyboard.press('Escape');

	// The reference in Help, with the default beside it.
	await own.goto('/help/shortcuts');
	const reference = own.getByRole('row', { name: /^Add a task/ });
	await expect(reference).toContainText('Changed');
	await expect(reference.locator('td kbd').first()).toHaveText('N');
	await expect(reference.locator('.default')).toContainText('default:');
	await expect(reference.locator('.default kbd')).toHaveText('Q');
});

test('a key another command has can be swapped', async ({ browser }) => {
	const { page: own } = await person(browser, 'Linden');
	await own.goto('/settings/keyboard');
	await record(own, 'X, shortcut for Complete or reopen, change', 't');
	// The live region says the same words 50 ms later, so name the element.
	await expect(own.locator('#key-problem')).toHaveText(
		'T already does “Add to My Day, or take out” on a task in a list. Swap to give “Add to My Day, or take out” X instead.'
	);
	await expect(own.getByRole('button', { name: 'Swap' })).toBeFocused();
	await own.keyboard.press('Enter');
	await saved(own, 'shortcuts', { 'task.complete': ['t'], 'task.my-day': ['x'] });

	// A command's own fixed key is one it has already: nothing is added,
	// and the app keeps working.
	await record(own, 'Add a key for Next task', 'ArrowDown');
	await expect(own.getByLabel(/^Press the keys for /)).toBeHidden();
	await expect(own.locator('[data-row="task.next"] [data-key]')).toHaveCount(1);

	await own.goto('/');
	await addTask(own, 'Swapped once');
	await addTask(own, 'Swapped twice');
	await row(own, 'Swapped twice').focus();
	await own.keyboard.press('x');
	await expect(own.getByRole('status').filter({ hasText: 'Added to My Day' })).toBeVisible();
	await row(own, 'Swapped once').focus();
	await own.keyboard.press('t');
	await expect(
		own.getByRole('status').filter({ hasText: 'Completed "Swapped once"' })
	).toBeVisible();

	// Resetting Complete takes X back from My Day, which is reset too.
	await own.goto('/settings/keyboard');
	await own.getByRole('button', { name: 'Reset Complete or reopen' }).click();
	await expect(
		own.getByText("Complete or reopen's default keys belong to “Add to My Day, or take out” now.", {
			exact: false
		})
	).toBeVisible();
	await expect(own.getByRole('button', { name: 'Reset both' })).toBeFocused();
	await own.keyboard.press('Enter');
	await saved(own, 'shortcuts', {});
	await expect(
		own.getByRole('button', { name: 'X, shortcut for Complete or reopen, change' })
	).toBeVisible();
});

test('reserved keys are refused, and the recorder keeps keys to itself', async ({ browser }) => {
	const { page: own } = await person(browser, 'Rowan');
	await own.goto('/');
	await addTask(own, 'Left open');
	await row(own, 'Left open').getByRole('button', { name: 'Left open', exact: true }).click();
	const panel = own.getByRole('complementary', { name: 'Task details' });
	await expect(panel).toBeVisible();
	// The same task stays open on another page.
	const there = new URL(own.url());
	there.pathname = '/settings/keyboard';
	await own.goto(there.href);
	await expect(panel).toBeVisible();

	await record(own, 'Q, shortcut for Add a task, change', 'Control+l');
	await expect(own.getByText("Ctrl+L is kept for the browser; Mila can't use it.")).toBeVisible();
	await own.getByRole('button', { name: 'Try another key' }).click();
	// Escape ends the recording, not the open task.
	await own.keyboard.press('Escape');
	await expect(own.getByLabel(/^Press the keys for /)).toBeHidden();
	await expect(panel).toBeVisible();
	await expect(
		own.getByRole('button', { name: 'Q, shortcut for Add a task, change' })
	).toBeFocused();

	// Ctrl+K is a key to record, not the palette.
	await record(own, 'Q, shortcut for Add a task, change', 'Control+k');
	await expect(own.getByRole('dialog', { name: 'Command palette' })).toBeHidden();
	await expect(own.getByText('Ctrl+K already does “Command palette” anywhere.')).toBeVisible();
	await own.getByRole('button', { name: 'Cancel' }).click();
	expect((await apiCall(own, 'GET', '/me')).body.preferences.shortcuts).toEqual({});
});

test('keys with a modifier work with single keys off, and plain ones are marked off', async ({
	browser
}) => {
	const { page: own } = await person(browser, 'Hazel');
	await own.goto('/settings/keyboard');
	await record(own, 'Q, shortcut for Add a task, change', 'Alt+n');
	await saved(own, 'shortcuts', { 'quick-add': ['Alt+n'] });
	await own.getByRole('checkbox', { name: 'Single-key shortcuts' }).uncheck({ force: true });
	await saved(own, 'singleKeyShortcuts', false);

	await record(own, 'Add a key for Add a task', 'n', 'Enter');
	await saved(own, 'shortcuts', { 'quick-add': ['Alt+n', 'n'] });
	await expect(own.getByText('This works once single-key shortcuts are on')).toBeVisible();
	await expect(own.locator('[data-row="quick-add"]')).toContainText(
		'off while single-key shortcuts are off'
	);

	await own.goto('/');
	await expect(own.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	await own.locator('body').press('n');
	await expect(quickAdd(own)).toBeHidden();
	await own.locator('body').press('Alt+n');
	await expect(quickAdd(own)).toBeVisible();
});

test('a sequence can be recorded and used', async ({ browser }) => {
	const { page: own } = await person(browser, 'Alder');
	await own.goto('/settings/keyboard');
	await record(own, 'G then I, shortcut for Go to Inbox, change', 'g', 'n');
	await saved(own, 'shortcuts', { 'go.inbox': ['g n'] });
	await own.goto('/today');
	await expect(own.getByRole('heading', { name: 'My Day' })).toBeVisible();
	await own.locator('body').press('g');
	await own.locator('body').press('i');
	await expect(own).toHaveURL(/\/today$/);
	await own.locator('body').press('g');
	await own.locator('body').press('n');
	await expect(own.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
});

test('reset all brings the defaults back, and undo takes them away again', async ({ browser }) => {
	const { page: own } = await person(browser, 'Willow');
	await own.goto('/settings/keyboard');
	await record(own, 'Q, shortcut for Add a task, change', 'n', 'Enter');
	await saved(own, 'shortcuts', { 'quick-add': ['n'] });
	await own.getByRole('button', { name: 'Reset all shortcuts' }).click();
	await saved(own, 'shortcuts', {});
	await expect(
		own.getByRole('button', { name: 'Q, shortcut for Add a task, change' })
	).toBeVisible();
	await own
		.locator('.toast')
		.filter({ hasText: 'Reset every shortcut' })
		.getByRole('button', { name: 'Undo' })
		.click();
	await saved(own, 'shortcuts', { 'quick-add': ['n'] });
	await expect(
		own.getByRole('button', { name: 'N, shortcut for Add a task, change' })
	).toBeVisible();
});

test('changed keys work offline, from what the device kept', async ({ browser }) => {
	const { page: own, context } = await person(browser, 'Cedar');
	await own.goto('/settings/keyboard');
	await record(own, 'Q, shortcut for Add a task, change', 'n', 'Enter');
	await saved(own, 'shortcuts', { 'quick-add': ['n'] });
	await own.goto('/');
	await own.evaluate(() => navigator.serviceWorker.ready);
	await own.reload(); // let the service worker take control
	await expect(own.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();

	await context.setOffline(true);
	await own.reload();
	await expect(own.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	await own.locator('body').press('n');
	await expect(quickAdd(own)).toBeVisible();
	await context.setOffline(false);
});
