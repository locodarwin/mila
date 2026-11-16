import { expect, test, type BrowserContext, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, newMember, OWNER, SIGNED_OUT, newPerson } from './helpers';
import { SERVER } from '../playwright.config';

// Chromium's headless shell has no notifications (Notification.permission
// stays "denied" whatever is granted); the full browser in headless mode
// has them.
test.use({ storageState: OWNER, channel: 'chromium' });

// The test browser has no push service, so subscriptions are made up in
// the page: the server stores them like real ones (the keys are RFC 8291's
// example browser's), and sending to them fails, as the endpoint's name
// never resolves. The subscription lives in localStorage, as a browser
// keeps it across reloads.
function fakePushService() {
	const KEY = 'test.push-subscription';
	const make = (endpoint: string, key: number[]) => ({
		endpoint,
		expirationTime: null,
		options: { userVisibleOnly: true, applicationServerKey: new Uint8Array(key).buffer },
		toJSON: () => ({
			endpoint,
			expirationTime: null,
			keys: {
				p256dh:
					'BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4',
				auth: 'BTBZMqHH6r4Tts7J_aSIgg'
			}
		}),
		unsubscribe: async () => {
			localStorage.removeItem(KEY);
			return true;
		}
	});
	const stored = () => {
		const s = localStorage.getItem(KEY);
		if (!s) return null;
		const { endpoint, key } = JSON.parse(s) as { endpoint: string; key: number[] };
		return make(endpoint, key);
	};
	PushManager.prototype.getSubscription = async function () {
		return stored() as unknown as PushSubscription;
	};
	PushManager.prototype.subscribe = async function (options?: PushSubscriptionOptionsInit) {
		const key = Array.from(new Uint8Array(options!.applicationServerKey as ArrayBuffer));
		const endpoint = 'https://push.invalid/send/' + crypto.randomUUID();
		localStorage.setItem(KEY, JSON.stringify({ endpoint, key }));
		return stored() as unknown as PushSubscription;
	};
}

async function pushContext(newContext: () => Promise<BrowserContext>) {
	const context = await newContext();
	await context.grantPermissions(['notifications'], { origin: SERVER });
	await context.addInitScript(fakePushService);
	return context;
}

const pushBox = (page: Page) => page.getByRole('checkbox', { name: 'Push on this device' });

test('push turns on and off for this device, and a test push says what happened', async ({
	browser
}) => {
	const { page: dana } = await newPerson(
		() => pushContext(() => browser.newContext(SIGNED_OUT)),
		'Dana'
	);
	await dana.goto('/settings/notifications');
	const push = dana.getByRole('region', { name: 'Push' });

	// The push column of the matrix is on now.
	await expect(dana.getByRole('checkbox', { name: 'Mentions: push' })).toBeEnabled();
	await expect(dana.getByRole('checkbox', { name: 'Mentions: push' })).toBeChecked();

	await expect(pushBox(dana)).toBeEnabled();
	await expect(pushBox(dana)).not.toBeChecked();
	await expect(push).toContainText('Your browser asks for permission when you turn it on.');
	await expect(push).toContainText('Push is not on for any of your devices.');
	await expect(push.getByRole('button', { name: 'Send a test push' })).toBeDisabled();
	await axe(dana);

	// From the keyboard.
	await pushBox(dana).focus();
	await dana.keyboard.press('Space');
	await expect(dana.getByText('Push is on for this device')).toBeVisible();
	await expect(pushBox(dana)).toBeChecked();
	await expect(push).toContainText('Pushes reach this device even when Mila is closed.');
	await expect(push).toContainText('Push is on for 1 of your devices.');
	// The box was disabled while push turned on; focus comes back to it.
	await expect(pushBox(dana)).toBeFocused();
	const info = await apiCall(dana, 'GET', '/me/push');
	expect(info.body.devices).toBe(1);
	await axe(dana);

	// Still on after a reload, and the app start re-registers it.
	await dana.reload();
	await expect(pushBox(dana)).toBeChecked();
	await expect(push).toContainText('Push is on for 1 of your devices.');

	// The made-up push service never answers, which the test push says.
	const testButton = push.getByRole('button', { name: 'Send a test push' });
	await testButton.focus();
	await dana.keyboard.press('Enter');
	await expect(
		dana.getByText('The push service for 1 device could not be reached. Try again later.')
	).toBeVisible();
	await expect(testButton).toBeEnabled();
	await expect(testButton).toBeFocused();

	await dana.getByText('Push on this device', { exact: true }).click();
	await expect(dana.getByText('Push is off for this device')).toBeVisible();
	await expect(pushBox(dana)).not.toBeChecked();
	await expect(push).toContainText('Push is not on for any of your devices.');
	expect((await apiCall(dana, 'GET', '/me/push')).body.devices).toBe(0);
});

test('signing out stops push on the device', async ({ browser }) => {
	const { page: dana } = await newPerson(
		() => pushContext(() => browser.newContext(SIGNED_OUT)),
		'Dana'
	);
	await dana.goto('/settings/notifications');
	await dana.getByText('Push on this device', { exact: true }).click();
	await expect(dana.getByText('Push is on for this device')).toBeVisible();
	expect(await dana.evaluate(() => localStorage.getItem('test.push-subscription'))).not.toBeNull();

	await dana.goto('/settings/security');
	await dana
		.getByRole('list', { name: 'Signed-in devices' })
		.getByRole('listitem')
		.filter({ hasText: 'This device' })
		.getByRole('button', { name: 'Sign out' })
		.click();
	await expect(dana).toHaveURL(/\/login/);
	expect(await dana.evaluate(() => localStorage.getItem('test.push-subscription'))).toBeNull();
});

