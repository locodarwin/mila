import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, newMember, OWNER, SIGNED_OUT } from './helpers';

test.use({ storageState: OWNER });

async function seed(page: Page, width = 1600) {
	await page.setViewportSize({ width, height: 900 });
	await page.goto('/');
	const ws: string = (await apiCall(page, 'POST', '/workspaces', { name: `Notes ${Date.now()}` }))
		.body.id;
	await page.goto(`/wiki?ws=${ws}`);
	const body = page.getByLabel('Page', { exact: true });
	await body.click();
	await page.keyboard.type('We roast on Tuesdays and Fridays for wholesale.');
	await expect(body).toContainText('Fridays');
	return { ws, body };
}

/** Selects some words of the page by searching for them, as a drag would. */
async function select(page: Page, words: string) {
	await page.evaluate((w) => {
		const root = document.querySelector('.wiki .doc-body')!;
		const walk = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
		for (let n = walk.nextNode(); n; n = walk.nextNode()) {
			const at = (n.textContent ?? '').indexOf(w);
			if (at < 0) continue;
			const range = document.createRange();
			range.setStart(n, at);
			range.setEnd(n, at + w.length);
			const s = getSelection()!;
			s.removeAllRanges();
			s.addRange(range);
			return;
		}
		throw new Error(`no such words: ${w}`);
	}, words);
}

const mark = (page: Page) => page.locator('.wiki .doc-body .comment-mark');
const card = (page: Page, words: string) =>
	page.getByRole('article', { name: `Comments on: ${words}` });

async function comment(page: Page, words: string, text: string) {
	const button = page
		.getByRole('toolbar', { name: 'Formatting' })
		.getByRole('button', { name: 'Comment' });
	// Right after typing the editor may not have taken a selection made from outside it.
	await expect(async () => {
		await select(page, words);
		await expect(button).toBeVisible({ timeout: 1000 });
	}).toPass();
	await button.click();
	const box = page.getByLabel('Your comment on the selected words');
	await expect(box).toBeFocused();
	await box.fill(text);
	await page
		.getByRole('form', { name: 'New comment' })
		.getByRole('button', { name: 'Comment' })
		.click();
	await expect(card(page, words)).toContainText(text);
}

test.describe('comments pinned to text', () => {
	test.skip(({ isMobile }) => isMobile, 'phones have their own test below');

	test('a thread is pinned, follows the text, resolves and reopens', async ({ page }) => {
		const { body } = await seed(page);
		await comment(page, 'Tuesdays and Fridays', 'Thursdays over the holidays?');

		await expect(mark(page)).toHaveText('Tuesdays and Fridays');
		// The highlight is only drawn: the Markdown has none of it.
		await page.getByRole('button', { name: 'Markdown', exact: true }).click();
		await expect(page.getByLabel('Page as Markdown')).toHaveValue(
			'We roast on Tuesdays and Fridays for wholesale.'
		);
		await page.getByRole('button', { name: 'Markdown', exact: true }).click();

		// Typing before the words moves the highlight with them.
		await body.click();
		await page.keyboard.press('ControlOrMeta+Home');
		await page.keyboard.type('Note: ');
		await expect(body).toContainText('Note: We roast');
		// At once, not only once the edit has been sent.
		await expect(mark(page)).toHaveText('Tuesdays and Fridays', { timeout: 300 });

		// It is still there after a reload, from the device's copy.
		await page.reload();
		await expect(mark(page)).toHaveText('Tuesdays and Fridays');

		await mark(page).click();
		const thread = card(page, 'Tuesdays and Fridays');
		await expect(mark(page)).toHaveClass(/active/);
		const reply = thread.getByRole('textbox');
		await reply.fill('Yes, until New Year.');
		await thread.getByRole('button', { name: 'Reply' }).click();
		await expect(thread).toContainText('Yes, until New Year.');

		await thread.getByRole('button', { name: 'Resolve' }).click();
		await expect(mark(page)).toHaveCount(0);
		await expect(page.getByRole('heading', { name: 'Resolved' })).toBeVisible();
		await thread.getByRole('button', { name: 'Reopen' }).click();
		await expect(mark(page)).toHaveText('Tuesdays and Fridays');
	});

	test('deleted words leave the thread, saying so', async ({ page }) => {
		const { body } = await seed(page);
		await comment(page, 'for wholesale', 'Retail too?');
		await body.click();
		await page.keyboard.press('ControlOrMeta+a');
		await page.keyboard.press('Delete');
		await expect(mark(page)).toHaveCount(0, { timeout: 300 });
		await page.keyboard.type('Nothing about it now.');
		await expect(card(page, 'for wholesale')).toContainText('The text this was on was removed.');
	});

	test('with no room beside the page, comments open over its edge', async ({ page }) => {
		await seed(page, 1100);
		await page.getByRole('button', { name: /^Comments on this page/ }).click();
		await expect(page.getByRole('group', { name: 'Comments' })).toBeVisible();
		await page.getByRole('button', { name: 'Close comments' }).click();
		await expect(page.getByRole('group', { name: 'Comments' })).toHaveCount(0);
	});

	test('the panel passes the accessibility checks', async ({ page }) => {
		await seed(page);
		await comment(page, 'Tuesdays and Fridays', 'Thursdays?');
		await axe(page);
	});

	test('the person in the thread is told, and the link opens it', async ({ browser, page }) => {
		const other = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Kit');
		const ws = other.workspace.id;
		await page.goto(`/wiki?ws=${ws}`);
		const body = page.getByLabel('Page', { exact: true });
		await body.click();
		await page.keyboard.type('We roast on Tuesdays and Fridays for wholesale.');

		await other.page.goto(`/wiki?ws=${ws}`);
		await expect(other.page.getByLabel('Page', { exact: true })).toContainText('Fridays');
		await comment(other.page, 'Tuesdays and Fridays', 'Thursdays?');

		const thread = card(page, 'Tuesdays and Fridays');
		await page.getByRole('button', { name: /^Comments on this page/ }).click();
		await thread.click();
		await thread.getByRole('textbox').fill('Yes.');
		await thread.getByRole('button', { name: 'Reply' }).click();

		const kinds = async () =>
			(await apiCall(other.page, 'GET', '/me/notifications')).body.items as {
				kind: string;
				url: string;
			}[];
		await expect.poll(async () => (await kinds()).map((n) => n.kind)).toContain('page_comment');
		const told = (await kinds()).find((n) => n.kind === 'page_comment')!;
		expect(told.url).toContain('thread=');
		await other.page.goto(told.url);
		await expect(card(other.page, 'Tuesdays and Fridays').getByRole('textbox')).toBeVisible();
	});
});

