import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, deliverEmail, newPerson, SIGNED_OUT, OWNER } from './helpers';

/** Someone signed in just now, on the Email to task page with one address. */
async function withAddress(browser: import('@playwright/test').Browser, name: string) {
	const person = await newPerson(() => browser.newContext(SIGNED_OUT), name);
	const page = person.page;
	await page.goto(`/settings/w/${person.workspace.id}/email`);
	await page.getByRole('button', { name: 'New address' }).click();
	const address = page.locator('article code').first();
	await expect(address).toContainText('@example.test');
	const me = (await apiCall(page, 'GET', '/me')).body.email as string;
	return { page, ws: person.workspace.id, address: (await address.textContent())!, me };
}

let seq = 0;
function mail(to: string, from: string, subject: string, more = '', body = 'The message.') {
	return [
		`From: ${from}`,
		`To: ${to}`,
		`Subject: ${subject}`,
		`Message-ID: <${Date.now()}-${++seq}@example.org>`,
		'Date: Tue, 3 Nov 2026 09:12:00 -0800',
		...(more ? [more] : []),
		'',
		body,
		''
	].join('\n');
}

const recent = (page: Page) => page.getByRole('table');

/** Waits for what the list says of the mail, which needs the page loaded afresh. */
async function expectListed(page: Page, said: string) {
	await expect
		.poll(
			async () => {
				await page.reload();
				await page.getByRole('heading', { name: 'Recent mail' }).waitFor();
				return recent(page).getByText(said).count();
			},
			{ timeout: 15_000 }
		)
		.toBe(1);
}

test.describe('email to task', () => {
	test.skip(({ isMobile }) => isMobile, 'the same on a phone, whose controls are measured below');

	test('mail to an address becomes a task', async ({ browser }) => {
		const { page, ws, address, me } = await withAddress(browser, 'Ines');
		deliverEmail(mail(address, me, 'Fwd: Invoice 4521', '', 'Please pay by the 14th.'));
		await expectListed(page, 'Made a task');
		await expect(recent(page)).toContainText('Fwd: Invoice 4521');

		// It opens from the list, with the subject as its title and the message as its notes.
		await recent(page).getByRole('button', { name: 'Open' }).click();
		await expect(page.getByLabel('Task title')).toHaveValue('Invoice 4521');
		await expect(page.getByLabel('Notes', { exact: true })).toContainText(
			'Please pay by the 14th.'
		);
		await expect(page.getByLabel('Notes', { exact: true })).toContainText(`By email from ${me}`);

		// The bell says so.
		const list = await apiCall(page, 'GET', '/me/notifications');
		const told = (list.body.items as { kind: string; title: string }[]).find(
			(n) => n.kind === 'email_task'
		);
		expect(told?.title).toContain('Invoice 4521');
		// And it is an ordinary task of this workspace, assigned to its maker.
		const tasks = await apiCall(page, 'GET', `/workspaces/${ws}/tasks`);
		expect(tasks.body.items.length).toBe(1);
	});

	test('a sender not listed is refused, until they are', async ({ browser }) => {
		const { page, address } = await withAddress(browser, 'Odell');
		deliverEmail(mail(address, 'dana@example.org', 'From a colleague'));
		await expectListed(page, 'sender not listed');

		await page.getByLabel('Another address to accept mail from').fill('dana@example.org');
		await page.getByRole('button', { name: 'Add', exact: true }).click();
		await expect(page.getByRole('list', { name: 'Accept mail from' })).toContainText(
			'dana@example.org'
		);

		deliverEmail(mail(address, 'dana@example.org', 'From a colleague, again'));
		await expectListed(page, 'Made a task');
	});

	test('where it goes can be changed, and a new address ends the old', async ({ browser }) => {
		const { page, ws, address, me } = await withAddress(browser, 'Priya');
		const project = (
			await apiCall(page, 'POST', `/workspaces/${ws}/projects`, { name: 'Wholesale' })
		).body.id as string;
		await page.reload();
		await page.getByLabel('Tasks go to').selectOption({ label: 'Wholesale' });
		await expect(page.getByRole('article', { name: 'Address going to Wholesale' })).toBeVisible();

		await page.getByRole('button', { name: 'Make a new one' }).click();
		await page.getByRole('button', { name: 'Confirm new address' }).click();
		await expect(page.locator('article code').first()).not.toHaveText(address);
		const fresh = (await page.locator('article code').first().textContent())!;

		// Mail to the old one leaves no trace; the new one works, in the project.
		deliverEmail(mail(address, me, 'To the old address'));
		deliverEmail(mail(fresh, me, 'To the new address'));
		await expectListed(page, 'Made a task');
		await expect(recent(page)).not.toContainText('To the old address');
		const tasks = await apiCall(page, 'GET', `/workspaces/${ws}/tasks`);
		expect(tasks.body.items[0].project_id).toBe(project);
	});

	test('the page passes the accessibility checks', async ({ browser }) => {
		const { page, address, me } = await withAddress(browser, 'Ravi');
		deliverEmail(mail(address, me, 'A task for the list'));
		await expectListed(page, 'Made a task');
		await axe(page);
	});

	test('guests have no such page', async ({ browser }) => {
		const guest = await newPerson(() => browser.newContext(SIGNED_OUT), 'Tess', 'guest');
		await guest.page.goto(`/settings/tokens?ws=${guest.workspace.id}`);
		await expect(guest.page.getByRole('link', { name: 'API tokens' })).toBeVisible();
		await expect(guest.page.getByRole('link', { name: 'Email to task' })).toHaveCount(0);
		const res = await apiCall(guest.page, 'GET', `/workspaces/${guest.workspace.id}/email-to-task`);
		expect(res.status).toBe(403);
	});
});

test('the owner sees it in the settings', async ({ browser, isMobile }) => {
	test.skip(isMobile, 'as above');
	const context = await browser.newContext({ storageState: OWNER });
	const page = await context.newPage();
	await page.goto('/settings/tokens');
	await page.getByRole('link', { name: 'Email to task' }).click();
	await expect(page.getByRole('heading', { name: 'Email to task' })).toBeVisible();
	await context.close();
});
