// Selecting several tasks: by Ctrl-click, Shift-click and
// keys in a list, on a board and in a table; the bar's changes as one undo step; a
// selected task's menu; and on a phone, picking from a held finger's menu.
import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, OWNER } from './helpers';

test.use({ storageState: OWNER });

const TITLES = [
	'Order seeds',
	'Turn the compost',
	'Mend the fence',
	'Plant garlic',
	'Mulch the beds'
];

async function seed(page: Page) {
	await page.goto('/');
	const ws: string = (await apiCall(page, 'POST', '/workspaces', { name: `Select ${Date.now()}` }))
		.body.id;
	const make = async (entity: string, body: unknown) =>
		(await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, body)).body.id as string;
	const project = await make('projects', { name: 'Garden' });
	const house = await make('projects', { name: 'House' });
	await make('statuses', { project_id: project, name: 'To do', category: 'todo' });
	await make('statuses', { project_id: project, name: 'Doing', category: 'in-progress' });
	await make('statuses', { project_id: project, name: 'Done', category: 'done' });
	await make('statuses', { project_id: house, name: 'To do', category: 'todo' });
	const label = await make('labels', { name: 'Outdoors', color: 'green' });
	const ids: string[] = [];
	for (const title of TITLES) ids.push(await make('tasks', { project_id: project, title }));
	return { ws, project, house, label, ids };
}

type Row = { id: string; title: string; priority: string; project_id: string | null };
async function tasks(page: Page, ws: string): Promise<Row[]> {
	return (await apiCall(page, 'GET', `/workspaces/${ws}/tasks`)).body.items;
}

const row = (page: Page, title: string) =>
	page
		.locator('[data-task-id]')
		.filter({ has: page.getByRole('button', { name: title, exact: true }) });
const bar = (page: Page) => page.getByRole('toolbar', { name: /selected$/ });