test('on a phone, comments open in a sheet', async ({ page, isMobile }) => {
	test.skip(!isMobile, 'for phones');
	await page.goto('/');
	const ws: string = (await apiCall(page, 'POST', '/workspaces', { name: `Phone ${Date.now()}` }))
		.body.id;
	await page.goto(`/wiki?ws=${ws}`);
	await page.getByRole('button', { name: /^Comments on this page/ }).click();
	const sheet = page.getByRole('dialog', { name: 'Comments' });
	await expect(sheet).toContainText('Nothing to discuss yet');
	await sheet.getByRole('button', { name: 'Close' }).click();
	await expect(sheet).toHaveCount(0);
});

test('what was typed survives a reload straight after it', async ({ page, isMobile }) => {
	test.skip(isMobile, 'the same on a phone');
	const { body } = await seed(page);
	await body.click();
	await page.keyboard.press('ControlOrMeta+End');
	await page.keyboard.type(' Typed just now.');
	await page.reload();
	await expect(page.getByLabel('Page', { exact: true })).toContainText('Typed just now.');
});

test.describe('page templates', () => {
	test.skip(({ isMobile }) => isMobile, 'the wiki on a phone has its own test');

	test('a page is saved as a template, and fills an empty page', async ({ page }) => {
		const { ws, body } = await seed(page);
		await page.getByRole('button', { name: 'New page under Workspace Wiki' }).first().click();
		await page.getByLabel('Page title').fill('Meeting on {{today}}');
		await page.getByLabel('Page title').press('Enter');
		await page.keyboard.type('Held on {{today}}.');
		await expect(body).toContainText('Held on');

		await page.getByRole('button', { name: 'More for this page' }).click();
		await page.getByRole('button', { name: 'Save as a template' }).click();
		const dialog = page.getByRole('dialog', { name: /as a template/ });
		await expect(dialog).toContainText('Comes along');
		await dialog.getByLabel('Template name').fill('Meeting notes');
		await dialog.getByRole('button', { name: 'Save template' }).click();
		await expect(dialog).toBeHidden();

		// An empty page offers it; using it fills in the title, the text and the date.
		await page
			.getByRole('button', { name: /^New page under/ })
			.first()
			.click();
		const starter = page.getByRole('group', { name: 'Start from a template' });
		await starter.getByRole('button', { name: 'Meeting notes' }).click();
		await expect(body).toContainText('Held on');
		await expect(body).not.toContainText('{{today}}');
		await expect(page.getByLabel('Page title')).not.toHaveValue(/today/);
		await expect(page.getByLabel('Page title')).toHaveValue(/^Meeting on /);
		await expect(starter).toHaveCount(0);

		// A page with anything in it offers nothing, and the toast takes it back.
		await page.getByRole('button', { name: 'Undo' }).click();
		await expect(body).not.toContainText('Held on');
		await expect(starter).toBeVisible();

		// It is listed in Settings, Templates.
		await page.goto(`/settings/w/${ws}/templates`);
		await expect(page.getByRole('list', { name: 'Pages templates' })).toContainText(
			'Meeting notes'
		);
	});

	test('guests are not offered page templates', async ({ browser, page }) => {
		const guest = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Gale', 'guest');
		const ws = guest.workspace.id;
		const make = async (entity: string, b: unknown) =>
			(await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, b)).body.id as string;
		const project = await make('projects', { name: 'Shared' });
		const me = (await apiCall(guest.page, 'GET', '/me')).body.id;
		await apiCall(page, 'PUT', `/workspaces/${ws}/projects/${project}/guests/${me}`);
		await make('templates', {
			kind: 'page',
			name: 'Secret outline',
			data: { v: 1, tasks: [], pages: [{ id: 'a', parent: null, title: 'T', body: 'B' }] }
		});
		await guest.page.goto(`/projects/${project}?tab=wiki&ws=${ws}`);
		await expect(guest.page.getByLabel('Page', { exact: true })).toBeVisible();
		await expect(guest.page.getByRole('group', { name: 'Start from a template' })).toHaveCount(0);
		await guest.page.getByRole('button', { name: 'More for this page' }).click();
		await expect(guest.page.getByRole('button', { name: 'Save as a template' })).toHaveCount(0);
	});
});
