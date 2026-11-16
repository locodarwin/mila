import { expect, test, type Page } from '@playwright/test';
import { OWNER } from './helpers';

test.use({ storageState: OWNER });

/** Fails the test on any console error or CSP violation. */
function watchForErrors(page: Page) {
	const errors: string[] = [];
	page.on('console', (m) => {
		if (m.type() === 'error') errors.push(m.text());
	});
	page.on('pageerror', (e) => errors.push(e.message));
	return errors;
}

test('the shell loads under the strict CSP with no errors', async ({ page }) => {
	const errors = watchForErrors(page);
	const response = await page.goto('/');
	expect(response?.headers()['content-security-policy']).toContain("script-src 'self' 'sha256-");
	await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	expect(errors).toEqual([]);
});

test('navigation moves between the three views', async ({ page }) => {
	await page.goto('/');
	const nav = page.getByRole('navigation', { name: 'Main' }).filter({ visible: true });
	await nav.getByRole('link', { name: 'My Day' }).click();
	await expect(page).toHaveURL(/\/today$/);
	await expect(page.getByRole('heading', { name: 'My Day' })).toBeVisible();
	await nav.getByRole('link', { name: 'Upcoming' }).click();
	await expect(page.getByRole('heading', { name: 'Upcoming' })).toBeVisible();
	await expect(nav.getByRole('link', { name: 'Upcoming' })).toHaveAttribute('aria-current', 'page');
});

test('a deep link loads the app directly', async ({ page }) => {
	await page.goto('/upcoming');
	await expect(page.getByRole('heading', { name: 'Upcoming' })).toBeVisible();
});

test('the saved theme is applied before the app starts', async ({ page }) => {
	await page.addInitScript(() =>
		localStorage.setItem('mila.appearance', JSON.stringify({ character: 'calm', scheme: 'dark' }))
	);
	// Block the app's JavaScript: only the pre-paint script can set these.
	await page.route('**/_app/**', (route) => route.abort());
	await page.goto('/');
	const html = page.locator('html');
	await expect(html).toHaveAttribute('data-theme', 'calm');
	await expect(html).toHaveAttribute('data-scheme', 'dark');
});

test('the layout fits the screen', async ({ page, isMobile }) => {
	await page.goto('/');
	const tabs = page.locator('nav.tabs');
	const sidebar = page.locator('nav.sidebar');
	if (isMobile) {
		await expect(tabs).toBeVisible();
		await expect(sidebar).toBeHidden();
	} else {
		await expect(sidebar).toBeVisible();
		await expect(tabs).toBeHidden();
	}
	const overflow = await page.evaluate(
		() => document.documentElement.scrollWidth > document.documentElement.clientWidth
	);
	expect(overflow).toBe(false);
});
