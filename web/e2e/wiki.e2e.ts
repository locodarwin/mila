import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, newMember, OWNER, SIGNED_OUT } from './helpers';

test.use({ storageState: OWNER });

/** A workspace of its own, with a project, so its wikis are this test's alone. */
async function seed(page: Page) {
	await page.goto('/');
	const ws: string = (await apiCall(page, 'POST', '/workspaces', { name: `Wiki ${Date.now()}` }))
		.body.id;
	const make = async (entity: string, body: unknown) =>
		(await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, body)).body.id as string;
	const project = await make('projects', { name: 'House' });
	await make('statuses', { project_id: project, name: 'To do', category: 'todo' });
	return { ws, project };
}

const tree = (page: Page) => page.getByRole('tree', { name: 'Pages' }).first();
const item = (page: Page, name: string) => tree(page).getByRole('treeitem', { name, exact: true });
const body = (page: Page) => page.getByLabel('Page', { exact: true });

async function newPage(page: Page, title: string) {
	await page
		.getByRole('button', { name: /^New page under/ })
		.first()
		.click();
	await expect(page.getByLabel('Page title')).toBeFocused();
	await page.getByLabel('Page title').fill(title);
	await page.getByLabel('Page title').press('Enter');
	await expect(item(page, title)).toBeVisible();
}

