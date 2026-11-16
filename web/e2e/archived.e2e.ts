import { expect, test } from '@playwright/test';
import { apiCall, OWNER, SIGNED_OUT, newPerson } from './helpers';

test.use({ storageState: OWNER });

test('an archived project keeps out of the lists across projects', async ({ browser }) => {
	const open = () => browser.newContext(SIGNED_OUT);
	const { page: own, workspace } = await newPerson(open, 'Ari');
	const ws = workspace.id;
	const make = async (entity: string, body: unknown) =>
		(await apiCall(own, 'POST', `/workspaces/${ws}/${entity}`, body)).body.id as string;
	const project = await make('projects', { name: 'Old project' });
	// Overdue here whatever the time zone, so My Day suggests it too.
	const due = new Date(Date.now() - 3 * 86_400_000).toISOString().slice(0, 10);
	await make('tasks', { project_id: project, title: 'Packed away', due_date: due });
	await make('tasks', { project_id: null, title: 'Still around', due_date: due });
	await make('pages', { project_id: project, title: 'Packed page' }).catch(() => '');

	await own.goto(`/upcoming?ws=${ws}`);
	await expect(own.getByText('Packed away').first()).toBeVisible();

	const archived = await apiCall(own, 'PATCH', `/workspaces/${ws}/projects/${project}`, {
		archived_at: new Date().toISOString()
	});
	expect(archived.status).toBeLessThan(300);
	await own.reload();

	await expect(own.getByText('Still around').first()).toBeVisible();
	await expect(own.getByText('Packed away')).toHaveCount(0);

	await own.goto(`/today?ws=${ws}`);
	await expect(own.getByText('Still around').first()).toBeVisible();
	await expect(own.getByText('Packed away')).toHaveCount(0);

	await own.keyboard.press('Control+k');
	const search = own.getByRole('combobox');
	await search.fill('Packed');
	await expect(
		own.getByRole('listbox', { name: 'Results' }).getByText('Nothing matches.')
	).toBeVisible();

	// The palette goes to Time as the key does.
	await search.fill('Go to Time');
	await own.keyboard.press('Enter');
	await expect(own).toHaveURL(/\/time/);
});

test('making a feed offers to sign in again when it asks for that', async ({ browser }) => {
	const { page: own, workspace } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Fay');
	await own.route(/\/calendar-feeds$/, (route) =>
		route.request().method() === 'POST'
			? route.fulfill({
					status: 403,
					contentType: 'application/json',
					body: JSON.stringify({ error: 'sign_in_again', message: 'Please sign in again first.' })
				})
			: route.continue()
	);
	await own.goto(`/settings/w/${workspace.id}/calendar`);
	await own.getByRole('button', { name: 'Make a feed' }).click();
	await expect(own.getByRole('button', { name: 'Sign in again' })).toBeVisible();
});
