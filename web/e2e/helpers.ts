import { expect, request, test, type BrowserContext, type Page } from '@playwright/test';
import { readFileSync, readdirSync, renameSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { deflateSync } from 'node:zlib';

export const DATA = join(import.meta.dirname, '../../tmp/e2e');
export const OWNER = join(import.meta.dirname, '.auth/owner.json');
export const INVITER = join(import.meta.dirname, '.auth/inviter.json');

/** Context options for a browser nobody has signed in on. */
export const SIGNED_OUT = { storageState: { cookies: [], origins: [] } };

/** A fresh address for each test, so parallel tests never share a mailbox. */
export function uniqueEmail(prefix: string): string {
	return `${prefix}-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`;
}

/** The emails sent so far, newest first. */
function emails(): string[] {
	const dir = join(DATA, 'mail');
	try {
		return readdirSync(dir)
			.filter((f) => f.endsWith('.eml'))
			.sort()
			.reverse()
			.map((f) => readFileSync(join(dir, f), 'utf8'));
	} catch {
		return [];
	}
}

/**
 * Waits for the newest email to an address that carries a sign-in or
 * invite link and returns the link. Notification emails, which have no
 * token, are passed over.
 */
export async function linkFromEmail(to: string): Promise<string> {
	let link = '';
	await expect
		.poll(
			() => {
				for (const text of emails()) {
					const found = text.match(/https?:\/\/\S+token=[\w-]+/)?.[0];
					if (text.includes(`To: ${to}\r\n`) && found) return (link = found);
				}
				return '';
			},
			{ message: `email to ${to}`, timeout: 10_000 }
		)
		.not.toBe('');
	return link;
}

/**
 * Waits for an email to an address whose subject matches, and whose text
 * holds some words if given, and returns it.
 */
export async function emailTo(to: string, subject: RegExp, holding = ''): Promise<string> {
	let found = '';
	await expect
		.poll(
			() => {
				found =
					emails().find(
						(text) =>
							text.includes(`To: ${to}\r\n`) &&
							subject.test(text.match(/^Subject: (.*)\r$/m)?.[1] ?? '') &&
							text.includes(holding)
					) ?? '';
				return found;
			},
			{ message: `email to ${to} about ${subject}`, timeout: 10_000 }
		)
		.not.toBe('');
	return found;
}

/** Calls the API from inside the page, so cookies and Origin are real. */
export async function apiCall(page: Page, method: string, path: string, body?: unknown) {
	return page.evaluate(
		async ([method, path, body]) => {
			const res = await fetch('/api/v1' + path, {
				method,
				headers: { 'Content-Type': 'application/json' },
				body: body === undefined ? undefined : JSON.stringify(body)
			});
			const text = await res.text();
			return { status: res.status, body: text ? JSON.parse(text) : null };
		},
		[method, path, body] as const
	);
}

/** Waits until the server has a preference as given. */
/**
 * The workspace Settings starts from, the one open in the sidebar: an
 * address from before Settings was grouped leads to its page there.
 */
export async function settingsWorkspace(page: Page): Promise<string> {
	await page.goto('/settings/workspace');
	await page.waitForURL(/\/settings\/w\/[^/]+\/general/);
	return page.url().match(/\/settings\/w\/([^/]+)\//)?.[1] ?? '';
}

/** A workspace's pages in Settings, by the last part of their address. */
export const WORKSPACE_SETTINGS = [
	'general',
	'people',
	'labels',
	'templates',
	'automations',
	'themes',
	'webhooks',
	'calendar',
	'email',
	'data',
	'data/csv',
	'data/apps'
];

export async function saved(page: Page, key: string, value: unknown) {
	await expect
		.poll(async () => (await apiCall(page, 'GET', '/me')).body.preferences[key])
		.toEqual(value);
}

/**
 * Creates a shared workspace as the owner and invites a new person into it,
 * then accepts in a new browser context. Returns that context, signed in.
 */
export async function newMember(
	owner: Page,
	newContext: () => Promise<BrowserContext>,
	name: string,
	role = 'member'
) {
	if (owner.url() === 'about:blank') await owner.goto('/');
	const ws = await apiCall(owner, 'POST', '/workspaces', { name: `${name}'s team` });
	expect(ws.status).toBe(201);
	const email = uniqueEmail(name.toLowerCase());
	const inv = await apiCall(owner, 'POST', `/workspaces/${ws.body.id}/invites`, { email, role });
	expect(inv.status).toBe(201);
	const joined = await accept(newContext, email, name, role);
	return { ...joined, email, workspace: ws.body as { id: string; name: string } };
}

/**
 * Someone new, signed in, for a test that never acts as the owner in their
 * workspace. Use newMember when it does. The workspace belongs to an account
 * that is only ever used through the API, so it costs no page any bootstraps:
 * every workspace the shared owner joins is more work for each of its pages.
 */
export async function newPerson(
	newContext: () => Promise<BrowserContext>,
	name: string,
	role = 'member'
) {
	const baseURL = test.info().project.use.baseURL!;
	const api = await request.newContext({
		baseURL,
		storageState: INVITER,
		extraHTTPHeaders: { Origin: baseURL }
	});
	const ws = await api.post('/api/v1/workspaces', { data: { name: `${name}'s team` } });
	expect(ws.status()).toBe(201);
	const workspace = (await ws.json()) as { id: string; name: string };
	const email = uniqueEmail(name.toLowerCase());
	const inv = await api.post(`/api/v1/workspaces/${workspace.id}/invites`, {
		data: { email, role }
	});
	expect(inv.status()).toBe(201);
	await api.dispose();
	const joined = await accept(newContext, email, name, role);
	return { ...joined, email, workspace };
}

/** Opens an invitation's link in a new browser context and joins. */
async function accept(
	newContext: () => Promise<BrowserContext>,
	email: string,
	name: string,
	role: string
) {
	const context = await newContext();
	const page = await context.newPage();
	await page.goto(await linkFromEmail(email));
	await page.getByLabel('Your name').fill(name);
	await page.getByRole('button', { name: /^Join/ }).click();
	// A guest has no inbox, and arrives at Projects.
	const arrives = role === 'guest' ? 'Projects' : 'Inbox';
	await expect(page.getByRole('heading', { level: 1, name: arrives, exact: true })).toBeVisible();
	return { context, page };
}

/**
 * A w×h PNG of soft color bands, made here so tests can choose a picture
 * without a file of their own.
 */
export function pngBuffer(w = 64, h = 64): Buffer {
	const crcTable = Array.from({ length: 256 }, (_, n) => {
		let c = n;
		for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
		return c >>> 0;
	});
	const crc = (buf: Buffer) => {
		let c = 0xffffffff;
		for (const b of buf) c = crcTable[(c ^ b) & 0xff]! ^ (c >>> 8);
		return (c ^ 0xffffffff) >>> 0;
	};
	const chunk = (type: string, data: Buffer) => {
		const len = Buffer.alloc(4);
		len.writeUInt32BE(data.length);
		const body = Buffer.concat([Buffer.from(type, 'ascii'), data]);
		const sum = Buffer.alloc(4);
		sum.writeUInt32BE(crc(body));
		return Buffer.concat([len, body, sum]);
	};
	const ihdr = Buffer.alloc(13);
	ihdr.writeUInt32BE(w, 0);
	ihdr.writeUInt32BE(h, 4);
	ihdr[8] = 8; // bits per channel
	ihdr[9] = 2; // RGB
	const rows = Buffer.alloc((w * 3 + 1) * h);
	for (let y = 0; y < h; y++)
		for (let x = 0; x < w; x++) {
			const at = y * (w * 3 + 1) + 1 + x * 3;
			rows[at] = (80 + (x * 160) / w) | 0;
			rows[at + 1] = (120 + (y * 100) / h) | 0;
			rows[at + 2] = 200;
		}
	return Buffer.concat([
		Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
		chunk('IHDR', ihdr),
		chunk('IDAT', deflateSync(rows)),
		chunk('IEND', Buffer.alloc(0))
	]);
}

/** Delivers an email to the server's mailbox, a folder in these tests; it reads it within a second or so. */
export function deliverEmail(raw: string) {
	const dir = join(DATA, 'inbound');
	const name = `${Date.now()}-${Math.floor(Math.random() * 1e9)}`;
	// Written whole under another name first, so the server never reads half of it.
	writeFileSync(join(dir, `${name}.tmp`), raw.replace(/\r?\n/g, '\r\n'));
	renameSync(join(dir, `${name}.tmp`), join(dir, `${name}.eml`));
}