test.describe('the wiki', () => {
	test.skip(({ isMobile }) => isMobile, 'phones have their own test below');

	test('the workspace wiki has a main page and pages under it', async ({ page }) => {
		// Room for the whole path beside the toolbar (wikitoolbar.e2e.ts tries a short bar).
		await page.setViewportSize({ width: 2000, height: 900 });
		const { ws } = await seed(page);
		await page.goto(`/wiki?ws=${ws}`);
		await expect(
			page.getByRole('heading', { name: 'Workspace Wiki Main page', level: 1 })
		).toBeVisible();
		await body(page).click();
		await page.keyboard.type('Everything about the house that is not a task.');

		await newPage(page, 'Utilities');
		await page.keyboard.type('Where to turn things off.');
		await newPage(page, 'Water');
		await expect(item(page, 'Water')).toHaveAttribute('aria-level', '3');
		await expect(page.getByRole('navigation', { name: 'Page path' })).toHaveText(
			/Workspace Wiki\s*\/\s*Utilities\s*\/\s*Water/
		);

		// The tree from the keyboard: Alt and Left takes Water out a level.
		await item(page, 'Water').focus();
		await page.keyboard.press('Alt+ArrowLeft');
		await expect(item(page, 'Water')).toHaveAttribute('aria-level', '2');
		await page.keyboard.press('Alt+ArrowUp');
		await expect(tree(page).getByRole('treeitem').nth(1)).toHaveAccessibleName('Water');
		await page.keyboard.press('Alt+ArrowDown');
		await expect(tree(page).getByRole('treeitem').nth(2)).toHaveAccessibleName('Water');
		await page.keyboard.press('Alt+ArrowRight');
		await expect(item(page, 'Water')).toHaveAttribute('aria-level', '3');

		// It all comes back after a reload, from the server.
		await page.reload();
		await item(page, 'Utilities').click();
		await expect(body(page)).toContainText('Where to turn things off.');
		await expect
			.poll(async () =>
				(await apiCall(page, 'GET', `/workspaces/${ws}/pages`)).body.items
					.map((p: { title: string }) => p.title)
					.sort()
			)
			.toEqual(['', 'Utilities', 'Water']);

		await axe(page);
	});

	test('folds its tree away, shows Markdown, and deletes with undo', async ({ page }) => {
		const { ws } = await seed(page);
		await page.goto(`/wiki?ws=${ws}`);
		await newPage(page, 'Recipes');
		await newPage(page, 'Soups');
		await item(page, 'Recipes').click();

		// The tree folds away and comes back, from its button or the keys.
		await page.getByRole('button', { name: 'Hide pages' }).click();
		await expect(page.getByRole('tree')).toHaveCount(0);
		await expect(page.getByRole('button', { name: 'Show pages' })).toBeFocused();
		await page.keyboard.press('Control+.');
		await expect(tree(page)).toBeVisible();

		// The Markdown view edits the page as Markdown.
		await body(page).click();
		await page.keyboard.type('Weeknight dinners.');
		await page.getByRole('button', { name: 'Markdown' }).click();
		const source = page.getByLabel('Page as Markdown');
		await expect(source).toHaveValue('Weeknight dinners.');
		await source.fill('## Dinners\n\nSheet-pan **chicken**.');
		await page.getByRole('button', { name: 'Markdown' }).click();
		await expect(body(page).getByRole('heading', { name: 'Dinners' })).toBeVisible();
		await expect(body(page).locator('strong')).toHaveText('chicken');

		// Deleting a page takes its subpages with it; Undo brings both back.
		await page.getByRole('button', { name: 'More for this page' }).click();
		await page.getByRole('button', { name: 'Delete' }).click();
		await expect(page.getByText('Deleted "Recipes" and 1 subpage')).toBeVisible();
		await expect(item(page, 'Recipes')).toHaveCount(0);
		await page.getByRole('button', { name: 'Undo' }).click();
		await expect(item(page, 'Recipes')).toBeVisible();
		await expect(item(page, 'Soups')).toBeVisible();

		// The main page cannot be deleted.
		await item(page, 'Workspace Wiki').click();
		await page.getByRole('button', { name: 'More for this page' }).click();
		await expect(page.getByRole('button', { name: 'Delete' })).toHaveCount(0);
		await expect(page.getByText('The main page goes with its workspace.')).toBeVisible();
	});

	test("a project's wiki is on its Project Wiki tab, which the project remembers", async ({
		page
	}) => {
		const { ws, project } = await seed(page);
		await page.goto(`/projects/${project}?ws=${ws}`);
		await page.getByRole('tab', { name: 'Project Wiki' }).click();
		await expect(page.getByRole('heading', { name: 'House Main page', level: 2 })).toBeVisible();
		await body(page).click();
		await page.keyboard.type('Notes for this fall.');
		await newPage(page, 'Hallway paint');

		// The project opens on the tab it was last on.
		await page.goto(`/projects/${project}`);
		await expect(page.getByRole('tab', { name: 'Project Wiki' })).toHaveAttribute(
			'aria-selected',
			'true'
		);
		// Left and Right move between the tabs.
		await page.getByRole('tab', { name: 'Project Wiki' }).focus();
		await page.keyboard.press('ArrowLeft');
		await expect(page.getByRole('tab', { name: 'Tasks' })).toHaveAttribute('aria-selected', 'true');
		await expect(page.getByRole('tab', { name: 'Tasks' })).toBeFocused();

		// The palette finds a page by what it says.
		await page.keyboard.press('Control+k');
		await page.getByLabel('Search for a task, project or command').fill('this fall');
		await expect(page.getByRole('option', { name: /House/ }).first()).toBeVisible();
		await page.keyboard.press('Enter');
		await expect(page.getByRole('tab', { name: 'Project Wiki' })).toHaveAttribute(
			'aria-selected',
			'true'
		);
		await expect(body(page)).toContainText('Notes for this fall.');

		await axe(page);
	});
});

test('a deleted page waits in the trash with its subpages', async ({ page, isMobile }) => {
	test.skip(isMobile);
	const { ws } = await seed(page);
	await page.goto(`/wiki?ws=${ws}`);
	await newPage(page, 'Old plans');
	await newPage(page, 'Older plans');
	await item(page, 'Old plans').click();
	await page.getByRole('button', { name: 'More for this page' }).click();
	await page.getByRole('button', { name: 'Delete' }).click();
	await expect(item(page, 'Old plans')).toHaveCount(0);

	await page.goto('/trash');
	const deleted = page.getByRole('list', { name: 'Deleted items' });
	await expect(deleted).toContainText('Old plans');
	await expect(deleted).not.toContainText('Older plans');
	await deleted
		.getByRole('listitem')
		.filter({ hasText: 'Old plans' })
		.getByRole('button', { name: 'Restore' })
		.click();
	await expect(page.getByText('Restored Old plans')).toBeVisible();
	await page.goto('/wiki');
	await expect(item(page, 'Old plans')).toBeVisible();
	await expect(item(page, 'Older plans')).toBeVisible();
});

