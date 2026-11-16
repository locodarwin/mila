import { readFileSync } from 'node:fs';
import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, OWNER } from './helpers';

test.use({ storageState: OWNER });

/** A project made in the app with one task, its workspace's path and IDs. */
async function seed(page: Page, name: string) {
	await page.goto('/projects');
	await page.getByLabel('Name a new project').fill(name);
	await page.getByLabel('Name a new project').press('Enter');
	await expect(page.getByLabel('Project name')).toHaveValue(name);
	const project = new URL(page.url()).pathname.split('/').pop()!;
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
				if (found) [W, todo] = [`/workspaces/${w.id}`, found.id];
			}
			return todo;
		})
		.not.toBe('');
	const task = (
		await apiCall(page, 'POST', `${W}/tasks`, {
			title: 'Wire the outlets',
			project_id: project,
			status_id: todo
		})
	).body.id as string;
	await page.reload();
	await expect(page.getByRole('button', { name: 'Wire the outlets', exact: true })).toBeVisible();
	return { W, project, task };
}

test('a task takes an estimate, time logged and a timer', async ({ page }, info) => {
	const name = `Time ${Date.now()}`;
	await seed(page, name);
	await page.getByRole('button', { name: 'Wire the outlets', exact: true }).click();
	const detail = page.getByRole('complementary', { name: 'Task details' });
	const time = detail.getByRole('region', { name: 'Time', exact: true });

	// The estimate reads what people type, and says what it cannot read.
	const estimate = time.getByLabel('Estimate');
	await estimate.fill('soon');
	await estimate.press('Enter');
	await expect(time.getByRole('alert')).toContainText('Write the estimate as');
	await estimate.fill('1h 30m');
	await estimate.press('Enter');
	await expect(time.getByRole('alert')).toHaveCount(0);
	await expect(time).toContainText(/Remaining\s*1h 30m/);

	// Logging time.
	await time.getByRole('button', { name: 'Log time' }).click();
	const form = time.getByRole('form', { name: 'Log time' });
	await form.getByLabel('Time spent').fill('45');
	await form.getByRole('button', { name: 'Log it' }).click();
	// A bare number is hours, and one entry holds a day at most.
	await expect(form.getByRole('alert')).toContainText('at most 24 hours');
	await form.getByLabel('Time spent').fill('4');
	await form.getByLabel('Note').fill('Pulled the wire');
	await form.getByRole('button', { name: 'Log it' }).click();
	await expect(time.getByRole('list', { name: 'Time logged' })).toContainText('Pulled the wire');
	await expect(time).toContainText(/Logged\s*4h/);
	await expect(time).toContainText(/Over\s*2h 30m/);

	// Changed in place to 2h: past the estimate by 30m.
	await time.getByRole('button', { name: /^Change 4h logged by/ }).click();
	const change = time.getByRole('form', { name: /^Change .* time$/ });
	await change.getByLabel('Time spent').fill('2h');
	await change.getByRole('button', { name: 'Save' }).click();
	await expect(time).toContainText(/Logged\s*2h/);
	await expect(time).toContainText(/Over\s*30m/);
	await axe(page);

	// The row says so too, in red.
	if (info.project.name !== 'phone') {
		await expect(page.getByRole('list', { name: 'Tasks' }).getByText('2h / 1h 30m')).toBeVisible();
	}

	// A timer: it runs in the sidebar, or above the tabs on a phone, and a
	// timer stopped before a minute logs nothing.
	await time.getByRole('button', { name: 'Start a timer' }).click();
	await expect(time.getByRole('button', { name: /Stop the timer/ })).toBeVisible();
	if (info.project.name !== 'phone') {
		const timer = page.getByRole('group', { name: 'Your timer' });
		await expect(timer).toContainText('Wire the outlets');
		await timer.getByRole('button', { name: 'Stop the timer on Wire the outlets' }).click();
	} else {
		await time.getByRole('button', { name: /Stop the timer/ }).click();
	}
	await expect(page.getByText('Stopped before a minute, so nothing was logged')).toBeVisible();
	await expect(time.getByRole('button', { name: 'Start a timer' })).toBeVisible();
	await expect(time).toContainText(/Logged\s*2h/);

	// The Time page adds it up for the project.
	await page.goto('/time');
	await page.getByRole('combobox', { name: 'Projects', exact: true }).selectOption({ label: name });
	const periodOf = page.getByRole('combobox', { name: 'Period', exact: true });
	await periodOf.selectOption({ label: 'Last month' });
	await periodOf.selectOption({ label: 'This week' });
	const report = page.getByRole('table', { name: /Time logged by person/ });
	await expect(report).toContainText('2h');
	// A week's days, so no chart by week.
	await expect(page.getByRole('heading', { name: 'By week' })).toHaveCount(0);
	await page.getByRole('combobox', { name: 'Rows', exact: true }).selectOption('project');
	await expect(page.getByRole('table', { name: /Time logged by project/ })).toContainText(name);
	await axe(page);
	const entries = page.waitForEvent('download');
	await page.getByRole('button', { name: 'Export CSV' }).click();
	await page.getByRole('button', { name: /^Every entry/ }).click();
	expect((await entries).suggestedFilename()).toMatch(/^Time .* to .*\.csv$/);

	// Eight weeks: a column a week, and the chart of them, this week so far.
	await periodOf.selectOption({ label: 'Last 8 weeks' });
	const weeks = page.getByRole('table', { name: /Time logged by project/ });
	await expect(weeks.getByRole('columnheader')).toHaveCount(10);
	const chart = page.getByRole('group', { name: /^Hours logged each week by project/ });
	await expect(chart).toBeVisible();
	await expect(page.getByText('The week under way is lighter')).toBeVisible();
	// Its summary says this week's 2h, and the arrow keys reach it.
	await chart.focus();
	await page.keyboard.press('End');
	await expect(page.locator('#announcer')).toContainText(/so far: 2h in all/);
	await axe(page);
	const table = page.waitForEvent('download');
	await page.getByRole('button', { name: 'Export CSV' }).click();
	await page.getByRole('button', { name: /^This table/ }).click();
	const file = await table;
	expect(file.suggestedFilename()).toMatch(/^Time by project .* to .*\.csv$/);
	const text = readFileSync(await file.path(), 'utf8').split('\r\n');
	expect(text[0]).toMatch(/^Project,(\d{4}-\d\d-\d\d to \d{4}-\d\d-\d\d,){8}Total \(hours\)$/);
	expect(text[1]).toMatch(new RegExp(`^"?${name}"?,(0,){7}2,2$`));
});

