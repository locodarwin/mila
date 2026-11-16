// axe on every main screen: the pages, the task panel, the board and the
// calendar, the dialogs and menus while they are open, toasts, and the
// sign-in flows. The standard theme runs here in light and dark on desktop
// and phone; the other three characters run on desktop below, with
// someone of their own, since a theme is a preference.
import { expect, test, type Browser, type Page } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { axe, axeViolations } from './axe';
import {
	apiCall,
	OWNER,
	pngBuffer,
	SIGNED_OUT,
	newPerson,
	settingsWorkspace,
	WORKSPACE_SETTINGS
} from './helpers';

type Scheme = 'light' | 'dark';

async function newProject(page: Page, name: string) {
	await page.goto('/projects');
	await page.getByLabel('Name a new project').fill(name);
	await page.getByLabel('Name a new project').press('Enter');
	await expect(page.getByLabel('Project name')).toHaveValue(name);
	return page.url();
}

async function addTask(page: Page, title: string) {
	await page.getByLabel('New task').fill(title);
	await page.getByLabel('New task').press('Enter');
	await expect(page.locator('#main [data-task-id]').filter({ hasText: title })).toBeVisible();
}

const detail = (page: Page) => page.getByRole('complementary', { name: 'Task details' });

/** Notes that use every kind of Markdown the editor draws, each heading level among them. */
const MARKDOWN = [
	'# Garden plan',
	'',
	'Text with a [link](https://example.com), `code`, **bold** and *emphasis*.',
	'',
	'## Beds',
	'',
	'- Tomatoes',
	'- Beans',
	'',
	'1. Dig',
	'2. Plant',
	'',
	'### Tools',
	'',
	'> Water in the morning.',
	'',
	'#### Soil',
	'',
	'```',
	'ph 6.5',
	'```',
	'',
	'##### Seeds',
	'',
	'| Bed | Crop |',
	'| --- | --- |',
	'| 1 | Beans |',
	'| 2 | Kale |',
	'',
	'###### Notes',
	'',
	'---',
	'',
	'Done.'
].join('\n');

/** The workspace a task is in, of those the page's person belongs to. */
async function workspaceOf(page: Page, task: string): Promise<string> {
	const list = (await apiCall(page, 'GET', '/workspaces')).body;
	for (const w of (Array.isArray(list) ? list : list.items) as { id: string }[])
		if ((await apiCall(page, 'GET', `/workspaces/${w.id}/tasks/${task}`)).status === 200)
			return w.id;
	throw new Error(`no workspace has task ${task}`);
}

/** Shows Inbox, My Day or Upcoming as a list, a board or another view. */
async function showAs(page: Page, view: string) {
	await page.getByRole('group', { name: 'Show as' }).getByRole('button', { name: view }).click();
	await expect(
		page.getByRole('group', { name: 'Show as' }).getByRole('button', { name: view })
	).toHaveAttribute('aria-pressed', 'true');
}

