import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, OWNER, SIGNED_OUT } from './helpers';

const op = (page: Page, id: string) => page.locator(`#op-${id}`);
const answer = (page: Page, id: string) =>
	op(page, id).getByRole('region', { name: /^Response to / });

/** Opens an operation from the list of endpoints, as a reader would. */
async function openOperation(page: Page, id: string, label: RegExp) {
	const nav = page.getByRole('navigation', { name: 'Endpoints' });
	const fold = nav.locator('summary');
	if (await fold.isVisible()) await fold.click();
	await nav.getByRole('link', { name: label }).click();
	await expect(op(page, id).getByRole('heading', { name: 'Try it' })).toBeVisible();
}

/** The workspace the app has open, and a project of the test's own in it. */
async function ownProject(page: Page, name: string) {
	await page.goto('/projects');
	await page.getByLabel('Name a new project').fill(name);
	await page.getByLabel('Name a new project').press('Enter');
	await expect(page.getByLabel('Project name')).toHaveValue(name);
	const project = new URL(page.url()).pathname.split('/').pop()!;
	const stored = await page.evaluate(() => localStorage.getItem('mila.workspace'));
	const ws: string = stored ?? (await apiCall(page, 'GET', '/workspaces')).body[0].id;
	// Once the app has sent it to the server.
	await expect
		.poll(async () => (await apiCall(page, 'GET', `/workspaces/${ws}/projects/${project}`)).status)
		.toBe(200);
	return { project, ws, url: page.url() };
}

test.describe('signed in', () => {
	test.use({ storageState: OWNER });

	test('lists every tag, tries requests and passes axe', async ({ page }) => {
		const { project, ws } = await ownProject(page, `Reference ${Date.now()}`);
		await page.goto('/api/docs');
		await expect(page.getByRole('heading', { level: 1, name: 'Mila API' })).toBeVisible();
		for (const tag of ['tasks', 'projects', 'custom-field-values', 'sync'])
			await expect(page.getByRole('heading', { level: 2, name: tag, exact: true })).toBeVisible();

		await openOperation(page, 'getMe', /^GET\s+\/me$/);
		await op(page, 'getMe').getByRole('button', { name: 'Send GET' }).click();
		await expect(answer(page, 'getMe')).toContainText('200');

		await openOperation(page, 'createTask', /Make a task$/);
		const form = op(page, 'createTask');
		await form.getByLabel('workspaceId path').selectOption(ws);
		await form
			.getByLabel('Body', { exact: true })
			.fill(JSON.stringify({ title: 'From the reference', project_id: project }));
		await form.getByRole('button', { name: 'Send POST' }).click();
		await expect(answer(page, 'createTask')).toContainText('201');
		await expect(form.getByLabel('Response body')).toContainText('"title": "From the reference"');
		await axe(page);
	});

	test('a REST write shows in an open project at once', async ({ page, baseURL }) => {
		const { project, ws, url } = await ownProject(page, `Scripted ${Date.now()}`);
		await page.goto(url);
		const res = await page.request.post(`/api/v1/workspaces/${ws}/tasks`, {
			data: { title: 'Written by a script', project_id: project },
			headers: { Origin: baseURL! }
		});
		expect(res.status()).toBe(201);
		await expect(
			page.locator('#main [data-task-id]').filter({ hasText: 'Written by a script' })
		).toBeVisible();
	});
});

test.describe('signed out', () => {
	test.use(SIGNED_OUT);

	test('the reference can be read, and trying asks to sign in', async ({ page }) => {
		await page.goto('/api/docs');
		await expect(page.getByRole('heading', { level: 1, name: 'Mila API' })).toBeVisible();
		await openOperation(page, 'listTasks', /List tasks$/);
		await expect(
			op(page, 'listTasks').getByRole('link', { name: 'Sign in', exact: true })
		).toBeVisible();
		// What needs no session can still be tried.
		await openOperation(page, 'getInstance', /^GET\s+\/instance$/);
		await op(page, 'getInstance').getByRole('button', { name: 'Send GET' }).click();
		await expect(answer(page, 'getInstance')).toContainText('200');
		await axe(page);
	});

	test('offline, the reference says it needs a connection', async ({ page, context }) => {
		await page.goto('/api/docs');
		await expect(page.getByRole('heading', { level: 1, name: 'Mila API' })).toBeVisible();
		await page.evaluate(() => navigator.serviceWorker.ready);
		await page.reload(); // let the service worker take control
		await context.setOffline(true);
		await page.goto('/api/docs');
		await expect(page.getByText(/needs a connection/)).toBeVisible();
		await context.setOffline(false);
	});
});
