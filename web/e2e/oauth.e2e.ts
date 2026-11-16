import { expect, test, type APIRequestContext } from '@playwright/test';
import { createHash, randomBytes } from 'node:crypto';
import { createServer } from 'node:http';
import type { AddressInfo } from 'node:net';
import { axe } from './axe';
import { newPerson, SIGNED_OUT } from './helpers';

/** An app on this computer, as an agent harness would be: it registers, and listens. */
async function fakeApp(request: APIRequestContext) {
	let got: URL | null = null;
	const server = createServer((req, res) => {
		got = new URL(req.url ?? '/', 'http://127.0.0.1');
		res.setHeader('Content-Type', 'text/html');
		res.end('<title>Back in the app</title>Done');
	});
	await new Promise<void>((done) => server.listen(0, '127.0.0.1', done));
	const port = (server.address() as AddressInfo).port;
	const redirect = `http://127.0.0.1:${port}/cb`;
	const registered = await request.post('/oauth/register', {
		data: { client_name: 'Helper', redirect_uris: [redirect] }
	});
	expect(registered.status()).toBe(201);
	const { client_id } = (await registered.json()) as { client_id: string };
	const verifier = randomBytes(32).toString('base64url');
	const challenge = createHash('sha256').update(verifier).digest('base64url');
	return {
		client_id,
		redirect,
		verifier,
		address: (baseURL: string, state: string) =>
			`/oauth/authorize?${new URLSearchParams({
				response_type: 'code',
				client_id,
				redirect_uri: redirect,
				state,
				code_challenge: challenge,
				code_challenge_method: 'S256',
				resource: `${baseURL}/api/v1/mcp`
			})}`,
		got: () => got,
		close: () => server.close()
	};
}

test.describe('connecting an app by OAuth', () => {
	test.skip(({ isMobile }) => isMobile, 'one run is enough: the page is the sign-in card');

	test('a person says yes, and the app trades the code for its tools', async ({
		browser,
		baseURL,
		request
	}) => {
		const person = await newPerson(() => browser.newContext(SIGNED_OUT), 'Hollis');
		const app = await fakeApp(request);
		await person.page.goto(app.address(baseURL!, 'state-1'));
		// It asks first, and says who it is for and where the answer goes.
		await expect(
			person.page.getByRole('heading', { name: 'Connect Helper to Mila?' })
		).toBeVisible();
		await expect(person.page.getByText(person.email)).toBeVisible();
		await expect(person.page.getByText('an app on this computer')).toBeVisible();
		await expect(person.page.getByLabel('Workspace')).toHaveValue(person.workspace.id);
		await axe(person.page);
		await person.page.getByLabel('Read and write').check();
		await person.page.getByRole('button', { name: 'Connect', exact: true }).click();
		await expect(person.page).toHaveURL(/^http:\/\/127\.0\.0\.1:\d+\/cb\?/);
		const got = app.got()!;
		expect(got.searchParams.get('state')).toBe('state-1');
		expect(got.searchParams.get('iss')).toBe(new URL(baseURL!).origin);

		const token = await request.post('/oauth/token', {
			form: {
				grant_type: 'authorization_code',
				client_id: app.client_id,
				code: got.searchParams.get('code')!,
				redirect_uri: app.redirect,
				code_verifier: app.verifier
			}
		});
		expect(token.status()).toBe(200);
		const { access_token, scope } = (await token.json()) as { access_token: string; scope: string };
		expect(scope).toBe('read write');
		const tools = await request.post('/api/v1/mcp', {
			headers: {
				Authorization: `Bearer ${access_token}`,
				Accept: 'application/json, text/event-stream'
			},
			data: { jsonrpc: '2.0', id: 1, method: 'tools/list' }
		});
		expect(tools.status()).toBe(200);
		expect(await tools.text()).toContain('add_task');

		// The app's token is listed with the person's, said to be an app's.
		await person.page.goto('/settings/tokens');
		await expect(person.page.getByText('An app, for its tools only')).toBeVisible();
		app.close();
	});

	test('no tells the app so, and a link that is not whole says so', async ({
		browser,
		baseURL,
		request
	}) => {
		const person = await newPerson(() => browser.newContext(SIGNED_OUT), 'Imre');
		const app = await fakeApp(request);
		await person.page.goto(app.address(baseURL!, 'state-2'));
		await person.page.getByRole('button', { name: "Don't connect" }).click();
		await expect(person.page).toHaveURL(/^http:\/\/127\.0\.0\.1:\d+\/cb\?/);
		expect(app.got()!.searchParams.get('error')).toBe('access_denied');
		expect(app.got()!.searchParams.get('code')).toBeNull();
		app.close();

		await person.page.goto('/oauth/authorize?client_id=x');
		await expect(
			person.page.getByRole('heading', { name: "This link isn't complete" })
		).toBeVisible();
		// An app that isn't registered is refused before anything is asked.
		await person.page.goto(
			'/oauth/authorize?response_type=code&client_id=app_nobody&redirect_uri=https%3A%2F%2Fevil.example%2F&code_challenge=x'
		);
		await expect(
			person.page.getByRole('heading', { name: "This app can't connect" })
		).toBeVisible();
	});
});