function ymd(days: number) {
	const d = new Date();
	d.setDate(d.getDate() + days);
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

/** A project with tasks that show what rows can carry: dates, overdue, a label-free mix. */
async function filledProject(page: Page, name: string) {
	const url = await newProject(page, name);
	for (const t of ['First task', 'Second task', 'Late task']) await addTask(page, t);
	await page
		.locator('#main [data-task-id]')
		.filter({ hasText: 'Late task' })
		.getByRole('button', { name: 'Late task', exact: true })
		.click();
	await detail(page).getByLabel('Due date').fill(ymd(-2));
	await detail(page).getByLabel('Priority', { exact: true }).selectOption('high');
	await detail(page).getByLabel('New checklist item').fill('A step');
	await detail(page).getByLabel('New checklist item').press('Enter');
	await detail(page).getByRole('button', { name: 'Close' }).click();
	await expect(detail(page)).toBeHidden();
	await page
		.locator('#main [data-task-id]')
		.filter({ hasText: 'First task' })
		.getByRole('button', { name: 'First task', exact: true })
		.click();
	await detail(page).getByLabel('Due date').fill(ymd(0));
	await detail(page).getByRole('button', { name: 'Close' }).click();
	await expect(detail(page)).toBeHidden();
	return url;
}

for (const scheme of ['light', 'dark'] as const) {
	test.describe(`${scheme} scheme`, () => {
		test.use({ colorScheme: scheme });

		test.describe('signed out', () => {
			test.use(SIGNED_OUT);

			test('signing in passes axe, before and after the link is sent', async ({ page }) => {
				await page.goto('/login');
				await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
				await axe(page);
				await page.getByLabel('Email').fill('nobody@example.com');
				await page.getByRole('button', { name: 'Email me a sign-in link' }).click();
				const sent = page.getByRole('heading', { name: 'Check your email' });
				await expect(sent).toBeVisible();
				// The form is gone, so focus moves to what replaced it.
				await expect(sent).toBeFocused();
				await axe(page);
			});

			test('setup, an invitation and a sign-in link pass axe', async ({ page }) => {
				await page.goto('/setup');
				await expect(page.getByRole('heading', { name: 'Set up Mila' })).toBeVisible();
				await axe(page);
				await page.goto('/setup?token=not-a-token');
				await expect(page.getByRole('heading', { name: 'Create your account' })).toBeVisible();
				await axe(page);
				await page.goto('/invite?token=not-a-token');
				await expect(
					page.getByRole('heading', { name: "This invitation can't be used" })
				).toBeVisible();
				await axe(page);
				await page.goto('/auth/link?token=not-a-token');
				await expect(page.getByRole('heading', { name: 'That link did not work' })).toBeVisible();
				await axe(page);
			});
		});

		test.describe('signed in', () => {
			test.use({ storageState: OWNER });

			test('a project with tasks passes axe as a list, a board, a calendar, a table and a timeline', async ({
				page
			}) => {
				const url = await filledProject(page, `Axe ${scheme} ${Date.now()}`);
				await axe(page);
				await page.goto(url + '?view=board');
				await expect(page.getByRole('heading', { name: 'To do' })).toBeVisible();
				await axe(page);
				await page.goto(url + '?view=calendar');
				await expect(page.getByRole('heading', { level: 2 }).first()).toBeVisible();
				await axe(page);
				await page.goto(url + '?view=table');
				await expect(page.getByRole('heading', { name: 'To do' })).toBeVisible();
				await axe(page);
				await page.goto(url + '?view=timeline');
				await expect(page.getByRole('heading', { name: /Unscheduled/ })).toBeVisible();
				await axe(page);
				await page.goto(url + '?tab=time');
				await expect(page.getByRole('heading', { name: 'This month' })).toBeVisible();
				await axe(page);
			});

			test('a saved view passes axe with its filter and display panels open', async ({ page }) => {
				await filledProject(page, `Axe view ${scheme} ${Date.now()}`);
				await page.getByRole('button', { name: 'Save as a view' }).click();
				await expect(page.getByLabel('View name')).toBeVisible();
				await page.getByRole('button', { name: 'Filter', exact: true }).click();
				await expect(page.getByRole('group', { name: 'Filter' })).toBeVisible();
				await axe(page);
				await page.getByRole('button', { name: 'Display' }).click();
				const display = page.getByRole('group', { name: 'Display' });
				await display.getByRole('button', { name: 'Add a sort' }).click();
				await display.getByLabel('Group by').selectOption('priority');
				await expect(page.getByRole('heading', { name: 'High' })).toBeVisible();
				await axe(page);
				await page
					.getByRole('group', { name: 'Show as' })
					.getByRole('button', { name: 'Board' })
					.click();
				await expect(display.getByLabel('Columns')).toBeVisible();
				await axe(page);
			});

			test('a task with everything filled in passes axe', async ({ page }) => {
				await newProject(page, `Axe detail ${scheme} ${Date.now()}`);
				await addTask(page, 'Everything');
				await page.getByRole('button', { name: 'Everything', exact: true }).click();
				const panel = detail(page);
				await panel.getByLabel('Priority', { exact: true }).selectOption('urgent');
				await panel.getByLabel('Due date').fill('2026-11-20');
				await panel.getByLabel('Repeat', { exact: true }).selectOption('custom');
				await expect(panel.getByRole('group', { name: 'Custom repeat' })).toBeVisible();
				await panel.getByRole('button', { name: 'Add a reminder' }).click();
				await panel.getByRole('button', { name: /^Tomorrow, 9:00/ }).click();
				await expect(panel.getByRole('list', { name: 'Reminders' })).toContainText('Tomorrow at');
				await panel.getByLabel('New subtask').fill('A step');
				await panel.getByLabel('New subtask').press('Enter');
				await panel.getByLabel('New checklist item').fill('An item');
				await panel.getByLabel('New checklist item').press('Enter');
				await panel.getByLabel('New comment').fill('A comment');
				await panel.getByRole('button', { name: 'Comment' }).click();
				await expect(panel.getByRole('list', { name: 'Comments' })).toContainText('A comment');
				// Check the panel from the top: scrolled, the sticky header can half
				// cover a control, which axe reports as a target too small to hit.
				await panel.locator('.detail').evaluate((el) => el.scrollTo(0, 0));
				await axe(page);
				// The menus in the panel, open.
				for (const menu of ['Edit labels', 'Edit assignees']) {
					await panel.getByRole('button', { name: menu }).click();
					await expect(page.locator('.menu:popover-open')).toBeVisible();
					await axe(page);
					await page.keyboard.press('Escape');
					await expect(page.locator('.menu:popover-open')).toHaveCount(0);
				}
			});

			for (const path of [
				'/',
				'/today',
				'/upcoming',
				'/projects',
				'/trash',
				'/help',
				'/help/shortcuts',
				'/help/quick-add',
				'/api/docs'
			]) {
				test(`${path} passes axe`, async ({ page }) => {
					await page.goto(path);
					await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
					await page.waitForLoadState('networkidle');
					await axe(page);
				});
			}

			test('My Day with tasks in every section passes axe', async ({ page }) => {
				await filledProject(page, `Axe day ${scheme} ${Date.now()}`);
				await page.locator('#main [data-task-id]').first().focus();
				await page.keyboard.press('t');
				await page.goto('/today');
				await expect(page.getByRole('heading', { name: 'Due today or overdue' })).toBeVisible();
				await axe(page);
			});

			for (const path of [
				'/settings/profile',
				'/settings/appearance',
				'/settings/preferences',
				'/settings/keyboard',
				'/settings/notifications',
				'/settings/security',
				'/settings/tokens',
				'/settings/admin'
			]) {
				test(`${path} passes axe`, async ({ page }) => {
					await page.goto(path);
					await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();
					await page.waitForLoadState('networkidle');
					await axe(page);
				});
			}

			// A test each, as the pages about you have: twelve in one could outlast
			// the test's time on a busy phone run.
			for (const p of WORKSPACE_SETTINGS) {
				test(`a workspace's ${p} settings pass axe`, async ({ page }) => {
					const ws = await settingsWorkspace(page);
					await page.goto(`/settings/w/${ws}/${p}`);
					await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();
					await page.waitForLoadState('networkidle');
					await axe(page);
				});
			}

			test('Inbox, My Day and Upcoming pass axe as boards, Upcoming by day', async ({
				browser
			}) => {
				const {
					page: own,
					context,
					workspace
				} = await newPerson(() => browser.newContext(SIGNED_OUT), `Boards-${scheme}`);
				const ws = workspace.id;
				for (const [title, due] of [
					['Mail the forms', ymd(0)],
					['Water the ferns', ymd(1)],
					['Renew the permit', ymd(-2)],
					['Undated chore', null]
				] as const)
					expect(
						(await apiCall(own, 'POST', `/workspaces/${ws}/tasks`, { title, due_date: due })).status
					).toBe(201);
				for (const path of ['/', '/today', '/upcoming']) {
					await own.goto(`${path}?ws=${ws}`);
					await showAs(own, 'Board');
					await expect(own.getByRole('group', { name: 'Board' })).toBeVisible();
					await own.waitForLoadState('networkidle');
					await axe(own);
				}
				await expect(
					own.getByRole('group', { name: 'Board' }).locator('section[aria-label="Tomorrow"]')
				).toContainText('Water the ferns');
				await context.close();
			});

			test('notes with every kind of Markdown pass axe', async ({ browser }) => {
				const {
					page: own,
					context,
					workspace
				} = await newPerson(() => browser.newContext(SIGNED_OUT), `Notes-${scheme}`);
				const ws = workspace.id;
				const res = await apiCall(own, 'POST', `/workspaces/${ws}/tasks`, {
					title: 'Written up',
					notes: MARKDOWN
				});
				expect(res.status).toBe(201);
				await own.goto(`/?ws=${ws}&task=${res.body.id}`);
				const notes = detail(own).getByLabel('Notes', { exact: true });
				await expect(notes.locator('h6')).toHaveText('Notes');
				await expect(notes.locator('table')).toBeVisible();
				await axe(own);
				await context.close();
			});

			test('the Time page passes axe by week, with its chart, Log time open and a timer running', async ({
				browser
			}) => {
				const {
					page: own,
					context,
					workspace
				} = await newPerson(() => browser.newContext(SIGNED_OUT), `Hours-${scheme}`);
				const ws = workspace.id;
				const made = await apiCall(own, 'POST', `/workspaces/${ws}/projects`, { name: 'Porch' });
				expect(made.status).toBe(201);
				await own.goto(`/time?ws=${ws}`);
				await own.getByRole('button', { name: 'Log time', exact: true }).click();
				const form = own.getByRole('form', { name: 'Log time to a project' });
				await form.getByLabel('Project').selectOption({ label: 'Porch' });
				await form.getByLabel('Time spent').fill('2h');
				await axe(own);
				await form.getByRole('button', { name: 'Log it' }).click();
				await expect(form).toHaveCount(0);
				await own
					.getByRole('combobox', { name: 'Period', exact: true })
					.selectOption({ label: 'Last 8 weeks' });
				await expect(own.getByRole('group', { name: /^Hours logged each week by/ })).toBeVisible();
				await axe(own);
				// With a timer running: at the foot of the sidebar, or above the tabs on a phone.
				await own.goto(`/projects/${made.body.id}?ws=${ws}&tab=time`);
				const panel = own.getByRole('tabpanel', { name: 'Time' });
				await panel.getByRole('button', { name: 'Start a timer' }).click();
				await expect(panel.getByRole('button', { name: /Stop the timer/ })).toBeVisible();
				await own.goto(`/time?ws=${ws}`);
				await expect(own.getByRole('group', { name: 'Your timer' })).toBeVisible();
				await axe(own);
				await context.close();
			});

			test('Settings pass axe two sections to a row, the weekly report turned on', async ({
				browser,
				isMobile
			}) => {
				test.skip(isMobile, 'a phone shows one section to a row');
				// The weekly report is a person's own: someone new.
				const {
					page: own,
					context,
					workspace
				} = await newPerson(() => browser.newContext(SIGNED_OUT), `Wide-${scheme}`);
				await own.setViewportSize({ width: 1920, height: 1080 });
				await own.goto('/settings/notifications');
				const weekly = own.getByLabel('Email me a weekly report');
				await expect(weekly).not.toBeChecked();
				await own.getByText('Email me a weekly report', { exact: true }).click();
				await expect(weekly).toBeChecked();
				await expect(own.getByLabel('Send them at')).toBeEnabled();
				await expect(own.getByRole('status').filter({ hasText: 'Saved' })).toBeVisible();
				// Two to a row: the summaries beside the time zone.
				const top = async (name: string) =>
					(await own.getByRole('heading', { name, exact: true }).boundingBox())!.y;
				expect(Math.abs((await top('Summaries by email')) - (await top('Time zone')))).toBeLessThan(
					2
				);
				await axe(own);
				for (const path of [
					'/settings/profile',
					'/settings/preferences',
					'/settings/security',
					`/settings/w/${workspace.id}/general`,
					`/settings/w/${workspace.id}/data`
				]) {
					await own.goto(path);
					await expect(own.getByRole('heading', { name: 'Settings' })).toBeVisible();
					await own.waitForLoadState('networkidle');
					await axe(own);
				}
				await context.close();
			});

			test('the picture cropper passes axe', async ({ page }) => {
				// Nothing is saved: the dialog is canceled.
				await page.goto('/settings/profile');
				await page.getByLabel('Upload picture').setInputFiles({
					name: 'me.png',
					mimeType: 'image/png',
					buffer: pngBuffer(300, 200)
				});
				const dialog = page.getByRole('dialog', { name: 'Crop your picture' });
				await expect(dialog).toBeVisible();
				await expect(dialog.getByRole('application', { name: 'Picture position' })).toBeFocused();
				await axe(page);
				await page.keyboard.press('Escape');
				await expect(dialog).toBeHidden();
			});

			test('the shortcut editor passes axe while recording and with a conflict', async ({
				page
			}) => {
				// Nothing here is saved: a recording and a refused key change nothing.
				await page.goto('/settings/keyboard');
				await page.getByRole('button', { name: 'Add a key for Add a task' }).click();
				await expect(page.getByLabel('Press the keys for Add a task')).toBeFocused();
				await axe(page);
				await page.keyboard.press('Control+l');
				// In the list, not the announcer, which may be saying the same.
				await expect(
					page
						.getByRole('list', { name: 'Anywhere' })
						.getByText("Ctrl+L is kept for the browser; Mila can't use it.")
				).toBeVisible();
				await expect(page.getByRole('button', { name: 'Try another key' })).toBeFocused();
				await axe(page);
				await page.getByRole('button', { name: 'Cancel' }).click();
			});

			test('the dialogs and menus pass axe while open', async ({ page, isMobile }) => {
				await newProject(page, `Axe menus ${scheme} ${Date.now()}`);
				await addTask(page, 'Something');
				// Out of the new-task field, where keys would be typed.
				await page.locator('#main').focus();
				for (const [open, name] of [
					['Control+k', 'Command palette'],
					['q', 'Add a task'],
					['?', 'Keyboard shortcuts']
				] as const) {
					if (isMobile && open !== 'Control+k') continue;
					await page.locator('body').press(open);
					await expect(page.getByRole('dialog', { name })).toBeVisible();
					await axe(page);
					await page.keyboard.press('Escape');
					await expect(page.getByRole('dialog', { name })).toBeHidden();
				}
				await page
					.getByRole('button', { name: /^Switch workspace/ })
					.filter({ visible: true })
					.click();
				await expect(page.locator('#workspace-menu')).toBeVisible();
				await axe(page);
				await page.keyboard.press('Escape');
				await page.getByRole('button', { name: 'Edit statuses' }).click();
				await expect(page.getByRole('list', { name: 'Statuses' })).toBeVisible();
				await axe(page);
				await page.keyboard.press('Escape');
			});

			test('toasts pass axe, an error and one with undo', async ({ page }) => {
				await newProject(page, `Axe toasts ${scheme} ${Date.now()}`);
				await addTask(page, 'Short-lived');
				await page.getByRole('button', { name: 'Delete Short-lived' }).click();
				await expect(page.getByRole('button', { name: 'Undo' })).toBeVisible();
				// A link to a workspace nobody here belongs to explains itself.
				await page.goto('/?ws=00000000-0000-0000-0000-000000000000');
				const error = page.getByRole('alert').filter({ hasText: 'not a member' });
				await expect(error).toBeVisible();
				await axe(page);
			});

			test('the collapsed sidebar passes axe and keeps its names', async ({ page, isMobile }) => {
				test.skip(isMobile, 'phones have no sidebar');
				const project = `Narrow ${Date.now().toString(36)}`;
				await newProject(page, project);
				const sidebar = page.getByRole('navigation', { name: 'Main' }).filter({ visible: true });
				await sidebar.getByRole('button', { name: 'Collapse sidebar' }).click();
				for (const name of ['Inbox', 'My Day', 'Upcoming', 'Trash'])
					await expect(sidebar.getByRole('link', { name, exact: true })).toBeVisible();
				for (const name of ['New task', 'Sign out'])
					await expect(sidebar.getByRole('button', { name, exact: true })).toBeVisible();
				// A project keeps its name for screen readers, shows it on hover,
				// and leaves no squeezed scrap of it beside the icon.
				const link = sidebar.getByRole('link', { name: project, exact: true });
				await expect(link).toHaveAttribute('title', project);
				expect((await link.locator('.name').boundingBox())?.width).toBeLessThanOrEqual(1);
				await expect(sidebar.getByRole('button', { name: /^Notifications/ })).toHaveAttribute(
					'title',
					'Notifications'
				);
				await axe(page);
				await sidebar.getByRole('button', { name: 'Expand sidebar' }).click();
			});
		});
	});
}

/** Every main screen, as one person in one theme, in light and dark. */
async function everyScreen(browser: Browser, character: string) {
	const { page: own, context } = await newPerson(
		() => browser.newContext(SIGNED_OUT),
		`Theme-${character}`
	);
	await own.goto('/settings/appearance');
	const names: Record<string, string> = {
		calm: 'Calm',
		playful: 'Playful',
		contrast: 'High contrast'
	};
	await own.getByText(names[character], { exact: true }).click();
	await expect(own.locator('html')).toHaveAttribute('data-theme', character);
	const url = await filledProject(own, `Themed ${character}`);
	await own.locator('#main [data-task-id]').first().focus();
	await own.keyboard.press('t');
	const taskId = await own.locator('#main [data-task-id]').first().getAttribute('data-task-id');
	// The open task's notes use every kind of Markdown, in the theme's Pages colors.
	const ws = await workspaceOf(own, taskId!);
	expect(
		(await apiCall(own, 'PATCH', `/workspaces/${ws}/tasks/${taskId}`, { notes: MARKDOWN })).status
	).toBe(200);
	// Time logged to the project, for the Time page by week and its chart.
	await own.goto('/time');
	await own.getByRole('button', { name: 'Log time', exact: true }).click();
	const log = own.getByRole('form', { name: 'Log time to a project' });
	await log.getByLabel('Project').selectOption({ label: `Themed ${character}` });
	await log.getByLabel('Time spent').fill('2h');
	await log.getByRole('button', { name: 'Log it' }).click();
	await expect(log).toHaveCount(0);

	const screens: [string, string][] = [
		['inbox', '/'],
		['my day', '/today'],
		['upcoming', '/upcoming'],
		['projects', '/projects'],
		['project list', url],
		['board', url + '?view=board'],
		['calendar', url + '?view=calendar'],
		['table', url + '?view=table'],
		['timeline', url + '?view=timeline'],
		['templates', '/settings/templates'],
		['time', '/time'],
		['reports', '/reports'],
		['project wiki', url + '?tab=wiki'],
		['project time', url + '?tab=time'],
		['wiki', '/wiki'],
		['open task', `${url}?task=${taskId}`],
		['trash', '/trash'],
		['settings', '/settings'],
		['preferences', '/settings/preferences'],
		['notifications', '/settings/notifications'],
		['help', '/help'],
		['shortcuts', '/help/shortcuts'],
		['quick add help', '/help/quick-add']
	];
	const found: string[] = [];
	for (const scheme of ['light', 'dark'] as Scheme[]) {
		await own.emulateMedia({ colorScheme: scheme });
		for (const [name, path] of screens) {
			await own.goto(path);
			await expect(own.getByRole('heading', { level: 1 }).first()).toBeAttached();
			await expect(own.locator('html')).toHaveAttribute('data-scheme', scheme);
			await own.waitForLoadState('networkidle');
			for (const v of await axeViolations(own)) found.push(`${scheme} ${name}: ${v}`);
		}
		// Inbox, My Day and Upcoming as boards, then back to their lists.
		for (const [name, path] of [
			['inbox', '/'],
			['my day', '/today'],
			['upcoming', '/upcoming']
		]) {
			await own.goto(path);
			await showAs(own, 'Board');
			await expect(own.getByRole('group', { name: 'Board' })).toBeVisible();
			await own.waitForLoadState('networkidle');
			for (const v of await axeViolations(own)) found.push(`${scheme} ${name} board: ${v}`);
			await showAs(own, 'List');
		}
		// The Time page by week, with its chart.
		await own.goto('/time');
		await own
			.getByRole('combobox', { name: 'Period', exact: true })
			.selectOption({ label: 'Last 8 weeks' });
		await expect(own.getByRole('group', { name: /^Hours logged each week by/ })).toBeVisible();
		for (const v of await axeViolations(own)) found.push(`${scheme} time by week: ${v}`);
		await own.goto(url);
		await expect(own.getByLabel('Project name')).toBeVisible();
		await own.locator('body').press('Control+k');
		await expect(own.getByRole('dialog', { name: 'Command palette' })).toBeVisible();
		for (const v of await axeViolations(own)) found.push(`${scheme} palette: ${v}`);
		await own.keyboard.press('Escape');
	}
	await context.close();
	return found;
}

test.describe('every theme', () => {
	test.use({ storageState: OWNER });
	test.skip(({ isMobile }) => isMobile, 'phones are checked in the standard theme above');

	for (const character of ['calm', 'playful', 'contrast']) {
		test(`the main screens pass axe in ${character}, light and dark`, async ({ browser }) => {
			test.setTimeout(240_000);
			expect(await everyScreen(browser, character)).toEqual([]);
		});
	}
});

/**
 * The theme editor, which has no Settings heading of its own: on a theme
 * of each built-in, in light and dark, with its color popover and its
 * problems open; and the manager and editor while a custom theme is in
 * use (the visual tests' sample).
 */
async function editorViolations(own: Page, isMobile: boolean, base: string) {
	const names: Record<string, string> = {
		standard: 'Standard',
		calm: 'Calm',
		playful: 'Playful',
		contrast: 'High contrast'
	};
	// The app in that character, editing a theme on it.
	await own.goto('/settings/appearance');
	// The picker's choice, not the theme manager's Start from, which names it too.
	await own
		.getByRole('group', { name: 'Theme', exact: true })
		.getByText(names[base], { exact: true })
		.click();
	await expect(own.locator('html')).toHaveAttribute('data-theme', base);
	await own.goto('/settings/themes');
	await own.getByLabel('Start from').selectOption({ label: names[base] });
	await own.getByRole('button', { name: 'New theme', exact: true }).click();
	await expect(own.getByLabel('Theme name')).toBeVisible();
	const found: string[] = [];
	for (const scheme of ['light', 'dark'] as Scheme[]) {
		await own.emulateMedia({ colorScheme: scheme });
		await expect(own.locator('html')).toHaveAttribute('data-scheme', scheme);
		for (const v of await axeViolations(own)) found.push(`${scheme} editor: ${v}`);
		if (isMobile) await own.getByRole('button', { name: /^Surfaces/ }).click();
		await own.getByRole('button', { name: /^Page:/ }).click();
		await expect(own.getByLabel('Hex', { exact: true })).toBeVisible();
		for (const v of await axeViolations(own)) found.push(`${scheme} color: ${v}`);
		await own.keyboard.press('Escape');
		await expect(own.getByLabel('Hex', { exact: true })).toBeHidden();
		// The contrast chip opens the problems and the goal.
		const chip = own.getByRole('button', { name: /at AA/ });
		await chip.click();
		await expect(own.getByRole('heading', { name: 'Contrast', exact: true })).toBeVisible();
		for (const v of await axeViolations(own)) found.push(`${scheme} problems: ${v}`);
		await chip.click();
	}
	return found;
}

test.describe('the theme editor', () => {
	test.use({ storageState: OWNER });

	for (const base of ['standard', 'calm', 'playful', 'contrast']) {
		test(`passes axe on a ${base} theme, light and dark`, async ({ browser, isMobile }) => {
			test.slow();
			const { page: own, context } = await newPerson(
				() => browser.newContext(SIGNED_OUT),
				`Editor-${base}`
			);
			expect(await editorViolations(own, isMobile, base)).toEqual([]);
			await context.close();
		});
	}

	test('the manager and the editor pass axe with a custom theme in use', async ({ browser }) => {
		test.slow();
		const { page: own, context } = await newPerson(
			() => browser.newContext(SIGNED_OUT),
			'Editor-custom'
		);
		// An id of its own: the desktop and phone runs are two people.
		const fixture = readFileSync(
			new URL('./visual/fixtures/sample.mila-theme.json', import.meta.url),
			'utf8'
		);
		const sample = { ...JSON.parse(fixture), id: randomUUID() };
		expect((await apiCall(own, 'PUT', `/me/themes/${sample.id}`, sample)).status).toBe(200);
		await own.goto('/settings/themes');
		await own
			.getByRole('list', { name: 'Your themes' })
			.getByRole('button', { name: 'Use Harbor' })
			.click();
		await expect(own.locator('html')).toHaveAttribute('data-custom-theme', sample.id);
		const found: string[] = [];
		for (const scheme of ['light', 'dark'] as Scheme[]) {
			await own.emulateMedia({ colorScheme: scheme });
			await own.goto('/settings/themes');
			await expect(own.getByRole('heading', { name: 'Your themes' })).toBeVisible();
			await own.waitForLoadState('networkidle');
			for (const v of await axeViolations(own)) found.push(`${scheme} manager: ${v}`);
			// A theme's More menu.
			await own
				.getByRole('list', { name: 'Your themes' })
				.getByRole('button', { name: 'More for Harbor' })
				.click();
			await expect(own.getByRole('button', { name: 'Export file' })).toBeVisible();
			for (const v of await axeViolations(own)) found.push(`${scheme} theme menu: ${v}`);
			await own.keyboard.press('Escape');
			await expect(own.getByRole('button', { name: 'Export file' })).toBeHidden();
			// The import's dry run, from the fixture as a file.
			await own.locator('#theme-file').setInputFiles({
				name: 'sample.mila-theme.json',
				mimeType: 'application/json',
				buffer: Buffer.from(fixture)
			});
			const dry = own.getByRole('dialog', { name: /^Import Harbor/ });
			await expect(dry).toBeVisible();
			for (const v of await axeViolations(own)) found.push(`${scheme} import: ${v}`);
			await dry.getByRole('button', { name: 'Cancel' }).click();
			await expect(dry).toBeHidden();
			await own.goto(`/settings/themes/${sample.id}`);
			await expect(own.getByLabel('Theme name')).toHaveValue('Harbor');
			for (const v of await axeViolations(own)) found.push(`${scheme} editor: ${v}`);
			// The preview of the whole app, and a search across the parts.
			await own.getByText('Whole app', { exact: true }).click();
			await own.getByLabel('Find a setting').fill('heading');
			await expect(own.getByRole('button', { name: /^Heading 1:/ })).toBeVisible();
			for (const v of await axeViolations(own)) found.push(`${scheme} whole app, search: ${v}`);
			await own.getByLabel('Find a setting').fill('');
			await own.goto('/settings/appearance');
			// The picker's choice, not Your themes or the manager's Start from,
			// which name it too.
			await expect(
				own.getByRole('group', { name: 'Theme', exact: true }).getByText('Harbor', { exact: true })
			).toBeVisible();
			for (const v of await axeViolations(own)) found.push(`${scheme} appearance: ${v}`);
		}
		expect(found).toEqual([]);
		await context.close();
	});
});
