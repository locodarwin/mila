// Diagrams: a code block in the language mermaid, drawn in a wiki page and a
// task's notes. Inserted from the `/` menu, its source shows beside or under
// the drawing while the caret is in it, a mistake keeps the last drawing, and
// the page keeps it as a fenced code block.
import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, OWNER } from './helpers';

test.use({ storageState: OWNER });

async function seed(page: Page) {
	await page.goto('/');
	const ws: string = (
		await apiCall(page, 'POST', '/workspaces', { name: `Diagrams ${Date.now()}` })
	).body.id;
	const make = async (entity: string, body: unknown) =>
		(await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, body)).body.id as string;
	const project = await make('projects', { name: 'Launch' });
	const status = await make('statuses', { project_id: project, name: 'To do', category: 'todo' });
	const task = await make('tasks', {
		project_id: project,
		status_id: status,
		title: 'Map the sign-in',
		notes:
			'How it goes:\n\n```mermaid\nsequenceDiagram\n  accTitle: Sign-in\n  User->>App: Email\n  App-->>User: Link\n```\n'
	});
	return { ws, project, task };
}

const body = (page: Page) => page.getByLabel('Page', { exact: true });

test.describe('diagrams', () => {
	test('a flowchart from the / menu, edited in place and kept as Markdown', async ({
		page,
		isMobile
	}) => {
		const { ws } = await seed(page);
		await page.goto(`/wiki?ws=${ws}`);
		await expect(
			page.getByRole('heading', { name: 'Workspace Wiki Main page', level: 1 })
		).toBeVisible();
		await body(page).click();
		await page.keyboard.type('Before it.');
		await page.keyboard.press('Enter');

		// Typing for a kind finds it in the / menu.
		await page.keyboard.type('/flow');
		await expect(page.getByRole('option', { name: 'Flowchart' })).toBeVisible();
		await page.getByRole('option', { name: 'Flowchart' }).click();
		const block = body(page).locator('.diagram-block');
		await expect(block).toHaveCount(1);
		const drawing = block.getByRole('img', { name: 'Request flow' });
		await expect(drawing).toBeVisible({ timeout: 15_000 });
		await expect(drawing.locator('svg')).toBeVisible();
		// The caret is in it, so its source shows.
		const source = block.locator('.diagram-source');
		await expect(source).toBeVisible();
		await expect(source).toContainText('A[Request] --> B{Approved?}');

		// A mistake keeps the last drawing and says where it is.
		await page.keyboard.type('\n  D --> E{');
		await expect(block.getByText(/Error on line \d+/)).toBeVisible({ timeout: 10_000 });
		if (process.env.LOOK)
			await block.screenshot({
				path: `${process.env.LOOK}/mistake-${isMobile ? 'phone' : 'wide'}.png`
			});
		await expect(drawing).toBeVisible();
		await page.keyboard.press('Backspace');
		await page.keyboard.type('[Done]');
		await expect(block.getByText(/Error on line/)).toBeHidden({ timeout: 10_000 });
		await expect(drawing.locator('svg')).toContainText('Done');
		if (process.env.LOOK)
			await block.screenshot({
				path: `${process.env.LOOK}/editing-${isMobile ? 'phone' : 'wide'}.png`
			});

		// Escape leaves the source; the drawing stays, with Edit to go back.
		await page.keyboard.press('Escape');
		await expect(source).toBeHidden();
		await block.getByRole('button', { name: 'Edit' }).click();
		await expect(source).toBeVisible();
		await block.getByRole('button', { name: 'Done' }).click();
		await expect(source).toBeHidden();

		if (!isMobile) await axe(page);

		// Full screen, and back.
		await block.getByRole('button', { name: 'Full screen' }).click();
		const full = page.getByRole('dialog', { name: 'Request flow' });
		await expect(full).toBeVisible();
		await full.getByRole('button', { name: 'Zoom in' }).click();
		await expect(full.getByText('125%')).toBeVisible();
		if (process.env.LOOK)
			await page.screenshot({
				path: `${process.env.LOOK}/full-${isMobile ? 'phone' : 'wide'}.png`
			});
		await full.getByRole('button', { name: 'Close' }).click();
		await expect(full).toBeHidden();

		// The page keeps it as a fenced code block.
		await page.getByRole('button', { name: 'Markdown' }).click();
		const md = page.getByLabel('Page as Markdown');
		await expect(md).toHaveValue(
			/Before it\.\n\n```mermaid\nflowchart LR\n {2}accTitle: Request flow\n/
		);
		await expect(md).toHaveValue(/D --> E\[Done\]\n```/);
	});

	test('the Diagram block asks which kind', async ({ page }) => {
		const { ws } = await seed(page);
		await page.goto(`/wiki?ws=${ws}`);
		await body(page).click();
		await page.keyboard.type('/diagram');
		await page.getByRole('option', { name: 'Diagram', exact: true }).click();
		const picker = page.getByRole('dialog', { name: 'Insert a diagram' });
		await expect(picker).toBeVisible();
		await picker.getByRole('button', { name: /Sequence diagram/ }).click();
		await expect(picker).toBeHidden();
		await expect(body(page).getByRole('img', { name: 'Sign-in sequence' })).toBeVisible({
			timeout: 15_000
		});
	});

	test("a task's notes draw a diagram too", async ({ page }) => {
		const { ws, project, task } = await seed(page);
		await page.goto(`/projects/${project}?ws=${ws}&task=${task}`);
		const notes = page
			.getByRole('complementary', { name: 'Task details' })
			.getByLabel('Notes', { exact: true });
		await expect(notes).toContainText('How it goes:');
		await expect(notes.getByRole('img', { name: 'Sign-in' })).toBeVisible({ timeout: 15_000 });
		// Its source shows only while the caret is in it.
		await expect(notes.locator('.diagram-source')).toBeHidden();
	});
});
