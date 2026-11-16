import { expect, test as setup } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { axe } from './axe';
import { DATA, INVITER, newMember, OWNER, SIGNED_OUT } from './helpers';

// The first account is made through the real setup page, from the link the
// server printed to its log.
setup('create the instance owner', async ({ page, browser }) => {
	let link = '';
	await expect
		.poll(() => {
			const log = readFileSync(join(DATA, 'server.log'), 'utf8');
			link = log.match(/http:\/\/localhost:\d+\/setup\?token=[\w-]+/)?.[0] ?? '';
			return link;
		})
		.not.toBe('');

	await page.goto(link);
	await page.getByLabel('Name').fill('Shawn');
	await page.getByLabel('Email').fill('owner@example.com');
	await page.getByRole('button', { name: 'Create account' }).click();
	await expect(page.getByRole('heading', { name: 'Add a passkey' })).toBeVisible();
	// The one time this step shows: check it here.
	await expect(page.getByRole('heading', { name: 'Add a passkey' })).toBeFocused();
	await axe(page);
	await page.getByRole('button', { name: 'Skip for now' }).click();
	await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	await page.context().storageState({ path: OWNER });

	// The account behind newPerson's workspaces.
	const { context } = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Inviter');
	await context.storageState({ path: INVITER });
	await context.close();
});
