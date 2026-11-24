// Drawings in wiki pages: one made from the / menu opens full screen, takes
// shapes, words and arrows, moves by keyboard, and closes into the page,
// which keeps a marker and a picture of it in its Markdown.
import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, OWNER } from './helpers';

test.use({ storageState: OWNER });

async function seed(page: Page) {
	await page.goto('/');
	const ws: string = (
		await apiCall(page, 'POST', '/workspaces', { name: `Drawings ${Date.now()}` })
	).body.id;
	return { ws };
}

const body = (page: Page) => page.getByLabel('Page', { exact: true });

async function shapes(page: Page, ws: string) {
	const res = await apiCall(page, 'GET', `/workspaces/${ws}/drawing-shapes?limit=200`);
	return (
		res.body.items as {
			id: string;
			kind: string;
			text: string;
			box: { x: number };
			deleted_at: string | null;
		}[]
	).filter((s) => !s.deleted_at);
}

test.describe('drawings', () => {
	test('a drawing from the / menu: shapes, words, keys, and its picture in the page', async ({
		page,
		isMobile
	}) => {
		test.skip(isMobile, 'drawn with a mouse at the desktop width; the phone has its own test');
		const { ws } = await seed(page);
		await page.goto(`/wiki?ws=${ws}`);
		await expect(
			page.getByRole('heading', { name: 'Workspace Wiki Main page', level: 1 })
		).toBeVisible();
		await body(page).click();
		await page.keyboard.type('/draw');
		await page.getByRole('option', { name: 'Drawing' }).click();
		const picker = page.getByRole('dialog', { name: 'Insert a drawing' });
		await picker.getByRole('button', { name: /Flowchart/ }).click();

		// It opens full screen with the starter's shapes.
		const editor = page.getByRole('dialog', { name: 'Edit Flowchart' });
		await expect(editor).toBeVisible();
		const stage = editor.getByRole('application');
		await expect(stage.getByRole('button', { name: 'Diamond: Approved?' })).toBeVisible();
		await expect.poll(async () => (await shapes(page, ws)).length).toBe(7);
		if (process.env.LOOK) await page.screenshot({ path: `${process.env.LOOK}/drawing-editor.png` });

		// A box, drawn with the Box tool, and its words.
		await editor
			.getByRole('toolbar', { name: 'Tools' })
			.getByRole('button', { name: 'Box' })
			.click();
		const area = (await stage.boundingBox())!;
		await page.mouse.click(area.x + area.width * 0.2, area.y + area.height * 0.85);
		await expect.poll(async () => (await shapes(page, ws)).length).toBe(8);
		const box = stage.getByRole('button', { name: 'Box', exact: true });
		await box.dblclick();
		const words = editor.getByLabel('Words in the shape');
		await expect(words).toBeVisible();
		await words.fill('Archive');
		await words.press('Enter');
		await expect(stage.getByRole('button', { name: 'Box: Archive' })).toBeVisible();

		// The keys move what is picked, and delete it.
		const picked = stage.getByRole('button', { name: 'Box: Archive' });
		await picked.focus();
		await expect
			.poll(async () => (await shapes(page, ws)).some((s) => s.text === 'Archive'))
			.toBe(true);
		const before = (await shapes(page, ws)).find((s) => s.text === 'Archive')!.box.x;
		await page.keyboard.press('ArrowRight');
		await expect
			.poll(async () => (await shapes(page, ws)).find((s) => s.text === 'Archive')!.box.x)
			.toBe(before + 8);

		// The side panel restyles it.
		await editor.getByRole('button', { name: 'Green fill' }).click();
		if (process.env.LOOK) await page.screenshot({ path: `${process.env.LOOK}/drawing-picked.png` });
		await expect
			.poll(async () => {
				const s = (await shapes(page, ws)).find((x) => x.text === 'Archive') as unknown as {
					style: { fill?: string };
				};
				return s.style.fill;
			})
			.toBe('green');

		// Undo takes the color back, and redo brings it again.
		await editor.getByRole('button', { name: 'Undo' }).click();
		await expect
			.poll(async () => {
				const s = (await shapes(page, ws)).find((x) => x.text === 'Archive') as unknown as {
					style: { fill?: string };
				};
				return s.style.fill ?? null;
			})
			.toBe(null);
		await editor.getByRole('button', { name: 'Redo' }).click();

		await picked.focus();
		await page.keyboard.press('Delete');
		await expect.poll(async () => (await shapes(page, ws)).length).toBe(7);

		await axe(page);

		// Done closes it into the page, drawn in the theme's colors.
		await editor.getByRole('button', { name: 'Done' }).click();
		await expect(editor).toBeHidden();
		const drawn = body(page).getByRole('img', { name: 'Flowchart' });
		await expect(drawn).toBeVisible();
		if (process.env.LOOK) await page.screenshot({ path: `${process.env.LOOK}/drawing-page.png` });

		// The page keeps a marker and a picture of it.
		await page.getByRole('button', { name: 'Markdown' }).click();
		const md = page.getByLabel('Page as Markdown');
		await expect(md).toHaveValue(
			/<!-- mila:drawing [0-9a-f-]{36} -->\n!\[Flowchart\]\(\/api\/v1\/workspaces\/[0-9a-f-]{36}\/attachments\/[0-9a-f-]{36}\/file\)/,
			{ timeout: 15_000 }
		);
	});

	test('anyone who can read the page sees it full screen', async ({ page }) => {
		const { ws } = await seed(page);
		await page.goto(`/wiki?ws=${ws}`);
		await body(page).click();
		await page.keyboard.type('/draw');
		await page.getByRole('option', { name: 'Drawing' }).click();
		await page
			.getByRole('dialog', { name: 'Insert a drawing' })
			.getByRole('button', { name: /Architecture/ })
			.click();
		const editor = page.getByRole('dialog', { name: 'Edit Architecture' });
		await expect(editor).toBeVisible();
		if (process.env.LOOK) {
			await page.waitForTimeout(500);
			await page.screenshot({
				path: `${process.env.LOOK}/drawing-arch-${test.info().project.name}.png`
			});
		}
		await editor.getByRole('button', { name: 'Done' }).click();
		await expect(editor).toBeHidden();
		const drawing = body(page).locator('.drawing-embed');
		await drawing.hover();
		await drawing.getByRole('button', { name: 'Full screen' }).click();
		const view = page.getByRole('dialog', { name: 'Architecture' });
		await expect(view).toBeVisible();
		await expect(view.getByRole('img', { name: 'Cylinder: Database' })).toBeAttached();
		await view.getByRole('button', { name: 'Close' }).click();
		await expect(view).toBeHidden();
	});

	test("a task's notes offer no drawing", async ({ page }) => {
		const { ws } = await seed(page);
		const project = (await apiCall(page, 'POST', `/workspaces/${ws}/projects`, { name: 'P' })).body
			.id;
		const status = (
			await apiCall(page, 'POST', `/workspaces/${ws}/statuses`, {
				project_id: project,
				name: 'To do',
				category: 'todo'
			})
		).body.id;
		const task = (
			await apiCall(page, 'POST', `/workspaces/${ws}/tasks`, {
				project_id: project,
				status_id: status,
				title: 'T'
			})
		).body.id;
		await page.goto(`/projects/${project}?ws=${ws}&task=${task}`);
		const notes = page
			.getByRole('complementary', { name: 'Task details' })
			.getByLabel('Notes', { exact: true });
		await notes.click();
		await page.keyboard.type('/dra');
		await expect(page.getByRole('option', { name: 'Diagram', exact: true })).toBeVisible();
		await expect(page.getByRole('option', { name: 'Drawing' })).toHaveCount(0);
	});
});

