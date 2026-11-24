// The wiki page's formatting toolbar: formatting and blocks from its buttons
// and menus, kept as Markdown; the keyboard; folding to fit a short bar, the
// path giving way first; the person's setting to hide it; and the row above
// the keyboard on a phone.
import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, newPerson, OWNER, saved, SIGNED_OUT } from './helpers';

test.use({ storageState: OWNER });

async function seed(page: Page) {
	await page.goto('/');
	const ws: string = (await apiCall(page, 'POST', '/workspaces', { name: `Toolbar ${Date.now()}` }))
		.body.id;
	return ws;
}

const body = (page: Page) => page.getByLabel('Page', { exact: true });
const toolbar = (page: Page) => page.getByRole('toolbar', { name: 'Format and insert' });
const tool = (page: Page, name: string) => toolbar(page).getByRole('button', { name, exact: true });
const markdown = async (page: Page, ws: string) =>
	(await apiCall(page, 'GET', `/workspaces/${ws}/pages`)).body.items[0]?.body ?? '';

async function openWiki(page: Page, ws: string) {
	await page.goto(`/wiki?ws=${ws}`);
	await expect(
		page.getByRole('heading', { name: 'Workspace Wiki Main page', level: 1 })
	).toBeVisible();
	await expect(toolbar(page)).toBeVisible();
}

