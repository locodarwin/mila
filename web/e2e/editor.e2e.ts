import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, OWNER, pngBuffer } from './helpers';

test.use({ storageState: OWNER });

/** A workspace of its own with a project and one task; the task's page. */
async function seed(page: Page) {
	await page.goto('/');
	const ws: string = (await apiCall(page, 'POST', '/workspaces', { name: `Editor ${Date.now()}` }))
		.body.id;
	const make = async (entity: string, body: unknown) =>
		(await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, body)).body.id as string;
	const project = await make('projects', { name: 'Garden' });
	const status = await make('statuses', { project_id: project, name: 'To do', category: 'todo' });
	const task = await make('tasks', { project_id: project, status_id: status, title: 'Beds' });
	return { ws, project, task, url: `/projects/${project}?ws=${ws}&task=${task}` };
}

const panel = (page: Page) => page.getByRole('complementary', { name: 'Task details' });
const notes = (page: Page) => panel(page).getByLabel('Notes', { exact: true });
const blocks = (page: Page) => page.getByRole('listbox', { name: 'Insert a block' });
const notesMarkdown = async (page: Page, ws: string, task: string) =>
	(await apiCall(page, 'GET', `/workspaces/${ws}/tasks/${task}`)).body.notes as string;

test.describe('the editor', () => {
	test('draws each heading level in notes a step smaller than the one before', async ({ page }) => {
		const { ws, task, url } = await seed(page);
		await apiCall(page, 'PATCH', `/workspaces/${ws}/tasks/${task}`, {
			notes: ['# One', '', '## Two', '', '### Three', '', '#### Four', '', 'Text'].join('\n')
		});
		await page.goto(url);
		const size = (tag: string) =>
			notes(page)
				.locator(tag)
				.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
		await expect(notes(page).locator('h4')).toHaveText('Four');
		const [one, two, three, four] = [
			await size('h1'),
			await size('h2'),
			await size('h3'),
			await size('h4')
		];
		expect(one).toBeGreaterThan(two);
		expect(two).toBeGreaterThan(three);
		expect(three).toBeGreaterThan(four);
	});
	test.skip(({ isMobile }) => isMobile, 'the phone has its own checks');

	test('/ inserts checklists and tables, and the bar formats', async ({ page }) => {
		const { ws, task, url } = await seed(page);
		await page.goto(url);
		await notes(page).click();

		// / at the start of a line opens the menu, which narrows as you type.
		await page.keyboard.type('/');
		await expect(blocks(page)).toBeVisible();
		await page.keyboard.type('check');
		await expect(blocks(page).getByRole('option')).toHaveText(['Checklist']);
		await page.keyboard.press('Enter');
		await expect(blocks(page)).toHaveCount(0);
		await page.keyboard.type('Buy seeds');
		const box = notes(page).getByRole('checkbox', { name: 'Buy seeds' });
		await expect(box).not.toBeChecked();
		await box.check();

		// Escape closes the menu and leaves the / as typed.
		await page.keyboard.press('Control+End');
		await page.keyboard.press('Enter');
		await page.keyboard.press('Enter');
		await page.keyboard.type('/');
		await expect(blocks(page)).toBeVisible();
		await page.keyboard.press('Escape');
		await expect(blocks(page)).toHaveCount(0);
		await expect(panel(page)).toBeVisible();
		await page.keyboard.press('Backspace');

		// A table, its header row first.
		await page.keyboard.type('/table');
		await page.keyboard.press('Enter');
		await page.keyboard.type('Bed');
		await page.keyboard.press('Tab');
		await page.keyboard.type('Size');
		await expect(notes(page).getByRole('columnheader', { name: 'Size' })).toBeVisible();

		// Selecting text brings the bar; Bold says it is on.
		await page.keyboard.press('Shift+Home');
		const bar = page.getByRole('toolbar', { name: 'Formatting' });
		await expect(bar).toBeVisible();
		await expect(bar.getByRole('button', { name: 'Add a row' })).toBeVisible();
		await bar.getByRole('button', { name: 'Bold' }).click();
		await expect(bar.getByRole('button', { name: 'Bold' })).toHaveAttribute('aria-pressed', 'true');
		await expect(notes(page).locator('th strong')).toHaveText('Size');

		// Alt+F10 reaches the bar from the text, and Escape goes back.
		await page.keyboard.press('Alt+F10');
		await expect(bar.getByRole('button', { name: 'Bold' })).toBeFocused();
		await page.keyboard.press('ArrowRight');
		await expect(bar.getByRole('button', { name: 'Italic' })).toBeFocused();
		await page.keyboard.press('Escape');
		await expect(notes(page)).toBeFocused();
		await expect(panel(page)).toBeVisible();

		// The server keeps it all as Markdown.
		await expect.poll(() => notesMarkdown(page, ws, task)).toContain('- [x] Buy seeds');
		await expect.poll(() => notesMarkdown(page, ws, task)).toContain('| Bed | **Size** |');
		// Away from the text, the bar goes.
		await panel(page).getByRole('heading', { name: 'Notes' }).click();
		await expect(bar).toBeHidden();
		await axe(page);
	});

	test('notes show as Markdown, open wide, and hold boxes', async ({ page }) => {
		const { ws, task, url } = await seed(page);
		await page.goto(url);
		await notes(page).click();
		await page.keyboard.type('/tip');
		await page.keyboard.press('Enter');
		await page.keyboard.type('Water in the morning.');
		await expect(notes(page).getByRole('note', { name: 'Tip' })).toContainText(
			'Water in the morning.'
		);

		// The Markdown view edits the notes as text.
		await panel(page).getByRole('button', { name: 'Markdown' }).click();
		const source = panel(page).getByLabel('Notes as Markdown');
		await expect(source).toHaveValue('> [!TIP]\n> Water in the morning.');
		await source.fill('> [!WARNING]\n> Frost tonight.');
		await panel(page).getByRole('button', { name: 'Markdown' }).click();
		await expect(notes(page).getByRole('note', { name: 'Warning' })).toContainText(
			'Frost tonight.'
		);
		await expect.poll(() => notesMarkdown(page, ws, task)).toBe('> [!WARNING]\n> Frost tonight.');

		// Wide notes put the rest aside; Escape brings it back, and the task stays open.
		await panel(page).getByRole('button', { name: 'Open the notes wide' }).click();
		await expect(panel(page).getByRole('heading', { name: 'Checklist' })).toBeHidden();
		await notes(page).click();
		await page.keyboard.press('Escape');
		await expect(panel(page).getByRole('heading', { name: 'Checklist' })).toBeVisible();
		await expect(panel(page)).toBeVisible();
	});

	test('a page takes an image uploaded to it', async ({ page }) => {
		const { ws, project } = await seed(page);
		await page.goto(`/projects/${project}?ws=${ws}&tab=wiki`);
		const body = page.getByLabel('Page', { exact: true });
		await body.click();
		await page.keyboard.type('Plan:');
		await page.keyboard.press('Enter');
		await page.keyboard.type('/ima');
		await page.keyboard.press('Enter');
		const picker = page.getByRole('dialog', { name: 'Insert an image' });
		await expect(picker).toBeVisible();
		await expect(picker).toContainText('This page has no images yet');
		await axe(page);
		await picker.getByLabel('Upload an image').setInputFiles({
			name: 'north-bed.png',
			mimeType: 'image/png',
			buffer: pngBuffer(120, 80)
		});
		await expect(picker).toBeHidden();
		const img = body.getByRole('img', { name: 'north-bed' });
		await expect(img).toBeVisible();
		await expect.poll(() => img.evaluate((i: HTMLImageElement) => i.naturalWidth)).toBe(120);

		// The file is the page's, and the page's Markdown points at it.
		const files = (await apiCall(page, 'GET', `/workspaces/${ws}/page-attachments`)).body.items;
		expect(files.map((f: { filename: string }) => f.filename)).toEqual(['north-bed.png']);
		await expect
			.poll(async () => (await apiCall(page, 'GET', `/workspaces/${ws}/pages`)).body.items[0].body)
			.toContain(`![north-bed](/api/v1/workspaces/${ws}/attachments/${files[0].id}/file)`);
	});
});
