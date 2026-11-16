// A keyboard-only walk through the app: the skip link, focus order and
// visible focus, one tab stop per list and per board, focus into and back
// out of the task panel, and Escape closing only the innermost thing.
// Desktop only; phones have their own focus test in phone.e2e.ts.
import { expect, test, type Locator, type Page } from '@playwright/test';
import { OWNER, SIGNED_OUT, newPerson } from './helpers';

test.use({ storageState: OWNER });
test.skip(({ isMobile }) => isMobile, 'a keyboard walk is a desktop thing');

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
	page.locator('#main [data-task-id]').filter({ hasText: title });
const detail = (page: Page) => page.getByRole('complementary', { name: 'Task details' });
const unique = (word: string) => `${word} ${Date.now().toString(36)}`;

/**
 * What the focused element looks like: whether something marks it (an
 * outline, a ring or an inset bar, on it or on the box it stands for),
 * whether it is on screen, and whether anything covers its middle.
 */
async function focusState(page: Page) {
	return page.evaluate(() => {
		const el = document.activeElement as HTMLElement | null;
		if (!el || el === document.body) return null;
		// A hidden checkbox draws its focus on the box beside it.
		const shown =
			el.matches('input.visually-hidden') && el.nextElementSibling
				? (el.nextElementSibling as HTMLElement)
				: el;
		const marked = (e: Element) => {
			// A resize edge lights up a line of its own.
			const after = getComputedStyle(e, '::after');
			if (after.content !== 'none' && after.backgroundColor !== 'rgba(0, 0, 0, 0)') return true;
			const s = getComputedStyle(e);
			const outline =
				s.outlineStyle !== 'none' &&
				parseFloat(s.outlineWidth) > 0 &&
				s.outlineColor !== 'rgba(0, 0, 0, 0)';
			return outline || s.boxShadow !== 'none';
		};
		const r = shown.getBoundingClientRect();
		const x = r.left + r.width / 2;
		const y = r.top + Math.min(r.height / 2, 10);
		const top = document.elementFromPoint(x, y);
		return {
			name: `${el.tagName.toLowerCase()} ${el.getAttribute('aria-label') ?? el.textContent?.trim().slice(0, 40)}`,
			marked: marked(shown) || marked(el),
			onScreen: r.bottom > 0 && r.top < innerHeight && r.right > 0 && r.left < innerWidth,
			covered: !!top && !shown.contains(top) && !top.contains(shown) && top !== el
		};
	});
}

/** Presses Tab some times, failing on any stop that is unmarked, off screen or covered. */
async function walk(page: Page, presses: number, key = 'Tab') {
	const problems: string[] = [];
	for (let i = 0; i < presses; i++) {
		await page.keyboard.press(key);
		const s = await focusState(page);
		if (!s) continue;
		if (!s.marked) problems.push(`no focus indicator: ${s.name}`);
		if (!s.onScreen) problems.push(`off screen: ${s.name}`);
		if (s.covered) problems.push(`covered: ${s.name}`);
	}
	return problems;
}

async function settled(page: Page) {
	await page.waitForFunction(() =>
		document.getAnimations().every((a) => a.playState !== 'running')
	);
}

test('the skip link goes past the sidebar to the page', async ({ page }) => {
	await page.goto('/');
	await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	await page.keyboard.press('Tab');
	const skip = page.getByRole('link', { name: 'Skip to content' });
	await expect(skip).toBeFocused();
	await expect(skip).toBeInViewport();
	await page.keyboard.press('Enter');
	await expect(page.locator('#main')).toBeFocused();
	// The page's first stop: its header's controls, then the list.
	await page.keyboard.press('Tab');
	await expect(
		page.getByRole('group', { name: 'Show as' }).getByRole('button', { name: 'List' })
	).toBeFocused();
});

test('every stop on the main screens shows where focus is', async ({ page }) => {
	const url = await newProject(page, unique('Walk'));
	for (const t of ['Walk one', 'Walk two', 'Walk three']) await addTask(page, t);
	for (const path of ['/', '/today', '/upcoming', url, url + '?view=board', '/settings']) {
		await page.goto(path);
		await expect(page.getByRole('heading', { level: 1 }).first()).toBeAttached();
		await settled(page);
		expect(await walk(page, 30), path).toEqual([]);
	}
});

