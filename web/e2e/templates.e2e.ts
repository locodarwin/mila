import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, OWNER } from './helpers';

test.use({ storageState: OWNER });

const day = (n: number, from = new Date()) => {
	const d = new Date(from);
	d.setDate(d.getDate() + n);
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
};

/** A project made in the app, with dated tasks and a dependency made through the API. */
async function seed(page: Page, name: string) {
	await page.goto('/projects');
	await page.getByLabel('Name a new project').fill(name);
	await page.getByLabel('Name a new project').press('Enter');
	await expect(page.getByLabel('Project name')).toHaveValue(name);
	const project = new URL(page.url()).pathname.split('/').pop()!;
	const list = (await apiCall(page, 'GET', '/workspaces')).body;
	let W = '';
	let todo = '';
	await expect
		.poll(async () => {
			for (const w of Array.isArray(list) ? list : list.items) {
				const res = await apiCall(
					page,
					'GET',
					`/workspaces/${w.id}/statuses?project_id=${project}`
				);
				const items =
					res.status === 200 ? (Array.isArray(res.body) ? res.body : res.body.items) : [];
				const found = items.find((s: { category: string }) => s.category === 'todo');
				if (found) [W, todo] = [`/workspaces/${w.id}`, found.id];
			}
			return todo;
		})
		.not.toBe('');
	const task = async (title: string, fields: Record<string, unknown>) =>
		(
			await apiCall(page, 'POST', `${W}/tasks`, {
				title,
				project_id: project,
				status_id: todo,
				...fields
			})
		).body.id as string;
	const plan = await task('Plan', { start_date: day(3), due_date: day(5) });
	const build = await task('Build', { start_date: day(8), due_date: day(10) });
	await apiCall(page, 'POST', `${W}/task-dependencies`, { task_id: build, blocker_id: plan });
	await page.reload();
	await expect(page.getByRole('button', { name: 'Build', exact: true })).toBeVisible();
	return { W, project };
}

test('a project saved as a template makes another, its dates moved', async ({ page }, info) => {
	test.skip(info.project.name === 'phone', 'the same on a phone; its menus are checked below');
	const name = `Template source ${Date.now()}`;
	const tpl = `Template ${Date.now()}`;
	const { W } = await seed(page, name);

	await page.getByRole('button', { name: 'Save as a template' }).click();
	const save = page.getByRole('dialog', { name: `Save ${name} as a template` });
	await expect(save).toContainText('2 open tasks with their subtasks');
	await save.getByLabel('Template name').fill(tpl);
	await axe(page);
	await save.getByRole('button', { name: 'Save template' }).click();
	await expect(page.getByText(`Saved ${tpl} as a template`)).toBeVisible();

	await page.goto('/settings/templates');
	await page.getByRole('button', { name: new RegExp(tpl) }).click();
	const preview = page.getByRole('region', { name: `${tpl}, a project template` });
	await expect(preview.getByRole('list', { name: 'Tasks' })).toContainText('Day 1 to 3');
	await expect(preview).toContainText('waits for Plan');
	await axe(page);

	// Made to start in a week: Plan, first, lands on the start.
	await preview.getByRole('button', { name: 'New project from it' }).click();
	const make = page.getByRole('dialog', { name: 'New project from a template' });
	await make.getByRole('radio', { name: new RegExp(tpl) }).check();
	await make.getByLabel('Project name').fill(`${tpl} copy`);
	await make.getByLabel('Start date').fill(day(7));
	await expect(make).toContainText('2 dated tasks run from');
	await axe(page);
	await make.getByRole('button', { name: `Make ${tpl} copy` }).click();
	await expect(page).toHaveURL(/\/projects\/[0-9a-f-]{36}$/);
	await expect(page.getByLabel('Project name')).toHaveValue(`${tpl} copy`);
	const copy = new URL(page.url()).pathname.split('/').pop()!;
	await expect
		.poll(async () => {
			const res = await apiCall(page, 'GET', `${W}/tasks?project_id=${copy}`);
			const items = res.status === 200 ? (Array.isArray(res.body) ? res.body : res.body.items) : [];
			return items
				.map((t: { title: string; start_date: string }) => `${t.title} ${t.start_date}`)
				.sort();
		})
		.toEqual([`Build ${day(12)}`, `Plan ${day(7)}`]);
	await expect(
		page.getByRole('list', { name: 'Tasks' }).getByText('Blocked by Plan', { exact: false })
	).toBeAttached();

	// Deleting it sends it to the trash.
	await page.goto('/settings/templates');
	await page.getByRole('button', { name: new RegExp(tpl) }).click();
	await page.getByRole('button', { name: 'Delete', exact: true }).click();
	await expect(page.getByText(`Deleted the template ${tpl}`)).toBeVisible();
	// The trash is the server's list: it shows once the delete has reached it.
	await expect(async () => {
		await page.goto('/trash');
		// Long enough for the page to load: a reload sooner cancels it, for ever.
		await expect(page.getByRole('list', { name: 'Deleted items' })).toContainText(tpl);
	}).toPass();
});

test('a task saved as a template is made again from New task', async ({ page }, info) => {
	const name = `Task templates ${Date.now()}`;
	await seed(page, name);
	const tpl = `Weekly check ${Date.now()}`;
	await page.getByRole('button', { name: 'Plan', exact: true }).click();
	const detail = page.getByRole('complementary', { name: 'Task details' });
	await detail.getByRole('button', { name: 'Save as a template' }).click();
	const save = page.getByRole('dialog', { name: 'Save Plan as a template' });
	await save.getByLabel('Template name').fill(tpl);
	await save.getByRole('button', { name: 'Save template' }).click();
	await expect(page.getByText(`Saved ${tpl} as a template`)).toBeVisible();
	await detail.getByRole('button', { name: 'Close' }).click();
	if (info.project.name === 'phone') {
		// The palette makes one where there is no sidebar.
		await page.goto('/settings/templates');
		await page.getByRole('button', { name: new RegExp(tpl) }).click();
		await axe(page);
		await page.getByRole('button', { name: 'New task from it' }).click();
	} else {
		await page.getByRole('button', { name: 'Make a task from a template' }).click();
		const menu = page.locator('.menu:popover-open');
		await expect(menu).toContainText(tpl);
		await axe(page);
		await menu.getByRole('button', { name: new RegExp(tpl) }).click();
	}
	const toast = page.getByText(new RegExp(`Made ${tpl} in`));
	await expect(toast).toBeVisible();
	await page.getByRole('button', { name: 'Open', exact: true }).click();
	await expect(page.getByLabel('Task title')).toHaveValue('Plan');
	// Due as many days after today as Plan was when saved.
	await expect(page.getByLabel('Due date')).toHaveValue(day(5));
});
