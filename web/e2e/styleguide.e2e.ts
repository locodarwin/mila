import { expect, test } from '@playwright/test';
import { axe, axeViolations } from './axe';

const characters = ['standard', 'calm', 'playful', 'contrast'];
const schemes = ['light', 'dark'];

// Every theme must pass WCAG 2.2 AA checks, contrast included, on the page
// that shows every token and component.
for (const character of characters) {
	for (const scheme of schemes) {
		test(`style guide passes axe in ${character} ${scheme}`, async ({ page }) => {
			await page.addInitScript((a) => localStorage.setItem('mila.appearance', JSON.stringify(a)), {
				character,
				scheme
			});
			await page.goto('/dev/ui');
			await expect(page.getByRole('heading', { name: 'Style guide' })).toBeVisible();
			await expect(page.locator('html')).toHaveAttribute('data-theme', character);
			await expect(page.locator('html')).toHaveAttribute('data-scheme', scheme);
			// The saved views' sections load apart (ViewsDemo.svelte).
			await expect(page.getByRole('heading', { name: 'Board by priority' })).toBeVisible();
			// So do Your data, Admin, the security log and sharing (AccountDemo.svelte).
			await expect(page.getByRole('heading', { name: 'Security log', exact: true })).toBeVisible();
			// And the colors and themes (ColorDemo.svelte, ThemesDemo.svelte).
			await expect(page.getByRole('heading', { name: 'Menus, panels and toasts' })).toBeVisible();
			await expect(page.getByRole('heading', { name: 'ThemePreview' })).toBeVisible();
			// The diagrams draw once Mermaid has loaded (DiagramsDemo.svelte).
			await expect(page.locator('.diagram .drawing > svg')).toHaveCount(11, { timeout: 20_000 });
			// A page of samples, not a page of the app: its structure is not checked.
			await axe(page, { structure: false });
		});
	}
}

test("trying a theme here leaves the user's preferences alone", async ({ page }) => {
	await page.goto('/dev/ui');
	const header = page.locator('.guide > header');
	await header.getByText('Playful', { exact: true }).click();
	await header.getByText('Dense', { exact: true }).click();
	await expect(page.locator('html')).toHaveAttribute('data-theme', 'playful');
	await expect(page.locator('html')).toHaveAttribute('data-density', 'dense');
	// Nothing waits to be saved to the account, and the next page paints as before.
	expect(
		await page.evaluate(
			() => JSON.parse(localStorage.getItem('mila.preferences') ?? '{}').unsaved ?? []
		)
	).toEqual([]);
	await page.reload();
	await expect(page.locator('html')).toHaveAttribute('data-theme', 'standard');
	await expect(page.locator('html')).toHaveAttribute('data-density', 'comfortable');
});

test('a custom accent keeps text on it readable', async ({ page }) => {
	await page.goto('/dev/ui');
	for (const hue of ['60', '100', '200']) {
		await page.getByRole('slider', { name: 'Accent hue' }).fill(hue);
		expect(await axeViolations(page, { rules: ['color-contrast'] }), `hue ${hue}`).toEqual([]);
	}
});

test('completing a task holds, collapses and can be undone', async ({ page }) => {
	await page.goto('/dev/ui');
	const row = page.locator('.list .title', { hasText: 'Renew the domain' });
	await page.getByRole('checkbox', { name: 'Renew the domain' }).check({ force: true });
	await expect(row).toBeVisible();
	await expect(row).toBeHidden({ timeout: 3000 });
	await page.getByRole('button', { name: 'Undo' }).first().click();
	await expect(row).toBeVisible();
});