test('a browser found signed out at start drops push, so the next account does not inherit it', async ({
	browser
}) => {
	const { page: dana } = await newPerson(
		() => pushContext(() => browser.newContext(SIGNED_OUT)),
		'Dana'
	);
	await dana.goto('/settings/notifications');
	await dana.getByText('Push on this device', { exact: true }).click();
	await expect(dana.getByText('Push is on for this device')).toBeVisible();

	// The session ends while the app is closed, as when it is revoked from
	// another device.
	await dana.context().clearCookies();
	await dana.goto('/');
	await expect(dana).toHaveURL(/\/login/);
	await expect
		.poll(() => dana.evaluate(() => localStorage.getItem('test.push-subscription')))
		.toBeNull();
	expect(await dana.evaluate(() => localStorage.getItem('mila.push-user'))).toBeNull();
});

for (const [state, setup, text] of [
	[
		'blocked notifications',
		() => Object.defineProperty(Notification, 'permission', { get: () => 'denied' }),
		'Notifications are blocked for Mila in this browser. Allow them in the site settings, then reload this page.'
	],
	[
		'a plain http connection',
		() => Object.defineProperty(window, 'isSecureContext', { get: () => false }),
		'Push needs a secure connection. Open Mila at its https:// address, or at localhost on the computer that runs it, to turn it on here.'
	],
	[
		'a browser without push',
		() => Reflect.deleteProperty(window, 'PushManager'),
		'This browser cannot receive push notifications. On an iPhone or iPad, add Mila to the Home Screen (Share, then Add to Home Screen) and turn push on from there.'
	]
] as const) {
	// Someone new: late in a full run the shared owner is in about a hundred
	// workspaces, and a fresh browser syncing them all can outlast the wait below.
	test(`push explains ${state}`, async ({ browser }) => {
		const { page: own } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Nia');
		await own.addInitScript(setup);
		await own.goto('/settings/notifications');
		await expect(pushBox(own)).toBeDisabled();
		await expect(own.getByRole('region', { name: 'Push' })).toContainText(text);
		await expect(pushBox(own)).toHaveAccessibleDescription(text);
		await axe(own);
	});
}

test('a push shows a notification, and its link opens the task in its workspace', async ({
	page,
	browser
}) => {
	const {
		context,
		page: dana,
		workspace
	} = await newMember(page, () => pushContext(() => browser.newContext(SIGNED_OUT)), 'Dana');

	// The owner makes a task in the shared workspace.
	await page.goto(`/?ws=${workspace.id}`);
	const title = `Fix the gate ${Date.now()}`;
	await page.getByLabel('New task').fill(title);
	await page.getByLabel('New task').press('Enter');
	await page
		.getByRole('list', { name: 'Tasks' })
		.getByRole('button', { name: title, exact: true })
		.click();
	await expect(page).toHaveURL(/task=/);
	const taskId = new URL(page.url()).searchParams.get('task')!;

	// Dana is in her own workspace.
	await dana.getByRole('button', { name: /^Switch workspace/ }).click();
	await dana.getByRole('button', { name: 'Personal' }).click();
	await expect(dana.getByRole('button', { name: /^Switch workspace/ })).toContainText('Personal');
	await dana.evaluate(() => navigator.serviceWorker.ready);

	// A push as the server sends it reaches the service worker.
	const cdp = await context.newCDPSession(dana);
	const registration = new Promise<string>((resolve) => {
		cdp.on('ServiceWorker.workerRegistrationUpdated', ({ registrations }) => {
			const r = registrations.find((r) => r.scopeURL === SERVER + '/' && !r.isDeleted);
			if (r) resolve(r.registrationId);
		});
	});
	await cdp.send('ServiceWorker.enable');
	const url = `/?task=${taskId}&ws=${workspace.id}`;
	const push = {
		origin: SERVER,
		registrationId: await registration,
		data: JSON.stringify({
			title: `Reminder: ${title}`,
			body: 'Due today at 9:00',
			url,
			tag: taskId
		})
	};
	const notifications = () =>
		dana.evaluate(async () =>
			(await (await navigator.serviceWorker.ready).getNotifications()).map((n) => ({
				title: n.title,
				body: n.body,
				tag: n.tag,
				icon: n.icon,
				data: n.data as unknown
			}))
		);
	// Now and then (about one run in 30 with eight at once) the headless
	// browser loses a notification: the worker's push handler runs and
	// showNotification resolves, yet getNotifications right after it finds
	// nothing. So the push is sent again until one stays. A push about the
	// same task carries the same tag and replaces the last, so a resend
	// can never leave two.
	await expect(async () => {
		await cdp.send('ServiceWorker.deliverPushMessage', push);
		await expect.poll(notifications, { timeout: 1000 }).not.toEqual([]);
	}).toPass({ timeout: 10_000 });
	expect(await notifications()).toEqual([
		{
			title: `Reminder: ${title}`,
			body: 'Due today at 9:00',
			tag: taskId,
			icon: SERVER + '/icon-192.png',
			data: { url }
		}
	]);

	// Clicking it makes the service worker send the window to the link
	// (openApp in pushNotice.ts): the window switches workspace and opens
	// the task.
	const [worker] = context.serviceWorkers();
	await worker!.evaluate(async (url) => {
		const sw = self as unknown as {
			clients: { matchAll(o: object): Promise<{ postMessage(m: unknown): void }[]> };
		};
		for (const c of await sw.clients.matchAll({ type: 'window', includeUncontrolled: true }))
			c.postMessage({ type: 'open', url });
	}, url);
	await expect(dana.getByRole('button', { name: /^Switch workspace/ })).toContainText(
		workspace.name
	);
	await expect(
		dana.getByRole('complementary', { name: 'Task details' }).getByLabel('Task title')
	).toHaveValue(title);
	await expect(dana).not.toHaveURL(/ws=/);
});
