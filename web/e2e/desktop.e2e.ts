import { expect, test, type BrowserContext } from '@playwright/test';
import { createHash, randomBytes } from 'node:crypto';
import { createServer } from 'node:http';
import type { AddressInfo } from 'node:net';
import { newPerson, SIGNED_OUT } from './helpers';

/** A window as the app's: its bridge gives the sign-in's code and secret, once. */
async function appWindow(
	context: BrowserContext,
	handoff: { code: string; verifier: string } | null
) {
	await context.addInitScript((given) => {
		let held = given;
		(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = {
			invoke: async (command: string) => {
				if (command !== 'desktop_take_sign_in') return undefined;
				const taken = held;
				held = null;
				return taken;
			}
		};
	}, handoff);
	return context.newPage();
}

/** Listens as the desktop app does, on a port of this computer, for the code. */
async function fakeApp() {
	let got: URL | null = null;
	const server = createServer((req, res) => {
		got = new URL(req.url ?? '/', 'http://127.0.0.1');
		res.setHeader('Content-Type', 'text/html');
		res.end('<title>Signed in</title>You can close this tab');
	});
	await new Promise<void>((done) => server.listen(0, '127.0.0.1', done));
	const verifier = randomBytes(32).toString('base64url');
	const challenge = createHash('sha256').update(verifier).digest('base64url');
	const state = randomBytes(16).toString('base64url');
	const port = (server.address() as AddressInfo).port;
	return {
		verifier,
		state,
		address: `/desktop?port=${port}&state=${state}&challenge=${challenge}`,
		got: () => got,
		close: () => server.close()
	};
}

test.describe('signing in to the desktop app', () => {
	test.skip(({ isMobile }) => isMobile, 'the desktop app is on a desktop');

	test('the browser hands a code to the app, whose window then signs in', async ({
		browser,
		baseURL
	}) => {
		const person = await newPerson(() => browser.newContext(SIGNED_OUT), 'Wren');
		const app = await fakeApp();
		await person.page.goto(app.address);
		// It asks first, and says who: a link alone signs nothing in.
		await expect(
			person.page.getByRole('heading', { name: 'Sign in to the Mila app?' })
		).toBeVisible();
		await expect(person.page.getByText(person.email)).toBeVisible();
		await person.page.getByRole('button', { name: 'Sign in to the app' }).click();
		await expect(person.page).toHaveURL(/127\.0\.0\.1/);
		const got = app.got()!;
		expect(got.searchParams.get('state')).toBe(app.state);
		// It says which server it is, for the app to compare with the one it asked.
		expect(got.searchParams.get('iss')).toBe(new URL(baseURL!).origin);
		const code = got.searchParams.get('code')!;
		expect(code).toBeTruthy();
		app.close();

		// A new window, as the app's, signs in as that person with the code and secret.
		const context = await browser.newContext(SIGNED_OUT);
		const window = await appWindow(context, { code, verifier: app.verifier });
		await window.goto('/desktop/finish');
		await expect(window.getByRole('navigation', { name: 'Main' })).toBeVisible();
		await expect(window).not.toHaveURL(/finish/);
		// It says whom it signed in as, and where, with a way back: someone else on this
		// computer could have handed the app their own code first.
		await expect(window.getByText(`as Wren (${person.email})`, { exact: false })).toBeVisible();
		await window.getByRole('button', { name: 'Not you? Sign out' }).click();
		await expect(window).toHaveURL(/\/login/);
		await context.close();
	});

	test('signed out, it goes through signing in and comes back', async ({ browser }) => {
		const app = await fakeApp();
		const context = await browser.newContext(SIGNED_OUT);
		const page = await context.newPage();
		await page.goto(app.address);
		await expect(page).toHaveURL(/\/login\?next=/);
		expect(decodeURIComponent(page.url())).toContain(app.address);
		app.close();
		await context.close();
	});

	test('cancel leaves the app signed out', async ({ browser }) => {
		const person = await newPerson(() => browser.newContext(SIGNED_OUT), 'Yara');
		const app = await fakeApp();
		await person.page.goto(app.address);
		await person.page.getByRole('button', { name: 'Cancel' }).click();
		await expect(person.page.getByRole('heading', { name: 'Canceled' })).toBeVisible();
		expect(app.got()).toBeNull();
		app.close();
	});

	test('a link that is not complete, or one pointing elsewhere, does nothing', async ({
		browser
	}) => {
		const person = await newPerson(() => browser.newContext(SIGNED_OUT), 'Zane');
		for (const query of [
			'',
			'?port=80&state=abcdefghijklmnop&challenge=' + 'a'.repeat(43),
			'?port=notaport&state=abcdefghijklmnop&challenge=' + 'a'.repeat(43),
			'?port=9000&state=x&challenge=y'
		]) {
			await person.page.goto('/desktop' + query);
			await expect(
				person.page.getByRole('heading', { name: "This link isn't complete" })
			).toBeVisible();
		}
	});

	test("in the app's window, a link for a new tab goes through the window's address", async ({
		browser
	}) => {
		const person = await newPerson(() => browser.newContext(SIGNED_OUT), 'Cleo');
		// Where the app would refuse it and pass it to the browser.
		await person.context.route('https://outside.invalid/**', (route) =>
			route.fulfill({ contentType: 'text/html', body: 'outside' })
		);
		await person.context.addInitScript(() => {
			(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = {
				invoke: async () => undefined
			};
		});
		await person.page.goto('/');
		await expect(person.page.getByRole('navigation', { name: 'Main' })).toBeVisible();
		await person.page.evaluate(() => {
			const link = document.createElement('a');
			link.href = 'https://outside.invalid/from-a-comment';
			link.target = '_blank';
			link.id = 'outside';
			link.textContent = 'outside';
			document.body.append(link);
		});
		const pages = person.context.pages().length;
		await person.page.locator('#outside').dispatchEvent('click');
		await expect(person.page).toHaveURL('https://outside.invalid/from-a-comment');
		expect(person.context.pages()).toHaveLength(pages);
	});

	test('a used code or a wrong secret says so', async ({ browser }) => {
		const context = await browser.newContext(SIGNED_OUT);
		const page = await appWindow(context, { code: 'nothing', verifier: 'nothing' });
		await page.goto('/desktop/finish');
		await expect(page.getByText('expired or was already used')).toBeVisible();
		await context.close();
	});

	test("a link holding someone's code signs nobody in to their account", async ({ browser }) => {
		// The owner of the code, who wants someone else working in their account.
		const sender = await newPerson(() => browser.newContext(SIGNED_OUT), 'Abel');
		const verifier = randomBytes(32).toString('base64url');
		const challenge = createHash('sha256').update(verifier).digest('base64url');
		const { code } = await sender.page.evaluate(
			async (body) =>
				(
					await fetch('/api/v1/auth/desktop/code', {
						method: 'POST',
						headers: { 'Content-Type': 'application/json' },
						body: JSON.stringify(body)
					})
				).json(),
			{ challenge }
		);
		expect(code).toBeTruthy();

		const victim = await newPerson(() => browser.newContext(SIGNED_OUT), 'Beth');
		const link = `/desktop/finish?code=${encodeURIComponent(code)}&verifier=${verifier}`;
		await victim.page.goto(link);
		await expect(victim.page.getByText('only for the Mila app')).toBeVisible();
		const who = await victim.page.evaluate(
			async () => (await (await fetch('/api/v1/auth/state')).json()).user?.email
		);
		expect(who).toBe(victim.email);

		// Nor in the app's window, when the app isn't signing in.
		const context = await browser.newContext(SIGNED_OUT);
		const window = await appWindow(context, null);
		await window.goto(link);
		await expect(window.getByText('only for the Mila app')).toBeVisible();
		await context.close();
	});
});

test("in the app, a right-click opens only Mila's menus, and text's", async ({
	browser,
	isMobile
}) => {
	test.skip(isMobile, 'the desktop app is on a desktop');
	// Whether the page stops the webview's own menu for a right-click on each.
	const stops = (page: import('@playwright/test').Page) =>
		page.evaluate(() =>
			['h1', 'input[type="email"]', 'body'].map((selector) => {
				const el = document.querySelector(selector)!;
				const e = new MouseEvent('contextmenu', { bubbles: true, cancelable: true, button: 2 });
				return !el.dispatchEvent(e);
			})
		);
	const app = await browser.newContext(SIGNED_OUT);
	const page = await appWindow(app, null);
	await page.goto('/');
	await expect(page.locator('input[type="email"]')).toBeVisible();
	// The heading and the page: nothing. The field keeps cut, copy and paste.
	expect(await stops(page)).toEqual([true, false, true]);
	// Words selected under the pointer keep it too.
	expect(
		await page.evaluate(() => {
			const h1 = document.querySelector('h1')!;
			document.getSelection()!.selectAllChildren(h1);
			return !h1.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
		})
	).toBe(false);
	await app.close();

	// In a browser, the browser's menu is left alone.
	const plain = await browser.newContext(SIGNED_OUT);
	const tab = await plain.newPage();
	await tab.goto('/');
	await expect(tab.locator('input[type="email"]')).toBeVisible();
	expect(await stops(tab)).toEqual([false, false, false]);
	await plain.close();
});
