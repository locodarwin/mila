import { expect, test, type Page } from '@playwright/test';
import { apiCall, newMember, OWNER, SIGNED_OUT } from './helpers';

test.use({ storageState: OWNER });

/** A workspace of its own with a project and one task; the task's page. */
async function seed(page: Page) {
	await page.goto('/');
	const ws: string = (await apiCall(page, 'POST', '/workspaces', { name: `Notes ${Date.now()}` }))
		.body.id;
	const make = async (entity: string, body: unknown) => {
		const res = await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, body);
		expect(res.status, JSON.stringify(res.body)).toBe(201);
		return res.body.id as string;
	};
	const project = await make('projects', { name: 'Garden' });
	const status = await make('statuses', { project_id: project, name: 'To do', category: 'todo' });
	const task = await make('tasks', { project_id: project, status_id: status, title: 'Beds' });
	return { ws, task, url: `/projects/${project}?ws=${ws}&task=${task}` };
}

const notes = (page: Page) =>
	page.getByRole('complementary', { name: 'Task details' }).getByLabel('Notes', { exact: true });

async function typeAtEnd(page: Page, text: string) {
	await notes(page).click();
	await page.keyboard.press('Control+End');
	await page.keyboard.type(text);
}

test.describe('live notes', () => {
	test.skip(({ isMobile }) => isMobile, 'two windows side by side are a desktop thing');

	test('merge what two windows type, even offline', async ({ page, browser }) => {
		const { ws, task, url } = await seed(page);
		await page.goto(url);
		const other = await browser.newContext({ storageState: OWNER });
		const phone = await other.newPage();
		await phone.goto(url);
		await expect(notes(phone)).toBeVisible();

		// What one window types shows in the other.
		await typeAtEnd(page, 'Buy cedar boards.');
		await expect(notes(phone)).toContainText('Buy cedar boards.');

		// Both type while one is offline: both edits stay.
		await other.setOffline(true);
		await typeAtEnd(phone, ' Phone, offline.');
		await notes(page).click();
		await page.keyboard.press('Control+Home');
		await page.keyboard.type('Laptop: ');
		await expect(notes(page)).toContainText('Laptop: Buy cedar boards.');
		await other.setOffline(false);
		for (const p of [page, phone])
			await expect(notes(p)).toContainText('Laptop: Buy cedar boards. Phone, offline.');

		// The server renders the notes as Markdown for everything else.
		await expect
			.poll(async () => (await apiCall(page, 'GET', `/workspaces/${ws}/tasks/${task}`)).body.notes)
			.toBe('Laptop: Buy cedar boards. Phone, offline.');

		// A write through the API reaches the open editors.
		const put = await apiCall(page, 'PATCH', `/workspaces/${ws}/tasks/${task}`, {
			notes: 'Rewritten **by the API**.'
		});
		expect(put.status).toBe(200);
		for (const p of [page, phone]) await expect(notes(p)).toContainText('Rewritten by the API.');
		await expect(notes(page).locator('strong')).toHaveText('by the API');

		// And all of it is still there after a reload.
		await phone.reload();
		await expect(notes(phone)).toContainText('Rewritten by the API.');
		await other.close();
	});

	test('keep one history entry for an editing session', async ({ page }) => {
		const { url } = await seed(page);
		await page.goto(url);
		await typeAtEnd(page, 'First thought.');
		await page.waitForTimeout(800);
		await typeAtEnd(page, ' Second thought.');
		await page.waitForTimeout(800);
		const detail = page.getByRole('complementary', { name: 'Task details' });
		await detail.getByRole('button', { name: 'Show history' }).click();
		const history = detail.getByRole('list', { name: 'History' });
		await expect(history).toContainText('created this task');
		await expect(history.getByRole('listitem').filter({ hasText: 'the notes' })).toHaveCount(1);
	});

	test("undo in the notes takes back typing, not the app's last edit", async ({ page }) => {
		const { url } = await seed(page);
		await page.goto(url);
		await typeAtEnd(page, 'Keep this.');
		await page.waitForTimeout(800);
		await page.keyboard.type(' Not this.');
		await page.keyboard.press('Control+z');
		await expect(notes(page)).toHaveText('Keep this.');
	});
});

test('show who else has the notes open, and where they type', async ({
	page,
	browser,
	isMobile
}) => {
	test.skip(isMobile, 'two people side by side are a desktop thing');
	const {
		context,
		page: pia,
		workspace
	} = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Pia');
	const make = async (entity: string, body: unknown) =>
		(await apiCall(page, 'POST', `/workspaces/${workspace.id}/${entity}`, body)).body.id as string;
	const project = await make('projects', { name: 'Shared' });
	const status = await make('statuses', { project_id: project, name: 'To do', category: 'todo' });
	const task = await make('tasks', { project_id: project, status_id: status, title: 'Plan' });
	const url = `/projects/${project}?ws=${workspace.id}&task=${task}`;
	const me = (await apiCall(page, 'GET', '/me')).body.name as string;

	await page.goto(url);
	await pia.goto(url);
	const panel = (p: Page) => p.getByRole('complementary', { name: 'Task details' });
	await expect(panel(pia).getByText(`${me} is here`)).toBeVisible();
	await expect(panel(page).getByText('Pia is here')).toBeVisible();

	// Pia's cursor shows in the notes with her name.
	await typeAtEnd(pia, 'Pia was here.');
	await expect(notes(page)).toContainText('Pia was here.');
	await expect(panel(page).locator('.peer-name')).toHaveText('Pia');

	// When she closes the task, she is gone.
	await pia.goto(`/projects/${project}?ws=${workspace.id}`);
	await expect(panel(page).getByText('Pia is here')).toHaveCount(0);
	await expect(panel(page).locator('.peer-name')).toHaveCount(0);
	await context.close();
});
