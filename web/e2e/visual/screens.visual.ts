import { expect, test, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { SEED, STATE, TODAY } from './screens';

// The app's main screens, in every theme, against the baselines in
// __screenshots__, from the workspace screens.setup.ts seeds. The theme is the
// one person's preference, so these run one at a time.

test.use({
	storageState: STATE,
	serviceWorkers: 'block',
	viewport: { width: 1500, height: 900 }
});
test.describe.configure({ mode: 'serial' });

const seed = () =>
	JSON.parse(readFileSync(SEED, 'utf8')) as { ws: string; project: string; task: string };

const SCREENS: Record<string, (s: ReturnType<typeof seed>) => string> = {
	inbox: () => '/',
	upcoming: () => '/upcoming',
	list: (s) => `/projects/${s.project}?tab=tasks`,
	board: (s) => `/projects/${s.project}?tab=tasks&view=board`,
	calendar: (s) => `/projects/${s.project}?tab=tasks&view=calendar`,
	table: (s) => `/projects/${s.project}?tab=tasks&view=table`,
	timeline: (s) => `/projects/${s.project}?tab=tasks&view=timeline`,
	task: (s) => `/projects/${s.project}?tab=tasks&task=${s.task}`,
	wiki: (s) => `/projects/${s.project}?tab=wiki`,
	reports: () => '/reports?tab=status'
};
const PHONE = ['inbox', 'board', 'task', 'wiki'];

async function wear(page: Page, theme: string, scheme: string) {
	await page.goto('/');
	const res = await page.evaluate(
		([theme, scheme]) =>
			fetch('/api/v1/me/settings/preferences', {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ theme, scheme })
			}).then((r) => r.status),
		[theme, scheme]
	);
	expect(res).toBeLessThan(300);
}

async function shoot(page: Page, url: string, file: string, theme: string, scheme: string) {
	await page.goto(url);
	await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
	await expect(page.locator('html')).toHaveAttribute('data-scheme', scheme);
	await expect(page.getByRole('heading', { level: 1 }).first()).toBeAttached();
	await page.waitForLoadState('networkidle');
	await page.evaluate(() => document.fonts.ready);
	await page.mouse.move(0, 0);
	// A whole screen is far bigger than a style guide section, so the config's
	// share of its pixels would let a small change through, a name cut short.
	await expect.soft(page).toHaveScreenshot(file, { maxDiffPixelRatio: 0.0001 });
}

for (const theme of ['standard', 'calm', 'playful', 'contrast']) {
	for (const scheme of ['light', 'dark']) {
		test(`main screens in ${theme} ${scheme}`, async ({ page }) => {
			test.setTimeout(180_000);
			await page.clock.setFixedTime(TODAY);
			await wear(page, theme, scheme);
			for (const [name, url] of Object.entries(SCREENS)) {
				await shoot(page, url(seed()), `screen-${theme}-${scheme}-${name}.png`, theme, scheme);
			}
		});
	}
}

for (const scheme of ['light', 'dark']) {
	test(`the collapsed sidebar in ${scheme}`, async ({ page }) => {
		await page.clock.setFixedTime(TODAY);
		await page.addInitScript(() => localStorage.setItem('mila.sidebar.collapsed', '1'));
		await wear(page, 'standard', scheme);
		await shoot(page, SCREENS.board!(seed()), `screen-collapsed-${scheme}.png`, 'standard', scheme);
	});

	test(`main screens on a phone in ${scheme}`, async ({ page }) => {
		test.setTimeout(120_000);
		await page.setViewportSize({ width: 412, height: 839 });
		await page.clock.setFixedTime(TODAY);
		await wear(page, 'standard', scheme);
		for (const name of PHONE) {
			const file = `screen-phone-${scheme}-${name}.png`;
			await shoot(page, SCREENS[name]!(seed()), file, 'standard', scheme);
		}
	});
}
