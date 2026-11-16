import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, OWNER } from './helpers';

test.use({ storageState: OWNER });

/** A workspace of its own with a project, a task, and a page under the project's main page. */
async function seed(page: Page) {
	await page.goto('/');
	const ws: string = (await apiCall(page, 'POST', '/workspaces', { name: `Links ${Date.now()}` }))
		.body.id;
	const make = async (entity: string, body: unknown) => {
		const res = await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, body);
		expect(res.status, JSON.stringify(res.body)).toBe(201);
		return res.body.id as string;
	};
	const project = await make('projects', { name: 'House' });
	const status = await make('statuses', { project_id: project, name: 'To do', category: 'todo' });
	const task = await make('tasks', { project_id: project, status_id: status, title: 'Buy paint' });
	return { ws, project, task, wiki: `/projects/${project}?ws=${ws}&tab=wiki` };
}

const body = (page: Page) => page.getByLabel('Page', { exact: true });
const menu = (page: Page) => page.getByRole('listbox', { name: 'Link to a page or task' });
const pagesOf = async (page: Page, ws: string) =>
	(await apiCall(page, 'GET', `/workspaces/${ws}/pages`)).body.items as {
		id: string;
		title: string;
		body: string;
	}[];

test.describe('links', () => {
	test.skip(({ isMobile }) => isMobile, 'the phone has its own checks');

	test('[[ links pages and tasks, which keep their names and say where they link from', async ({
		page
	}) => {
		const { ws, project, task, wiki } = await seed(page);
		await page.goto(wiki);
		await body(page).click();

		// [[ makes a new subpage and links to it.
		await page.keyboard.type('Plan: [[Paint plan');
		await expect(menu(page).getByRole('option', { name: /New page “Paint plan”/ })).toBeVisible();
		await page.keyboard.press('ArrowUp');
		await expect(menu(page).getByRole('option', { selected: true })).toContainText('New page');
		await page.keyboard.press('Enter');
		await expect(body(page).getByRole('link', { name: 'Paint plan' })).toBeVisible();
		await expect(page.getByRole('treeitem', { name: 'Paint plan' })).toBeVisible();

		// [[ finds a task; its link shows the task's status.
		await page.keyboard.type('then [[buy');
		await expect(menu(page).getByRole('option').first()).toContainText('Buy paint');
		await page.keyboard.press('Enter');
		await expect(body(page).getByRole('link', { name: 'Buy paint' })).toBeVisible();
		await expect(body(page).locator('.link-note')).toHaveText('To do');

		await expect
			.poll(async () => (await pagesOf(page, ws)).some((p) => p.title === 'Paint plan'))
			.toBe(true);
		const plan = (await pagesOf(page, ws)).find((p) => p.title === 'Paint plan')!;
		const href = `/projects/${project}?tab=wiki&page=${plan.id}`;
		await expect
			.poll(async () => (await pagesOf(page, ws)).find((p) => !p.title)?.body)
			.toBe(`Plan: [Paint plan](${href}) then [Buy paint](/?task=${task}) `);

		// A task link opens the task beside the page, which lists the page.
		await body(page).getByRole('link', { name: 'Buy paint' }).click();
		const panel = page.getByRole('complementary', { name: 'Task details' });
		await expect(panel.getByRole('heading', { name: 'Linked from' })).toBeVisible();
		await expect(panel.getByRole('link', { name: 'House' })).toBeVisible();
		await page.keyboard.press('Escape');

		// A page link opens the page, which ends with what links to it.
		await body(page).getByRole('link', { name: 'Paint plan' }).click();
		await expect(page.getByLabel('Page title')).toHaveValue('Paint plan');
		await expect(page.getByRole('heading', { name: 'Linked from' })).toBeVisible();
		await axe(page);

		// Renamed, the page's links take its new name.
		await page.getByLabel('Page title').fill('Paint and colors');
		await page.getByLabel('Page title').press('Enter');
		await expect
			.poll(async () => (await pagesOf(page, ws)).find((p) => !p.title)?.body)
			.toBe(`Plan: [Paint and colors](${href}) then [Buy paint](/?task=${task}) `);
		await page.getByRole('treeitem', { name: 'House' }).click();
		await expect(body(page).getByRole('link', { name: 'Paint and colors' })).toBeVisible();
	});

	test("a page's history shows what each version changed, and restores one", async ({ page }) => {
		const { ws, wiki } = await seed(page);
		await page.goto(wiki);
		await body(page).click();
		await page.keyboard.type('Paint the hall white.');
		await expect.poll(async () => (await pagesOf(page, ws))[0]?.body).toBe('Paint the hall white.');
		// A body written whole is a version of its own.
		const main = (await pagesOf(page, ws))[0];
		const put = await apiCall(page, 'PATCH', `/workspaces/${ws}/pages/${main.id}`, {
			body: 'Paint the hall sage green.'
		});
		expect(put.status).toBe(200);
		await expect(body(page)).toContainText('sage green');

		await page.getByRole('button', { name: 'More for this page' }).click();
		await page.getByRole('button', { name: 'History' }).click();
		const history = page.getByRole('dialog', { name: 'History of House' });
		const versions = history.getByRole('list', { name: 'Versions' }).getByRole('button');
		await expect(versions).toHaveCount(2);
		await expect(history.locator('ins')).toContainText('sage green');
		await expect(history.locator('del')).toContainText('white');
		await expect(history.getByText('This is how the page reads now.')).toBeVisible();
		await axe(page);

		// The older version goes back; the page as it was stays in the history.
		await versions.nth(1).click();
		await history.getByRole('button', { name: 'Restore this version' }).click();
		await expect(history).toBeHidden();
		await expect(body(page)).toContainText('Paint the hall white.');
		await expect
			.poll(
				async () =>
					(await apiCall(page, 'GET', `/workspaces/${ws}/pages/${main.id}/versions`)).body.length
			)
			.toBe(3);
	});
});
