import { expect, test, type Browser, type Page } from '@playwright/test';
import { apiCall, newMember, OWNER, SIGNED_OUT } from './helpers';

test.use({ storageState: OWNER });

/** A guest given one project of a shared workspace, on their own page. */
async function guestOf(page: Page, browser: Browser) {
	const { page: guest, workspace } = await newMember(
		page,
		() => browser.newContext(SIGNED_OUT),
		'Gwen',
		'guest'
	);
	const ws = workspace.id;
	const make = async (entity: string, body: unknown) =>
		(await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, body)).body.id as string;
	const project = await make('projects', { name: 'Shared garden' });
	const task = await make('tasks', { project_id: project, title: 'Water the beds' });
	const me = (await apiCall(guest, 'GET', '/me')).body.id as string;
	const shared = await apiCall(page, 'PUT', `/workspaces/${ws}/projects/${project}/guests/${me}`);
	expect(shared.status).toBeLessThan(300);
	await guest.reload();
	return { guest, ws, project, task };
}

test('a guest is not offered what the server would refuse', async ({ page, browser }) => {
	const { guest, ws, project } = await guestOf(page, browser);

	await guest.goto(`/projects/${project}?ws=${ws}`);
	await expect(
		guest.getByRole('button', { name: 'Add a task' }).or(guest.getByLabel('New task')).first()
	).toBeVisible();
	await expect(guest.getByLabel('Project name')).not.toBeEditable();
	await expect(guest.getByLabel('Project description')).not.toBeEditable();
	await expect(guest.getByRole('button', { name: /^Archive / })).toHaveCount(0);
	await expect(guest.getByRole('button', { name: /^Delete Shared garden/ })).toHaveCount(0);
	await expect(guest.getByRole('button', { name: 'New project' })).toHaveCount(0);

	await guest.goto(`/projects?ws=${ws}`);
	await expect(guest.getByLabel('Name a new project')).toHaveCount(0);

	await guest.goto(`/settings?ws=${ws}`);
	const tabs = guest.getByRole('navigation', { name: 'Settings' });
	await expect(tabs.getByRole('link', { name: 'Profile' })).toBeVisible();
	await expect(tabs.getByRole('link', { name: 'General' })).toBeVisible();
	await expect(tabs.getByRole('link', { name: 'Labels and fields' })).toHaveCount(0);
	await expect(tabs.getByRole('link', { name: 'People' })).toHaveCount(0);
});

test('a guest has no inbox, and quick add asks for a project', async ({ page, browser }) => {
	const { guest, ws, task } = await guestOf(page, browser);

	// A link to a task, such as a notification's, still opens it.
	await guest.goto(`/?ws=${ws}&task=${task}`);
	await expect(guest).toHaveURL(new RegExp(`/projects\\?.*task=${task}`));
	await expect(guest.getByRole('complementary', { name: 'Task details' })).toBeVisible();

	// However they get there, a guest lands on Projects.
	await guest.goto(`/?ws=${ws}`);
	await expect(guest.getByRole('heading', { level: 1, name: 'Projects' })).toBeVisible();
	await expect(guest).toHaveURL(/\/projects/);
	await expect(guest.getByLabel('New task')).toHaveCount(0);
	await expect(guest.getByRole('link', { name: 'Inbox', exact: true })).toHaveCount(0);
	await guest.goto('/upcoming');
	await expect(guest.getByRole('heading', { name: 'Upcoming', exact: true })).toBeVisible();
	await guest.keyboard.press('g');
	await guest.keyboard.press('i');
	await expect(guest).toHaveURL(/\/projects/);

	await guest.keyboard.press('q');
	const add = guest.getByRole('dialog').getByRole('textbox').first();
	await add.fill('Nowhere to go');
	await expect(guest.getByText('Name a project with #, or open one first.')).toBeVisible();
	await guest.keyboard.press('Enter');
	await expect(add).toBeVisible();
	await expect(add).toHaveValue('Nowhere to go');

	// Named, it goes there.
	await add.fill('Somewhere to go #shared');
	await expect(guest.locator('.foot .hint')).not.toContainText('Name a project');
	await guest.keyboard.press('Enter');
	await expect(guest.getByRole('status').filter({ hasText: 'to Shared garden' })).toBeVisible();
});

test('an edit the server refuses is taken back, and says so', async ({ page, browser }) => {
	const { guest, ws } = await guestOf(page, browser);

	// The tab is gone, but the address still opens.
	await guest.goto(`/settings/w/${ws}/labels`);
	await guest.getByRole('textbox', { name: 'New label' }).fill('Not mine to make');
	await guest.keyboard.press('Enter');
	await expect(guest.getByRole('alert').filter({ hasText: 'could not be saved' })).toBeVisible();
	await expect(guest.getByLabel('Name of Not mine to make')).toHaveCount(0);
});

test('a guest reads real names in a task history', async ({ page, browser }) => {
	const { guest, ws, project, task } = await guestOf(page, browser);
	const owner = (await apiCall(page, 'GET', '/me')).body.name as string;

	await guest.goto(`/projects/${project}?ws=${ws}&task=${task}`);
	const panel = guest.getByRole('complementary', { name: 'Task details' });
	await panel.getByRole('button', { name: 'Show history' }).click();
	await expect(panel.locator('strong', { hasText: owner }).first()).toBeVisible();
	await expect(panel.getByText('Someone', { exact: true })).toHaveCount(0);
});
