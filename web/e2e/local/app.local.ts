import { expect, test, type Page } from '@playwright/test';
import { axe } from '../axe';

const KEY = 'a-key-for-the-browser-tests-0123456789';

// One browser state for the file: the key works once, as it does for the app's window.
test.describe.configure({ mode: 'serial' });
let page: Page;

test.beforeAll(async ({ browser }) => {
	const context = await browser.newContext();
	page = await context.newPage();
	await page.goto(`/api/v1/auth/local?key=${KEY}`);
	await expect(page.getByRole('navigation', { name: 'Main' })).toBeVisible();
});

test('the key has signed the window in, and works only once', async ({ browser }) => {
	const context = await browser.newContext();
	const other = await context.newPage();
	const res = await other.goto(`/api/v1/auth/local?key=${KEY}`);
	expect(res?.status()).toBe(403);
	await other.goto('/');
	await expect(other).toHaveURL(/login/);
	await context.close();
});

test('there is no signing out, inviting or sharing', async () => {
	await page.goto('/');
	await expect(page.getByRole('navigation', { name: 'Main' })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Sign out' })).toHaveCount(0);
	await page.goto('/settings/workspace');
	await expect(page.getByRole('heading', { name: 'Invite people' })).toHaveCount(0);
});

test('Settings leaves out what needs other people or email', async () => {
	await page.goto('/settings');
	const tabs = page.getByRole('navigation', { name: 'Settings' });
	await expect(tabs.getByRole('link', { name: 'Preferences' })).toBeVisible();
	await expect(tabs.getByRole('link', { name: 'Import and export' })).toBeVisible();
	for (const name of [
		'Security',
		'Calendar feeds',
		'Email to task',
		'API tokens',
		'Webhooks',
		'Admin'
	]) {
		await expect(tabs.getByRole('link', { name })).toHaveCount(0);
	}
	await expect(page).toHaveURL(/\/settings\/profile$/);
	await expect(page.getByRole('heading', { name: 'Email address' })).toHaveCount(0);
	await expect(page.getByRole('heading', { name: 'Delete your account' })).toHaveCount(0);
	await page.goto('/settings/notifications');
	await expect(page.getByRole('heading', { name: 'What reaches you' })).toBeVisible();
	for (const name of ['Summaries by email', 'Email', 'Push']) {
		await expect(page.getByRole('heading', { name, exact: true })).toHaveCount(0);
	}
	await expect(page.getByRole('columnheader', { name: 'Email' })).toHaveCount(0);
	await page.goto('/settings/data');
	await expect(page.getByRole('heading', { name: 'Export' })).toBeVisible();
});

test('the rest is Mila as usual, and an export works without signing in again', async () => {
	await page.goto('/');
	await page.keyboard.press('q');
	const add = page.getByLabel('New task').first();
	await add.fill('Renew the passport');
	await add.press('Enter');
	await page.goto('/settings/data');
	await page
		.getByRole('button', { name: /export/i })
		.first()
		.click();
	await expect(page.getByText(/sign in again|recent sign-in/i)).toHaveCount(0);
});

test('its pages pass the accessibility checks', async () => {
	for (const [path, lands] of [
		['/settings/profile', /\/settings\/profile$/],
		['/settings/notifications', /\/settings\/notifications$/],
		// An address from before Settings was grouped goes on to the workspace's page.
		['/settings/data', /\/settings\/w\/[^/]+\/data$/]
	] as const) {
		await page.goto(path);
		await expect(page).toHaveURL(lands);
		await axe(page);
	}
});
