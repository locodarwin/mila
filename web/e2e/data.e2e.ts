import { expect, test, type Browser } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { axe } from './axe';
import { errandsBoard, gardenBoard } from './fixtures/monday';
import { notionExport } from './fixtures/notion';
import { apiCall, newMember, newPerson, OWNER, SIGNED_OUT } from './helpers';
import { SYNC_SCHEMA } from '../src/lib/sync/entities';

// Exports, imports and account deletion belong to a person, and the
// server lets one person have only a few exports under way, so each test
// signs up someone new rather than using the owner other tests share.
test.use({ storageState: OWNER });

async function person(browser: Browser, name: string) {
	return newPerson(() => browser.newContext(SIGNED_OUT), name);
}

test('a workspace exports, downloads and comes back as a new one', async ({ browser }) => {
	test.setTimeout(60_000);
	const { page: own, context } = await person(browser, 'Exporter');
	await own.goto('/settings/data');
	await expect(own.getByRole('heading', { name: 'Export' })).toBeVisible();
	await own.getByRole('radio', { name: /^JSON/ }).check();
	await own.getByRole('button', { name: /^Export / }).click();
	await expect(own.getByText('Export started')).toBeVisible();

	// The list follows the export until its download is ready.
	const download = own.getByRole('link', { name: /^Download JSON export of / });
	await expect(download).toBeVisible({ timeout: 15_000 });
	const [file] = await Promise.all([own.waitForEvent('download'), download.click()]);
	expect(file.suggestedFilename()).toMatch(/^mila-.+-json-\d{4}-\d{2}-\d{2}\.zip$/);
	const path = await file.path();

	// The bell says so too.
	const notes = await apiCall(own, 'GET', '/me/notifications');
	expect(notes.body.items[0]).toMatchObject({
		kind: 'export_ready',
		url: expect.stringMatching(/^\/settings\/w\/[^/]+\/data$/)
	});

	// The zip goes back in with new IDs, as a workspace of its own.
	const before = (await apiCall(own, 'GET', '/workspaces')).body.length;
	await own.getByLabel('JSON export (.zip)').setInputFiles({
		name: 'export.zip',
		mimeType: 'application/zip',
		buffer: readFileSync(path)
	});
	await own.getByRole('button', { name: 'Import', exact: true }).click();
	await expect(own.getByText('Import started')).toBeVisible();
	await expect(own.getByRole('link', { name: /^Open / })).toBeVisible({ timeout: 15_000 });
	await expect
		.poll(async () => (await apiCall(own, 'GET', '/workspaces')).body.length)
		.toBe(before + 1);

	// A finished export can be deleted; it asks twice.
	await own.getByRole('button', { name: /^Delete JSON export/ }).click();
	await own.getByRole('button', { name: /^Confirm delete JSON export/ }).click();
	await expect(own.getByText('No exports yet.')).toBeVisible();
	await context.close();
});

test('a CSV file comes in after a dry run', async ({ browser }) => {
	const { page: own, context } = await person(browser, 'Spreadsheet');
	await own.goto('/settings/data/csv');
	const csv = [
		'Task,Due date,Tags,Priority,Status',
		'Order paint,2026-11-08,"house, weekend",high,In progress',
		'Measure the porch,soon,house,,',
		',2026-11-02,,,'
	].join('\n');
	await own.getByLabel('CSV file').setInputFiles({
		name: 'Porch.csv',
		mimeType: 'text/csv',
		buffer: Buffer.from(csv)
	});
	// Columns are matched by their headings and can be changed.
	await expect(own.getByLabel('Task goes to')).toHaveValue('title');
	await expect(own.getByLabel('Tags goes to')).toHaveValue('labels');
	await expect(own.getByLabel("New project's name")).toHaveValue('Porch');
	const importButton = own.getByRole('button', { name: /^Import \d* ?tasks?$/ });
	await expect(importButton).toBeDisabled();

	await own.getByRole('button', { name: 'Check the import' }).click();
	const dryRun = own.getByRole('region', { name: 'What the import would do' });
	await expect(dryRun).toContainText('2 tasks would come in, into a new project, Porch.');
	await expect(dryRun).toContainText('1 row is left out.');
	await expect(dryRun).toContainText('Row 3: "soon" is not a date like 2026-11-03.');
	await expect(dryRun).toContainText('labels house, weekend');
	await axe(own);

	// Changing a choice asks for the dry run again.
	await own.getByLabel('Priority goes to').selectOption('skip');
	await expect(importButton).toBeDisabled();
	await own.getByRole('button', { name: 'Check the import' }).click();
	await own.getByRole('button', { name: 'Import 2 tasks' }).click();
	await expect(own.getByRole('heading', { name: 'Imported 2 tasks' })).toBeFocused();
	await own.getByRole('link', { name: 'Open the project' }).click();
	await expect(own.getByLabel('Project name')).toHaveValue('Porch');
	for (const title of ['Order paint', 'Measure the porch'])
		await expect(
			own.locator('#main').getByRole('button', { name: title, exact: true })
		).toBeVisible();
	await context.close();
});