test("a guest sees their projects' wikis and not the workspace's", async ({
	page,
	browser,
	isMobile
}) => {
	test.skip(isMobile);
	const {
		context,
		page: gus,
		workspace
	} = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Gus', 'guest');
	const make = async (entity: string, b: unknown) =>
		(await apiCall(page, 'POST', `/workspaces/${workspace.id}/${entity}`, b)).body.id as string;
	const project = await make('projects', { name: 'Shared' });
	const gusId = (await apiCall(gus, 'GET', '/me')).body.id as string;
	expect(
		(await apiCall(page, 'PUT', `/workspaces/${workspace.id}/projects/${project}/guests/${gusId}`))
			.status
	).toBe(204);
	await gus.goto(`/projects/${project}?ws=${workspace.id}&tab=wiki`);
	await expect(gus.getByRole('heading', { name: 'Shared Main page' })).toBeVisible();
	await expect(gus.getByRole('link', { name: 'Workspace Wiki', exact: true })).toHaveCount(0);
	await gus.goto(`/wiki?ws=${workspace.id}`);
	await expect(gus.getByText("This workspace's wiki is for its members")).toBeVisible();
	await context.close();
});

test('on a phone the pages open as a sheet', async ({ page, isMobile }) => {
	test.skip(!isMobile);
	const { ws } = await seed(page);
	await page.goto(`/wiki?ws=${ws}`);
	await expect(page.getByRole('tree')).toHaveCount(0);
	await page.getByRole('button', { name: 'Show pages' }).click();
	const sheet = page.getByRole('dialog', { name: 'Wiki pages' });
	await expect(sheet.getByRole('treeitem', { name: 'Workspace Wiki' })).toBeVisible();
	await sheet.getByRole('button', { name: 'New page' }).click();
	await expect(sheet).toBeHidden();
	await page.getByLabel('Page title').fill('Groceries');
	await page.getByLabel('Page title').press('Enter');
	await page.getByRole('button', { name: 'Show pages' }).click();
	await expect(sheet.getByRole('treeitem', { name: 'Groceries' })).toBeVisible();
});

test('the page and the list of pages can be made wider, and stay that way', async ({
	page,
	isMobile
}) => {
	test.skip(isMobile, 'A phone uses all of its width.');
	await page.setViewportSize({ width: 1500, height: 900 });
	await page.goto('/wiki');
	const doc = page.locator('#wiki-page');
	const pages = page.locator('#wiki-pages');
	await expect(doc).toBeVisible();
	const width = async (at: typeof doc) => Math.round((await at.boundingBox())!.width);
	expect(await width(doc)).toBe(760);
	expect(await width(pages)).toBe(240);

	// By keyboard, then by dragging.
	const edge = page.getByRole('separator', { name: 'Resize the page' });
	await edge.focus();
	await page.keyboard.press('Shift+ArrowRight');
	expect(await width(doc)).toBe(760 + 64);
	const box = (await edge.boundingBox())!;
	await page.mouse.move(box.x + box.width / 2, box.y + 100);
	await page.mouse.down();
	await page.mouse.move(box.x + box.width / 2 + 100, box.y + 100, { steps: 4 });
	await page.mouse.up();
	expect(await width(doc)).toBe(760 + 64 + 100);

	const list = page.getByRole('separator', { name: 'Resize the list of pages' });
	await list.focus();
	await page.keyboard.press('ArrowRight');
	expect(await width(pages)).toBe(240 + 16);

	// End goes as wide as the space there is, and no further off the screen.
	await edge.focus();
	await page.keyboard.press('End');
	const leaf = (await page.locator('.leaf').boundingBox())!;
	const wide = (await doc.boundingBox())!;
	expect(wide.x + wide.width).toBeLessThanOrEqual(leaf.x + leaf.width + 1);
	expect(wide.width).toBeGreaterThan(950);
	await axe(page);

	// This device remembers.
	const remembered = await width(doc);
	await page.reload();
	await expect(doc).toBeVisible();
	await expect.poll(() => width(doc)).toBe(remembered);
	expect(await width(pages)).toBe(240 + 16);
});

/** A page long enough to scroll, with headings far apart. */
async function longPage(page: Page, url: string) {
	await page.setViewportSize({ width: 1500, height: 800 });
	await page.goto(url);
	await body(page).click();
	await page.keyboard.type('## Beds\n');
	for (let i = 0; i < 40; i++) await page.keyboard.type(`Row ${i}\n`);
	await page.keyboard.type('### South fence\n');
	for (let i = 0; i < 40; i++) await page.keyboard.type(`Row ${i}\n`);
	await page.keyboard.type('## Harvest\nThe end.');
}

