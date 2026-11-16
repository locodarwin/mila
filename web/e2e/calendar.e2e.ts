import { expect, request, test, type Browser, type Page } from '@playwright/test';
import { axe } from './axe';
import { OWNER, SIGNED_OUT, newPerson } from './helpers';

test.use({ storageState: OWNER });

const unique = (word: string) => `${word}${Date.now().toString(36)}`;

async function newProject(page: Page, name: string) {
	await page.goto('/projects');
	await page.getByLabel('Name a new project').fill(name);
	await page.getByLabel('Name a new project').press('Enter');
	await expect(page.getByLabel('Project name')).toHaveValue(name);
	return page.url();
}

const row = (page: Page, title: string) =>
	page.getByRole('list', { name: 'Tasks' }).getByRole('listitem').filter({ hasText: title });

const detail = (page: Page) => page.getByRole('complementary', { name: 'Task details' });

function ymd(d: Date) {
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

/**
 * Someone signed in just now, in a workspace the owner shares with them:
 * making a feed needs a recent sign-in, which the owner's session, made
 * when the run began, may no longer be.
 */
async function freshMember(browser: Browser, clipboard = false) {
	return newPerson(async () => {
		const context = await browser.newContext(SIGNED_OUT);
		if (clipboard) await context.grantPermissions(['clipboard-read', 'clipboard-write']);
		return context;
	}, 'Cal');
}

/** Fetches a feed as a calendar app would: no cookies, no session. */
async function fetchFeed(link: string, etag?: string) {
	const app = await request.newContext();
	const res = await app.get(link, { headers: etag ? { 'If-None-Match': etag } : {} });
	const out = { status: res.status(), headers: res.headers(), body: await res.text() };
	await app.dispose();
	return out;
}

test('a project feed reaches a calendar app, and revoking it ends it', async ({ browser }) => {
	const { context, page } = await freshMember(browser);
	const project = unique('Porch');
	const url = await newProject(page, project);
	await page.getByLabel('New task').fill('Paint the porch');
	await page.getByLabel('New task').press('Enter');
	await row(page, 'Paint the porch')
		.getByRole('button', { name: 'Paint the porch', exact: true })
		.click();
	const due = new Date();
	due.setDate(due.getDate() + 3);
	await detail(page).getByLabel('Due date').fill(ymd(due));
	await detail(page).getByLabel('Due time').fill('14:30');

	// The project's page leads to the feed settings with the project chosen.
	await page.goto(url);
	await page.getByRole('link', { name: 'Subscribe in a calendar app' }).click();
	await expect(page).toHaveURL(/\/settings\/w\/[^/]+\/calendar\?project=/);
	const pick = page.getByLabel('Show in the feed');
	await expect(pick.locator('option:checked')).toHaveText(project);
	await page.getByRole('button', { name: 'Make a feed' }).click();

	const shown = page.getByRole('group', { name: `Your link for ${project}` });
	const field = shown.getByLabel('Calendar feed link');
	await expect(field).toHaveValue(/^http:\/\/localhost:4173\/ical\/[A-Za-z0-9_-]{43}\.ics$/);
	const link = await field.inputValue();
	await expect(shown.getByRole('link', { name: 'Open in a calendar app' })).toHaveAttribute(
		'href',
		link.replace(/^http:/, 'webcal:')
	);
	await axe(page);

	// A calendar app reads it once the task, and then its time (a change of
	// its own), has reached the server. 14:30 where the browser is, in UTC.
	const start = new Date(`${ymd(due)}T14:30:00`).toISOString().replace(/[-:]|\.\d+/g, '');
	await expect
		.poll(async () => (await fetchFeed(link)).body, { timeout: 15_000 })
		.toContain(`DTSTART:${start}\r\n`);
	const feed = await fetchFeed(link);
	expect(feed.status).toBe(200);
	expect(feed.headers['content-type']).toBe('text/calendar; charset=utf-8');
	expect(feed.headers['cache-control']).toBe('private, max-age=300');
	expect(feed.body).toMatch(/^BEGIN:VCALENDAR\r\n/);
	expect(feed.body).toContain(`X-WR-CALNAME:${project}\r\n`);
	expect(feed.body).toContain('SUMMARY:Paint the porch\r\n');
	expect(feed.body).toMatch(/URL:http:\/\/localhost:4173\/\?task=[0-9a-f-]+&ws=/);
	// Unchanged since the app last read it.
	expect((await fetchFeed(link, feed.headers['etag'])).status).toBe(304);

	// The list says it has been read; revoking takes a second click.
	await page.reload();
	const feeds = page.getByRole('list', { name: 'Calendar feeds' });
	const item = feeds.getByRole('listitem').filter({ hasText: project });
	await expect(item).toContainText('last read');
	await expect(page.getByLabel('Calendar feed link')).toBeHidden();
	await axe(page);
	await item.getByRole('button', { name: `Revoke ${project}` }).click();
	await item.getByRole('button', { name: `Confirm revoke ${project}` }).click();
	await expect(page.getByText(`Revoked the feed ${project}`)).toBeVisible();
	await expect(item).toBeHidden();
	expect((await fetchFeed(link)).status).toBe(404);
	await context.close();
});

test.describe('the keyboard', () => {
	test.skip(({ isMobile }) => isMobile, 'phones have no keyboard');

	test('a feed of my tasks is made and copied from the keyboard', async ({ browser }) => {
		const { context, page } = await freshMember(browser, true);
		await page.goto('/settings/calendar');
		const pick = page.getByLabel('Show in the feed');
		await pick.focus();
		await expect(pick).toHaveValue('me');
		await page.keyboard.press('Tab');
		await expect(page.getByRole('button', { name: 'Make a feed' })).toBeFocused();
		await page.keyboard.press('Enter');

		const shown = page.getByRole('group', { name: 'Your link for My tasks' });
		const field = shown.getByLabel('Calendar feed link');
		await expect(field).toHaveValue(/\/ical\/[A-Za-z0-9_-]{43}\.ics$/);
		const link = await field.inputValue();
		// Focus moves to the link, which says it is shown only once.
		await expect(field).toBeFocused();
		await expect(field).toHaveAccessibleDescription(/only time Mila shows it/);
		await page.keyboard.press('Tab');
		await expect(shown.getByRole('button', { name: 'Copy' })).toBeFocused();
		await page.keyboard.press('Enter');
		await expect(shown.getByRole('status')).toHaveText('Copied the link');
		expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(link);

		const feed = await fetchFeed(link);
		expect(feed.status).toBe(200);
		expect(feed.body).toContain('X-WR-CALNAME:My tasks\r\n');
		expect(feed.body).toMatch(/END:VCALENDAR\r\n$/);

		// Done puts the link away; it is never shown again, and focus goes
		// back to the picker.
		await page.keyboard.press('Tab');
		await expect(shown.getByRole('link', { name: 'Open in a calendar app' })).toBeFocused();
		await page.keyboard.press('Tab');
		await expect(shown.getByRole('button', { name: 'Done' })).toBeFocused();
		await page.keyboard.press('Enter');
		await expect(shown).toBeHidden();
		await expect(pick).toBeFocused();
		const item = page
			.getByRole('list', { name: 'Calendar feeds' })
			.getByRole('listitem')
			.filter({ hasText: 'My tasks' })
			.last();
		await expect(item).toContainText('Your tasks and My Day');
		// Revoking ends it too.
		await item.getByRole('button', { name: 'Revoke My tasks' }).focus();
		await page.keyboard.press('Enter');
		await page.keyboard.press('Enter');
		await expect(page.getByText('Revoked the feed My tasks')).toBeVisible();
		expect((await fetchFeed(link)).status).toBe(404);
		await context.close();
	});
});