test('every stop shows where focus is on a table, a timeline, the boards by day, the Time page and wide Settings', async ({
	page
}) => {
	const url = await newProject(page, unique('Walk more'));
	for (const t of ['Walk one', 'Walk two']) await addTask(page, t);
	for (const path of [url + '?view=table', url + '?view=timeline', '/time']) {
		await page.goto(path);
		await expect(page.getByRole('heading', { level: 1 }).first()).toBeAttached();
		await settled(page);
		expect(await walk(page, 30), path).toEqual([]);
	}
	// Upcoming by day, a board with its resize edge.
	await page.goto('/upcoming');
	await page.getByRole('group', { name: 'Show as' }).getByRole('button', { name: 'Board' }).click();
	await expect(page.getByRole('group', { name: 'Board' })).toBeVisible();
	await settled(page);
	expect(await walk(page, 30), 'upcoming board').toEqual([]);
	await page.getByRole('group', { name: 'Show as' }).getByRole('button', { name: 'List' }).click();
	// Settings two sections to a row.
	await page.setViewportSize({ width: 1920, height: 1080 });
	for (const path of ['/settings/preferences', '/settings/notifications']) {
		await page.goto(path);
		await expect(page.getByRole('heading', { level: 1 }).first()).toBeAttached();
		await settled(page);
		expect(await walk(page, 40), path).toEqual([]);
	}
});

test('a list is one tab stop, and its rows are reached with arrows', async ({ page }) => {
	await newProject(page, unique('Stops'));
	for (const t of ['Row one', 'Row two', 'Row three', 'Row four']) await addTask(page, t);
	await page.getByLabel('New task').focus();
	await page.keyboard.press('Tab');
	await expect(row(page, 'Row one')).toBeFocused();
	// Then the row's own controls, then out of the list.
	const stops: string[] = [];
	for (let i = 0; i < 5; i++) {
		await page.keyboard.press('Tab');
		stops.push(
			await page.evaluate(
				() =>
					document.activeElement?.closest('[data-task-id]')?.querySelector('.title')?.textContent ??
					'out'
			)
		);
	}
	expect(stops).toEqual(['Row one', 'Row one', 'Row one', 'Row one', 'out']);
	// Arrows move between rows, and the list remembers the last one.
	await row(page, 'Row one').focus();
	await page.keyboard.press('ArrowDown');
	await page.keyboard.press('ArrowDown');
	await expect(row(page, 'Row three')).toBeFocused();
	await page.getByLabel('New task').focus();
	await page.keyboard.press('Tab');
	await expect(row(page, 'Row three')).toBeFocused();
});

test('a task opens with focus inside and closes back to its row', async ({ page }) => {
	await newProject(page, unique('Panel'));
	await addTask(page, 'Open me');
	await addTask(page, 'Neighbor');
	await row(page, 'Open me').focus();
	await page.keyboard.press('Enter');
	await expect(detail(page)).toBeVisible();
	await expect(detail(page)).toBeFocused();
	await page.keyboard.press('Tab');
	await expect(detail(page).locator(':focus')).toHaveCount(1);
	// Escape from inside the panel closes it and puts focus back on the row.
	await page.keyboard.press('Escape');
	await expect(detail(page)).toBeHidden();
	await expect(row(page, 'Open me')).toBeFocused();
	// So does the Close button.
	await page.keyboard.press('Enter');
	await detail(page).getByRole('button', { name: 'Close' }).focus();
	await page.keyboard.press('Enter');
	await expect(detail(page)).toBeHidden();
	await expect(row(page, 'Open me')).toBeFocused();
	// Deleting it from the panel leaves focus on the list rather than nowhere.
	await page.keyboard.press('Enter');
	await detail(page).getByRole('button', { name: 'Delete task' }).click();
	await expect(detail(page)).toBeHidden();
	await expect(row(page, 'Neighbor')).toBeFocused();
});

test('fields in the panel are never hidden under its header', async ({ page }) => {
	await newProject(page, unique('Sticky'));
	await addTask(page, 'Long task');
	await row(page, 'Long task').getByRole('button', { name: 'Long task', exact: true }).click();
	for (const item of ['One', 'Two', 'Three', 'Four', 'Five', 'Six']) {
		await detail(page).getByLabel('New checklist item').fill(item);
		await detail(page).getByLabel('New checklist item').press('Enter');
	}
	await page.setViewportSize({ width: 1280, height: 520 });
	await detail(page).getByLabel('New comment').focus();
	await settled(page);
	expect(await walk(page, 25, 'Shift+Tab')).toEqual([]);
});

