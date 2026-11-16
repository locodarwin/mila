import { expect, test, type Page } from '@playwright/test';
import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { PROJECT, sizesJson, STATE, takeAll, TASK, type Seed, type Shot } from './shots';

// The pictures in the help topics, in light and dark. Each crops away the sidebar and
// anything showing a key, since a picture can't follow a person's own keys.

const OUT = join(import.meta.dirname, '../../static/help-images');
const SIZES = join(import.meta.dirname, '../../src/lib/help/shots.json');

test.use({ storageState: STATE });

const main = (page: Page) => page.locator('main');
const screen = (url: (s: Seed) => string) => async (page: Page, s: Seed) => {
	await page.goto(url(s));
	await expect(page.getByRole('heading', { level: 1 }).first()).toBeAttached();
	return main(page);
};
const project = (s: Seed, query: string) => `/projects/${s.projects[PROJECT]}?${query}`;
const panel = async (page: Page, s: Seed) => {
	await page.goto(project(s, `tab=tasks&task=${s.tasks[TASK]}`));
	const details = page.getByRole('complementary', { name: 'Task details' });
	await expect(details).toBeVisible();
	return details;
};

const WIDE = { width: 1180, height: 720 };
const SHOTS: Shot[] = [
	{
		name: 'quick-add',
		// Its last lines show keys.
		style: '.syntax, .foot { display: none !important; }',
		take: async (page) => {
			await page.goto('/');
			await page.getByRole('button', { name: /^New task/ }).click();
			const box = page.getByRole('dialog', { name: 'Add a task' });
			await box.getByLabel('Quick add').fill('Call the broker tomorrow 3pm #orders !high');
			await expect(box).toBeVisible();
			return box;
		}
	},
	{ name: 'list', height: 470, take: screen((s) => project(s, 'tab=tasks')) },
	{
		name: 'board',
		viewport: WIDE,
		height: 510,
		take: screen((s) => project(s, 'tab=tasks&view=board'))
	},
	{ name: 'calendar', take: screen((s) => project(s, 'tab=tasks&view=calendar')) },
	{
		name: 'table',
		viewport: WIDE,
		height: 570,
		take: screen((s) => project(s, 'tab=tasks&view=table'))
	},
	{ name: 'timeline', take: screen((s) => project(s, 'tab=tasks&view=timeline')) },
	{ name: 'upcoming', height: 640, take: screen(() => '/upcoming') },
	{ name: 'wiki', take: screen((s) => `/wiki?page=${s.pages['Roast profiles']}`) },
	{ name: 'task-boxes', height: 560, preferences: { taskPanel: 'boxes' }, take: panel },
	{ name: 'task-summary', height: 560, preferences: { taskPanel: 'summary' }, take: panel },
	{
		name: 'theme-picker',
		viewport: WIDE,
		height: 345,
		take: async (page) => {
			await page.goto('/settings/appearance');
			await expect(page.getByRole('heading', { name: 'Appearance', exact: true })).toBeVisible();
			// The page beside Settings' list, with its own margins.
			return page.locator('.content');
		}
	}
];

test('the pictures in help', async ({ page }) => {
	mkdirSync(OUT, { recursive: true });
	const sizes = await takeAll(page, SHOTS, OUT, { viewport: { width: 1000, height: 720 } });
	writeFileSync(SIZES, sizesJson(sizes));
});