test('a view filters by time, and the palette logs it', async ({ page }, info) => {
	test.skip(info.project.name === 'phone', 'the same on a phone; the panel is checked above');
	const { W, task } = await seed(page, `Time filter ${Date.now()}`);
	await apiCall(page, 'PATCH', `${W}/tasks/${task}`, { estimate_minutes: 60 });
	await page.reload();
	await expect(
		page.getByRole('list', { name: 'Tasks' }).getByText('1h', { exact: true })
	).toBeVisible();

	// The palette opens the task with Log time ready.
	await page.getByRole('button', { name: 'Wire the outlets', exact: true }).click();
	await page.locator('body').press('Control+k');
	const palette = page.getByRole('dialog', { name: 'Command palette' });
	await palette.getByRole('combobox').fill('log time');
	await palette.getByRole('option', { name: /Log time on the task/ }).click();
	const form = page.getByRole('form', { name: 'Log time' });
	await expect(form.getByLabel('Time spent')).toBeFocused();
	await form.getByLabel('Time spent').fill('1:15');
	await form.getByRole('button', { name: 'Log it' }).click();
	await expect(
		page
			.getByRole('region', { name: 'Time', exact: true })
			.getByRole('list', { name: 'Time logged' })
	).toContainText('1h 15m');
});

test('a project takes time of its own, counted with its tasks on the Time page', async ({
	page
}, info) => {
	const name = `Hours ${Date.now()}`;
	const { project } = await seed(page, name);
	await page.getByRole('tab', { name: 'Time' }).click();
	const panel = page.getByRole('tabpanel', { name: 'Time' });
	await expect(panel.getByRole('heading', { name: 'This month' })).toBeVisible();

	// Logged to the project itself, not to a task.
	await panel.getByRole('button', { name: 'Log time' }).click();
	const form = panel.getByRole('form', { name: `Log time to ${name}` });
	await form.getByLabel('Time spent').fill('1h 30m');
	await form.getByLabel('Note').fill('Supplier call');
	await form.getByRole('button', { name: 'Log it' }).click();
	const list = panel.getByRole('list', { name: 'Time logged' });
	await expect(list).toContainText('The project itself');
	await expect(list).toContainText('Supplier call');
	await expect(panel).toContainText(/1h 30m\s*On the project itself/);
	await axe(page);

	// Changed in place, then deleted and brought back.
	await panel.getByRole('button', { name: /^Change .*'s 1h 30m on Today$/ }).click();
	const change = panel.getByRole('form', { name: /^Change .* time$/ });
	await change.getByLabel('Time spent').fill('45m');
	await change.getByRole('button', { name: 'Save' }).click();
	await expect(panel).toContainText(/45m\s*On the project itself/);
	await panel.getByRole('button', { name: /^Change .*'s 45m on Today$/ }).click();
	await panel.getByRole('button', { name: 'Delete' }).click();
	await expect(list).toHaveCount(0);
	await page.getByRole('button', { name: 'Undo' }).click();
	await expect(panel.getByRole('list', { name: 'Time logged' })).toContainText('Supplier call');

	// A timer on the project: in the sidebar under its name, or here on a phone.
	await panel.getByRole('button', { name: 'Start a timer' }).click();
	await expect(panel.getByRole('button', { name: /Stop the timer/ })).toBeVisible();
	if (info.project.name !== 'phone') {
		const timer = page.getByRole('group', { name: 'Your timer' });
		await expect(timer).toContainText(name);
		await timer.getByRole('button', { name: `Stop the timer on ${name}` }).click();
	} else {
		await panel.getByRole('button', { name: /Stop the timer/ }).click();
	}
	await expect(page.getByText('Stopped before a minute, so nothing was logged')).toBeVisible();

	// The Time page counts it with the project, and by label as not on a task.
	await page.goto('/time');
	await page.getByRole('combobox', { name: 'Projects', exact: true }).selectOption({ label: name });
	await page.getByRole('combobox', { name: 'Rows', exact: true }).selectOption('project');
	await expect(page.getByRole('table', { name: /Time logged by project/ })).toContainText('45m');
	await page.getByRole('combobox', { name: 'Rows', exact: true }).selectOption('label');
	await expect(page.getByRole('table', { name: /Time logged by label/ })).toContainText(
		'Not on a task'
	);

	// The palette's Log time to the project opens the form, ready to type.
	if (info.project.name !== 'phone') {
		await page.goto(`/projects/${project}?tab=time`);
		await expect(panel).toBeVisible();
		await page.keyboard.press('Control+k');
		await page.getByLabel('Search for a task, project or command').fill(`Log time to ${name}`);
		await page.keyboard.press('Enter');
		await expect(
			page.getByRole('form', { name: `Log time to ${name}` }).getByLabel('Time spent')
		).toBeFocused();
	}
});

test('the Time page logs time to a project itself', async ({ page }) => {
	const name = `Logged here ${Date.now()}`;
	const { project } = await seed(page, name);
	await page.goto('/time');
	const table = page.getByRole('table', { name: /Time logged by project/ });
	await page.getByRole('combobox', { name: 'Rows', exact: true }).selectOption('project');

	// With every project shown, the form asks which one.
	await page.getByRole('button', { name: 'Log time', exact: true }).click();
	const form = page.getByRole('form', { name: 'Log time to a project' });
	await expect(form.getByLabel('Project')).toBeFocused();
	await form.getByLabel('Time spent').fill('2h');
	await form.getByRole('button', { name: 'Log it' }).click();
	await expect(form.getByRole('alert')).toHaveText('Choose the project the time was spent on.');
	await form.getByLabel('Project').selectOption({ label: name });
	await form.getByLabel('Note').fill('Site walk');
	await axe(page);
	await form.getByRole('button', { name: 'Log it' }).click();
	await expect(form).toHaveCount(0);
	await expect(page.getByRole('button', { name: 'Log time', exact: true })).toBeFocused();
	await expect(page.getByText(`Logged 2h to ${name}`)).toBeVisible();
	await page.getByRole('combobox', { name: 'Projects', exact: true }).selectOption({ label: name });
	await expect(table).toContainText('2h');

	// Undo takes it back.
	await page.getByRole('button', { name: 'Undo' }).click();
	await expect(table).toHaveCount(0);

	// With one project shown, the form starts on it.
	await page.getByRole('button', { name: 'Log time', exact: true }).click();
	await expect(form.getByLabel('Project')).toHaveValue(/.+/);
	await expect(form.getByLabel('Time spent')).toBeFocused();
	await page.keyboard.type('20m');
	await page.keyboard.press('Enter');
	await expect(table).toContainText('20m');

	// It is on the project's own Time tab as time on the project itself.
	await page.goto(`/projects/${project}?tab=time`);
	await expect(page.getByRole('list', { name: 'Time logged' })).toContainText('The project itself');
});
