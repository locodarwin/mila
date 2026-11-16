import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, newMember, OWNER, SIGNED_OUT } from './helpers';

test.use({ storageState: OWNER });

/**
 * A workspace of its own with a project of three statuses, an Hours
 * field, three tasks and two subtasks, so the table's groups and
 * summaries are known exactly.
 */
async function seed(page: Page) {
	await page.goto('/');
	const ws: string = (await apiCall(page, 'POST', '/workspaces', { name: `Table ${Date.now()}` }))
		.body.id;
	const make = async (entity: string, body: unknown) => {
		const res = await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, body);
		expect(res.status, `${entity} ${JSON.stringify(res.body)}`).toBe(201);
		return res.body.id as string;
	};
	const project = await make('projects', { name: 'Garden' });
	const todo = await make('statuses', {
		project_id: project,
		name: 'To do',
		category: 'todo',
		sort_key: 'a0'
	});
	const doing = await make('statuses', {
		project_id: project,
		name: 'Doing',
		category: 'in-progress',
		sort_key: 'a1'
	});
	await make('statuses', { project_id: project, name: 'Done', category: 'done', sort_key: 'a2' });
	const hours = await make('custom-fields', { project_id: project, name: 'Hours', type: 'number' });
	const task = (title: string, over: Record<string, unknown>) =>
		make('tasks', { project_id: project, status_id: todo, title, ...over });
	const beds = await task('Rebuild the beds', { priority: 'high', sort_key: 'a0' });
	const garlic = await task('Plant garlic', { priority: 'urgent', sort_key: 'a1' });
	const seeds = await task('Order seeds', { status_id: doing, sort_key: 'a2' });
	await task('Buy boards', { parent_id: beds, sort_key: 'a0' });
	await task('Cut the frames', { parent_id: beds, sort_key: 'a1' });
	await make('custom-field-values', { task_id: beds, field_id: hours, value: 12 });
	await make('custom-field-values', { task_id: garlic, field_id: hours, value: 2 });
	return { ws, project, make, ids: { beds, garlic, seeds } };
}

const grid = (page: Page) => page.getByRole('treegrid', { name: 'Tasks' });
const rowOf = (page: Page, title: string) =>
	grid(page)
		.locator('tr[data-key]')
		.filter({ has: page.getByRole('button', { name: title, exact: true }) });
const heading = (page: Page, name: string) =>
	grid(page).getByRole('columnheader', { name: new RegExp(`^${name}`) });
/** A group's rows, by its title, as task titles in order. */
async function titlesIn(page: Page, group: string) {
	return grid(page)
		.locator('tbody')
		.filter({ has: page.getByRole('heading', { name: group, exact: true }) })
		.locator('tr[data-key] .titletext')
		.allTextContents();
}