test.describe('the wiki toolbar', () => {
	test.skip(({ isMobile }) => isMobile, 'the toolbar in the bar is tried at the desktop width');

	test('formats, inserts blocks and links, and is kept as Markdown', async ({ page }) => {
		await page.setViewportSize({ width: 1600, height: 1000 });
		const ws = await seed(page);
		await openWiki(page, ws);

		// Before the text has had the caret, a button acts at the end of the page.
		await tool(page, 'Bold').click();
		await page.keyboard.type('Planting plan');
		await expect(body(page).locator('strong')).toHaveText('Planting plan');
		await expect(tool(page, 'Bold')).toHaveAttribute('aria-pressed', 'true');

		// Text style names the line and changes it.
		await expect(toolbar(page).getByRole('button', { name: 'Text style: Text' })).toBeVisible();
		await toolbar(page)
			.getByRole('button', { name: /^Text style/ })
			.click();
		await page.getByRole('menuitemradio', { name: 'Heading 2' }).click();
		await expect(body(page).locator('h2')).toHaveText('Planting plan');
		await expect(
			toolbar(page).getByRole('button', { name: 'Text style: Heading 2' })
		).toBeVisible();
		// The caret stayed in the text.
		await expect(body(page)).toBeFocused();

		// A list, toggled on the line.
		await page.keyboard.press('End');
		await page.keyboard.press('Enter');
		await tool(page, 'Bulleted list').click();
		await page.keyboard.type('Tomatoes');
		await expect(body(page).locator('ul li')).toHaveText('Tomatoes');
		await expect(tool(page, 'Bulleted list')).toHaveAttribute('aria-pressed', 'true');
		await page.keyboard.press('Enter');
		await page.keyboard.press('Enter');

		// A table goes on a line of its own and leaves the words around the caret whole.
		await page.keyboard.type('Beds by size');
		await page.keyboard.press('ArrowLeft');
		await tool(page, 'Table').click();
		await expect(body(page).locator('table')).toBeVisible();
		await expect(body(page).locator('p', { hasText: 'Beds by size' })).toHaveText('Beds by size');
		// No table inside a table.
		await expect(tool(page, 'Table')).toHaveAttribute('aria-disabled', 'true');

		// Out of the table, a link: no words selected, so the address goes in as one.
		await page.keyboard.press('Control+End');
		await page.keyboard.type('Seeds come from ');
		await tool(page, 'Link').click();
		const address = page.getByLabel('Link address');
		await address.fill('https://example.com/seeds');
		await address.press('Enter');
		await expect(body(page).locator('a[href="https://example.com/seeds"]')).toHaveText(
			'https://example.com/seeds'
		);

		// Insert holds the whole `/` menu: a foldable section.
		await page.keyboard.type('every spring.');
		await page.keyboard.press('Enter');
		await toolbar(page).getByRole('button', { name: 'Insert' }).click();
		const insert = page.getByRole('menu', { name: 'Insert' });
		for (const name of ['Table', 'Two columns', 'Warning box', 'Image or SVG file', 'View'])
			await expect(insert.getByRole('menuitem', { name, exact: true })).toBeVisible();
		await insert.getByRole('menuitem', { name: 'Foldable section' }).click();
		await expect(body(page).locator('.fold')).toBeVisible();
		await page.keyboard.type('Soil mix');

		// Diagram and image open the same pickers `/` opens.
		await page.keyboard.press('Control+End');
		await tool(page, 'Diagram').click();
		const kinds = page.getByRole('dialog', { name: /diagram/i });
		await expect(kinds).toBeVisible();
		await kinds
			.getByRole('button', { name: /Flowchart/ })
			.first()
			.click();
		await expect(body(page).locator('.diagram-block')).toBeVisible();
		await body(page).locator('p').last().click();
		await tool(page, 'Image or SVG file').click();
		const images = page.getByRole('dialog', { name: /image/i });
		await expect(images).toBeVisible();
		await page.keyboard.press('Escape');
		await expect(images).toBeHidden();

		// Link to a page or task opens the `[[` list.
		await body(page).locator('p').last().click();
		await page.keyboard.press('End');
		await toolbar(page).getByRole('button', { name: 'Insert' }).click();
		await insert.getByRole('menuitem', { name: 'Link to a page or task' }).click();
		await page.keyboard.type('Compost');
		const links = page.getByRole('listbox', { name: 'Link to a page or task' });
		await expect(links.getByRole('option', { name: /New page “Compost”/ })).toBeVisible();
		await page.keyboard.press('Escape');
		await expect(body(page)).toContainText('[[Compost');

		await expect.poll(() => markdown(page, ws)).toContain('## **Planting plan**');
		const md = await markdown(page, ws);
		expect(md).toContain('- Tomatoes');
		expect(md).toContain('Beds by size');
		expect(md).toContain('| ');
		expect(md).toContain(
			'Seeds come from [https://example.com/seeds](https://example.com/seeds) every spring.'
		);
		expect(md).toContain('<summary>Soil mix</summary>');
		expect(md).toContain('```mermaid');
		await axe(page);
	});

	test('is reached with Alt+F10 and left with Escape', async ({ page }) => {
		await page.setViewportSize({ width: 1600, height: 1000 });
		const ws = await seed(page);
		await openWiki(page, ws);
		await body(page).click();
		await page.keyboard.type('Water in the morning');

		// Nothing selected: the toolbar, one Tab stop, arrows along it.
		await page.keyboard.press('Alt+F10');
		await expect(toolbar(page).getByRole('button', { name: /^Text style/ })).toBeFocused();
		await expect(toolbar(page).locator('[tabindex="0"]')).toHaveCount(1);
		await page.keyboard.press('ArrowRight');
		await expect(tool(page, 'Bold')).toBeFocused();
		await page.keyboard.press('End');
		await expect(toolbar(page).getByRole('button', { name: 'Insert' })).toBeFocused();
		// The menu opens into its first choice from the keys.
		await page.keyboard.press('Enter');
		await expect(page.getByRole('menuitem', { name: 'Heading' })).toBeFocused();
		await page.keyboard.press('ArrowDown');
		await expect(page.getByRole('menuitem', { name: 'Bulleted list' })).toBeFocused();
		await page.keyboard.press('Escape');
		await expect(toolbar(page).getByRole('button', { name: 'Insert' })).toBeFocused();
		await page.keyboard.press('Escape');
		await expect(body(page)).toBeFocused();

		// Words selected: the bar over them, as before.
		await page.keyboard.press('Shift+Home');
		const over = page.getByRole('toolbar', { name: 'Formatting' });
		await expect(over).toBeVisible();
		await page.keyboard.press('Alt+F10');
		await expect(over.getByRole('button', { name: 'Bold' })).toBeFocused();
	});

	test('folds to fit, the path giving way first', async ({ page }) => {
		await page.setViewportSize({ width: 1600, height: 1000 });
		const ws = await seed(page);
		await openWiki(page, ws);
		let main = '';
		await expect
			.poll(async () => {
				main = (await apiCall(page, 'GET', `/workspaces/${ws}/pages`)).body.items?.[0]?.id ?? '';
				return main;
			})
			.not.toBe('');
		const make = async (parent: string, title: string) =>
			(await apiCall(page, 'POST', `/workspaces/${ws}/pages`, { parent_id: parent, title })).body
				.id as string;
		const garden = await make(main, 'Garden handbook for the allotment');
		const beds = await make(garden, 'Raised beds and their soil');
		const leaf = await make(beds, 'Bed three');
		await page.setViewportSize({ width: 2400, height: 1000 });
		await page.goto(`/wiki?ws=${ws}&page=${leaf}`);
		await expect(toolbar(page)).toBeVisible();
		const between = page.getByRole('button', { name: /^Pages between/ });
		await expect(tool(page, 'Table')).toBeVisible();
		await expect(between).toBeHidden();

		// Narrowing the window step by step: the path gives way first, then
		// the blocks, the pictures (both still in Insert) and the lists.
		const gone: string[] = [];
		const watch: [string, () => Promise<boolean>][] = [
			['path', () => between.isVisible()],
			['blocks', async () => !(await tool(page, 'Table').isVisible())],
			['pictures', async () => !(await tool(page, 'Diagram').isVisible())],
			['lists', async () => !(await tool(page, 'Bulleted list').isVisible())]
		];
		const insertButton = toolbar(page).getByRole('button', { name: 'Insert' });
		for (let width = 2400; width >= 640 && gone.length < watch.length; width -= 20) {
			await page.setViewportSize({ width, height: 1000 });
			await page.evaluate(
				() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))
			);
			// Once the bar settles, Insert is never cut off by the toolbar's edge.
			await expect
				.poll(
					async () => {
						const button = (await insertButton.boundingBox())!;
						const bar = (await toolbar(page).boundingBox())!;
						return bar.x + bar.width + 0.5 - (button.x + button.width);
					},
					{ message: `Insert cut off at ${width} px`, timeout: 2000 }
				)
				.toBeGreaterThanOrEqual(0);
			for (const [name, isGone] of watch)
				if (!gone.includes(name) && (await isGone())) gone.push(name);
		}
		expect(gone).toEqual(['path', 'blocks', 'pictures', 'lists']);
		// What always stays.
		await expect(tool(page, 'Bold')).toBeVisible();
		await expect(toolbar(page).getByRole('button', { name: 'Insert' })).toBeVisible();
		await toolbar(page).getByRole('button', { name: 'More formatting' }).click();
		await expect(page.getByRole('menuitemcheckbox', { name: 'Bulleted list' })).toBeVisible();
		await page.keyboard.press('Escape');

		// The pages between are a menu away.
		await between.click();
		await page.getByRole('button', { name: 'Raised beds and their soil', exact: true }).click();
		await expect(page).toHaveURL(new RegExp(`page=${beds}`));

		// The bar never runs past the window.
		const bar = await toolbar(page).evaluate((el) => {
			const b = el.closest('.bar')!;
			return { scroll: b.scrollWidth, client: b.clientWidth };
		});
		expect(bar.scroll).toBeLessThanOrEqual(bar.client + 1);
		await axe(page);
	});

	test('each person can hide it', async ({ browser }) => {
		const { page, workspace } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Rowan');
		await page.setViewportSize({ width: 1600, height: 1000 });
		await openWiki(page, workspace.id);
		await page.getByRole('button', { name: 'More for this page' }).click();
		const setting = page.getByRole('checkbox', { name: 'Show the formatting toolbar' });
		const label = page.getByText('Show the formatting toolbar', { exact: true });
		await expect(setting).toBeChecked();
		await label.click();
		await expect(setting).not.toBeChecked();
		await expect(toolbar(page)).toBeHidden();
		await saved(page, 'wikiToolbar', false);
		await page.reload();
		await expect(body(page)).toBeVisible();
		await expect(toolbar(page)).toBeHidden();
		await page.getByRole('button', { name: 'More for this page' }).click();
		await label.click();
		await expect(setting).toBeChecked();
		await expect(toolbar(page)).toBeVisible();
		await saved(page, 'wikiToolbar', true);
		await page.context().close();
	});

	test('steps aside while the page shows as Markdown', async ({ page }) => {
		await page.setViewportSize({ width: 1600, height: 1000 });
		const ws = await seed(page);
		await openWiki(page, ws);
		await page.getByRole('button', { name: 'Markdown' }).click();
		await expect(toolbar(page)).toBeHidden();
		await page.getByRole('button', { name: 'Markdown' }).click();
		await expect(toolbar(page)).toBeVisible();
	});
});

test.describe('the wiki toolbar on a phone', () => {
	test.skip(({ isMobile }) => !isMobile, 'the row above the keyboard is a phone’s');

	test('sits above the keyboard while the page is edited', async ({ page }) => {
		const ws = await seed(page);
		await page.goto(`/wiki?ws=${ws}`);
		await expect(body(page)).toBeVisible();
		await expect(toolbar(page)).toBeHidden();
		await body(page).tap();
		await expect(toolbar(page)).toBeVisible();
		await tool(page, 'Bold').tap();
		await page.keyboard.type('Compost');
		await expect(body(page).locator('strong')).toHaveText('Compost');
		await expect(body(page)).toBeFocused();
		// Undo is on the row, since a phone has no keys for it.
		await expect(tool(page, 'Undo')).toBeVisible();
		await axe(page);
	});
});