test('Escape closes only the innermost thing', async ({ page }) => {
	await newProject(page, unique('Escape'));
	await addTask(page, 'Stays open');
	await row(page, 'Stays open').getByRole('button', { name: 'Stays open', exact: true }).click();
	await expect(detail(page)).toBeVisible();
	const title = detail(page).getByLabel('Task title');
	await title.focus();

	await page.keyboard.press('Control+k');
	const palette = page.getByRole('dialog', { name: 'Command palette' });
	await expect(palette).toBeVisible();
	await page.keyboard.press('Escape');
	await expect(palette).toBeHidden();
	await expect(detail(page)).toBeVisible();
	// The dialog gives focus back to where it was.
	await expect(title).toBeFocused();

	await page.locator('#main').focus();
	await page.keyboard.press('q');
	await expect(page.getByRole('dialog', { name: 'Add a task' })).toBeVisible();
	await page.keyboard.press('Escape');
	await expect(page.getByRole('dialog', { name: 'Add a task' })).toBeHidden();
	await expect(detail(page)).toBeVisible();

	// A menu in the panel closes first, then the panel.
	await detail(page).getByRole('button', { name: 'Edit labels' }).click();
	await expect(page.locator('.menu:popover-open')).toBeVisible();
	await page.keyboard.press('Escape');
	await expect(page.locator('.menu:popover-open')).toHaveCount(0);
	await expect(detail(page)).toBeVisible();
	await page.keyboard.press('Escape');
	await expect(detail(page)).toBeHidden();
});

test('a field shown by a button takes focus, ready for typing', async ({ page, isMobile }) => {
	test.skip(isMobile, 'the sidebar is a desktop thing');
	// A new project's name (GitHub issue #1).
	await page.goto('/');
	await page.getByRole('button', { name: 'New project', exact: true }).click();
	await expect(page.getByLabel('New project name')).toBeFocused();
	const name = unique('Focused');
	await page.keyboard.type(name);
	await page.keyboard.press('Enter');
	await expect(page.getByLabel('Project name')).toHaveValue(name);

	// A new workspace's name.
	await page.getByRole('button', { name: /^Switch workspace/ }).click();
	await page.getByRole('button', { name: 'New workspace' }).click();
	await expect(page.getByLabel('New workspace name')).toBeFocused();
});

test('Escape in a field of the page leaves the open task alone', async ({ page }) => {
	const url = await newProject(page, unique('Fields'));
	await addTask(page, 'Card task');
	await page.goto(url + '?view=board');
	await page.locator('[data-card]').filter({ hasText: 'Card task' }).click();
	await expect(detail(page)).toBeVisible();
	await page.getByRole('button', { name: '+ Add a task' }).first().click();
	const input = page.getByLabel(/^New task in /).first();
	await expect(input).toBeFocused();
	await page.keyboard.press('Escape');
	await expect(input).toBeHidden();
	await expect(detail(page)).toBeVisible();
});

test('the board is one tab stop, walked with arrows, and moves are announced', async ({ page }) => {
	const url = await newProject(page, unique('Cards'));
	for (const t of ['Card A', 'Card B']) await addTask(page, t);
	await page.goto(url + '?view=board');
	const card = (t: string) => page.locator('[data-card]').filter({ hasText: t });
	await expect(card('Card A')).toBeVisible();
	await expect(card('Card A')).toHaveAttribute('tabindex', '0');
	await expect(card('Card B')).toHaveAttribute('tabindex', '-1');
	await card('Card A').focus();
	await page.keyboard.press('ArrowDown');
	await expect(card('Card B')).toBeFocused();
	// Its name is what the card shows; how to move it is its description.
	await expect(card('Card B')).toHaveAccessibleName(/^Card B/);
	await expect(card('Card B')).toHaveAccessibleDescription(/Alt and arrow keys/);
	await page.keyboard.press('Alt+ArrowRight');
	await expect(announcer(page)).toHaveText(/Moved Card B to In progress/);
	await expect(card('Card B')).toBeFocused();
	await page.keyboard.press('ArrowLeft');
	await expect(card('Card A')).toBeFocused();
});

