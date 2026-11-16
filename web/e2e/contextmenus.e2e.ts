// Right-click menus (docs/PLAN.md section 3): a task's, a view's and a project's, from
// the pointer and from the keyboard, and on a phone from a held finger.
import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, OWNER } from './helpers';

test.use({ storageState: OWNER });

async function seed(page: Page) {
	await page.goto('/');
	const ws: string = (await apiCall(page, 'POST', '/workspaces', { name: `Menus ${Date.now()}` }))
		.body.id;
	const make = async (entity: string, body: unknown) =>
		(await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, body)).body.id as string;
	const project = await make('projects', { name: 'Garden' });
	await make('projects', { name: 'House' });
	await make('statuses', { project_id: project, name: 'To do', category: 'todo' });
	await make('statuses', { project_id: project, name: 'Done', category: 'done' });
	const task = await make('tasks', { project_id: project, title: 'Order seeds' });
	await make('tasks', { project_id: project, title: 'Turn the compost' });
	const view = await make('saved-views', {
		project_id: project,
		name: 'Garden list',
		type: 'list',
		config: { filter: {} }
	});
	return { ws, project, task, view };
}

const menu = (page: Page, name: RegExp) => page.getByRole('menu', { name });

test.describe('right-click menus', () => {
	test.skip(({ isMobile }) => isMobile, 'phones have their own test below');

	test("a task's menu sets its priority, duplicates it and deletes it", async ({ page }) => {
		const { ws, project } = await seed(page);
		await page.goto(`/projects/${project}?ws=${ws}`);
		const title = page.getByRole('button', { name: 'Order seeds', exact: true });
		await title.click({ button: 'right' });
		const actions = menu(page, /^Order seeds, task actions/);
		await expect(actions).toBeVisible();
		await axe(page);
		if (process.env.LOOK) await page.screenshot({ path: `${process.env.LOOK}/menu.png` });
		await actions.getByRole('menuitem', { name: 'Priority' }).click();
		if (process.env.LOOK) await page.screenshot({ path: `${process.env.LOOK}/priority.png` });
		await actions.getByRole('menuitemcheckbox', { name: 'High' }).click();
		await expect(actions).toBeHidden();
		await expect
			.poll(async () => {
				const tasks = (await apiCall(page, 'GET', `/workspaces/${ws}/tasks`)).body.items;
				return tasks.find((t: { title: string }) => t.title === 'Order seeds')?.priority;
			})
			.toBe('high');

		await title.click({ button: 'right' });
		await menu(page, /^Order seeds/)
			.getByRole('menuitem', { name: 'Duplicate' })
			.click();
		await expect(
			page.getByRole('button', { name: 'Order seeds (copy)', exact: true })
		).toBeVisible();

		await page
			.getByRole('button', { name: 'Order seeds (copy)', exact: true })
			.click({ button: 'right' });
		await menu(page, /^Order seeds \(copy\)/)
			.getByRole('menuitem', { name: 'Delete' })
			.click();
		await expect(
			page.getByRole('button', { name: 'Order seeds (copy)', exact: true })
		).toBeHidden();
		await expect(page.getByRole('status').getByText('Deleted "Order seeds (copy)"')).toBeVisible();
	});

	test('the keyboard opens a menu with Shift F10, moves in it, and goes back', async ({ page }) => {
		const { ws, project } = await seed(page);
		await page.goto(`/projects/${project}?ws=${ws}`);
		const title = page.getByRole('button', { name: 'Turn the compost', exact: true });
		await title.focus();
		await page.keyboard.press('Shift+F10');
		const actions = menu(page, /^Turn the compost, task actions/);
		await expect(actions).toBeVisible();
		await expect(actions.getByRole('menuitem', { name: /^Select/ })).toBeFocused();
		await page.keyboard.press('ArrowDown');
		await expect(actions.getByRole('menuitem', { name: 'Open' })).toBeFocused();
		await page.keyboard.press('Escape');
		await expect(actions).toBeHidden();
		await expect(title).toBeFocused();

		// Move to opens its own list, and Left comes back from it.
		await page.keyboard.press('Shift+F10');
		const moveTo = actions.getByRole('menuitem', { name: 'Move to' });
		await moveTo.focus();
		await page.keyboard.press('ArrowRight');
		await expect(actions.getByRole('menuitemcheckbox', { name: 'House' })).toBeVisible();
		await page.keyboard.press('ArrowLeft');
		await expect(actions.getByRole('menuitem', { name: 'Move to' })).toBeFocused();
		await page.keyboard.press('Escape');
	});

	test("a view's and a project's menus, from the sidebar", async ({ page }) => {
		const { ws, view } = await seed(page);
		await page.goto(`/?ws=${ws}`);
		await page
			.getByRole('list', { name: 'Saved views' })
			.getByRole('link', { name: 'Garden list' })
			.click({ button: 'right' });
		await menu(page, /^Garden list, view actions/)
			.getByRole('menuitem', { name: 'Rename' })
			.click();
		await expect(page).toHaveURL(new RegExp(`/views/${view}`));
		await expect(page.getByLabel('View name')).toBeFocused();

		await page
			.getByRole('navigation', { name: 'Main' })
			.getByRole('link', { name: 'House' })
			.click({ button: 'right' });
		const projectActions = menu(page, /^House, project actions/);
		await projectActions.getByRole('menuitem', { name: 'Color and icon' }).click();
		await projectActions.getByRole('menuitemcheckbox', { name: 'Purple' }).click();
		await expect
			.poll(async () => {
				const all = (await apiCall(page, 'GET', `/workspaces/${ws}/projects`)).body.items;
				return all.find((p: { name: string }) => p.name === 'House')?.color;
			})
			.toBe('purple');
		// Shift and right-click opens the same menu.
		await page.keyboard.press('Escape');
		await expect(page.getByRole('menu')).toHaveCount(0);
		await page
			.getByRole('navigation', { name: 'Main' })
			.getByRole('link', { name: 'House' })
			.click({ button: 'right', modifiers: ['Shift'] });
		await expect(menu(page, /^House, project actions/)).toBeVisible();
	});
});

test('on a phone, holding a task brings its menu up as a sheet', async ({ page, isMobile }) => {
	test.skip(!isMobile, 'phones only');
	const { ws, project } = await seed(page);
	await page.goto(`/projects/${project}?ws=${ws}`);
	const title = page.getByRole('button', { name: 'Order seeds', exact: true });
	await expect(title).toBeVisible();
	const box = (await title.boundingBox())!;
	const x = box.x + box.width / 2;
	const y = box.y + box.height / 2;
	// A finger held still: down, a wait, then up.
	await page.evaluate(
		async ([px, py]) => {
			const el = document.elementFromPoint(px, py)!;
			const opts = {
				bubbles: true,
				pointerType: 'touch',
				clientX: px,
				clientY: py,
				isPrimary: true
			};
			el.dispatchEvent(new PointerEvent('pointerdown', opts));
			await new Promise((r) => setTimeout(r, 650));
			el.dispatchEvent(new PointerEvent('pointerup', opts));
		},
		[x, y]
	);
	const sheet = menu(page, /^Order seeds, task actions/);
	await expect(sheet).toBeVisible();
	await expect(page.locator('.cmenu.sheet')).toBeVisible();
	if (process.env.LOOK) await page.screenshot({ path: `${process.env.LOOK}/sheet.png` });
	await sheet.getByRole('menuitem', { name: 'Add to My Day' }).click();
	await expect(sheet).toBeHidden();
});
