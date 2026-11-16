import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, OWNER } from './helpers';

test.use({ storageState: OWNER });

const day = (n: number) => {
	const d = new Date();
	d.setDate(d.getDate() + n);
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
};

/** A project of dated tasks made through the API, shown as a timeline. */
async function seed(page: Page, name: string) {
	// Made in the app, so it has the starter statuses.
	await page.goto('/projects');
	await page.getByLabel('Name a new project').fill(name);
	await page.getByLabel('Name a new project').press('Enter');
	await expect(page.getByLabel('Project name')).toHaveValue(name);
	const project = new URL(page.url()).pathname.split('/').pop()!;
	// The workspace it is in, once the app has sent it and its statuses.
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
				if (found) {
					W = `/workspaces/${w.id}`;
					todo = found.id;
				}
			}
			return todo;
		})
		.not.toBe('');
	const task = async (title: string, fields: Record<string, unknown>) => {
		const res = await apiCall(page, 'POST', `${W}/tasks`, {
			title,
			project_id: project,
			status_id: todo,
			...fields
		});
		expect(res.status).toBe(201);
		return res.body.id as string;
	};
	const ids = {
		drywall: await task('Drywall', { start_date: day(1), due_date: day(3) }),
		paint: await task('Paint', { start_date: day(5), due_date: day(6) }),
		inspect: await task('Inspection', { due_date: day(2) }),
		colors: await task('Pick colors', {})
	};
	await apiCall(page, 'POST', `${W}/task-dependencies`, {
		task_id: ids.paint,
		blocker_id: ids.drywall
	});
	await page.goto(`/projects/${project}?view=timeline`);
	await expect(page.getByRole('list', { name: 'Tasks' })).toBeVisible();
	return { W, project, ids };
}

const taskOf = async (page: Page, W: string, id: string) =>
	(await apiCall(page, 'GET', `${W}/tasks/${id}`)).body as {
		start_date: string | null;
		due_date: string | null;
	};

test('a timeline moves tasks, links them and schedules the tray', async ({ page }, info) => {
	test.skip(info.project.name === 'phone', 'the dots and the title column are for a desktop');
	const { W, ids } = await seed(page, `Timeline ${Date.now()}`);
	const rows = page.getByRole('list', { name: 'Tasks' });

	// Bars and a diamond, an arrow, and the undated task in the tray.
	await expect(page.locator(`[data-bar="${ids.drywall}"]`)).toHaveClass(/bar/);
	await expect(page.locator(`[data-bar="${ids.inspect}"]`)).toHaveClass(/mark/);
	await expect(page.locator('svg.arrows path.link')).toHaveCount(1);
	const tray = page.getByRole('region', { name: 'Unscheduled' });
	await expect(tray.getByRole('button', { name: /Pick colors/ })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Week', exact: true })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
	await axe(page);

	// The keyboard: a week later at Week zoom, then its due date a day at Day zoom.
	const drywall = rows.getByRole('button', { name: /^Drywall, / });
	await drywall.focus();
	await page.keyboard.press('Alt+ArrowRight');
	await expect.poll(async () => (await taskOf(page, W, ids.drywall)).start_date).toBe(day(8));
	expect((await taskOf(page, W, ids.drywall)).due_date).toBe(day(10));
	// The drywall now ends after the paint starts: offered to move it too.
	await page.getByRole('button', { name: 'Move the 1 task waiting on it too' }).click();
	await expect.poll(async () => (await taskOf(page, W, ids.paint)).start_date).toBe(day(12));
	await expect(page.locator('svg.arrows path.link.bad')).toHaveCount(0);

	await page.getByRole('button', { name: 'Day', exact: true }).click();
	await drywall.focus();
	await page.keyboard.press('Alt+Shift+ArrowRight');
	await expect.poll(async () => (await taskOf(page, W, ids.drywall)).due_date).toBe(day(11));
	// Due the day the paint starts: the arrow turns red.
	await page.keyboard.press('Alt+Shift+ArrowRight');
	await expect.poll(async () => (await taskOf(page, W, ids.drywall)).due_date).toBe(day(12));
	await expect(page.locator('svg.arrows path.link.bad')).toHaveCount(1);

	// Dragging a bar two days earlier, at 40 pixels a day.
	const bar = page.locator(`[data-bar="${ids.paint}"]`);
	const box = (await bar.boundingBox())!;
	await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
	await page.mouse.down();
	await page.mouse.move(box.x + box.width / 2 - 40, box.y + box.height / 2, { steps: 5 });
	await page.mouse.move(box.x + box.width / 2 - 80, box.y + box.height / 2, { steps: 5 });
	await page.mouse.up();
	await expect.poll(async () => (await taskOf(page, W, ids.paint)).start_date).toBe(day(10));

	// The dot after the inspection, dropped on the drywall, makes the drywall wait.
	const mark = page.locator(`[data-bar="${ids.inspect}"]`);
	await mark.hover();
	const dot = mark.locator('[data-part="dot-after"]');
	const from = (await dot.boundingBox())!;
	const to = (await page.locator(`[data-bar="${ids.drywall}"]`).boundingBox())!;
	await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
	await page.mouse.down();
	await page.mouse.move(to.x + to.width / 2, to.y + to.height / 2, { steps: 8 });
	await page.mouse.up();
	await expect(page.getByText('Drywall now waits for Inspection').first()).toBeVisible();
	await expect(page.locator('svg.arrows path.link')).toHaveCount(2);

	// Choosing a tray task makes it due today, and it takes a row.
	await tray.getByRole('button', { name: /Pick colors/ }).press('Enter');
	await expect.poll(async () => (await taskOf(page, W, ids.colors)).due_date).toBe(day(0));
	await expect(rows.getByRole('button', { name: /^Pick colors, / })).toBeFocused();
	await expect(tray.getByText('Every task here has a date.')).toBeVisible();

	// Enter opens the task.
	await page.keyboard.press('Enter');
	await expect(page.getByLabel('Task title')).toHaveValue('Pick colors');
});

test('a saved timeline keeps its zoom, and a phone shows titles on the bars', async ({
	page
}, info) => {
	const { ids } = await seed(page, `Saved timeline ${Date.now()}`);
	if (info.project.name === 'phone') {
		await expect(page.getByRole('button', { name: 'Day', exact: true })).toHaveCount(0);
		await expect(page.locator(`[data-bar="${ids.drywall}"]`)).toContainText('Drywall');
		await axe(page);
		return;
	}
	await page.getByRole('button', { name: 'Save as a view' }).click();
	await expect(page).toHaveURL(/\/views\//);
	await expect(page.getByRole('button', { name: 'Timeline', exact: true })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
	await page.getByRole('button', { name: 'Month', exact: true }).click();
	await expect(page.getByRole('button', { name: 'Month', exact: true })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
	await page.reload();
	await expect(page.getByRole('button', { name: 'Month', exact: true })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
	await axe(page);
});