test("the page's grip stays on screen, shows the width, and goes back", async ({
	page,
	isMobile
}) => {
	test.skip(isMobile, 'A phone uses all of its width.');
	test.slow();
	const { ws, project } = await seed(page);
	await longPage(page, `/projects/${project}?ws=${ws}&tab=wiki`);
	const doc = page.locator('#wiki-page');
	const edge = page.getByRole('separator', { name: 'Resize the page' });
	const grip = edge.locator('.mark');
	const leaf = page.locator('.leaf');

	// In the page's corner on screen, wherever the page has scrolled to.
	for (const to of [0, 500]) {
		await leaf.evaluate((el, top) => el.scrollTo({ top }), to);
		await expect(grip).toBeVisible();
		const at = (await grip.boundingBox())!;
		const shown = (await leaf.boundingBox())!;
		const page_ = (await doc.boundingBox())!;
		expect(at.y + at.height).toBeLessThanOrEqual(shown.y + shown.height);
		expect(at.y + at.height).toBeGreaterThan(shown.y + shown.height - 12);
		expect(Math.abs(at.x + at.width - (page_.x + page_.width))).toBeLessThan(4);
	}

	const at = (await grip.boundingBox())!;
	await page.mouse.move(at.x + 14, at.y + 14);
	await page.mouse.down();
	await page.mouse.move(at.x + 54, at.y + 14, { steps: 3 });
	await expect(edge.getByText('800 px')).toBeVisible();
	await page.mouse.up();
	await expect(edge.getByText('800 px')).toBeHidden();
	expect(Math.round((await doc.boundingBox())!.width)).toBe(800);

	await edge.dblclick();
	await expect.poll(async () => Math.round((await doc.boundingBox())!.width)).toBe(760);
	await edge.focus();
	await page.keyboard.press('ArrowRight');
	await page.keyboard.press('Enter');
	await expect.poll(async () => Math.round((await doc.boundingBox())!.width)).toBe(760);
});

