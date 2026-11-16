import { expect, test } from '@playwright/test';
import { OWNER, SIGNED_OUT, newPerson } from './helpers';

test.use({ storageState: OWNER });

// A Chromium virtual authenticator stands in for a phone or security key.
test('add a passkey, then sign in with it', async ({ browser }) => {
	const { context, page: kim } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Kim');
	const cdp = await context.newCDPSession(kim);
	await cdp.send('WebAuthn.enable');
	await cdp.send('WebAuthn.addVirtualAuthenticator', {
		options: {
			protocol: 'ctap2',
			transport: 'internal',
			hasResidentKey: true,
			hasUserVerification: true,
			isUserVerified: true,
			automaticPresenceSimulation: true
		}
	});

	await kim.goto('/settings/security');
	await kim.getByRole('button', { name: 'Add a passkey' }).click();
	const keys = kim.getByRole('list', { name: 'Passkeys' });
	await expect(keys.getByRole('listitem')).toHaveCount(1);
	await expect(keys).toContainText('Chrome');

	// Sign out on this device, then back in with no email at all.
	const current = kim
		.getByRole('list', { name: 'Signed-in devices' })
		.getByRole('listitem')
		.filter({ hasText: 'This device' });
	await current.getByRole('button', { name: 'Sign out' }).click();
	await expect(kim).toHaveURL(/\/login/);
	await kim.getByRole('button', { name: 'Sign in with a passkey' }).click();
	await expect(kim.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();

	await kim.goto('/settings/security');
	await expect(keys).toContainText('last used');
	await context.close();
});
