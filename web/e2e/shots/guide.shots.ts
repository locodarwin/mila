import { expect, test, type Page } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { PROJECT, STATE, takeAll, TASK, type Seed, type Shot } from './shots';

// The user guide's pictures (docs/guide/images), in light and dark: the whole
// window, sidebar and all, since the guide is read on a wide page. Unlike help's,
// these may show keys: the guide names the keys as they come.

const OUT = join(import.meta.dirname, '../../../docs/guide/images');

test.use({ storageState: STATE });

const body = (page: Page) => page.locator('body');
/** Goes to an address and waits for its page to be drawn. */
const at = async (page: Page, url: string) => {
	await page.goto(url);
	await expect(page.getByRole('heading', { level: 1 }).first()).toBeAttached();
	await page.waitForLoadState('networkidle');
};
const whole = (url: (s: Seed) => string) => async (page: Page, s: Seed) => {
	await at(page, url(s));
	return body(page);
};
const project = (s: Seed, query: string, name = PROJECT) =>
	`/projects/${s.projects[name]}?${query}`;
/** A row of a task list, by its task's title. */
const row = (page: Page, title: string) => page.getByRole('button', { name: title, exact: true });

const SHOTS: Shot[] = [
	{
		name: 'app',
		take: whole((s) => project(s, `tab=tasks&task=${s.tasks[TASK]}`))
	},
	{ name: 'my-day', take: whole(() => '/today') },
	{ name: 'upcoming', take: whole(() => '/upcoming') },
	{ name: 'list', take: whole((s) => project(s, 'tab=tasks')) },
	{ name: 'board', take: whole((s) => project(s, 'tab=tasks&view=board')) },
	{ name: 'calendar', take: whole((s) => project(s, 'tab=tasks&view=calendar')) },
	{ name: 'table', take: whole((s) => project(s, 'tab=tasks&view=table')) },
	{ name: 'timeline', take: whole((s) => project(s, 'tab=tasks&view=timeline')) },
	{
		name: 'quick-add',
		take: async (page, s) => {
			await at(page, project(s, 'tab=tasks'));
			await page.getByRole('button', { name: /^New task/ }).click();
			const box = page.getByRole('dialog', { name: 'Add a task' });
			await box.getByLabel('Quick add').fill('Order more 1 kg bags friday #orders !high');
			return body(page);
		}
	},
	{ name: 'saved-view', take: whole((s) => `/views/${s.views['Pipeline']}`) },
	{ name: 'wiki', take: whole((s) => `/wiki?page=${s.pages['Roast profiles']}`) },
	{
		name: 'comments',
		take: async (page, s) => {
			await at(page, `/?task=${s.tasks['Northgate: contract and credit check']}`);
			const details = page.getByRole('complementary', { name: 'Task details' });
			const comments = details.getByRole('heading', { name: /^Comments/ }).first();
			await comments.scrollIntoViewIfNeeded();
			return body(page);
		}
	},
	{
		name: 'time',
		take: async (page) => {
			await at(page, '/time');
			// Weeks show the chart by week above the table.
			await page
				.getByRole('combobox', { name: 'Period', exact: true })
				.selectOption({ label: 'Last 8 weeks' });
			return body(page);
		}
	},
	{ name: 'project-time', take: whole((s) => project(s, 'tab=time')) },
	{ name: 'reports', take: whole(() => '/reports?tab=status') },
	{ name: 'workload', take: whole(() => '/reports?tab=workload') },
	{ name: 'templates', take: whole((s) => `/settings/w/${s.ws}/templates`) },
	{ name: 'themes', take: whole(() => '/settings/appearance') },
	{ name: 'people', take: whole((s) => `/settings/w/${s.ws}/people`) },
	{ name: 'import-apps', take: whole((s) => `/settings/w/${s.ws}/data/apps`) },
	{
		name: 'palette',
		take: async (page, s) => {
			await at(page, project(s, 'tab=tasks'));
			await page.keyboard.press('Control+k');
			const palette = page.getByRole('dialog').first();
			await expect(palette).toBeVisible();
			await page.keyboard.type('roast');
			await page.waitForTimeout(300);
			return body(page);
		}
	},
	{
		name: 'context-menu',
		take: async (page, s) => {
			await at(page, project(s, 'tab=tasks'));
			await row(page, 'Decide on the second shift').click({ button: 'right' });
			await expect(page.getByRole('menu').first()).toBeVisible();
			return body(page);
		}
	},
	{
		name: 'selecting',
		take: async (page, s) => {
			await at(page, project(s, 'tab=tasks'));
			for (const title of [
				'Decide on the second shift',
				"Clean the roaster's chaff collector",
				'Food safety audit'
			])
				await row(page, title).click({ modifiers: ['ControlOrMeta'] });
			await expect(page.getByRole('toolbar', { name: /selected$/ })).toBeVisible();
			return body(page);
		}
	},
	{
		name: 'notifications',
		take: async (page, s) => {
			await at(page, project(s, 'tab=tasks'));
			await page
				.getByRole('button', { name: /^Notifications/ })
				.first()
				.click();
			return body(page);
		}
	},
	{
		name: 'phone',
		viewport: { width: 390, height: 780 },
		take: whole(() => '/upcoming')
	}
];

test('the pictures in the guide', async ({ page }) => {
	mkdirSync(OUT, { recursive: true });
	await takeAll(page, SHOTS, OUT, { viewport: { width: 1280, height: 800 }, quality: 0.85 });
});