test('dragging a shape drags only the shape, never the block the drawing sits in', async ({
	page,
	isMobile
}) => {
	test.skip(isMobile, 'dragged with a mouse at the desktop width');
	const { ws } = await seed(page);
	await page.goto(`/wiki?ws=${ws}`);
	await body(page).click();
	await page.keyboard.type('/draw');
	await page.getByRole('option', { name: 'Drawing' }).click();
	await page
		.getByRole('dialog', { name: 'Insert a drawing' })
		.getByRole('button', { name: /Flowchart/ })
		.click();
	const editor = page.getByRole('dialog', { name: 'Edit Flowchart' });
	const diamond = editor
		.getByRole('application')
		.getByRole('button', { name: 'Diamond: Approved?' });
	await expect(diamond).toBeVisible();
	await expect.poll(async () => (await shapes(page, ws)).length).toBe(7);

	// The browser's own drag (a picture of the page's block following the
	// pointer) never goes ahead from inside the editor: any that starts there
	// is canceled before it reaches the window.
	await page.evaluate(() => {
		(window as unknown as { drags: number }).drags = 0;
		window.addEventListener('dragstart', (e) => {
			if (!e.defaultPrevented) (window as unknown as { drags: number }).drags++;
		});
	});
	const before = (await shapes(page, ws)).find((s) => s.text === 'Approved?')!.box.x;
	const at = (await diamond.boundingBox())!;
	await page.mouse.move(at.x + at.width / 2, at.y + at.height / 2);
	await page.mouse.down();
	await page.mouse.move(at.x + at.width / 2 + 60, at.y + at.height / 2 + 10, { steps: 8 });
	await page.mouse.move(at.x + at.width / 2 + 120, at.y + at.height / 2 + 20, { steps: 8 });
	await page.mouse.up();
	expect(await page.evaluate(() => (window as unknown as { drags: number }).drags)).toBe(0);
	await expect
		.poll(async () => (await shapes(page, ws)).find((s) => s.text === 'Approved?')!.box.x)
		.toBeGreaterThan(before + 60);

	// Chromium under test never starts that drag from a pointer (Firefox and
	// WebKit, the desktop app's engine on Linux, do), so the block is also
	// asked directly: a drag of it is canceled while the editor is open.
	expect(
		await page.evaluate(() => {
			const drag = new DragEvent('dragstart', { bubbles: true, cancelable: true });
			document.querySelector('.drawing-embed')!.dispatchEvent(drag);
			return drag.defaultPrevented;
		})
	).toBe(true);
});