test('moving a row with Alt and the arrows is announced', async ({ page }) => {
	await newProject(page, unique('Moves'));
	for (const t of ['Move one', 'Move two', 'Move three']) await addTask(page, t);
	await row(page, 'Move three').focus();
	await page.keyboard.press('Alt+ArrowUp');
	await expect(announcer(page)).toHaveText('Moved Move three to 2 of 3');
});

test('moving a calendar entry to another day is announced', async ({ page }) => {
	const url = await newProject(page, unique('Days'));
	await addTask(page, 'Chip task');
	await row(page, 'Chip task').getByRole('button', { name: 'Chip task', exact: true }).click();
	const d = new Date();
	const ymd = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
	await detail(page).getByLabel('Due date').fill(ymd);
	await page.keyboard.press('Escape');
	await expect(row(page, 'Chip task').getByRole('button', { name: /^Due / })).toBeVisible();
	await page.goto(url + '?view=calendar');
	const chip = page.locator('[data-chip]').filter({ hasText: 'Chip task' });
	await expect(page.locator('[aria-current="date"]').first()).toContainText(String(d.getDate()));
	await chip.focus();
	await page.keyboard.press('Alt+ArrowDown');
	await expect(announcer(page)).toHaveText(/^Moved Chip task to \w+day, /);
	await expect(chip).toBeFocused();
});

test('closing a task opened from the calendar goes back to its entry', async ({ page }) => {
	const url = await newProject(page, unique('Back'));
	await addTask(page, 'Entry task');
	await row(page, 'Entry task').getByRole('button', { name: 'Entry task', exact: true }).click();
	const d = new Date();
	const ymd = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
	await detail(page).getByLabel('Due date').fill(ymd);
	await page.keyboard.press('Escape');
	await expect(row(page, 'Entry task').getByRole('button', { name: /^Due / })).toBeVisible();
	await page.goto(url + '?view=calendar');
	const chip = page.locator('[data-chip]').filter({ hasText: 'Entry task' });
	await expect(chip).toBeVisible();
	await chip.focus();
	await page.keyboard.press('Enter');
	await expect(detail(page)).toBeFocused();
	await page.keyboard.press('Escape');
	await expect(detail(page)).toBeHidden();
	await expect(chip).toBeFocused();
});

test('the completed list is one tab stop too, and X reopens', async ({ page }) => {
	await newProject(page, unique('Done'));
	for (const t of ['Done one', 'Done two', 'Still open']) await addTask(page, t);
	for (const t of ['Done one', 'Done two']) {
		await row(page, t).focus();
		await page.keyboard.press('x');
		await expect(
			page.getByRole('list', { name: 'Tasks' }).locator('[data-task-id]').filter({ hasText: t })
		).toHaveCount(0);
	}
	await page.getByRole('button', { name: /Completed · 2/ }).click();
	const done = page.getByRole('list', { name: 'Completed tasks' }).locator('[data-task-id]');
	await expect(done).toHaveCount(2);
	await expect(done.nth(0)).toHaveAttribute('tabindex', '0');
	await expect(done.nth(1)).toHaveAttribute('tabindex', '-1');
	await done.nth(0).focus();
	await page.keyboard.press('ArrowDown');
	await expect(done.nth(1)).toBeFocused();
	const title = await done.nth(1).locator('.title').textContent();
	await page.keyboard.press('x');
	await expect(done).toHaveCount(1);
	await expect(
		page.getByRole('list', { name: 'Tasks' }).locator('[data-task-id]').filter({ hasText: title! })
	).toBeVisible();
});

test('a toast undone from the keyboard gives focus back and lets later toasts go', async ({
	page
}) => {
	await newProject(page, unique('Undo'));
	for (const t of ['Undo me', 'Stay here']) await addTask(page, t);
	await row(page, 'Undo me').focus();
	await page.keyboard.press('Delete');
	await expect(row(page, 'Stay here')).toBeFocused();
	const toast = page.locator('.toaster .item').filter({ hasText: 'Deleted "Undo me"' });
	await toast.getByRole('button', { name: 'Undo' }).focus();
	await page.keyboard.press('Enter');
	await expect(row(page, 'Undo me')).toBeVisible();
	await expect(toast).toBeHidden();
	// Back where the keyboard was before it went to the toast.
	await expect(row(page, 'Stay here')).toBeFocused();
	// The toasts are not left paused: the next one still goes on its own.
	await page.keyboard.press('t');
	const next = page.locator('.toaster .item').filter({ hasText: 'Added to My Day' });
	await expect(next).toBeVisible();
	await expect(next).toBeHidden({ timeout: 10_000 });
});

