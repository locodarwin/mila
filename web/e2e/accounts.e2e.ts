import { expect, request, test, type Page } from '@playwright/test';
import { axe } from './axe';
import {
	apiCall,
	INVITER,
	emailTo,
	linkFromEmail,
	newMember,
	OWNER,
	SIGNED_OUT,
	uniqueEmail,
	newPerson
} from './helpers';

test.use({ storageState: OWNER });

test('signed-out visitors are sent to sign in, then back', async ({ browser }) => {
	const context = await browser.newContext(SIGNED_OUT);
	const page = await context.newPage();
	await page.goto('/upcoming');
	await expect(page).toHaveURL(/\/login\?next=%2Fupcoming$/);
	await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
	await context.close();
});

test('the owner can rename themselves', async ({ page }) => {
	await page.goto('/settings/profile');
	const name = page.getByLabel('Name');
	await expect(name).toHaveValue(/Shawn/);
	await name.fill('Shawn');
	await expect(page.getByText('Signed in as owner@example.com')).toBeVisible();
});

test('an invited person joins, then signs in again with an email link', async ({ browser }) => {
	const {
		context,
		page: dana,
		email,
		workspace
	} = await newPerson(() => browser.newContext(SIGNED_OUT), 'Dana');

	// Accepting opens the workspace Dana joined; her own is in the menu.
	const switcher = dana.getByRole('button', { name: /^Switch workspace/ });
	await expect(switcher).toContainText(workspace.name);
	await switcher.click();
	await expect(dana.getByRole('button', { name: 'Personal' })).toBeVisible();
	await dana.keyboard.press('Escape');

	// Sign out, then back in by email.
	await signOut(dana);
	await dana.getByLabel('Email').fill(email);
	await dana.getByRole('button', { name: 'Email me a sign-in link' }).click();
	await expect(dana.getByRole('heading', { name: 'Check your email' })).toBeVisible();
	await dana.goto(await linkFromEmail(email));
	await expect(dana.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();

	// The link works once.
	const used = await linkFromEmail(email);
	const other = await (await browser.newContext(SIGNED_OUT)).newPage();
	await other.goto(used);
	await expect(other.getByRole('heading', { name: 'That link did not work' })).toBeVisible();
	await context.close();
});

test('an invitation opened while signed in as someone else says so', async ({ browser }) => {
	const {
		context,
		page: dana,
		workspace
	} = await newPerson(() => browser.newContext(SIGNED_OUT), 'Dana');
	const baseURL = test.info().project.use.baseURL!;
	const inviter = await request.newContext({
		baseURL,
		storageState: INVITER,
		extraHTTPHeaders: { Origin: baseURL }
	});
	const invite = async (ws: string, email: string) =>
		expect(
			(
				await inviter.post(`/api/v1/workspaces/${ws}/invites`, { data: { email, role: 'member' } })
			).status()
		).toBe(201);

	// To the workspace Dana is in already: it says so, and opens it.
	const fay = uniqueEmail('fay');
	await invite(workspace.id, fay);
	await dana.goto(await linkFromEmail(fay));
	await expect(dana.getByText(/who is in Dana's team already/)).toBeVisible();
	await expect(dana.getByText(`To accept it as ${fay}, sign out first.`)).toBeVisible();
	await expect(dana.getByRole('button', { name: 'Sign out first' })).toBeVisible();
	await dana.getByRole('button', { name: "Open Dana's team" }).click();
	await expect(dana.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();

	// To another workspace, for another address: Dana can join as herself, or
	// sign out and let it go to the address it was sent to.
	const other = await inviter.post('/api/v1/workspaces', { data: { name: "Erin's team" } });
	const erin = uniqueEmail('erin');
	await invite((await other.json()).id, erin);
	await dana.goto(await linkFromEmail(erin));
	await expect(dana.getByText(`This invitation was sent to ${erin}.`)).toBeVisible();
	await expect(
		dana.getByText(`To accept it as ${erin}, sign out first, or join as Dana.`)
	).toBeVisible();
	await expect(dana.getByRole('button', { name: 'Join as Dana' })).toBeVisible();
	await axe(dana);
	await dana.getByRole('button', { name: 'Sign out first' }).click();
	await dana.getByLabel('Your name').fill('Erin');
	await dana.getByRole('button', { name: "Join Erin's team" }).click();
	await expect(dana.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	expect((await apiCall(dana, 'GET', '/me')).body.email).toBe(erin);
	await inviter.dispose();
	await context.close();
});

// Signs out from Settings, which works on phones and desktops alike.
async function signOut(page: Page) {
	await page.goto('/settings/security');
	await page
		.getByRole('list', { name: 'Signed-in devices' })
		.getByRole('listitem')
		.filter({ hasText: 'This device' })
		.getByRole('button', { name: 'Sign out' })
		.click();
	await expect(page).toHaveURL(/\/login/);
}

test('owners manage members and invitations', async ({ page, browser }) => {
	const { context, workspace } = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Ari');
	await page.goto(`/settings/w/${workspace.id}/people`);

	const members = page.getByRole('list', { name: 'Members' });
	const ari = members.getByRole('listitem').filter({ hasText: 'Ari' });
	await expect(ari).toBeVisible();
	await members.getByLabel('Role for Ari').selectOption('admin');
	await expect(members.getByLabel('Role for Ari')).toHaveValue('admin');

	await page.getByLabel('Email').fill('later@example.com');
	await page.getByRole('button', { name: 'Send invitation' }).click();
	const open = page.getByRole('list', { name: 'Open invitations' });
	await expect(open.getByText('later@example.com')).toBeVisible();
	await open.getByRole('button', { name: 'Cancel' }).click();
	await expect(open).toBeHidden();

	await ari.getByRole('button', { name: 'Remove' }).click();
	await expect(ari).toBeHidden();
	await context.close();
});

test('signing out another device ends its session', async ({ browser }) => {
	const {
		context,
		page: bea,
		email
	} = await newPerson(() => browser.newContext(SIGNED_OUT), 'Bea');
	// A second device, signed in by email.
	const laptop = await (await browser.newContext(SIGNED_OUT)).newPage();
	await laptop.goto('/login');
	await laptop.getByLabel('Email').fill(email);
	await laptop.getByRole('button', { name: 'Email me a sign-in link' }).click();
	// Wait for the email to go out, or the invite sent earlier is the newest.
	await expect(laptop.getByRole('heading', { name: 'Check your email' })).toBeVisible();
	await laptop.goto(await linkFromEmail(email));
	await expect(laptop.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();

	await bea.goto('/settings/security');
	const devices = bea.getByRole('list', { name: 'Signed-in devices' });
	await expect(devices.getByRole('listitem')).toHaveCount(2);
	await devices
		.getByRole('listitem')
		.filter({ hasNotText: 'This device' })
		.getByRole('button', { name: 'Sign out' })
		.click();
	await expect(devices.getByRole('listitem')).toHaveCount(1);

	await laptop.goto('/upcoming');
	await expect(laptop).toHaveURL(/\/login/);
	await context.close();
});

test('a new email address takes over once its link is opened', async ({ browser }) => {
	const {
		context,
		page: dana,
		email
	} = await newPerson(() => browser.newContext(SIGNED_OUT), 'Dana');
	const next = uniqueEmail('dana-new');
	await dana.goto('/settings/profile');
	await dana.getByLabel('New address').fill(next);
	await dana.getByRole('button', { name: 'Send link' }).click();
	await expect(dana.getByText(`We sent a link to ${next}.`)).toBeVisible();
	await expect(dana.getByText(`Signed in as ${email}.`)).toBeVisible();

	// The link opens wherever the mail is read; Dana's open Settings follow.
	const mailApp = await browser.newContext(SIGNED_OUT);
	const opened = await mailApp.newPage();
	await opened.goto(await linkFromEmail(next));
	await expect(opened.getByRole('heading', { name: 'Email address changed' })).toBeVisible();
	await expect(opened.getByText(`is now ${next}.`)).toBeVisible();
	await expect(dana.getByText(`Signed in as ${next}.`)).toBeVisible();
	await emailTo(email, /your email address changed/, next);

	// It works once.
	await opened.reload();
	await expect(opened.getByRole('heading', { name: 'That link did not work' })).toBeVisible();

	await dana.goto('/settings/security');
	await expect(dana.getByText(`Changed the email address from ${email} to ${next}`)).toBeVisible();
	await mailApp.close();
	await context.close();
});

test('signing in again by email link comes back, and leaves one session', async ({ browser }) => {
	const {
		context,
		page: dana,
		email
	} = await newPerson(() => browser.newContext(SIGNED_OUT), 'Dana');
	// Where "Sign in again" leads when something needs a recent sign-in.
	await dana.goto('/login?again=1&next=%2Fsettings%2Fsecurity');
	await expect(dana.getByRole('heading', { name: 'Sign in again' })).toBeVisible();
	await expect(dana.getByLabel('Email')).toHaveValue(email);
	await dana.getByRole('button', { name: /email/i }).click();
	await expect(dana.getByRole('heading', { name: 'Check your email' })).toBeVisible();
	await dana.goto(await linkFromEmail(email));
	await expect(dana).toHaveURL(/\/settings\/security$/);
	await expect(
		dana.getByRole('list', { name: 'Signed-in devices' }).getByRole('listitem')
	).toHaveCount(1);
	await context.close();
});

test('a sign-in link for another account asks before switching', async ({ browser }) => {
	const { context, page: dana } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Dana');
	const res = await apiCall(dana, 'POST', '/auth/magic-link', { email: 'owner@example.com' });
	expect(res.status).toBe(204);
	await dana.goto(await linkFromEmail('owner@example.com'));
	await expect(dana.getByRole('heading', { name: 'Switch accounts?' })).toBeVisible();
	await expect(dana.getByText('This link signs in to owner@example.com.')).toBeVisible();
	await dana.getByRole('link', { name: /^Stay signed in to dana-/ }).click();
	await expect(dana.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	const me = await apiCall(dana, 'GET', '/me');
	expect(me.body.name).toBe('Dana');
	await context.close();
});

test('switching accounts by link leaves nothing of the first on the device', async ({
	browser
}) => {
	const {
		context,
		page: dana,
		workspace
	} = await newPerson(() => browser.newContext(SIGNED_OUT), 'Dana');
	const eli = await newPerson(() => browser.newContext(SIGNED_OUT), 'Eli');
	await eli.context.close();
	await dana.goto('/');
	await expect(dana.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	// One change that cannot be sent yet.
	// Every tab of hers shares the edits on the device, so none may send it.
	await context.route('**/api/v1/workspaces/*/sync/push', (route) => route.abort());
	await dana.getByLabel('New task').fill('Dana alone knows this');
	await dana.getByLabel('New task').press('Enter');
	await expect(
		dana.getByRole('button', { name: 'Dana alone knows this', exact: true })
	).toBeVisible();
	// A second tab of Dana's, open meanwhile.
	const other = await context.newPage();
	await other.goto('/');
	await expect(other.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();

	const res = await apiCall(dana, 'POST', '/auth/magic-link', { email: eli.email });
	expect(res.status).toBe(204);
	await dana.goto(await linkFromEmail(eli.email));
	await expect(dana.getByRole('heading', { name: 'Switch accounts?' })).toBeVisible();
	await expect(dana.getByText(/1 change not sent yet included/)).toBeVisible();
	await context.unroute('**/api/v1/workspaces/*/sync/push');
	await dana.getByRole('button', { name: `Sign in to ${eli.email}` }).click();
	await expect(dana.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	expect((await apiCall(dana, 'GET', '/me')).body.name).toBe('Eli');

	// Nothing of Dana's is left on the device, and her change went nowhere.
	const held = await dana.evaluate(async (ws) => {
		const db = await new Promise<IDBDatabase>((ok, fail) => {
			const req = indexedDB.open('mila');
			req.onsuccess = () => ok(req.result);
			req.onerror = () => fail(req.error);
		});
		const count = (store: string) =>
			new Promise<number>((ok) => {
				const req = db.transaction(store).objectStore(store).index('ws').count(ws);
				req.onsuccess = () => ok(req.result);
			});
		const n = { rows: await count('rows'), outbox: await count('outbox') };
		db.close();
		return n;
	}, workspace.id);
	expect(held).toEqual({ rows: 0, outbox: 0 });
	const tasks = await apiCall(dana, 'GET', `/workspaces/${workspace.id}/tasks`);
	expect(tasks.status).not.toBe(200);

	// The other tab starts again as Eli.
	await expect(other.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	await expect.poll(() => other.evaluate(() => localStorage.getItem('mila.owner'))).not.toBe(null);
	expect(await other.evaluate(() => localStorage.getItem('mila.owner'))).toBe(
		await dana.evaluate(() => localStorage.getItem('mila.owner'))
	);
	await context.close();
});

test('a tab that finds its session ended signs nobody else out', async ({ browser }) => {
	const { context, page: dana } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Dana');
	const eli = await newPerson(() => browser.newContext(SIGNED_OUT), 'Eli');
	await eli.context.close();
	await dana.goto('/');
	await expect(dana.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	const other = await context.newPage();
	await other.goto('/');
	await expect(other.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();

	expect((await apiCall(dana, 'POST', '/auth/magic-link', { email: eli.email })).status).toBe(204);
	await dana.goto(await linkFromEmail(eli.email));
	await dana.getByRole('button', { name: `Sign in to ${eli.email}` }).click();
	await expect(dana.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	// The other tab starts again as Eli.
	const eliId = await dana.evaluate(() => localStorage.getItem('mila.owner'));
	await expect.poll(() => other.evaluate(() => localStorage.getItem('mila.owner'))).toBe(eliId);
	await expect(other.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();

	// One push comes back refused, as a request sent with Dana's session
	// just before it ended does.
	let stale = true;
	await other.route('**/api/v1/workspaces/*/sync/push', (route) => {
		if (!stale) return route.continue();
		stale = false;
		return route.fulfill({ status: 401, json: { error: 'unauthorized', message: 'Sign in.' } });
	});
	const refused = other.waitForResponse(
		(r) => r.url().endsWith('/sync/push') && r.status() === 401
	);
	await other.getByLabel('New task').fill('Eli writes this');
	await other.getByLabel('New task').press('Enter');
	await refused;
	// The tab either starts again or goes to sign in; wait for whichever.
	await Promise.race([other.waitForEvent('load'), other.waitForURL(/\/login/)]);

	// Eli's session is not the tab's to end: it starts again, as Eli, with
	// the task still there.
	await expect(other.getByRole('button', { name: 'Eli writes this', exact: true })).toBeVisible();
	await expect(other).not.toHaveURL(/\/login/);
	expect((await apiCall(dana, 'GET', '/me')).body.name).toBe('Eli');
	await context.close();
});

test('a sign-in link this browser did not ask for asks before it signs in', async ({ browser }) => {
	const { context, email } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Dana');
	await context.close();
	const elsewhere = await browser.newContext(SIGNED_OUT);
	const page = await elsewhere.newPage();
	await page.goto('/login');
	const res = await apiCall(page, 'POST', '/auth/magic-link', { email });
	expect(res.status).toBe(204);
	const link = await linkFromEmail(email);
	await page.goto(link);
	await expect(page.getByRole('heading', { name: 'Sign in to Mila?' })).toBeVisible();
	// Opening it spent nothing.
	expect((await apiCall(page, 'GET', '/me')).status).toBe(401);
	await page.reload();
	await page.getByRole('button', { name: `Sign in to ${email}` }).click();
	await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	await elsewhere.close();
});