test.describe('a table', () => {
	test.skip(({ isMobile }) => isMobile, 'phones show a table as list rows');

	test('shows subtasks under their task and sums up each group', async ({ page }) => {
		const { ws, project } = await seed(page);
		await page.goto(`/projects/${project}?ws=${ws}&view=table`);
		await expect(grid(page)).toBeVisible();

		expect(await titlesIn(page, 'To do')).toEqual([
			'Rebuild the beds',
			'Buy boards',
			'Cut the frames',
			'Plant garlic'
		]);
		await expect(rowOf(page, 'Buy boards')).toHaveAttribute('aria-level', '2');
		await expect(rowOf(page, 'Rebuild the beds')).toHaveAttribute('aria-expanded', 'true');

		// The summaries count subtasks too, and add up the hours.
		const todo = grid(page)
			.locator('tbody')
			.filter({ has: page.getByRole('heading', { name: 'To do' }) });
		await expect(todo.locator('tr.sum')).toContainText('4 tasks');
		await expect(todo.locator('tr.sum')).toContainText('14');

		// Folding hides the subtasks, and is remembered.
		await page.getByRole('button', { name: 'Hide subtasks of Rebuild the beds' }).click();
		await expect(rowOf(page, 'Buy boards')).toHaveCount(0);
		await page.reload();
		await expect(rowOf(page, 'Rebuild the beds')).toHaveAttribute('aria-expanded', 'false');
		await page.getByRole('button', { name: 'Show subtasks of Rebuild the beds' }).click();
		await expect(rowOf(page, 'Buy boards')).toBeVisible();

		await axe(page);
	});

	test('sorts by its headings and edits cells in place', async ({ page }) => {
		const { ws, project } = await seed(page);
		await page.goto(`/projects/${project}?ws=${ws}&view=table`);
		const priority = heading(page, 'Priority');

		await priority.click();
		await expect(priority).toHaveAttribute('aria-sort', 'ascending');
		expect(
			(await titlesIn(page, 'To do')).filter((t) => !['Buy boards', 'Cut the frames'].includes(t))
		).toEqual(['Rebuild the beds', 'Plant garlic']);
		await priority.click();
		await expect(priority).toHaveAttribute('aria-sort', 'descending');
		expect((await titlesIn(page, 'To do'))[0]).toBe('Plant garlic');
		// Shift adds a second key, numbered on both headings.
		await heading(page, 'Task').click({ modifiers: ['Shift'] });
		await expect(heading(page, 'Task')).toContainText('2');
		// A plain click on a heading sorted descending takes the sort away.
		await priority.click();
		await expect(priority).not.toHaveAttribute('aria-sort', /.+/);
		await expect(heading(page, 'Task')).not.toContainText('2');

		// A picker sets the priority.
		await rowOf(page, 'Plant garlic').locator('td').nth(2).click();
		await page.getByRole('menuitemradio', { name: 'Low' }).click();
		await expect(rowOf(page, 'Plant garlic')).toContainText('Low');

		// Moving to another status moves the row to its group.
		await rowOf(page, 'Order seeds').locator('td').nth(1).click();
		await page.getByRole('menuitemradio', { name: 'To do' }).click();
		await expect.poll(() => titlesIn(page, 'To do')).toContain('Order seeds');
		await expect(grid(page).getByRole('heading', { name: 'Doing' })).toHaveCount(0);

		// The pencil renames in place.
		await rowOf(page, 'Plant garlic').hover();
		await page.getByRole('button', { name: 'Rename Plant garlic' }).click();
		await expect(page.getByLabel('Task title')).toHaveValue('Plant garlic');
		await page.getByLabel('Task title').fill('Plant the garlic');
		await page.getByLabel('Task title').press('Enter');
		await expect(rowOf(page, 'Plant the garlic')).toBeVisible();
	});

	test('works from the keyboard', async ({ page }) => {
		const { ws, project } = await seed(page);
		await page.goto(`/projects/${project}?ws=${ws}&view=table`);
		await expect(grid(page)).toBeVisible();

		// One Tab stop; the arrows walk the cells.
		const first = heading(page, 'Task');
		await first.focus();
		await page.keyboard.press('ArrowDown');
		const titleCell = rowOf(page, 'Rebuild the beds').locator('td').first();
		await expect(titleCell).toBeFocused();
		// Left on a title folds its subtasks, Right shows them.
		await page.keyboard.press('ArrowLeft');
		await expect(rowOf(page, 'Buy boards')).toHaveCount(0);
		await page.keyboard.press('ArrowRight');
		await expect(rowOf(page, 'Buy boards')).toBeVisible();

		// Enter on a cell opens its picker; Escape closes it back to the cell.
		await page.keyboard.press('ArrowRight');
		await page.keyboard.press('ArrowRight');
		const priorityCell = rowOf(page, 'Rebuild the beds').locator('td').nth(2);
		await expect(priorityCell).toBeFocused();
		await page.keyboard.press('Enter');
		await expect(page.getByRole('menuitemradio', { name: 'High' })).toBeFocused();
		await page.keyboard.press('Escape');
		await expect(priorityCell).toBeFocused();
		// The list's keys work on a row.
		await page.keyboard.press('1');
		await expect(rowOf(page, 'Rebuild the beds')).toContainText('Urgent');

		// Alt and the arrows move a row among its group.
		await page.keyboard.press('Alt+ArrowDown');
		await expect
			.poll(async () =>
				(await titlesIn(page, 'To do')).filter((t) => !t.startsWith('Buy') && !t.startsWith('Cut'))
			)
			.toEqual(['Plant garlic', 'Rebuild the beds']);

		// And a column on its heading.
		const priority = heading(page, 'Priority');
		await priority.focus();
		await page.keyboard.press('Alt+ArrowLeft');
		await expect
			.poll(async () =>
				(await grid(page).getByRole('columnheader').allTextContents())
					.map((n) => n.trim())
					.slice(0, 3)
			)
			.toEqual(['Task', 'Priority', 'Status']);
		await expect(heading(page, 'Priority')).toBeFocused();

		// Alt and Shift with the arrows make it wider or narrower.
		const widthOf = async () => Math.round((await heading(page, 'Priority').boundingBox())!.width);
		const before = await widthOf();
		await page.keyboard.press('Alt+Shift+ArrowRight');
		await page.keyboard.press('Alt+Shift+ArrowRight');
		await expect.poll(widthOf).toBe(before + 32);
		// Said, with focus kept on the heading.
		await expect(page.locator('#announcer')).toContainText(/Priority is \d+ pixels wide/);
		await expect(heading(page, 'Priority')).toBeFocused();
		await page.keyboard.press('Alt+Shift+ArrowLeft');
		await expect.poll(widthOf).toBe(before + 16);
	});

	test('in a saved view, drags a row to another group and keeps its layout', async ({ page }) => {
		const { ws, project, make } = await seed(page);
		const view = await make('saved-views', {
			name: 'Garden table',
			type: 'table',
			project_id: project,
			config: { filter: {} },
			display: { group: { by: 'priority' } }
		});
		await page.goto(`/views/${view}?ws=${ws}`);
		await expect(grid(page)).toBeVisible();
		expect(await titlesIn(page, 'Urgent')).toEqual(['Plant garlic']);

		// Dragging Plant garlic onto the High group sets its priority.
		await rowOf(page, 'Plant garlic').hover();
		const handle = page.getByRole('button', { name: 'Move Plant garlic' });
		const from = (await handle.boundingBox())!;
		const to = (await rowOf(page, 'Rebuild the beds').boundingBox())!;
		await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
		await page.mouse.down();
		await page.mouse.move(to.x + 40, to.y + to.height * 0.8, { steps: 8 });
		await page.mouse.up();
		await expect.poll(() => titlesIn(page, 'High')).toContain('Plant garlic');
		await expect(rowOf(page, 'Plant garlic')).toContainText('High');

		// A summary of the hours is chosen from its cell, and kept.
		const high = grid(page)
			.locator('tbody')
			.filter({ has: page.getByRole('heading', { name: 'High' }) });
		await high.locator('tr.sum td').last().click();
		await page.getByRole('menuitemradio', { name: 'Average' }).click();
		await expect(high.locator('tr.sum td').last()).toContainText('7');

		// Widths and the sort are kept with the view.
		const status = heading(page, 'Status');
		const before = (await status.boundingBox())!.width;
		const grip = status.locator('.grip');
		const g = (await grip.boundingBox())!;
		await page.mouse.move(g.x + g.width / 2, g.y + g.height / 2);
		await page.mouse.down();
		await page.mouse.move(g.x + 80, g.y + g.height / 2, { steps: 5 });
		await page.mouse.up();
		await heading(page, 'Hours').click();
		// Shown means saved on the device.
		await expect(heading(page, 'Hours')).toHaveAttribute('aria-sort', 'ascending');
		await page.reload();
		await expect(heading(page, 'Hours')).toHaveAttribute('aria-sort', 'ascending');
		await expect
			.poll(async () => (await heading(page, 'Status').boundingBox())!.width)
			.toBeGreaterThan(before + 60);
		await expect(high.locator('tr.sum td').last()).toContainText('7');
	});
});