test('moving a project in the sidebar is announced', async ({ page }) => {
	await newProject(page, unique('Upper'));
	const name = unique('Lower');
	await newProject(page, name);
	const link = page.locator('[data-project-nav]').filter({ hasText: name });
	await link.focus();
	await page.keyboard.press('Alt+ArrowUp');
	await expect(announcer(page)).toHaveText(new RegExp(`^Moved ${name} to \\d+ of \\d+$`));
	await expect(link).toBeFocused();
});

test('the palette says how many results inside the dialog, where it can be heard', async ({
	page
}) => {
	await page.goto('/');
	await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	await page.keyboard.press('Control+k');
	const palette = page.getByRole('dialog', { name: 'Command palette' });
	await expect(palette).toBeVisible();
	const status = palette.getByRole('status');
	// Nothing said before opening is read again.
	await expect(status).toHaveText('');
	await page.keyboard.type('inb');
	await expect(status).toHaveText(/^\s*\d+ results?\s*$/);
	// A modal dialog makes the rest of the page inert, its live regions too.
	expect(await status.evaluate((el) => !!el.closest('dialog:modal'))).toBe(true);
	await page.keyboard.press('Escape');
	await expect(palette).toBeHidden();
});

const announcer = (page: Page): Locator => page.locator('#announcer');

test('? on a row shows its keys with focus on the heading, and Escape goes back to the row', async ({
	page
}) => {
	await newProject(page, unique('Sheet'));
	await addTask(page, 'Asked about');
	await row(page, 'Asked about').focus();
	await page.keyboard.press('?');
	const sheet = page.getByRole('dialog', { name: 'Keyboard shortcuts' });
	await expect(sheet.getByRole('heading', { name: 'Keyboard shortcuts' })).toBeFocused();
	await expect(sheet.getByRole('table', { name: 'Anywhere' })).toBeVisible();
	await expect(sheet.getByRole('table', { name: 'On a task in a list' })).toBeVisible();
	await expect(sheet.getByRole('table', { name: 'On a board card' })).toHaveCount(0);
	await page.keyboard.press('Escape');
	await expect(sheet).toBeHidden();
	await expect(row(page, 'Asked about')).toBeFocused();

	// All shortcuts closes it on the way to the full list.
	await page.keyboard.press('?');
	await sheet.getByRole('link', { name: 'All shortcuts' }).click();
	await expect(sheet).toBeHidden();
	await expect(page).toHaveURL(/\/help\/shortcuts$/);
	await expect(page.getByRole('table', { name: 'On a board card' })).toBeVisible();
});

// A dialog's close event comes a frame after it closes. A key that opens it
// again within that frame must not be undone by the late event.
test('a dialog opened again straight after Escape stays open', async ({ page }) => {
	await page.goto('/');
	await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	await page.locator('#main').focus();
	for (const [key, name] of [
		['?', 'Keyboard shortcuts'],
		['q', 'Add a task'],
		['Control+k', 'Command palette']
	]) {
		const dialog = page.getByRole('dialog', { name });
		await page.keyboard.press(key);
		await expect(dialog).toBeVisible();
		await page.keyboard.press('Escape');
		await page.keyboard.press(key);
		// Long enough for any late close event to arrive.
		await page.waitForTimeout(300);
		await expect(dialog).toBeVisible();
		await page.keyboard.press('Escape');
		await expect(dialog).toBeHidden();
		await page.locator('#main').focus();
	}
});

