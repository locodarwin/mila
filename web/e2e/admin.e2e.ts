import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import {
	apiCall,
	emailTo,
	freshSignIn,
	linkFromEmail,
	newMember,
	OWNER,
	SIGNED_OUT,
	uniqueEmail,
	newPerson
} from './helpers';

// The admin page, the security log and sharing projects with guests. The
// owner set up the instance, so they are its admin; anyone else is
// someone new, so changes to them reach no other test.
test.use({ storageState: OWNER });

const accounts = (page: Page) => page.getByRole('list', { name: 'Accounts' });

test('the admin page shows the instance and backs it up', async ({ page }) => {
	await page.goto('/settings');
	await page
		.getByRole('navigation', { name: 'Settings' })
		.getByRole('link', { name: 'Admin' })
		.click();
	await expect(page.getByRole('heading', { name: 'This Mila' })).toBeVisible();
	await expect(page.getByText('Written to a folder (for tests)')).toBeVisible();
	await expect(page.getByText(/^SQLite, schema \d+/)).toBeVisible();

	await page.getByRole('button', { name: 'Back up now' }).click();
	await expect(page.getByText('Backed up', { exact: true })).toBeVisible({ timeout: 20_000 });
	await expect(
		page.getByRole('list', { name: 'Backups kept' }).getByRole('listitem').first()
	).toBeVisible();
	await expect(page.getByText(/^Last backed up/)).toBeVisible();
	await page.waitForLoadState('networkidle');
	await axe(page);
});

test('an admin signs Mila in to a Microsoft mailbox with a code', async ({ page }) => {
	// This server's mailbox is a folder, so the server's side is stood in for
	// here: it is tested in Go against a stand-in for Microsoft.
	type Mailbox = Record<string, unknown> & { state: string };
	let mailbox: Mailbox = { state: 'not-connected', mailbox: 'tasks@contoso.com' };
	let polls = 0;
	await page.route('**/api/v1/admin/status', async (route) => {
		const status = await (await route.fetch()).json();
		if (mailbox.state === 'signing-in' && ++polls >= 2)
			mailbox = {
				state: 'connected',
				mailbox: 'tasks@contoso.com',
				account: 'shawn@contoso.com',
				connectedAt: new Date().toISOString(),
				connectedBy: 'Shawn'
			};
		status.emailIn = { enabled: true, auth: 'microsoft', microsoft: mailbox };
		await route.fulfill({ json: status });
	});
	await page.route('**/api/v1/admin/mailbox/microsoft', async (route) => {
		if (route.request().method() === 'POST') {
			mailbox = {
				state: 'signing-in',
				mailbox: 'tasks@contoso.com',
				userCode: 'K7PQ2XMF',
				verificationUri: 'https://microsoft.com/devicelogin',
				expiresAt: new Date(Date.now() + 15 * 60_000).toISOString()
			};
			await route.fulfill({ json: mailbox });
		} else {
			mailbox = { state: 'not-connected', mailbox: 'tasks@contoso.com' };
			await route.fulfill({ status: 204 });
		}
	});

	await page.goto('/settings/admin');
	const panel = page.getByRole('region', { name: 'Mailbox for email to task' });
	await expect(panel.locator('.pill')).toHaveText('Not connected');
	await expect(page.getByRole('region', { name: 'Not set up yet' })).toContainText(
		'Mila is not signed in to it yet'
	);
	await axe(page);

	await panel.getByRole('button', { name: 'Connect a Microsoft mailbox' }).click();
	await expect(panel.getByText('K7PQ-2XMF')).toBeVisible();
	await expect(panel.getByText('https://microsoft.com/devicelogin')).toBeVisible();
	await axe(page);
	// The page reads the status again by itself until the sign-in is made.
	await expect(panel.locator('.pill')).toHaveText('Connected', { timeout: 10_000 });
	await expect(panel).toContainText('tasks@contoso.com, signed in as shawn@contoso.com');
	// A sign-in as someone else reaches their other mailboxes too, and says so.
	await expect(panel).toContainText('every mailbox shawn@contoso.com can open');
	await expect(panel).toContainText('by Shawn');
	await expect(page.getByRole('region', { name: 'Not set up yet' })).toHaveCount(0);

	await panel.getByRole('button', { name: 'Disconnect' }).click();
	await panel
		.getByRole('group', { name: 'Disconnect the mailbox' })
		.getByRole('button', { name: 'Disconnect' })
		.click();
	await expect(panel.locator('.pill')).toHaveText('Not connected');
});

