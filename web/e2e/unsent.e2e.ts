import { expect, test, type Page } from '@playwright/test';
import { SERVER } from '../playwright.config';
import { linkFromEmail, OWNER, SIGNED_OUT, newPerson } from './helpers';

test.use({ storageState: OWNER });

/** Signs in by email, with the new link and not the invitation before it. */
async function signInAs(page: Page, email: string) {
	const invited = await linkFromEmail(email);
	await page.getByLabel(/email/i).fill(email);
	await page.getByRole('button', { name: 'Email me a sign-in link' }).click();
	await expect(page.getByRole('heading', { name: 'Check your email' })).toBeVisible();
	await expect.poll(() => linkFromEmail(email)).not.toBe(invited);
	await page.goto(await linkFromEmail(email));
}

test('signing out warns of edits not sent yet', async ({ browser, isMobile }) => {
	test.skip(isMobile, 'the sidebar has this button; a phone signs out from Settings, Security');
	const { page: own, context } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Una');
	await context.setOffline(true);
	await own.getByLabel('New task').fill('Written offline');
	await own.keyboard.press('Enter');
	await expect(
		own.getByRole('link', { name: 'Written offline' }).or(own.getByText('Written offline')).first()
	).toBeVisible();

	await own.getByRole('button', { name: 'Sign out', exact: true }).click();
	await expect(own.getByRole('alert').filter({ hasText: '1 change not sent yet' })).toBeVisible();
	await expect(own.getByRole('button', { name: 'Sign out anyway' })).toBeVisible();
	await expect(own).not.toHaveURL(/login/);

	// Back online it goes, and signing out asks nothing more.
	await context.setOffline(false);
	await own.getByRole('button', { name: 'Sign out', exact: true }).click();
	await expect(own).toHaveURL(/login/);
});

test('edits outlast a session that ends while the app is open', async ({ browser }) => {
	const {
		page: own,
		context,
		email
	} = await newPerson(() => browser.newContext(SIGNED_OUT), 'Otto');
	await context.setOffline(true);
	await own.getByLabel('New task').fill('Kept through signing in');
	await own.keyboard.press('Enter');
	await expect(own.getByText('Kept through signing in').first()).toBeVisible();

	// The session ends elsewhere while this page cannot hear of it.
	const ended = await context.request.post('/api/v1/auth/sign-out', {
		headers: { Origin: SERVER }
	});
	expect(ended.ok()).toBe(true);
	await context.setOffline(false);
	await expect(own).toHaveURL(/login/);
	await expect(
		own.getByRole('alert').filter({ hasText: 'Sign in again to send 1 change' })
	).toBeVisible();

	await signInAs(own, email);
	await expect(own.getByText('Kept through signing in').first()).toBeVisible();
	await expect
		.poll(async () => {
			const workspaces = await context.request.get('/api/v1/workspaces');
			const list = (await workspaces.json()) as { id: string; isPersonal: boolean }[];
			let found = 0;
			for (const w of list) {
				const tasks = await context.request.get(`/api/v1/workspaces/${w.id}/tasks`);
				if ((await tasks.text()).includes('Kept through signing in')) found++;
			}
			return found;
		})
		.toBe(1);
});

test('someone else signing in here does not send them', async ({ browser }) => {
	const open = () => browser.newContext(SIGNED_OUT);
	const { page: own, context } = await newPerson(open, 'Ida');
	const { email: other } = await newPerson(open, 'Ned');
	await context.setOffline(true);
	await own.getByLabel('New task').fill('Not for Ned');
	await own.keyboard.press('Enter');
	await expect(own.getByText('Not for Ned').first()).toBeVisible();
	const ended = await context.request.post('/api/v1/auth/sign-out', {
		headers: { Origin: SERVER }
	});
	expect(ended.ok()).toBe(true);
	await context.setOffline(false);
	await expect(own).toHaveURL(/login/);

	await signInAs(own, other);
	await expect(own.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	await expect(own.getByText('Not for Ned')).toHaveCount(0);
	expect(await own.evaluate(() => localStorage.getItem('mila.unsent'))).toBeNull();
});
