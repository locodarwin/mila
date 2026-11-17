// The wiki's formatting beyond Markdown: highlights and
// fonts from the formatting bar, columns, a foldable section and a view from the
// `/` menu, and the page's own font and size from its menu, each kept as the
// Markdown the page is saved in.
import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, OWNER, pngBuffer } from './helpers';

test.use({ storageState: OWNER });

async function seed(page: Page) {
	await page.goto('/');
	const ws: string = (await apiCall(page, 'POST', '/workspaces', { name: `Format ${Date.now()}` }))
		.body.id;
	const make = async (entity: string, body: unknown) =>
		(await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, body)).body.id as string;
	const project = await make('projects', { name: 'Garden' });
	await make('statuses', { project_id: project, name: 'To do', category: 'todo' });
	await make('tasks', { project_id: project, title: 'Order seeds' });
	await make('tasks', { project_id: project, title: 'Turn the compost' });
	const view = await make('saved-views', {
		project_id: project,
		name: 'Garden, this month',
		type: 'list',
		config: { filter: {} }
	});
	return { ws, view };
}

const body = (page: Page) => page.getByLabel('Page', { exact: true });
const bar = (page: Page) => page.getByRole('toolbar', { name: 'Formatting' });

/**
 * Selects the line the caret is on. A selection made within a few
 * milliseconds of typing on a new page can be missed: the editor's own
 * cursor update lands before the browser reports the selection, and the
 * next redraw puts the caret back. Only a test is that quick; it tries again.
 */
async function selectLine(page: Page) {
	await expect(async () => {
		await page.keyboard.press('End');
		await page.keyboard.press('Shift+Home');
		await expect(bar(page)).toBeVisible({ timeout: 1000 });
	}).toPass();
}