test('someone else has no admin page, and the API refuses them', async ({ browser }) => {
	const { page: dana, context } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Nadmin');
	await dana.goto('/settings');
	await expect(
		dana
			.getByRole('navigation', { name: 'Settings' })
			.getByRole('link', { name: 'Import and export' })
	).toBeVisible();
	await expect(
		dana.getByRole('navigation', { name: 'Settings' }).getByRole('link', { name: 'Admin' })
	).toHaveCount(0);
	for (const path of ['/admin/status', '/admin/users', '/admin/invites']) {
		expect((await apiCall(dana, 'GET', path)).status).toBe(403);
	}
	expect((await apiCall(dana, 'POST', '/admin/backups')).status).toBe(403);
	await dana.goto('/settings/admin');
	await expect(dana.getByText('Only an admin of this Mila can see this page.')).toBeVisible();
	await context.close();
});

test('an admin makes someone an admin, and signs them out everywhere', async ({
	page,
	browser
}) => {
	const {
		page: dana,
		context,
		email
	} = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Ada');
	// Making someone an admin needs a sign-in from the last ten minutes,
	// which the owner's session, made when the run began, may not be.
	const ownerContext = await browser.newContext(SIGNED_OUT);
	const owner = await freshSignIn(ownerContext, 'owner@example.com');
	await owner.goto('/settings/admin');
	const row = accounts(owner).getByRole('listitem').filter({ hasText: email });
	await row.getByRole('checkbox', { name: 'Admin: Ada' }).check();
	await expect(owner.getByText('Ada is now an admin')).toBeVisible();
	await expect(row.locator('.badge', { hasText: 'Admin' })).toBeVisible();

	await dana.goto('/settings');
	await expect(
		dana.getByRole('navigation', { name: 'Settings' }).getByRole('link', { name: 'Admin' })
	).toBeVisible();
	await row.getByRole('checkbox', { name: 'Admin: Ada' }).uncheck();
	await expect(owner.getByText('Ada is no longer an admin')).toBeVisible();

	// Signing out everywhere asks twice; her app leaves at its next request.
	await row.getByRole('button', { name: 'Sign out everywhere: Ada' }).click();
	await row.getByRole('button', { name: 'Confirm sign out: Ada' }).click();
	await expect(owner.getByText('Signed Ada out of 1 device')).toBeVisible();
	await dana.goto('/upcoming');
	await expect(dana).toHaveURL(/\/login/);

	// Her log says so.
	const again = await (await browser.newContext(SIGNED_OUT)).newPage();
	await again.goto('/login');
	await again.getByLabel('Email').fill(email);
	await again.getByRole('button', { name: 'Email me a sign-in link' }).click();
	await expect(again.getByRole('heading', { name: 'Check your email' })).toBeVisible();
	await again.goto(await linkFromEmail(email));
	await expect(again.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	await again.goto('/settings/security');
	const log = again.getByRole('list', { name: 'Recent activity' });
	await expect(log.getByText(/^Signed out on every device by .+, an admin/)).toBeVisible();
	await expect(log.getByText(/^No longer an admin, by/)).toBeVisible();
	await context.close();
	await again.context().close();
	await ownerContext.close();
});

test('an admin cancels any open invitation', async ({ page }) => {
	await page.goto('/');
	const ws = await apiCall(page, 'POST', '/workspaces', { name: `Invites ${Date.now()}` });
	const email = uniqueEmail('pending');
	expect(
		(await apiCall(page, 'POST', `/workspaces/${ws.body.id}/invites`, { email, role: 'guest' }))
			.status
	).toBe(201);
	await page.goto('/settings/admin');
	const open = page.getByRole('list', { name: 'Open invitations' });
	await open.getByRole('button', { name: `Cancel: the invitation to ${email}` }).click();
	await open.getByRole('button', { name: `Confirm cancel: the invitation to ${email}` }).click();
	await expect(page.getByText(`Canceled the invitation to ${email}`)).toBeVisible();
	await expect(open.getByText(email)).toHaveCount(0);
});

test('a sign-in from a new browser is in the log and emailed', async ({ browser }) => {
	const { page: bo, context, email } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Bo');
	const phone = await (await browser.newContext(SIGNED_OUT)).newPage();
	await phone.goto('/login');
	await phone.getByLabel('Email').fill(email);
	await phone.getByRole('button', { name: 'Email me a sign-in link' }).click();
	await expect(phone.getByRole('heading', { name: 'Check your email' })).toBeVisible();
	await phone.goto(await linkFromEmail(email));
	await expect(phone.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	const mail = await emailTo(email, /^Mila: new sign-in on /);
	expect(mail).toContain('/settings/security');

	await bo.goto('/settings/security');
	const log = bo.getByRole('list', { name: 'Recent activity' });
	const signIn = log.getByRole('listitem').filter({ hasText: 'Signed in with an email link' });
	await expect(signIn).toContainText('New device');
	await expect(log.getByText('Signed in by accepting an invitation')).toBeVisible();
	await expect(log.getByText(/^Joined /)).toBeVisible();
	// The devices list says where each was last used.
	await expect(
		bo
			.getByRole('list', { name: 'Signed-in devices' })
			.getByText(/127\.0\.0\.1/)
			.first()
	).toBeVisible();
	await bo.waitForLoadState('networkidle');
	await axe(bo);
	await context.close();
	await phone.context().close();
});

test('a guest is given a project and loses it again', async ({ page, browser, isMobile }) => {
	const {
		page: gus,
		context,
		workspace
	} = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Gus', 'guest');
	const name = `Garden ${Date.now()}`;
	// The owner works in the shared workspace.
	await page.evaluate((id) => localStorage.setItem('mila.workspace', id), workspace.id);
	await page.goto('/projects');
	await page.getByLabel('Name a new project').fill(name);
	await page.getByLabel('Name a new project').press('Enter');
	await expect(page.getByLabel('Project name')).toHaveValue(name);
	await page.getByLabel('New task').fill('Plant beans');
	await page.getByLabel('New task').press('Enter');

	await gus.evaluate((id) => localStorage.setItem('mila.workspace', id), workspace.id);
	await gus.goto('/projects');
	await expect(gus.getByRole('heading', { name: 'Projects', level: 1 })).toBeVisible();
	await expect(gus.getByText(name)).toHaveCount(0);

	await page.getByRole('button', { name: `Share ${name} with guests` }).click();
	const dialog = page.getByRole('dialog', { name: `Share ${name}` });
	const box = dialog.getByRole('checkbox', { name: `Gus can see ${name}` });
	await box.check();
	await expect(page.getByText(`Shared ${name} with Gus`).last()).toBeVisible();
	await expect(box).toBeChecked();
	if (!isMobile) await axe(page);

	// His open app loads the workspace afresh, and the project is there.
	await expect(gus.locator('#main').getByText(name)).toBeVisible({ timeout: 15_000 });
	await gus.locator('#main').getByText(name).click();
	await expect(
		gus.locator('#main [data-task-id]').filter({ hasText: 'Plant beans' })
	).toBeVisible();

	await box.uncheck();
	await expect(page.getByText(`${name} is no longer shared with Gus`).last()).toBeVisible();
	await dialog.getByRole('button', { name: 'Done' }).click();
	await expect(dialog).toBeHidden();
	await gus.goto('/projects');
	await expect(gus.locator('#main').getByText(name)).toHaveCount(0, { timeout: 15_000 });
	await context.close();
});
