import { expect, test } from '@playwright/test';
import { axe } from './axe';
import { apiCall, newMember, newPerson, OWNER, SIGNED_OUT } from './helpers';

test.use({ storageState: OWNER });
test.skip(({ isMobile }) => isMobile, 'the phone sees the same forms; phone.e2e.ts measures them');

test('a token is made, shown once, works as its owner, and is revoked', async ({
	browser,
	request
}, info) => {
	const { page: own, context } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Tess');
	const origin = info.project.use.baseURL!;
	await own.goto('/settings/tokens');
	await expect(own.getByRole('heading', { name: 'API tokens', exact: true })).toBeVisible();
	await expect(own.getByText('You have no API tokens.')).toBeVisible();
	await axe(own);

	await own.getByLabel('Name').fill('Backup script');
	await own.getByLabel('Where').selectOption({ label: 'All my workspaces, now and later' });
	await own.getByRole('button', { name: 'Make a token' }).click();
	const secretField = own.getByRole('textbox', { name: 'Your new token, Backup script' });
	await expect(secretField).toBeFocused();
	const secret = await secretField.inputValue();
	expect(secret).toMatch(/^mila_/);
	await axe(own);

	// The list says what it is, and never the token.
	const list = own.getByRole('list', { name: 'API tokens' });
	await expect(list).toContainText('Backup script');
	await expect(list).toContainText('Read only');
	await expect(list).toContainText('not used yet');
	await expect(list).not.toContainText(secret);

	// It works, from outside the browser, within what it was made for.
	const headers = { Authorization: `Bearer ${secret}` };
	const me = await request.get(`${origin}/api/v1/me`, { headers });
	expect(me.status()).toBe(200);
	const ws = await apiCall(own, 'GET', '/workspaces');
	const tasks = `${origin}/api/v1/workspaces/${ws.body[0].id}/tasks`;
	expect((await request.get(tasks, { headers })).status()).toBe(200);
	const refused = await request.post(tasks, { headers, data: { title: 'No' } });
	expect(refused.status()).toBe(403);
	expect(await refused.text()).toContain('read only');
	expect((await request.get(`${origin}/api/v1/me/tokens`, { headers })).status()).toBe(403);

	await own.reload();
	await expect(own.getByRole('textbox', { name: 'Your new token, Backup script' })).toHaveCount(0);
	await expect(list).toContainText('last used');

	// In the security log.
	await own.goto('/settings/security');
	await expect(own.getByText('Made the API token Backup script')).toBeVisible();

	await own.goto('/settings/tokens');
	await own.getByRole('button', { name: 'Revoke Backup script' }).click();
	await own.getByRole('button', { name: 'Confirm revoke Backup script' }).click();
	await expect(own.getByText('You have no API tokens.')).toBeVisible();
	expect((await request.get(`${origin}/api/v1/me`, { headers })).status()).toBe(401);
	await context.close();
});

test('a webhook is added, tested, paused and deleted by an admin only', async ({
	page,
	browser
}) => {
	const {
		page: admin,
		workspace,
		context
	} = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Ada', 'admin');
	await admin.goto(`/settings/w/${workspace.id}/webhooks`);
	await expect(admin.getByRole('heading', { name: 'Webhooks', exact: true })).toBeVisible();
	await expect(admin.getByText('This workspace has no webhooks.')).toBeVisible();
	await axe(admin);

	// The server's rule, in the form's words.
	const address = admin.getByLabel('Address');
	await address.fill('https://127.0.0.1/hook');
	await admin.getByRole('button', { name: 'Add webhook' }).click();
	await expect(admin.getByText('must not point inside this server')).toBeVisible();

	await address.fill('https://hooks.example.com/mila');
	await admin.getByRole('checkbox', { name: /Comments/ }).check();
	await admin.getByRole('button', { name: 'Add webhook' }).click();
	const secret = admin.getByRole('textbox', { name: 'Its signing secret' });
	await expect(secret).toBeFocused();
	expect(await secret.inputValue()).toMatch(/^whsec_/);
	await admin.getByRole('button', { name: 'Done' }).click();

	const hook = admin.locator('details.hook');
	await expect(hook).toContainText('https://hooks.example.com/mila');
	await expect(hook).toContainText('Tasks, Comments');
	await expect(hook).toContainText('Working');

	await hook.locator('summary').click();
	await expect(hook.getByText('Nothing sent yet.', { exact: false })).toBeVisible();
	await hook.getByRole('button', { name: 'Send a test' }).click();
	await expect(hook.getByRole('table')).toContainText('ping');
	await axe(admin);

	await hook.getByRole('button', { name: 'Pause' }).click();
	await expect(hook).toContainText('Paused');
	await hook.getByRole('button', { name: 'Turn back on' }).click();
	await expect(hook).toContainText('Working');

	await hook.getByRole('button', { name: /^Delete the webhook/ }).click();
	await hook.getByRole('button', { name: /^Confirm delete/ }).click();
	await expect(admin.getByText('This workspace has no webhooks.')).toBeVisible();
	await context.close();
});

test('a member has no Webhooks tab, and the page says why', async ({ page, browser }) => {
	const {
		page: dana,
		workspace,
		context
	} = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Dana');
	await dana.goto(`/settings?ws=${workspace.id}`);
	await expect(dana.getByRole('link', { name: 'API tokens' })).toBeVisible();
	await expect(dana.getByRole('link', { name: 'Webhooks' })).toHaveCount(0);
	await dana.goto('/settings/webhooks');
	await expect(dana.getByText('Only owners and admins', { exact: false })).toBeVisible();
	await context.close();
});

test("a task's history names the token a change came through", async ({
	browser,
	request
}, info) => {
	const {
		page: own,
		context,
		workspace
	} = await newPerson(() => browser.newContext(SIGNED_OUT), 'Vic');
	const origin = info.project.use.baseURL!;
	await own.goto('/settings/tokens');
	await own.getByLabel('Name').fill('Zapier');
	await own.getByLabel('Where').selectOption({ label: 'All my workspaces, now and later' });
	await own.getByRole('radio', { name: 'Read and write' }).check();
	await own.getByRole('button', { name: 'Make a token' }).click();
	const secret = await own.getByRole('textbox', { name: 'Your new token, Zapier' }).inputValue();
	const headers = { Authorization: `Bearer ${secret}` };
	const made = await request.post(`${origin}/api/v1/workspaces/${workspace.id}/tasks`, {
		headers,
		data: { title: 'From a script', sort_key: 'a0' }
	});
	expect(made.status()).toBe(201);
	const { id } = await made.json();
	await own.goto(`/?task=${id}&ws=${workspace.id}`);
	const history = own.getByRole('list', { name: 'History' });
	await own.getByRole('button', { name: 'Show history' }).click();
	await expect(history).toContainText('through Zapier');
	await context.close();
});