test('G then H goes to Help, and Ctrl+/ opens the shortcuts', async ({ page }) => {
	await page.goto('/');
	await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	await page.locator('body').press('g');
	await page.locator('body').press('h');
	await expect(page).toHaveURL(/\/help$/);
	await page.locator('#main').focus();
	await page.keyboard.press('Control+/');
	await expect(page.getByRole('dialog', { name: 'Keyboard shortcuts' })).toBeVisible();
	await page.keyboard.press('Escape');
	// Inside a field, the key belongs to the field.
	await page.getByLabel('Search help').focus();
	await page.keyboard.press('Control+/');
	await expect(page.getByRole('dialog', { name: 'Keyboard shortcuts' })).toBeHidden();
});

test('the theme editor works from the keyboard', async ({ browser }) => {
	// A theme is a person's own: someone new.
	const { page: own, context } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Kit');
	await own.emulateMedia({ colorScheme: 'light' });
	await own.goto('/settings/themes');
	await own.getByLabel('Start from').selectOption({ label: 'Calm' });
	await own.getByLabel('Start from').focus();
	await own.keyboard.press('Tab');
	await expect(own.getByRole('button', { name: 'New theme', exact: true })).toBeFocused();
	await own.keyboard.press('Enter');
	await expect(own.getByLabel('Theme name')).toBeVisible();

	// The parts of the theme are buttons in turn; Enter shows one.
	const nav = own.getByRole('navigation', { name: 'Parts of the theme' });
	await nav.getByRole('button', { name: /^Surfaces/ }).focus();
	await own.keyboard.press('Tab');
	await expect(nav.getByRole('button', { name: /^Text/ })).toBeFocused();
	await own.keyboard.press('Enter');
	await expect(nav.getByRole('button', { name: /^Text/ })).toHaveAttribute('aria-current', 'true');
	await expect(own.getByRole('heading', { name: 'Text', exact: true })).toBeVisible();

	// A color: its row opens in place with Enter, takes a hex, and Escape closes it, back on the row.
	const swatch = own.getByRole('button', { name: /^Muted text:/ });
	await swatch.focus();
	await own.keyboard.press('Enter');
	const hex = own.getByLabel('Hex', { exact: true });
	await expect(hex).toBeVisible();
	await hex.focus();
	await own.keyboard.press('Control+a');
	await own.keyboard.type('#d0d0d0');
	await own.keyboard.press('Enter');
	await own.keyboard.press('Escape');
	await expect(hex).toBeHidden();
	await expect(swatch).toBeFocused();
	// The closed row marks the shortfall.
	await expect(swatch).toContainText('contrast problem');

	// Undo and redo, from where focus is.
	await own.keyboard.press('Control+z');
	await expect(swatch).not.toContainText('contrast problem');
	await own.keyboard.press('Control+Shift+z');
	await expect(swatch).toContainText('contrast problem');

	// A search across the parts: the count is said, Tab goes on to what it
	// found, a row opens in place, and Escape in the field clears it.
	const find = own.getByLabel('Find a setting');
	await find.focus();
	await own.keyboard.type('heading 1');
	await expect(own.getByRole('heading', { name: 'Settings matching heading 1' })).toBeVisible();
	await expect(own.locator('#announcer')).toContainText(/\d+ settings?/);
	const found = own.getByRole('button', { name: /^Heading 1:/ });
	for (let i = 0; i < 30 && !(await found.evaluate((el) => el === document.activeElement)); i++)
		await own.keyboard.press('Tab');
	await expect(found).toBeFocused();
	await own.keyboard.press('Enter');
	await expect(found).toHaveAttribute('aria-expanded', 'true');
	await own.keyboard.press('Escape');
	await expect(found).toHaveAttribute('aria-expanded', 'false');
	await expect(found).toBeFocused();
	await find.focus();
	await own.keyboard.press('Escape');
	await expect(find).toHaveValue('');
	await expect(own.getByRole('heading', { name: 'Text', exact: true })).toBeVisible();

	// The problems, behind the contrast button, then the goal: each problem
	// takes focus to its token.
	await own.getByRole('button', { name: /problems? at AA/ }).focus();
	await own.keyboard.press('Enter');
	await expect(own.getByRole('heading', { name: 'Contrast', exact: true })).toBeFocused();
	await own.keyboard.press('Tab');
	await expect(own.getByLabel('Contrast goal')).toBeFocused();
	await own.keyboard.press('Tab');
	await own.keyboard.press('Enter');
	await expect
		.poll(() => own.evaluate(() => !!document.activeElement?.closest('[id^="token-"]')))
		.toBe(true);
	await context.close();
});
