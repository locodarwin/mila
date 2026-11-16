import { expect, test, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { axe } from './axe';
import { apiCall, OWNER } from './helpers';

test.use({ storageState: OWNER });

const DAY = 24 * 3600 * 1000;
/** A moment so many days ago. */
const ago = (days: number) => new Date(Date.now() - days * DAY).toISOString();
/** A day so many days from today, as tasks keep them, where the browser is. */
const day = (n: number) => {
	const d = new Date(Date.now() + n * DAY);
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
};

/**
 * A workspace of its own, so nothing other tests make is counted: a project
 * with tasks made and done over the weeks before today. The server keeps
 * when each was made from the create, which may say so once.
 */
async function seed(page: Page) {
	// A page of the server's with no app on it: with the app open, it could load
	// the new workspace part way through seeding, and the Reports page would then
	// start from that copy and catch up only after the test had looked.
	await page.goto('/healthz');
	const ws: string = (await apiCall(page, 'POST', '/workspaces', { name: `Reports ${Date.now()}` }))
		.body.id;
	const W = `/workspaces/${ws}`;
	const make = async (entity: string, body: unknown) => {
		const res = await apiCall(page, 'POST', `${W}/${entity}`, body);
		expect(res.status, JSON.stringify(res.body)).toBeLessThan(300);
		return res.body.id as string;
	};
	const me = (await apiCall(page, 'GET', '/me')).body.id as string;
	const project = await make('projects', { name: 'Kitchen' });
	const [todo, doing, done] = await Promise.all(
		[
			['To do', 'todo', 'a0'],
			['Cooking', 'in-progress', 'a1'],
			['Done', 'done', 'a2']
		].map(([name, category, sort_key]) =>
			make('statuses', { project_id: project, name, category, sort_key })
		)
	);
	const task = (title: string, fields: Record<string, unknown>) =>
		make('tasks', { project_id: project, status_id: todo, title, ...fields });

	// Done three weeks ago, never in progress: left out of the cycle time.
	await task('Clear the cabinets', {
		status_id: done,
		made_at: ago(30),
		completed_at: ago(20)
	});
	const late = await task('Order the tiles', {
		made_at: ago(10),
		due_date: day(-3),
		estimate_minutes: 60
	});
	await make('task-assignees', { task_id: late, user_id: me });
	const waits = await task('Grout the tiles', { made_at: ago(5), estimate_minutes: 120 });
	await make('task-dependencies', { task_id: waits, blocker_id: late });
	const over = await task('Patch the plaster', { made_at: ago(4), estimate_minutes: 30 });
	await make('time-entries', { task_id: over, user_id: me, day: day(0), minutes: 60 });
	await task('Choose the handles', { made_at: ago(2) });
	// Started now, and done now: under a day from start to done.
	const quick = await task('Fit the sink', { status_id: doing });
	await apiCall(page, 'PATCH', `${W}/tasks/${quick}`, {
		status_id: done,
		completed_at: new Date().toISOString()
	});
	return { ws, project };
}

test('Reports count status, workload and throughput, filter them and download CSV', async ({
	page,
	isMobile
}) => {
	const { ws } = await seed(page);
	await page.goto(`/reports?ws=${ws}`);
	await expect(page.getByRole('heading', { name: 'Reports', level: 1 })).toBeVisible();
	const tabs = page.getByRole('tablist', { name: 'Reports' });
	await expect(tabs.getByRole('tab', { name: 'Project status' })).toHaveAttribute(
		'aria-selected',
		'true'
	);
	const panel = page.getByRole('tabpanel');

	// The tiles: four open, two done of six, one overdue.
	await expect(panel.getByText('Done of 6')).toBeVisible();
	await expect(panel.locator('.tile').first()).toContainText('4');
	await expect(panel.locator('.tile.alert')).toContainText('1');

	// What needs attention, the latest first.
	const attention = panel.getByRole('region', { name: 'Needs attention' });
	const items = attention.getByRole('listitem');
	await expect(items).toHaveCount(3);
	await expect(items.nth(0)).toContainText('Order the tiles');
	await expect(items.nth(0)).toContainText('3 days late');
	await expect(items.nth(1)).toContainText('Waiting on 1');
	await expect(items.nth(2)).toContainText('30m over');

	// A chart as a table, and back.
	const burnup = panel.getByRole('region', { name: 'Burnup' });
	await burnup.getByRole('button', { name: 'Show as table' }).click();
	const table = burnup.getByRole('region', { name: 'Burnup, as a table' });
	await expect(table.getByRole('row')).toHaveCount(7);
	await expect(table.getByRole('row').last()).toContainText(/6\s*2/);
	await burnup.getByRole('button', { name: 'Show as table' }).click();

	// The keyboard steps through the burnup, saying each week.
	if (!isMobile) {
		const chart = burnup.getByRole('group', { name: /^Burnup/ });
		await chart.focus();
		await page.keyboard.press('End');
		await expect(burnup.locator('.tip')).toContainText('All tasks');
		await expect(burnup.locator('.tip')).toContainText('6 tasks');
	}
	await axe(page);

	// Opening a task from the list.
	await items.nth(0).getByRole('button').click();
	await expect(page).toHaveURL(/task=/);
	await page.goto(`/reports?ws=${ws}`);

	// Workload: one person with an hour, and the unassigned last.
	await tabs.getByRole('tab', { name: 'Workload' }).click();
	const load = panel.getByRole('region', { name: 'Open work by person' });
	await expect(load).toContainText('Tasks with no estimate count as tasks but add no hours: 1');
	await load.getByRole('button', { name: 'Show as table' }).click();
	const rows = load
		.getByRole('region', { name: 'Open work by person, as a table' })
		.getByRole('row');
	await expect(rows).toHaveCount(3);
	await expect(rows.nth(1)).toContainText(/1\s*1\s*1/);
	await expect(rows.nth(2)).toContainText('Unassigned');
	await expect(panel.getByRole('region', { name: 'Due by week' })).toContainText(
		'The 3 open tasks with no due date are not counted here.'
	);
	await axe(page);

	// Throughput and cycle time.
	await tabs.getByRole('tab', { name: 'Throughput and cycle time' }).click();
	await expect(panel.getByText('under an hour').first()).toBeVisible();
	await expect(panel).toContainText('One task done without ever going in progress is left out.');
	await axe(page);

	// The tab's tables as CSV.
	const download = page.waitForEvent('download');
	await page.getByRole('button', { name: 'Download CSV' }).click();
	const file = await download;
	expect(file.suggestedFilename()).toMatch(/^Reports, Throughput and cycle time, .* to .*\.csv$/);
	const csv = readFileSync((await file.path())!, 'utf8');
	expect(csv).toContain('Made and done each week\r\nWeek of,Made,Done\r\n');
	expect(csv).toContain('How long tasks took\r\nTook,Tasks\r\nUnder a day,1\r\n');

	// The filter narrows every tab; one that matches nothing says so.
	await page.getByRole('button', { name: 'Filter' }).click();
	const words = page.getByLabel('Words in the title or notes');
	await words.fill('tiles');
	await words.press('Enter');
	await tabs.getByRole('tab', { name: 'Project status' }).click();
	await expect(panel.getByText('Done of 2')).toBeVisible();
	await expect(page.getByRole('checkbox', { name: 'Show finished tasks' })).toHaveCount(0);
	await words.fill('nothing like this');
	await words.press('Enter');
	await expect(panel.getByRole('heading', { name: 'No tasks match this filter' })).toBeVisible();

	// The tab, filter and period are kept on this device.
	await page.getByRole('combobox', { name: 'Period' }).selectOption('year');
	await page.reload();
	await expect(panel.getByRole('heading', { name: 'No tasks match this filter' })).toBeVisible();
	await expect(page.getByRole('combobox', { name: 'Period' })).toHaveValue('year');
	await panel.getByRole('button', { name: 'Clear the filter' }).click();
	await expect(panel.getByText('Done of 6')).toBeVisible();
});

test('Reports show one project alone, with its own statuses, from its menu', async ({
	page,
	isMobile
}) => {
	const { ws, project } = await seed(page);
	const W = `/workspaces/${ws}`;
	const make = async (entity: string, body: unknown) =>
		(await apiCall(page, 'POST', `${W}/${entity}`, body)).body.id as string;
	// A second project, with a label on one of the first's tasks.
	const garden = await make('projects', { name: 'Garden' });
	const sow = await make('tasks', { project_id: garden, title: 'Sow the beans', priority: 'high' });
	const label = await make('labels', { name: 'Tiling', color: 'green' });
	const tiles = (await apiCall(page, 'GET', `${W}/tasks?limit=50`)).body.items.find(
		(t: { title: string }) => t.title === 'Order the tiles'
	).id;
	await make('task-labels', { task_id: tiles, label_id: label });
	expect(sow).toBeTruthy();

	await page.goto(`/reports?ws=${ws}`);
	const panel = page.getByRole('tabpanel');
	// All projects: statuses by kind, and the open work of each project.
	await expect(panel.getByText('Done of 7')).toBeVisible();
	const byProject = panel.getByRole('region', { name: 'Open work by project' });
	await byProject.getByRole('button', { name: 'Show as table' }).click();
	const projectRows = byProject.getByRole('row');
	await expect(projectRows.nth(1)).toContainText(/Kitchen\s*4/);
	await expect(projectRows.nth(2)).toContainText(/Garden\s*1/);
	await expect(panel.getByRole('region', { name: 'Tasks by status' })).toContainText(
		'by kind of status'
	);

	// One project, from its menu (or its address on a phone).
	if (isMobile) await page.goto(`/reports?ws=${ws}&project=${project}`);
	else {
		await page
			.getByRole('navigation', { name: 'Main' })
			.getByRole('link', { name: 'Kitchen' })
			.click({ button: 'right' });
		await page.getByRole('menuitem', { name: 'Open its reports' }).click();
	}
	await expect(page).toHaveURL(new RegExp(`project=${project}`));
	await expect(page.getByRole('combobox', { name: 'Project' })).toHaveValue(project);
	await expect(panel.getByText('Done of 6')).toBeVisible();
	await expect(panel.getByRole('region', { name: 'Open work by project' })).toHaveCount(0);
	const shares = panel.getByRole('region', { name: 'Tasks by status' });
	await expect(shares).toContainText("Kitchen's statuses");
	await shares.getByRole('button', { name: 'Show as table' }).click();
	const statusRows = shares.getByRole('row');
	await expect(statusRows.nth(1)).toContainText(/To do\s*4\s*67%/);
	await expect(statusRows.nth(2)).toContainText(/Done\s*2\s*33%/);
	const late = panel.getByRole('region', { name: 'How overdue' });
	await late.getByRole('button', { name: 'Show as table' }).click();
	await expect(late.getByRole('row').nth(1)).toContainText(/Under a week late\s*1/);
	await expect(panel.getByRole('region', { name: 'Open work by label' })).toContainText('Tiling');
	await axe(page);

	// The flow by day, and each task's time.
	await page.getByRole('tab', { name: 'Throughput and cycle time' }).click();
	const flow = panel.getByRole('region', { name: 'Cumulative flow' });
	await flow.getByRole('button', { name: 'Show as table' }).click();
	await expect(flow.getByRole('row').last()).toContainText(/4\s*0\s*2$/);
	const dots = panel.getByRole('region', { name: 'Cycle time, task by task' });
	await dots.getByRole('button', { name: 'Show as table' }).click();
	await expect(dots.getByRole('row')).toHaveCount(2);
	await expect(dots.getByRole('row').nth(1)).toContainText('Fit the sink');
	await axe(page);

	// The CSV is named for the project, and the choice is kept on this device.
	const download = page.waitForEvent('download');
	await page.getByRole('button', { name: 'Download CSV' }).click();
	expect((await download).suggestedFilename()).toMatch(/^Reports, Kitchen, Throughput/);
	await page.goto(`/reports?ws=${ws}`);
	await expect(page.getByRole('combobox', { name: 'Project' })).toHaveValue(project);
	await page.getByRole('combobox', { name: 'Project' }).selectOption('');
	await expect(page).not.toHaveURL(/project=/);
	await page.getByRole('tab', { name: 'Project status' }).click();
	await expect(panel.getByText('Done of 7')).toBeVisible();
});

test('A report saved as a view keeps its setup, and its charts go in a page', async ({
	page,
	isMobile
}) => {
	const { ws, project } = await seed(page);
	await page.goto(`/reports?ws=${ws}&project=${project}`);
	await page.getByRole('button', { name: 'Save as view' }).click();
	await expect(page).toHaveURL(/\/views\/[0-9a-f-]{36}/);
	await expect(page.getByLabel('View name')).toHaveValue('Kitchen report');
	await expect(page.locator('header .scope')).toHaveText('Kitchen · Only you');
	// A report of one project reports on that project alone, with no choice of another.
	await expect(page.getByRole('region', { name: 'Burnup' })).toBeVisible();
	await expect(page.getByRole('combobox', { name: 'Project' })).toHaveCount(0);

	// What changes on it is saved to it.
	await page.getByRole('tab', { name: 'Throughput and cycle time' }).click();
	const id = () => page.url().match(/\/views\/([0-9a-f-]{36})/)![1];
	await expect
		.poll(
			async () =>
				(await apiCall(page, 'GET', `/workspaces/${ws}/saved-views/${id()}`)).body.display?.report
					?.tab
		)
		.toBe('flow');
	await page.reload();
	await expect(page.getByRole('tab', { name: 'Throughput and cycle time' })).toHaveAttribute(
		'aria-selected',
		'true'
	);
	await expect(page.getByRole('region', { name: 'Made and done each week' })).toBeVisible();
	await axe(page);

	// Shared, it can go in a page.
	await page.getByRole('button', { name: 'Share with the workspace' }).click();
	await expect(page.locator('header .scope')).toHaveText('Kitchen · Shared');
	if (isMobile) return; // the formatting bar is tried at the desktop width

	await page.goto(`/wiki?ws=${ws}`);
	const body = page.getByLabel('Page', { exact: true });
	await body.click();
	await page.keyboard.type('Where the kitchen stands.');
	await page.keyboard.press('Enter');
	await page.keyboard.type('/report');
	await page.keyboard.press('Enter');
	await page
		.getByRole('dialog', { name: 'Show a report here' })
		.getByRole('button', { name: /Kitchen report/ })
		.click();
	// It opens on the report's own tab, with its first chart.
	const block = body.getByRole('region', { name: /from Kitchen report, a report$/ });
	await expect(block.getByRole('region', { name: 'Made and done each week' })).toBeVisible();
	await expect(block.getByRole('link', { name: 'Kitchen report' })).toBeVisible();
	await expect(block.getByText('Counts only the tasks you can see.')).toBeVisible();

	// Chart, in the formatting bar, shows another of its charts.
	await block.getByText('Counts only the tasks you can see.').click();
	const bar = page.getByRole('toolbar', { name: 'Formatting' });
	await bar.getByRole('button', { name: 'Chart' }).click();
	await bar.getByRole('button', { name: 'Burnup' }).click();
	await expect(block.getByRole('region', { name: 'Burnup' })).toBeVisible();
	await expect(block.getByRole('region', { name: 'Made and done each week' })).toHaveCount(0);
	// The chart takes the arrow keys inside the page, as on the Reports page,
	// and the page's text stays as it was.
	const before = await body.textContent();
	await block.getByRole('group', { name: /^Burnup/ }).focus();
	await page.keyboard.press('ArrowRight');
	await expect(block.locator('.tip')).toBeVisible();
	await page.keyboard.press('Escape');
	expect(await body.textContent()).toBe(before);
	await axe(page);

	await page.getByRole('button', { name: 'Markdown' }).click();
	await expect(page.getByLabel('Page as Markdown')).toHaveValue(
		/<!-- mila:report [0-9a-f-]{36} burnup -->\n\[Kitchen report\]\(\/views\/[0-9a-f-]{36}\)/
	);
});

test('Reports say so when there is nothing to count', async ({ page }) => {
	await page.goto('/');
	const ws: string = (await apiCall(page, 'POST', '/workspaces', { name: `Empty ${Date.now()}` }))
		.body.id;
	await page.goto(`/reports?ws=${ws}`);
	await expect(page.getByRole('heading', { name: 'Nothing to report yet' })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Download CSV' })).toBeDisabled();
	await page.getByRole('main').getByRole('link', { name: 'Time', exact: true }).click();
	await expect(page.getByRole('heading', { name: 'Time', level: 1 })).toBeVisible();
});