test.describe('selecting several tasks', () => {
	test.skip(({ isMobile }) => isMobile, 'phones have their own test below');

	test('Ctrl-click and Shift-click select, and the bar changes them all as one step', async ({
		page
	}) => {
		const { ws, project } = await seed(page);
		await page.goto(`/projects/${project}?ws=${ws}`);
		await page.getByRole('button', { name: 'Order seeds', exact: true }).click({
			modifiers: ['ControlOrMeta']
		});
		// A plain click elsewhere still opens; the selection stays.
		await expect(page).not.toHaveURL(/task=/);
		await expect(bar(page)).toHaveAccessibleName('1 task selected');
		await page
			.getByRole('button', { name: 'Mend the fence', exact: true })
			.click({ modifiers: ['Shift'] });
		await expect(bar(page)).toHaveAccessibleName('3 tasks selected');
		for (const t of TITLES.slice(0, 3))
			await expect(page.getByRole('checkbox', { name: `Select ${t}` })).toBeChecked();
		await expect(page.getByRole('checkbox', { name: 'Select Plant garlic' })).not.toBeChecked();
		await axe(page);

		await bar(page).getByRole('button', { name: 'Priority' }).click();
		await page
			.getByRole('menu', { name: /^Priority, 3 tasks/ })
			.getByRole('menuitemcheckbox', { name: 'High' })
			.click();
		await expect
			.poll(async () => (await tasks(page, ws)).filter((t) => t.priority === 'high').length)
			.toBe(3);
		await page.getByRole('button', { name: 'Undo' }).click();
		await expect
			.poll(async () => (await tasks(page, ws)).filter((t) => t.priority === 'high').length)
			.toBe(0);
		// The selection stayed through the change and its undo.
		await expect(bar(page)).toHaveAccessibleName('3 tasks selected');

		// Labels: on some shows a dash; choosing it puts it on all.
		await bar(page).getByRole('button', { name: 'Labels' }).click();
		const labels = page.getByRole('menu', { name: /^Labels, 3 tasks/ });
		await labels.getByRole('menuitemcheckbox', { name: 'Outdoors' }).click();
		await expect(labels.getByRole('menuitemcheckbox', { name: 'Outdoors' })).toHaveAttribute(
			'aria-checked',
			'true'
		);
		await page.keyboard.press('Escape');

		// Plain click on the circle takes one out; Escape on a row clears.
		await page.getByRole('checkbox', { name: 'Select Turn the compost' }).click();
		await expect(bar(page)).toHaveAccessibleName('2 tasks selected');
		await row(page, 'Order seeds').focus();
		await page.keyboard.press('Escape');
		await expect(bar(page)).toBeHidden();
	});

	test('keys select, and the task keys act on the selection', async ({ page }) => {
		const { ws, project } = await seed(page);
		await page.goto(`/projects/${project}?ws=${ws}`);
		await row(page, 'Order seeds').focus();
		await page.keyboard.press('Space');
		await page.keyboard.press('Shift+ArrowDown');
		await page.keyboard.press('Shift+ArrowDown');
		await expect(bar(page)).toHaveAccessibleName('3 tasks selected');
		// Back up takes off what was passed.
		await page.keyboard.press('Shift+ArrowUp');
		await expect(bar(page)).toHaveAccessibleName('2 tasks selected');
		await page.keyboard.press('2');
		await expect
			.poll(async () =>
				(await tasks(page, ws))
					.filter((t) => t.priority === 'high')
					.map((t) => t.title)
					.sort()
			)
			.toEqual(['Order seeds', 'Turn the compost']);
		await page.keyboard.press('ControlOrMeta+a');
		await expect(bar(page)).toHaveAccessibleName('5 tasks selected');
		await page.keyboard.press('Delete');
		await expect(page.getByRole('button', { name: 'Order seeds', exact: true })).toBeHidden();
		await expect(bar(page)).toBeHidden();
		await page
			.locator('.toast', { hasText: 'Deleted 5 tasks' })
			.getByRole('button', { name: 'Undo' })
			.click();
		for (const t of TITLES)
			await expect(page.getByRole('button', { name: t, exact: true })).toBeVisible();
		await expect(bar(page)).toHaveAccessibleName('5 tasks selected');
	});

	test("a selected task's menu acts on all of them, and Move to takes them away", async ({
		page
	}) => {
		const { ws, project, house } = await seed(page);
		await page.goto(`/projects/${project}?ws=${ws}`);
		await expect(page.getByRole('button', { name: 'Plant garlic', exact: true })).toBeVisible();
		for (const t of ['Plant garlic', 'Mulch the beds'])
			await page
				.getByRole('button', { name: t, exact: true })
				.click({ modifiers: ['ControlOrMeta'] });
		await page
			.getByRole('button', { name: 'Plant garlic', exact: true })
			.click({ button: 'right' });
		const actions = page.getByRole('menu', { name: /^2 tasks, selection actions/ });
		await expect(actions).toBeVisible();
		await actions.getByRole('menuitem', { name: 'Move to' }).click();
		await actions.getByRole('menuitemcheckbox', { name: 'House' }).click();
		await expect
			.poll(async () => (await tasks(page, ws)).filter((t) => t.project_id === house).length)
			.toBe(2);
		// They left the list, so the selection is empty.
		await expect(bar(page)).toBeHidden();

		// An unselected task's menu is its own, with Select first.
		await page.getByRole('button', { name: 'Order seeds', exact: true }).click({ button: 'right' });
		const own = page.getByRole('menu', { name: /^Order seeds, task actions/ });
		await expect(own.getByRole('menuitem').first()).toHaveText(/Select/);
		await own.getByRole('menuitem', { name: /^Select/ }).click();
		await expect(bar(page)).toHaveAccessibleName('1 task selected');
		// Going to another page clears it.
		await page.getByRole('link', { name: 'Inbox' }).first().click();
		await expect(bar(page)).toBeHidden();
	});

	test('on a board, dragging a selected card carries the others', async ({ page }) => {
		const { ws, project } = await seed(page);
		await page.goto(`/projects/${project}?ws=${ws}&view=board`);
		const card = (t: string) => page.locator('[data-card]').filter({ hasText: t });
		await card('Order seeds').click({ modifiers: ['ControlOrMeta'] });
		await card('Plant garlic').click({ modifiers: ['ControlOrMeta'] });
		await expect(bar(page)).toHaveAccessibleName('2 tasks selected');
		const doing = page.getByRole('region', { name: 'Doing', exact: true });
		await card('Order seeds').dragTo(doing);
		await expect(doing.locator('[data-card]')).toHaveCount(2);
		await page.getByRole('button', { name: 'Undo' }).click();
		await expect(doing.locator('[data-card]')).toHaveCount(0);
	});
});

test.describe('selecting on a phone', () => {
	test.skip(({ isMobile }) => !isMobile, 'a phone test');

	test("a held finger's Select starts picking, and taps select", async ({ page }) => {
		const { ws, project } = await seed(page);
		await page.goto(`/projects/${project}?ws=${ws}`);
		const title = page.getByRole('button', { name: 'Order seeds', exact: true });
		await title.dispatchEvent('contextmenu', { button: 2 });
		await page.getByRole('menuitem', { name: /^Select/ }).click();
		const strip = page.getByRole('group', { name: 'Selection' });
		await expect(strip).toContainText('1 selected');
		// While picking, a tap selects instead of opening, and no checkbox completes.
		await page.getByRole('button', { name: 'Plant garlic', exact: true }).click();
		await expect(page).not.toHaveURL(/task=/);
		await expect(strip).toContainText('2 selected');
		await expect(page.getByRole('checkbox', { name: 'Order seeds', exact: true })).toHaveCount(0);
		await strip.getByRole('button', { name: 'All' }).click();
		await expect(strip).toContainText('5 selected');
		await bar(page).getByRole('button', { name: 'Complete' }).click();
		await expect
			.poll(
				async () =>
					(await tasks(page, ws)).filter(
						(t) => (t as Row & { completed_at: string | null }).completed_at
					).length
			)
			.toBe(5);
		await axe(page);
		await strip.getByRole('button', { name: 'Done' }).click();
		await expect(strip).toBeHidden();
	});
});
