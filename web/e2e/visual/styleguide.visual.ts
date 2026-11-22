import { expect, test } from '@playwright/test';

// Every section of the style guide, in every theme, against the baselines
// in __screenshots__ (playwright.visual.config.ts says where they are made).
// The clock is fixed, so the dates the samples show never change, and
// motion is reduced and stopped.

const characters = ['standard', 'calm', 'playful', 'contrast'];
const schemes = ['light', 'dark'];
const NOW = new Date('2026-11-03T15:00:00Z');

const slug = (s: string) =>
	s
		.toLowerCase()
		.replace(/[^a-z0-9]+/g, '-')
		.replace(/^-|-$/g, '');

for (const character of characters) {
	for (const scheme of schemes) {
		test(`style guide in ${character} ${scheme}`, async ({ page }) => {
			// Every section of a long page, each a screenshot of its own.
			test.setTimeout(180_000);
			await page.clock.setFixedTime(NOW);
			await page.addInitScript((a) => localStorage.setItem('mila.appearance', JSON.stringify(a)), {
				character,
				scheme
			});
			await page.goto('/dev/ui');
			await expect(page.getByRole('heading', { name: 'Style guide' })).toBeVisible();
			// The saved views' sections load apart (ViewsDemo.svelte).
			await expect(page.getByRole('heading', { name: 'Board by priority' })).toBeVisible();
			// The project board, and the selection bar.
			await expect(page.locator('.board-demo [data-card]').first()).toBeVisible();
			await expect(page.getByRole('toolbar', { name: '2 tasks selected' }).first()).toBeVisible();
			// So do the API reference's (ApiDocsDemo.svelte).
			await expect(page.getByRole('heading', { name: 'OperationList' })).toBeVisible();
			// And Your data, Admin, the security log and sharing (AccountDemo.svelte).
			await expect(page.getByRole('heading', { name: 'Security log', exact: true })).toBeVisible();
			// Comments' mentions and links (MentionsDemo.svelte).
			await expect(page.getByLabel('Sample comment')).toBeVisible();
			// The registry's colors (ColorDemo.svelte) and the themes' parts (ThemesDemo.svelte).
			await expect(page.getByRole('heading', { name: 'Menus, panels and toasts' })).toBeVisible();
			await expect(page.getByRole('heading', { name: 'ThemePreview' })).toBeVisible();
			// The themes from Omarchy load apart, in each theme picker on the page,
			// and push what is under them down when they come.
			for (const picker of await page.locator('fieldset.picker').all())
				await expect(picker.getByText('From Omarchy', { exact: true })).toBeVisible();
			// The diagrams draw once Mermaid has loaded (DiagramsDemo.svelte).
			await expect(page.locator('.diagram .drawing > svg')).toHaveCount(11, { timeout: 20_000 });
			await expect(page.locator('html')).toHaveAttribute('data-theme', character);
			await expect(page.locator('html')).toHaveAttribute('data-scheme', scheme);
			await page.evaluate(() => document.fonts.ready);
			const sections = page.locator('.guide > section');
			const count = await sections.count();
			expect(count).toBeGreaterThan(20);
			for (let i = 0; i < count; i++) {
				const section = sections.nth(i);
				const name = slug((await section.locator('h2').first().textContent()) ?? `section-${i}`);
				await section.scrollIntoViewIfNeeded();
				await expect.soft(section).toHaveScreenshot(`${character}-${scheme}-${name}.png`);
			}
		});
	}
}
