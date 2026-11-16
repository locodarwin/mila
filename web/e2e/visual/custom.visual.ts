import { readFileSync } from 'node:fs';
import { expect, test } from '@playwright/test';

// The style guide wearing a custom theme (fixtures/sample.mila-theme.json,
// which changes something in every group of tokens), in light and dark:
// the sections that show the most of it. The theme and the preference
// that puts it in use are seeded as this device would keep them, so the
// app builds the theme's stylesheet as it does for anyone.

const SAMPLE = JSON.parse(
	readFileSync(new URL('./fixtures/sample.mila-theme.json', import.meta.url), 'utf8')
);
const SECTIONS = [
	'Color',
	'Type',
	'Controls',
	'Task rows',
	'Cards',
	'Board',
	'Phone',
	'Task panel'
];
const NOW = new Date('2026-11-03T15:00:00Z');

const slug = (s: string) =>
	s
		.toLowerCase()
		.replace(/[^a-z0-9]+/g, '-')
		.replace(/^-|-$/g, '');

for (const scheme of ['light', 'dark']) {
	test(`the style guide in a custom theme, ${scheme}`, async ({ page }) => {
		await page.clock.setFixedTime(NOW);
		await page.addInitScript(
			({ theme, scheme }) => {
				localStorage.setItem(
					'mila.themes',
					JSON.stringify({ themes: [theme], unsaved: [], deleted: [], notSaved: {}, user: null })
				);
				localStorage.setItem(
					'mila.preferences',
					JSON.stringify({
						preferences: { theme: theme.base, scheme, customTheme: theme.id },
						unsaved: [],
						user: null
					})
				);
			},
			{ theme: SAMPLE, scheme }
		);
		await page.goto('/dev/ui');
		await expect(page.getByRole('heading', { name: 'Style guide' })).toBeVisible();
		await expect(page.getByRole('heading', { name: 'Board by priority' })).toBeVisible();
		await expect(page.getByRole('heading', { name: 'Menus, panels and toasts' })).toBeVisible();
		await expect(page.locator('html')).toHaveAttribute('data-custom-theme', SAMPLE.id);
		await expect(page.locator('html')).toHaveAttribute('data-scheme', scheme);
		await page.evaluate(() => document.fonts.ready);
		for (const name of SECTIONS) {
			const section = page
				.locator('.guide > section')
				.filter({ has: page.getByRole('heading', { level: 2, name, exact: true }) });
			await expect(section).toHaveCount(1);
			await section.scrollIntoViewIfNeeded();
			await expect.soft(section).toHaveScreenshot(`custom-${scheme}-${slug(name)}.png`);
		}
	});
}