test.describe('a filtered table', () => {
	test.skip(({ isMobile }) => isMobile, 'phones show a table as list rows');

	test('shows a matching subtask under its task, and counts only matches', async ({ page }) => {
		const { ws, make } = await seed(page);
		const view = await make('saved-views', {
			name: 'Boards to buy',
			type: 'table',
			project_id: null,
			owner_id: null,
			sort_key: 'a0',
			config: { filter: { text: 'buy boards' } }
		});
		await page.goto(`/views/${view}?ws=${ws}`);
		await expect(grid(page)).toBeVisible();
		// Found under the task that holds it; its sibling stays out.
		const rows = grid(page).locator('tr[data-key] .titletext');
		await expect(rows).toHaveText(['Rebuild the beds', 'Buy boards']);
		await expect(grid(page).locator('tr.sum').first()).toContainText(/\b1\b/);
	});

	test('with many columns, draws those near the view and still reaches the rest', async ({
		page
	}) => {
		const { ws, project, make, ids } = await seed(page);
		let last = '';
		for (let i = 0; i < 40; i++)
			last = await make('custom-fields', {
				project_id: project,
				name: `Extra ${i}`,
				type: 'number'
			});
		await make('custom-field-values', { task_id: ids.garlic, field_id: last, value: 4321 });
		await page.goto(`/projects/${project}?ws=${ws}&view=table`);
		await expect(grid(page)).toBeVisible();
		await expect(grid(page).getByRole('columnheader', { name: /^Extra 39/ })).toBeAttached();

		const row = rowOf(page, 'Plant garlic');
		const drawn = await row.getByRole('gridcell').count();
		expect(drawn, 'cells drawn in a row').toBeLessThan(30);
		await expect(row.getByText('4,321')).toHaveCount(0);

		// The keyboard reaches the far end, and the value is there.
		await row.getByRole('gridcell').first().focus();
		await page.keyboard.press('End');
		await expect(row.getByText('4,321')).toBeVisible();
		await expect(row.getByRole('gridcell').last()).toBeFocused();
		await axe(page);

		// Scrolled there by hand, the cells beside it are drawn too.
		await page.evaluate(() => {
			const el = document.querySelector('[data-table]');
			if (el) el.scrollLeft = el.scrollWidth / 2;
		});
		await expect(grid(page).getByRole('columnheader', { name: /^Extra 20/ })).toBeInViewport();
		const middle = await page.evaluate(() => {
			const row = document.querySelector('tr[data-task]');
			const box = document.querySelector('[data-table]')?.getBoundingClientRect();
			if (!row || !box) return 0;
			return [...row.querySelectorAll('td[role=gridcell]')].filter((c) => {
				const r = c.getBoundingClientRect();
				return r.right > box.left && r.left < box.right;
			}).length;
		});
		expect(middle, 'cells in view').toBeGreaterThan(3);
	});

	test('names someone in a person field after they leave', async ({ page, browser }) => {
		await page.goto('/');
		const {
			workspace,
			context,
			page: leaves
		} = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Lea');
		const ws = workspace.id;
		const lea = (await apiCall(page, 'GET', `/workspaces/${ws}/members`)).body.find(
			(m: { name: string }) => m.name === 'Lea'
		).userId as string;
		const make = async (entity: string, body: unknown) =>
			(await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, body)).body.id as string;
		const project = await make('projects', { name: 'Handover' });
		const field = await make('custom-fields', { project_id: project, name: 'Lead', type: 'user' });
		const task = await make('tasks', { project_id: project, title: 'Pass it on', sort_key: 'a0' });
		await make('custom-field-values', { task_id: task, field_id: field, value: lea });
		// Someone with work left behind keeps a name.
		await apiCall(leaves, 'POST', `/workspaces/${ws}/tasks`, {
			project_id: project,
			title: 'Her own',
			created_by: lea,
			sort_key: 'a1'
		});
		expect((await apiCall(page, 'DELETE', `/workspaces/${ws}/members/${lea}`)).status).toBe(204);
		await context.close();

		await page.goto(`/projects/${project}?ws=${ws}&task=${task}`);
		const lead = page.getByRole('combobox', { name: 'Lead' });
		await expect(lead).toHaveValue(lea);
		await expect(lead.locator('option:checked')).toHaveText('Lea');
	});
});

test('a phone shows a table as list rows', async ({ page, isMobile }) => {
	test.skip(!isMobile, 'the desktop draws the table');
	const { ws, project } = await seed(page);
	await page.goto(`/projects/${project}?ws=${ws}&view=table`);
	const list = page.getByRole('list', { name: 'Tasks' });
	await expect(list).toContainText('Plant garlic');
	await expect(page.getByRole('treegrid')).toHaveCount(0);
});