test('issues from GitHub come in after a dry run, with their comments', async ({ browser }) => {
	const { page: own, context, workspace } = await person(browser, 'Octavia');
	// The file gh wrote for this repository's own issues (all closed), and
	// one more that is open, with a label, a milestone and two assignees:
	// Octavia, matched by name, and someone who is not in the workspace.
	const issues = JSON.parse(
		readFileSync(join(import.meta.dirname, 'fixtures/github-issues.json'), 'utf8')
	);
	issues.unshift({
		number: 7,
		title: 'Porch light flickers',
		body: 'Since the storm.',
		state: 'OPEN',
		closedAt: null,
		createdAt: '2026-11-04T10:00:00Z',
		labels: [{ id: 'LA_1', name: 'bug', description: '', color: 'd73a4a' }],
		assignees: [
			{ id: 'U_1', login: 'octavia', name: 'Octavia' },
			{ id: 'U_2', login: 'zed', name: 'Zed' }
		],
		milestone: { number: 1, title: 'Autumn', description: '', dueOn: null },
		comments: [],
		url: 'https://github.com/locodarwin/mila/issues/7'
	});
	issues.push('not an issue');

	await own.goto('/settings/data');
	await own.getByRole('link', { name: 'import from another app' }).click();
	await expect(own).toHaveURL(/\/settings\/w\/[^/]+\/data\/apps$/);
	await expect(own.getByRole('button', { name: 'GitHub Issues' })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
	await expect(own.getByText('gh issue list -s all -L 5000 --json')).toBeVisible();

	// A file that is not a list of issues is refused, and nothing is planned.
	const input = own.getByLabel('Issues file (JSON)');
	await input.setInputFiles({
		name: 'issues.json',
		mimeType: 'application/json',
		buffer: Buffer.from('{"__proto__": {"admin": true}}')
	});
	await expect(own.getByText('That file is not a list of issues.')).toBeVisible();
	await expect(own.getByRole('region', { name: 'What the import would do' })).toHaveCount(0);

	await input.setInputFiles({
		name: 'issues.json',
		mimeType: 'application/json',
		buffer: Buffer.from(JSON.stringify(issues))
	});
	await expect(own.getByLabel('Put it in')).toHaveValue('new');
	await expect(own.getByLabel("New project's name")).toHaveValue('mila');
	const dryRun = own.getByRole('region', { name: 'What the import would do' });
	await expect(dryRun).toContainText('7 tasks would come in, into a new project, mila.');
	await expect(dryRun).toContainText('1 entry is left out.');
	await expect(dryRun).toContainText('Entry 8: It is not an issue.');
	await expect(dryRun).toContainText('1 open and 6 closed');
	await expect(dryRun).toContainText('4 comments, each starting with who wrote it and when');
	await expect(dryRun).toContainText('labels bug, Autumn');
	await expect(dryRun).toContainText('zed: Issue #7');
	await axe(own);

	await own.getByRole('button', { name: 'Import 7 tasks' }).click();
	await expect(own.getByRole('heading', { name: 'Imported 7 tasks' })).toBeFocused();
	await own.getByRole('link', { name: 'Open the project' }).click();
	await expect(own.getByLabel('Project name')).toHaveValue('mila');
	await expect(
		own.locator('#main').getByRole('button', { name: 'Porch light flickers', exact: true })
	).toBeVisible();

	// On the server once synced: closed issues done when they closed, the
	// open one assigned to Octavia, and each comment saying who wrote it.
	const me = (await apiCall(own, 'GET', '/me')).body;
	const ws = workspace.id;
	await expect
		.poll(async () => (await apiCall(own, 'GET', `/workspaces/${ws}/comments`)).body.items.length)
		.toBe(4);
	const tasks = (await apiCall(own, 'GET', `/workspaces/${ws}/tasks?limit=500`)).body.items as {
		id: string;
		title: string;
		completed_at: string | null;
	}[];
	const byTitle = new Map(tasks.map((t) => [t.title, t]));
	expect(new Date(byTitle.get('Wiki size adjustability')!.completed_at!).toISOString()).toBe(
		'2026-11-03T04:02:46.000Z'
	);
	const porch = byTitle.get('Porch light flickers')!;
	expect(porch.completed_at).toBeNull();
	const assigned = (
		await apiCall(own, 'GET', `/workspaces/${ws}/task-assignees?task_id=${porch.id}`)
	).body.items;
	expect(assigned.map((a: { user_id: string }) => a.user_id)).toEqual([me.id]);
	const comments = (await apiCall(own, 'GET', `/workspaces/${ws}/comments`)).body.items;
	for (const c of comments) {
		expect(c.author_id).toBe(me.id);
		expect(c.body).toMatch(/^Originally by locodarwin, \S.*2026\n\n\S/);
	}
	await context.close();
});

test('a ClickUp file opens the column screen matched, and comes in after a dry run', async ({
	browser
}) => {
	// A made-up export of ClickUp's shape: Ada, who is here, and Zed Quinn,
	// who isn't, a subtask, a Lead and an Account, a row twice. (The file's
	// Ada Park becomes this test's Ada, matched by name.)
	const { page: own, context, workspace } = await person(browser, 'Ada');
	const csv = readFileSync(join(import.meta.dirname, 'fixtures/clickup-tasks.csv'), 'utf8');
	await own.goto('/settings/data/apps');
	await own.getByRole('button', { name: 'ClickUp', exact: true }).click();
	await expect(own.getByRole('button', { name: 'ClickUp', exact: true })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
	await expect(own.getByText('Only the Workspace owner can.')).toBeVisible();
	await own.getByLabel('ClickUp file (CSV)').setInputFiles({
		name: 'export.csv',
		mimeType: 'text/csv',
		buffer: Buffer.from(csv.replaceAll('Ada Park', 'Ada'))
	});
	// ClickUp's columns are matched already; any can still be changed.
	await expect(own.getByLabel('Task Name goes to')).toHaveValue('title');
	await expect(own.getByLabel('List Name goes to')).toHaveValue('project');
	await expect(own.getByLabel('Parent ID goes to')).toHaveValue('parent');
	await expect(own.getByLabel('Time Spent goes to')).toHaveValue('skip');
	await expect(own.getByRole('radio', { name: 'A project for each list' })).toBeChecked();

	await own.getByRole('button', { name: 'Check the import' }).click();
	const dryRun = own.getByRole('region', { name: 'What the import would do' });
	await expect(dryRun).toContainText('7 tasks would come in, into 4 places.');
	await expect(dryRun).toContainText('3 rows are left out.');
	await expect(dryRun).toContainText('Row 6: It is a ClickUp Lead, not a task.');
	await expect(dryRun).toContainText('Row 7: It is a ClickUp Account, not a task.');
	await expect(dryRun).toContainText('Row 9: It is the same task as row 4.');
	await expect(dryRun).toContainText(
		'2 rows are not tasks in ClickUp, so they are left out: 1 of type Lead, 1 of type Account'
	);
	await expect(dryRun).toContainText('Row 8: Its parent task is left out');
	await expect(dryRun).toContainText('1 subtask, each under its parent task');
	await expect(dryRun).toContainText('projects Garden, Errands (Home), Errands (Work), Prospects');
	await expect(dryRun).toContainText('Zed Quinn: Row 3');
	await expect(dryRun).toContainText('Time tracked in ClickUp is left out');
	await axe(own);

	await own.getByRole('button', { name: 'Import 7 tasks' }).click();
	await expect(own.getByRole('heading', { name: 'Imported 7 tasks' })).toBeFocused();

	// On the server once synced: the subtask under its parent, the
	// estimate, checklist, assignee and comments, each saying who wrote it.
	const ws = workspace.id;
	const me = (await apiCall(own, 'GET', '/me')).body;
	await expect
		.poll(async () => (await apiCall(own, 'GET', `/workspaces/${ws}/comments`)).body.items.length)
		.toBe(2);
	const tasks = (await apiCall(own, 'GET', `/workspaces/${ws}/tasks?limit=500`)).body.items as {
		id: string;
		title: string;
		parent_id: string | null;
		estimate_minutes: number | null;
		made_at: string;
		notes: string;
	}[];
	const byTitle = new Map(tasks.map((t) => [t.title, t]));
	const plant = byTitle.get('Plant the bulbs')!;
	expect(byTitle.get('Order compost')!.parent_id).toBe(plant.id);
	expect(plant.estimate_minutes).toBe(90);
	expect(new Date(plant.made_at).getUTCFullYear()).toBe(2026);
	expect(byTitle.has('Corner bakery')).toBe(false);
	const items = (await apiCall(own, 'GET', `/workspaces/${ws}/checklist-items?task_id=${plant.id}`))
		.body.items;
	expect(items.map((i: { text: string; done: boolean }) => [i.text, i.done]).sort()).toEqual([
		['Buy bulbs', true],
		['Dig the beds', false]
	]);
	const assigned = (
		await apiCall(own, 'GET', `/workspaces/${ws}/task-assignees?task_id=${plant.id}`)
	).body.items;
	expect(assigned.map((a: { user_id: string }) => a.user_id)).toEqual([me.id]);
	const comments = (await apiCall(own, 'GET', `/workspaces/${ws}/comments`)).body.items;
	// Neither address is anyone's here, so each is named by its first part.
	expect(comments.map((c: { body: string }) => c.body.split('\n')[0]).sort()).toEqual([
		expect.stringMatching(/^Originally by ada, \S/),
		expect.stringMatching(/^Originally by zed\.quinn, \S/)
	]);
	const projects = (await apiCall(own, 'GET', `/workspaces/${ws}/projects`)).body.items;
	expect(projects.map((p: { name: string }) => p.name).sort()).toEqual([
		'Errands (Home)',
		'Errands (Work)',
		'Garden',
		'Prospects'
	]);
	await context.close();
});

test('an Asana file comes in with its sections, subtasks and dependencies', async ({ browser }) => {
	// A made-up export of Asana's shape (e2e/fixtures/asana-tasks.csv); the
	// file's Ada becomes this test's, matched by address.
	const { page: own, context, workspace, email } = await person(browser, 'Ada');
	const csv = readFileSync(join(import.meta.dirname, 'fixtures/asana-tasks.csv'), 'utf8');
	await own.goto('/settings/data/apps');
	await own.getByRole('button', { name: 'Asana', exact: true }).click();
	await expect(own.getByText('Do this for each project.')).toBeVisible();
	await own.getByLabel('Asana file (CSV)').setInputFiles({
		name: 'Launch_plan.csv',
		mimeType: 'text/csv',
		buffer: Buffer.from(csv.replaceAll('ada@example.com', email))
	});
	await expect(own.getByLabel('Section/Column goes to')).toHaveValue('status');
	await expect(own.getByLabel('Blocked By (Dependencies) goes to')).toHaveValue('blockedBy');
	await expect(own.getByRole('radio', { name: 'The project each task is in' })).toBeChecked();

	await own.getByRole('button', { name: 'Check the import' }).click();
	const dryRun = own.getByRole('region', { name: 'What the import would do' });
	await expect(dryRun).toContainText('7 tasks would come in');
	await expect(dryRun).toContainText('1 subtask, each under its parent task');
	await expect(dryRun).toContainText('2 dependencies between the tasks');
	await expect(dryRun).toContainText('A dependency in it would make a loop');
	await expect(dryRun).toContainText('zed@example.net: Row 3');
	await expect(dryRun).toContainText("Comments and attachments are not in Asana's file");
	await axe(own);

	await own.getByRole('button', { name: 'Import 7 tasks' }).click();
	await expect(own.getByRole('heading', { name: 'Imported 7 tasks' })).toBeFocused();

	// On the server once synced.
	const ws = workspace.id;
	await expect
		.poll(
			async () =>
				(await apiCall(own, 'GET', `/workspaces/${ws}/task-dependencies`)).body.items.length
		)
		.toBe(2);
	const tasks = (await apiCall(own, 'GET', `/workspaces/${ws}/tasks?limit=500`)).body.items as {
		id: string;
		title: string;
		parent_id: string | null;
		notes: string;
	}[];
	const byTitle = new Map(tasks.map((t) => [t.title, t]));
	const brief = byTitle.get('Write the brief')!;
	expect(byTitle.get('Choose a font')!.parent_id).toBe(brief.id);
	expect(brief.notes).toContain('(https://app.asana.com/0/0/4100000000000001)');
	const deps = (await apiCall(own, 'GET', `/workspaces/${ws}/task-dependencies`)).body.items as {
		task_id: string;
		blocker_id: string;
	}[];
	expect(deps).toContainEqual(
		expect.objectContaining({
			task_id: byTitle.get('Book the venue')!.id,
			blocker_id: brief.id
		})
	);
	const assigned = (
		await apiCall(own, 'GET', `/workspaces/${ws}/task-assignees?task_id=${brief.id}`)
	).body.items;
	expect(assigned).toHaveLength(1);
	const projects = (await apiCall(own, 'GET', `/workspaces/${ws}/projects`)).body.items;
	expect(projects.map((p: { name: string }) => p.name)).toEqual(['Launch plan']);
	await context.close();
});

test('Todoist files come in, a project each, with sections, subtasks and repeats', async ({
	browser
}) => {
	// Made-up exports of Todoist's shape (e2e/fixtures/todoist-*.csv); the
	// files' Ada Park becomes this test's Ada, matched by name.
	const { page: own, context, workspace } = await person(browser, 'Ada');
	const file = (name: string, fixture: string) => ({
		name,
		mimeType: 'text/csv',
		buffer: Buffer.from(
			readFileSync(join(import.meta.dirname, 'fixtures', fixture), 'utf8').replaceAll(
				'Ada Park (1001)',
				'Ada (1001)'
			)
		)
	});
	await own.goto('/settings/data/apps');
	await own.getByRole('button', { name: 'Todoist', exact: true }).click();
	await expect(own.getByText('Do this for each project.')).toBeVisible();
	await own
		.getByLabel('Todoist files (CSV)')
		.setInputFiles([
			file('Home.csv', 'todoist-home.csv'),
			file('Errands.csv', 'todoist-errands.csv')
		]);
	await expect(own.getByText('Home.csv and 1 more: 10 tasks.')).toBeVisible();
	await expect(own.getByLabel('Section goes to')).toHaveValue('status');
	await expect(own.getByLabel('Date goes to')).toHaveValue('due');
	await expect(own.getByRole('radio', { name: 'A project for each file' })).toBeChecked();

	await own.getByRole('button', { name: 'Check the import' }).click();
	const dryRun = own.getByRole('region', { name: 'What the import would do' });
	await expect(dryRun).toContainText('10 tasks would come in');
	await expect(dryRun).toContainText('Home.csv, row 4: It is a comment on the project');
	await expect(dryRun).toContainText('Home.csv, row 16: "when it stops raining" is not a date');
	await expect(dryRun).toContainText('Zed Quinn: Home.csv, row 10');
	await expect(dryRun).toContainText('2 tasks repeat');
	await expect(dryRun).toContainText('so they count from today');
	await axe(own);

	await own.getByRole('button', { name: 'Import 10 tasks' }).click();
	await expect(own.getByRole('heading', { name: 'Imported 10 tasks' })).toBeFocused();

	// On the server once synced.
	const ws = workspace.id;
	await expect
		.poll(async () => (await apiCall(own, 'GET', `/workspaces/${ws}/comments`)).body.items.length)
		.toBe(2);
	const tasks = (await apiCall(own, 'GET', `/workspaces/${ws}/tasks?limit=500`)).body.items as {
		id: string;
		title: string;
		parent_id: string | null;
		recurrence: string | null;
		recurrence_mode: string;
		due_time: string | null;
	}[];
	const byTitle = new Map(tasks.map((t) => [t.title, t]));
	const gate = byTitle.get('Fix the gate')!;
	expect(byTitle.get('Buy hinges')!.parent_id).toBe(gate.id);
	expect(gate.due_time).toBe('09:00');
	expect(byTitle.get('Water the plants')).toMatchObject({
		recurrence: 'FREQ=DAILY;INTERVAL=3',
		recurrence_mode: 'completion'
	});
	expect(byTitle.get('Ask next door for tape')!.parent_id).toBe(byTitle.get('Find some tape')!.id);
	const assigned = (
		await apiCall(own, 'GET', `/workspaces/${ws}/task-assignees?task_id=${gate.id}`)
	).body.items;
	expect(assigned).toHaveLength(1);
	const projects = (await apiCall(own, 'GET', `/workspaces/${ws}/projects`)).body.items;
	expect(projects.map((p: { name: string }) => p.name).sort()).toEqual(['Errands', 'Home']);
	await context.close();
});

test('monday.com boards come in, a project each, with groups, subitems and updates', async ({
	browser
}) => {
	// Made-up boards of monday.com's shape (e2e/fixtures/monday.ts), their
	// Ada Park this test's Ada, matched by name.
	const { page: own, context, workspace } = await person(browser, 'Ada');
	const file = (name: string, data: Uint8Array) => ({
		name,
		mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
		buffer: Buffer.from(data)
	});
	await own.goto('/settings/data/apps');
	await own.getByRole('button', { name: 'monday.com', exact: true }).click();
	await expect(own.getByText('Do this for each board.')).toBeVisible();
	await own
		.getByLabel('monday.com files (Excel)')
		.setInputFiles([
			file('Garden.xlsx', gardenBoard('Ada')),
			file('Errands.xlsx', errandsBoard('Ada'))
		]);
	await expect(own.getByText('Garden.xlsx and 1 more: 10 tasks.')).toBeVisible();
	await expect(own.getByLabel('Group and tags goes to')).toHaveValue('labels');
	await expect(own.getByLabel('Parent item goes to')).toHaveValue('parent');
	await expect(own.getByRole('radio', { name: 'A project for each board' })).toBeChecked();

	await own.getByRole('button', { name: 'Check the import' }).click();
	const dryRun = own.getByRole('region', { name: 'What the import would do' });
	await expect(dryRun).toContainText('10 tasks would come in');
	await expect(dryRun).toContainText(
		'Garden.xlsx, updates, row 6: It is an update on an item that is not in the file'
	);
	await expect(dryRun).toContainText('Zed Quinn: Garden.xlsx, row 9');
	await expect(dryRun).toContainText('1 item has subitems that are not in the file');
	await axe(own);

	await own.getByRole('button', { name: 'Import 10 tasks' }).click();
	await expect(own.getByRole('heading', { name: 'Imported 10 tasks' })).toBeFocused();

	// On the server once synced.
	const ws = workspace.id;
	await expect
		.poll(async () => (await apiCall(own, 'GET', `/workspaces/${ws}/comments`)).body.items.length)
		.toBe(3);
	const tasks = (await apiCall(own, 'GET', `/workspaces/${ws}/tasks?limit=500`)).body.items as {
		id: string;
		title: string;
		parent_id: string | null;
		start_date: string | null;
		due_date: string | null;
	}[];
	const byTitle = new Map(tasks.map((t) => [t.title, t]));
	const seeds = byTitle.get('Order seeds')!;
	expect([seeds.start_date, seeds.due_date]).toEqual(['2026-11-07', '2026-11-08']);
	expect(byTitle.get('Pick varieties')!.parent_id).toBe(seeds.id);
	const assigned = (
		await apiCall(own, 'GET', `/workspaces/${ws}/task-assignees?task_id=${seeds.id}`)
	).body.items;
	expect(assigned).toHaveLength(1);
	const labels = (await apiCall(own, 'GET', `/workspaces/${ws}/labels`)).body.items;
	expect(labels.map((l: { name: string }) => l.name)).toEqual(
		expect.arrayContaining(['This week', 'Later', 'Town', 'spring'])
	);
	const projects = (await apiCall(own, 'GET', `/workspaces/${ws}/projects`)).body.items;
	expect(projects.map((p: { name: string }) => p.name).sort()).toEqual(['Errands', 'Garden Club']);
	await context.close();
});

test("Confluence's Markdown pages come in under a page of their own", async ({ browser }) => {
	// Made up, in the shape Confluence's Export to Markdown writes.
	const { page: own, context, workspace } = await person(browser, 'Ada');
	await own.goto('/settings/data/apps');
	await own.getByRole('button', { name: 'Confluence', exact: true }).click();
	await expect(own.getByText('Export to Markdown')).toBeVisible();
	const md = (name: string, text: string) => ({
		name,
		mimeType: 'text/markdown',
		buffer: Buffer.from(text)
	});
	await own
		.getByLabel('Confluence pages (Markdown files)')
		.setInputFiles([
			md(
				'Seed suppliers.md',
				'# Seed suppliers\n\n\n\nWho we buy from.\n\n| **Supplier** | **Good for** |\n| --- | --- |\n| Hillside | Tomatoes \\- peppers |\n\nSee [the plan](https://garden.atlassian.net/wiki/spaces/G/pages/1/Garden+plan).\n'
			),
			md('Garden plan.md', '# Garden plan\n\nWhat goes where.\n\nAttached photo:\n'),
			md('photo.png', 'not a page')
		]);
	const plan = own.getByRole('region', { name: 'The pages the import would make' });
	await expect(plan).toContainText('2 pages would come in, under a new page, Confluence');
	await expect(plan).toContainText('1 link goes to Confluence, and keeps going there.');
	await expect(plan).toContainText('photo.png: left out, not a Markdown file');
	await expect(plan).toContainText('Pictures and attached files are not in Confluence');
	await axe(own);

	await own.getByRole('button', { name: 'Import 2 pages' }).click();
	await expect(own.getByRole('heading', { name: 'Imported 2 pages' })).toBeFocused();

	// On the server once synced: under a page of their own, in title order,
	// the heading taken out of the text.
	type Page = {
		id: string;
		title: string;
		parent_id: string | null;
		body: string;
		sort_key: string;
	};
	const ws = workspace.id;
	await expect
		.poll(async () => {
			const items = (await apiCall(own, 'GET', `/workspaces/${ws}/pages?limit=500`)).body
				.items as Page[];
			return items.find((p) => p.title === 'Seed suppliers')?.body ?? '';
		})
		.toContain('| Hillside |');
	const all = (await apiCall(own, 'GET', `/workspaces/${ws}/pages?limit=500`)).body.items as Page[];
	const byTitle = new Map(all.map((p) => [p.title, p]));
	const root = byTitle.get('Confluence')!;
	const plan2 = byTitle.get('Garden plan')!;
	const seeds = byTitle.get('Seed suppliers')!;
	expect(plan2.parent_id).toBe(root.id);
	expect(seeds.parent_id).toBe(root.id);
	expect(plan2.sort_key < seeds.sort_key).toBe(true);
	expect(seeds.body).not.toContain('# Seed suppliers');
	expect(seeds.body).toContain('https://garden.atlassian.net/wiki/spaces/G/pages/1/Garden+plan');
	expect(plan2.body).toContain('Attached photo:');

	// The page shows its table.
	await own.goto(`/wiki?ws=${ws}&page=${seeds.id}`);
	await expect(own.getByRole('cell', { name: 'Tomatoes - peppers' })).toBeVisible();
	await context.close();
});

test('a Notion export comes in: pages in the wiki, a database as tasks, another as pages', async ({
	browser
}) => {
	// A made-up export of Notion's shape (e2e/fixtures/notion.ts), its Ada
	// Park this test's Ada, matched by name.
	const { page: own, context, workspace } = await person(browser, 'Ada');
	await own.goto('/settings/data/apps');
	await own.getByRole('button', { name: 'Notion', exact: true }).click();
	await expect(own.getByText('Choose Markdown & CSV')).toBeVisible();
	await own.getByLabel('Notion export (zip)').setInputFiles({
		name: 'Export.zip',
		mimeType: 'application/zip',
		buffer: Buffer.from(notionExport('Ada'))
	});
	await expect(own.getByText('Export.zip: 5 pages and 2 databases.')).toBeVisible();
	await own.getByLabel('Database: Reading list, 2 rows').selectOption('pages');
	const pages = own.getByRole('region', { name: 'The pages the import would make' });
	await expect(pages).toContainText('6 pages would come in, under a new page, Notion');
	await expect(pages).toContainText('3 files, 2 of them images');
	await expect(pages).toContainText('1 link goes to Notion pages not in the export');
	const tasks = own.getByRole('region', { name: 'What the import would do' });
	await expect(tasks).toContainText('4 tasks would come in');
	await expect(tasks).toContainText('Zed Quinn: Tasks, Book the venue');
	await own.getByRole('button', { name: 'Match columns' }).click();
	await expect(own.getByLabel('Name goes to')).toHaveValue('title');
	await axe(own);

	await own.getByRole('button', { name: 'Import 6 pages and 4 tasks' }).click();
	await expect(own.getByRole('heading', { name: 'Imported 6 pages and 4 tasks' })).toBeFocused();

	// On the server once synced.
	const ws = workspace.id;
	type Page = { id: string; title: string; parent_id: string | null; body: string };
	await expect
		.poll(async () => {
			const items = (await apiCall(own, 'GET', `/workspaces/${ws}/pages?limit=500`)).body
				.items as Page[];
			return items.find((p) => p.title === 'Team handbook')?.body ?? '';
		})
		.toContain('[Onboarding](');
	const all = (await apiCall(own, 'GET', `/workspaces/${ws}/pages?limit=500`)).body.items as Page[];
	const byTitle = new Map(all.map((p) => [p.title, p]));
	const root = byTitle.get('Notion')!;
	const handbook = byTitle.get('Team handbook')!;
	const onboarding = byTitle.get('Onboarding')!;
	expect(handbook.parent_id).toBe(root.id);
	expect(onboarding.parent_id).toBe(handbook.id);
	expect(byTitle.get('Rivers of Salt')!.parent_id).toBe(byTitle.get('Reading list')!.id);
	expect(handbook.body).toContain(`page=${onboarding.id}`);
	expect(onboarding.body).toMatch(
		/!\[Desk\]\(\/api\/v1\/workspaces\/[^/]+\/attachments\/[^/]+\/file\)/
	);
	const files = (await apiCall(own, 'GET', `/workspaces/${ws}/page-attachments`)).body.items as {
		filename: string;
	}[];
	expect(files.map((f) => f.filename).sort()).toEqual(['checklist.pdf', 'desk.png']);
	const list = (await apiCall(own, 'GET', `/workspaces/${ws}/tasks?limit=500`)).body.items as {
		id: string;
		title: string;
		notes: string;
		due_date: string | null;
	}[];
	const offsite = list.find((t) => t.title === 'Plan the offsite')!;
	expect(offsite.due_date).toBe('2025-04-30');
	expect(offsite.notes).toContain(`page=${onboarding.id}`);
	const assigned = (
		await apiCall(own, 'GET', `/workspaces/${ws}/task-assignees?task_id=${offsite.id}`)
	).body.items;
	expect(assigned).toHaveLength(1);
	await context.close();
});

test('someone deletes their account; their work stays as Deleted user', async ({
	page,
	browser
}) => {
	const {
		page: own,
		workspace,
		email
	} = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Leaver');
	const me = (await apiCall(own, 'GET', '/me')).body;
	// A task they made in the shared workspace, which should outlive them.
	const task = crypto.randomUUID();
	const pushed = await apiCall(own, 'POST', `/workspaces/${workspace.id}/sync/push`, {
		schema: SYNC_SCHEMA,
		ops: [
			{
				opId: crypto.randomUUID(),
				entity: 'task',
				id: task,
				hlc: `${String(Date.now()).padStart(13, '0')}-0000-e2e`,
				fields: { title: 'Left behind', sort_key: 'a0', created_by: me.id }
			}
		]
	});
	expect(pushed.body.results[0].status).toBe('applied');

	await own.goto('/settings/profile');
	await own.getByRole('button', { name: 'Delete my account…' }).click();
	const field = own.getByLabel(/^Type your email address/);
	await expect(field).toBeFocused();
	const go = own.getByRole('button', { name: 'Delete my account for good' });
	await field.fill('not-me@example.com');
	await expect(go).toBeDisabled();
	await field.fill(email);
	await axe(own);
	await go.click();
	await expect(own).toHaveURL(/\/login/);
	expect((await apiCall(own, 'GET', '/me')).status).toBe(401);

	// The owner still has the task, credited to Deleted user.
	const former = await apiCall(page, 'GET', `/workspaces/${workspace.id}/former-members`);
	expect(former.body).toEqual([{ userId: me.id, name: 'Deleted user', deleted: true }]);
	const snap = await apiCall(
		page,
		'GET',
		`/workspaces/${workspace.id}/sync/bootstrap?schema=${SYNC_SCHEMA}`
	);
	expect(snap.body.rows.some((r: { id: string }) => r.id === task)).toBe(true);
	await own.context().close();
});
