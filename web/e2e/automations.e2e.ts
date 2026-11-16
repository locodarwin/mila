import { expect, test } from '@playwright/test';
import { apiCall, newMember, newPerson, OWNER, SIGNED_OUT } from './helpers';
import { axe } from './axe';

test.use({ storageState: OWNER });

test.describe('automations', () => {
	test.skip(
		({ isMobile }) => isMobile,
		'the builder is tried at the desktop width; the phone test measures it'
	);

	test('a rule made here runs when its task changes, and says so', async ({ browser }) => {
		const { page, context } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Rae');
		const spaces = (await apiCall(page, 'GET', '/workspaces')).body;
		const ws = (spaces.items ?? spaces.workspaces ?? spaces)[0].id;
		const project = (
			await apiCall(page, 'POST', `/workspaces/${ws}/projects`, { name: `Rules ${Date.now()}` })
		).body;
		await page.goto(`/settings/w/${ws}/automations`);
		await expect(page.getByRole('heading', { level: 2, name: 'Automations' })).toBeVisible();
		await expect(page.getByText('No automations yet.')).toBeVisible();

		await page.getByRole('button', { name: 'New automation' }).click();
		const builder = page.getByRole('region', { name: 'New automation' });
		await builder.getByLabel('Name').fill('Urgent on arrival');
		await builder.getByRole('combobox', { name: 'In', exact: true }).selectOption(project.id);
		await builder.getByRole('combobox', { name: 'Change 1', exact: true }).selectOption('priority');
		await builder.getByRole('combobox', { name: 'Change 1, which' }).selectOption('urgent');
		await builder.getByRole('button', { name: 'Add a change' }).click();
		await builder.getByRole('combobox', { name: 'Change 2', exact: true }).selectOption('subtask');
		await builder.getByLabel('What it says').fill('Call them back');

		// The rule reads as a sentence, and the test says what it would do, changing nothing.
		await expect(builder.getByRole('status')).toContainText('When a task is created in Rules');
		await expect(builder.getByRole('status')).toContainText(
			'set the priority to Urgent and add a subtask, Call them back'
		);
		await axe(page);
		await builder.getByRole('button', { name: 'Save' }).click();
		const list = page.getByRole('list', { name: 'Automations' });
		await expect(list.getByText('Urgent on arrival')).toBeVisible();
		await expect(builder).toBeHidden();

		// Once the rule has reached the server, a task made in the project gets the changes.
		await expect
			.poll(async () => {
				const rows = (await apiCall(page, 'GET', `/workspaces/${ws}/automations`)).body;
				return (rows.items ?? rows).length;
			})
			.toBe(1);
		const made = await apiCall(page, 'POST', `/workspaces/${ws}/tasks`, {
			title: 'New order',
			project_id: project.id
		});
		expect(made.status).toBe(201);
		await expect
			.poll(
				async () =>
					(await apiCall(page, 'GET', `/workspaces/${ws}/tasks/${made.body.id}`)).body.priority,
				{
					timeout: 15_000
				}
			)
			.toBe('urgent');

		await page.reload();
		const runs = page.getByRole('region', { name: 'Recent runs' });
		await expect(runs.getByRole('cell', { name: 'New order' })).toBeVisible();
		await expect(runs.getByText('Ran', { exact: true })).toBeVisible();

		// Off, it does nothing.
		await list.getByRole('checkbox', { name: 'Urgent on arrival is on' }).uncheck();
		await expect
			.poll(async () => {
				const rows = (await apiCall(page, 'GET', `/workspaces/${ws}/automations`)).body;
				return (rows.items ?? rows)[0].enabled;
			})
			.toBe(false);
		const later = await apiCall(page, 'POST', `/workspaces/${ws}/tasks`, {
			title: 'Quiet',
			project_id: project.id
		});
		await page.waitForTimeout(1500);
		expect(
			(await apiCall(page, 'GET', `/workspaces/${ws}/tasks/${later.body.id}`)).body.priority
		).toBe('none');

		// Deleting offers Undo.
		await list.getByRole('button', { name: 'Delete Urgent on arrival' }).click();
		await expect(page.getByText('No automations yet.')).toBeVisible();
		await page.getByRole('button', { name: 'Undo' }).click();
		await expect(list.getByText('Urgent on arrival')).toBeVisible();
		await context.close();
	});

	test('a member can make rules for a project only, and guests have none', async ({
		page,
		browser
	}) => {
		const { context, page: dana } = await newMember(
			page,
			() => browser.newContext(SIGNED_OUT),
			'Dana'
		);
		await dana.goto('/settings/automations');
		await dana.getByRole('button', { name: 'New automation' }).click();
		const place = dana
			.getByRole('region', { name: 'New automation' })
			.getByRole('combobox', { name: 'In', exact: true });
		await expect(place.getByRole('option', { name: 'The whole workspace' })).toHaveCount(0);
		await context.close();

		const guest = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Lee', 'guest');
		await guest.page.goto('/settings');
		await expect(guest.page.getByRole('link', { name: 'Automations' })).toHaveCount(0);
		await guest.context.close();
	});
});