test.describe('formatting beyond Markdown', () => {
	test.skip(({ isMobile }) => isMobile, 'the formatting bar is tried at the desktop width');

	test('highlights, fonts, columns, folds, a view and the page type', async ({ page }) => {
		const { ws } = await seed(page);
		await page.goto(`/wiki?ws=${ws}`);
		await expect(
			page.getByRole('heading', { name: 'Workspace Wiki Main page', level: 1 })
		).toBeVisible();
		await body(page).click();

		// A highlight and a font, from the formatting bar.
		await page.keyboard.type('After the last frost');
		await selectLine(page);
		await bar(page).getByRole('button', { name: 'Highlight' }).click();
		await bar(page).getByRole('button', { name: 'Green' }).click();
		await expect(body(page).locator('mark[data-color="green"]')).toHaveText('After the last frost');
		await bar(page).getByRole('button', { name: 'Font' }).click();
		await bar(page).getByRole('button', { name: 'Source Serif' }).click();
		await expect(body(page).locator('span[data-font="source-serif"]')).toHaveText(
			'After the last frost'
		);
		// The bar goes once the editor has the caret at the end, and not before.
		await page.keyboard.press('End');
		await expect(bar(page)).toBeHidden();
		await page.keyboard.press('Enter');

		// A foldable section: Enter in its title goes under it.
		await page.keyboard.type('/fold');
		await page.keyboard.press('Enter');
		await page.keyboard.type('Soil mix');
		await page.keyboard.press('Enter');
		await page.keyboard.type('A third compost.');
		const fold = body(page).locator('.fold');
		await expect(fold.locator('.fold-title')).toHaveText('Soil mix');
		const toggle = fold.getByRole('button', { name: 'Close Soil mix' });
		await toggle.click();
		await expect(fold.getByText('A third compost.')).toBeHidden();
		await fold.getByRole('button', { name: 'Open Soil mix' }).click();
		await expect(fold.getByText('A third compost.')).toBeVisible();

		// Columns, side by side, after it.
		await page.keyboard.press('ArrowDown');
		await page.keyboard.press('ArrowDown');
		await page.keyboard.type('/two');
		await page.keyboard.press('Enter');
		await page.keyboard.type('Sun all day');
		await body(page).locator('.column').nth(1).click();
		await page.keyboard.type('Part shade');
		const columns = body(page).locator('.columns');
		await expect(columns.locator('.column')).toHaveCount(2);
		await expect(columns.locator('.column').nth(1)).toHaveText('Part shade');

		// A shared view's tasks.
		await body(page).locator('.column').nth(1).click();
		await page.keyboard.press('ArrowDown');
		await page.keyboard.press('ArrowDown');
		await page.keyboard.type('/view');
		await page.keyboard.press('Enter');
		await page
			.getByRole('dialog', { name: 'Show a view here' })
			.getByRole('button', { name: /Garden, this month/ })
			.click();
		const card = body(page).getByRole('region', { name: 'Garden, this month, a saved view' });
		await expect(card.getByRole('button', { name: 'Order seeds' })).toBeVisible();
		await expect(card.getByRole('button', { name: 'Turn the compost' })).toBeVisible();

		// The page's own font and size, from its menu.
		await page.getByRole('button', { name: 'More for this page' }).click();
		await page.getByRole('button', { name: 'Font and size' }).click();
		const type = page.getByRole('dialog', { name: 'Font and size' });
		await type.getByLabel('Font').selectOption('nunito');
		await type.getByRole('radio', { name: 'Large' }).check();
		await type.getByRole('button', { name: 'Done' }).click();
		await expect(page.locator('.wiki .doc.paper')).toHaveAttribute('style', /--doc-size: 1.15/);

		await axe(page);
		if (process.env.LOOK)
			await page.screenshot({ path: `${process.env.LOOK}/page.png`, fullPage: true });

		// All of it is the page's Markdown.
		await page.getByRole('button', { name: 'Markdown' }).click();
		const md = page.getByLabel('Page as Markdown');
		await expect(md).toHaveValue(/<!-- mila:page font=nunito size=large -->/);
		await expect(md).toHaveValue(
			/<mark data-color="green"><span data-font="source-serif">After the last frost<\/span><\/mark>|<span data-font="source-serif"><mark data-color="green">After the last frost<\/mark><\/span>/
		);
		await expect(md).toHaveValue(/<details open>\n<summary>Soil mix<\/summary>/);
		await expect(md).toHaveValue(
			/<!-- mila:columns -->[\s\S]*Sun all day[\s\S]*<!-- mila:column -->[\s\S]*Part shade[\s\S]*<!-- mila:end -->/
		);
		await expect(md).toHaveValue(
			/<!-- mila:view [0-9a-f-]{36} -->\n\[Garden, this month\]\(\/views\/[0-9a-f-]{36}\)/
		);
	});

	test('the / menu keeps the chosen block in view as the arrows move', async ({ page }) => {
		const { ws } = await seed(page);
		await page.goto(`/wiki?ws=${ws}`);
		await body(page).click();
		await page.keyboard.type('Beds');
		await page.keyboard.press('Enter');
		await page.keyboard.type('/');
		const menu = page.getByRole('listbox', { name: 'Insert a block' });
		const options = menu.getByRole('option');
		await expect(options.first()).toBeInViewport();
		// Up from the first goes round to the last, below the menu's edge.
		await page.keyboard.press('ArrowUp');
		await expect(options.last()).toHaveAttribute('aria-selected', 'true');
		await expect(options.last()).toBeInViewport();
		await expect(options.first()).not.toBeInViewport();
		await page.keyboard.press('ArrowDown');
		await expect(options.first()).toBeInViewport();
	});

	test("the / menu's choice stays put while a new page's first words save", async ({ page }) => {
		const { ws } = await seed(page);
		await page.goto(`/wiki?ws=${ws}`);
		await body(page).click();
		await page.keyboard.type('Beds');
		await page.keyboard.press('Enter');
		await page.keyboard.type('/');
		const options = page.getByRole('listbox', { name: 'Insert a block' }).getByRole('option');
		await page.keyboard.press('ArrowDown');
		await page.keyboard.press('ArrowDown');
		await expect(options.nth(2)).toHaveAttribute('aria-selected', 'true');
		// The saved page coming back redraws the editor, which used to send
		// the choice back to the top.
		await expect
			.poll(async () => (await apiCall(page, 'GET', `/workspaces/${ws}/pages`)).body.items[0]?.body)
			.toMatch(/Beds/);
		await page.waitForTimeout(1000); // for it to reach this page
		await expect(page.locator('.wiki .tiptap')).toHaveAttribute(
			'aria-activedescendant',
			(await options.nth(2).getAttribute('id'))!
		);
		await expect(options.nth(2)).toHaveAttribute('aria-selected', 'true');
		await page.keyboard.press('Enter');
		await expect(options.first()).not.toBeAttached();
	});

	test("an image takes a size, a side and a caption, kept in the page's Markdown", async ({
		page
	}) => {
		const { ws } = await seed(page);
		await page.goto(`/wiki?ws=${ws}`);
		await body(page).click();
		await page.keyboard.type('/ima');
		await page.keyboard.press('Enter');
		await page
			.getByRole('dialog', { name: 'Insert an image' })
			.getByLabel('Upload an image')
			.setInputFiles({ name: 'beds.png', mimeType: 'image/png', buffer: pngBuffer(120, 80) });
		const img = body(page).getByRole('img', { name: 'beds' });
		await expect(img).toBeVisible();
		await img.click();
		await bar(page).getByRole('button', { name: 'Image size' }).click();
		await bar(page).getByRole('button', { name: 'Half' }).click();
		await img.click();
		await bar(page).getByRole('button', { name: 'Where it sits' }).click();
		await bar(page).getByRole('button', { name: 'On the right' }).click();
		await img.click();
		await bar(page).getByRole('button', { name: 'Caption' }).click();
		await bar(page).getByLabel('Caption').fill('The three beds');
		await bar(page).getByRole('button', { name: 'Apply' }).click();
		const figure = body(page).locator('.doc-image');
		await expect(figure).toHaveAttribute('data-size', '50');
		await expect(figure).toHaveAttribute('data-side', 'right');
		await expect(figure.locator('.doc-image-caption')).toHaveText('The three beds');
		await expect
			.poll(async () => (await apiCall(page, 'GET', `/workspaces/${ws}/pages`)).body.items[0].body)
			.toMatch(/!\[beds\]\([^)]+\)<!-- mila:image 50 right -->\n\*The three beds\*/);
	});

	test('an image put in a new page waits for the page to reach the server', async ({ page }) => {
		const { ws } = await seed(page);
		// The wiki's main page is made on this visit; its row is slow to send.
		await page.route('**/api/v1/workspaces/*/sync/push', async (route) => {
			await new Promise((done) => setTimeout(done, 1500));
			await route.continue();
		});
		await page.goto(`/wiki?ws=${ws}`);
		await body(page).click();
		await page.keyboard.type('/ima');
		await page.keyboard.press('Enter');
		await page
			.getByRole('dialog', { name: 'Insert an image' })
			.getByLabel('Upload an image')
			.setInputFiles({ name: 'beds.png', mimeType: 'image/png', buffer: pngBuffer(120, 80) });
		await expect(body(page).getByRole('img', { name: 'beds' })).toBeVisible();
		await expect(page.getByText('No such page.')).toHaveCount(0);
	});

	test('a board view shows as a board, moves cards, and shows as a table when asked', async ({
		page
	}) => {
		await page.goto('/');
		const ws: string = (await apiCall(page, 'POST', '/workspaces', { name: `Board ${Date.now()}` }))
			.body.id;
		const make = async (entity: string, b: unknown) =>
			(await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, b)).body.id as string;
		const project = await make('projects', { name: 'Garden' });
		const todo = await make('statuses', { project_id: project, name: 'To do', category: 'todo' });
		const doing = await make('statuses', {
			project_id: project,
			name: 'Doing',
			category: 'in-progress'
		});
		const seeds = await make('tasks', {
			project_id: project,
			title: 'Order seeds',
			status_id: todo
		});
		await make('tasks', { project_id: project, title: 'Turn the compost', status_id: doing });
		await make('saved-views', {
			project_id: project,
			name: 'Garden board',
			type: 'board',
			config: { filter: {} }
		});

		await page.goto(`/wiki?ws=${ws}`);
		await body(page).click();
		await page.keyboard.type('/view');
		await page.keyboard.press('Enter');
		await page
			.getByRole('dialog', { name: 'Show a view here' })
			.getByRole('button', { name: /Garden board/ })
			.click();
		const card = body(page).getByRole('region', { name: 'Garden board, a saved view' });
		const board = card.getByRole('group', { name: 'Garden board as a board' });
		const column = (name: string) => board.getByRole('group', { name, exact: true });
		await expect(column('To do').getByRole('button', { name: 'Order seeds' })).toBeVisible();
		await expect(column('Doing').getByRole('button', { name: 'Turn the compost' })).toBeVisible();

		// A card dragged to another column moves its task, as the board does.
		await column('To do')
			.getByRole('listitem')
			.filter({ hasText: 'Order seeds' })
			.dragTo(column('Doing'));
		await expect(column('Doing').getByRole('button', { name: 'Order seeds' })).toBeVisible();
		await expect
			.poll(
				async () => (await apiCall(page, 'GET', `/workspaces/${ws}/tasks/${seeds}`)).body.status_id
			)
			.toBe(doing);
		await axe(page);
		if (process.env.LOOK) await card.screenshot({ path: `${process.env.LOOK}/board.png` });

		// One dropped in the page's text leaves the page as it was.
		await column('Doing')
			.getByRole('listitem')
			.filter({ hasText: 'Turn the compost' })
			.dragTo(body(page).locator('p').last());
		await expect(column('Doing').getByRole('listitem')).toHaveCount(2);
		await expect(body(page).locator('p').last()).toHaveText('');
		// Picked, the block shows as a table instead, kept in its marker.
		await card.locator('header strong').click();
		await bar(page).getByRole('button', { name: 'Show as' }).click();
		await bar(page).getByRole('button', { name: 'Table' }).click();
		const table = card.getByRole('table', { name: 'Garden board as a table' });
		await expect(table.getByRole('button', { name: 'Order seeds' })).toBeVisible();
		await expect(table.getByRole('columnheader').first()).toBeVisible();
		await expect
			.poll(async () => (await apiCall(page, 'GET', `/workspaces/${ws}/pages`)).body.items[0].body)
			.toMatch(/<!-- mila:view [0-9a-f-]{36} table -->\n\[Garden board\]/);
		await axe(page);
		if (process.env.LOOK) await card.screenshot({ path: `${process.env.LOOK}/table.png` });
	});
});