test('the panel beside a page shows its contents or the pages', async ({ page, isMobile }) => {
	test.skip(isMobile, 'A phone has no room beside the page.');
	test.slow();
	const { ws, project } = await seed(page);
	const url = `/projects/${project}?ws=${ws}&tab=wiki`;
	await page.setViewportSize({ width: 1500, height: 800 });
	await page.goto(url);
	const side = page.getByRole('group', { name: 'Beside the page' });
	await expect(side.getByText('Headings in this page will show here.')).toBeVisible();
	await page.setViewportSize({ width: 1500, height: 800 });

	// The headings follow what is typed.
	await longPage(page, url);
	const contents = side.getByRole('navigation', { name: 'Contents' });
	const entry = (name: string) => contents.getByRole('button', { name, exact: true });
	await expect(contents.getByRole('button')).toHaveText(['Beds', 'South fence', 'Harvest']);

	// A deeper heading goes further in.
	const left = async (name: string) =>
		entry(name).evaluate((el) => parseFloat(getComputedStyle(el).paddingLeft));
	expect(await left('South fence')).toBeGreaterThan(await left('Beds'));

	// Going to one brings it into view below the bar, and marks it.
	await entry('South fence').click();
	const heading = page.locator('.doc-body h3', { hasText: 'South fence' });
	await expect(heading).toBeInViewport();
	const bar = (await page.locator('.leaf .bar').boundingBox())!;
	expect((await heading.boundingBox())!.y).toBeGreaterThanOrEqual(bar.y + bar.height);
	await expect(entry('South fence')).toHaveAttribute('aria-current', 'location');
	await axe(page);

	// The panel stays on screen as the page scrolls.
	await expect(side).toBeInViewport();

	// Markdown has no headings to go to.
	await page.getByRole('button', { name: 'Markdown' }).click();
	await expect(side.getByText('when the page is not showing Markdown')).toBeVisible();
	await page.getByRole('button', { name: 'Markdown' }).click();
	await expect(contents).toBeVisible();

	// Or the wiki's pages, the same as on the left, and this device remembers.
	await side.getByRole('button', { name: 'Pages', exact: true }).click();
	await expect(side.getByRole('button', { name: 'Pages', exact: true })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
	await expect(side.getByRole('tree', { name: 'Pages' })).toBeVisible();
	await axe(page);
	await page.reload();
	await expect(side.getByRole('tree', { name: 'Pages' })).toBeVisible();

	// A page wide enough to take the space takes it; narrower, the panel is back.
	const edge = page.getByRole('separator', { name: 'Resize the page' });
	await edge.focus();
	await page.keyboard.press('End');
	await expect(side).toBeHidden();
	await page.keyboard.press('Enter');
	await expect(side).toBeVisible();
});

test('a page that fits does not scroll, and marks no heading as being read', async ({
	page,
	isMobile
}) => {
	test.skip(isMobile, 'A phone has no room beside the page.');
	const { ws, project } = await seed(page);
	await page.setViewportSize({ width: 1500, height: 900 });
	await page.goto(`/projects/${project}?ws=${ws}&tab=wiki`);
	await body(page).click();
	await page.keyboard.type('## Only heading\nA short page.');
	const contents = page.getByRole('navigation', { name: 'Contents' });
	await expect(contents.getByRole('button', { name: 'Only heading' })).toBeVisible();
	const leaf = page.locator('.leaf');
	expect(await leaf.evaluate((el) => el.scrollHeight - el.clientHeight)).toBe(0);
	await expect(contents.locator('[aria-current]')).toHaveCount(0);
});

test('hiding the panel gives the page all the room, until it is dragged', async ({
	page,
	isMobile
}) => {
	test.skip(isMobile, 'A phone has no room beside the page.');
	const { ws, project } = await seed(page);
	await page.setViewportSize({ width: 1500, height: 800 });
	await page.goto(`/projects/${project}?ws=${ws}&tab=wiki`);
	const doc = page.locator('#wiki-page');
	const leaf = page.locator('.leaf');
	const side = page.getByRole('group', { name: 'Beside the page' });
	const hide = page.getByRole('button', { name: 'Hide contents and pages' });
	const show = page.getByRole('button', { name: 'Show contents and pages' });
	const edge = page.getByRole('separator', { name: 'Resize the page' });
	const widths = async () => ({
		doc: Math.round((await doc.boundingBox())!.width),
		room: await leaf.evaluate((el) => el.clientWidth)
	});

	await expect(side).toBeVisible();
	await expect(show).toHaveCount(0);
	expect((await widths()).doc).toBe(760);

	// Hidden, the page has all the room, and focus stays on the button that moved.
	await hide.click();
	await expect(side).toBeHidden();
	await expect(show).toBeFocused();
	let now = await widths();
	expect(now.doc).toBe(now.room);
	await axe(page);

	// Folding the list of pages too gives it more.
	const before = now.room;
	await page.getByRole('button', { name: 'Hide pages' }).first().click();
	await expect.poll(async () => (await widths()).room).toBeGreaterThan(before);
	await expect
		.poll(async () => {
			const { doc, room } = await widths();
			return doc === room;
		})
		.toBe(true);
	await page.getByRole('button', { name: 'Show pages' }).first().click();

	// Dragged narrower it stays there, and this device remembers.
	await edge.focus();
	await page.keyboard.press('Home');
	await expect.poll(async () => (await widths()).doc).toBe(420);
	await page.reload();
	await expect(side).toBeHidden();
	await expect.poll(async () => (await widths()).doc).toBe(420);

	// Double-clicking gives it all the room again.
	const fills = async () => {
		const { doc, room } = await widths();
		return doc === room;
	};
	await edge.dblclick();
	await expect.poll(fills).toBe(true);
	await page.reload();
	await expect(doc).toBeVisible();
	await expect.poll(fills).toBe(true);

	// Back again, it is the width it had beside the panel.
	await show.click();
	await expect(side).toBeVisible();
	await expect(hide).toBeFocused();
	expect((await widths()).doc).toBe(760);

	// Where the page leaves too little, showing the panel makes the room.
	await edge.focus();
	await page.keyboard.press('End');
	await expect(side).toBeHidden();
	await show.click();
	await expect(side).toBeVisible();
	now = await widths();
	expect(now.room - now.doc).toBeGreaterThanOrEqual(200);
});
