import { expect, test as setup, type Browser, type Page } from '@playwright/test';
import { readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { inflateRawSync } from 'node:zlib';
import { api, DATA, SAMPLE, SEED, SHOTS_SERVER, STATE } from './shots';

// The workspace the documentation's pictures show: scripts/sample-data.py's
// small business, a coffee roaster, imported as its owner, Imani Reyes. An
// import keeps one person (syncer/restore.go), so the others are invited here
// and given back what the import credited to Imani: their comments and their
// time, written again by each of them, and work of their own to do.

interface Row {
	entity: string;
	id: string;
	fields: Record<string, unknown>;
}
interface Person {
	id: string;
	name: string;
	email: string;
	role: string;
}

/** The export's workspace.json, read from its zip with only what Node has. */
function readSample(): { rows: Row[]; people: Person[] } {
	const zip = readFileSync(SAMPLE);
	// The central directory's end, then the entry named workspace.json.
	const end = zip.lastIndexOf(Buffer.from([0x50, 0x4b, 0x05, 0x06]));
	let at = zip.readUInt32LE(end + 16);
	for (let n = zip.readUInt16LE(end + 10); n > 0; n--) {
		const nameLength = zip.readUInt16LE(at + 28);
		const skip = nameLength + zip.readUInt16LE(at + 30) + zip.readUInt16LE(at + 32);
		const name = zip.toString('utf8', at + 46, at + 46 + nameLength);
		if (name === 'workspace.json') {
			const local = zip.readUInt32LE(at + 42);
			const method = zip.readUInt16LE(at + 10);
			const size = zip.readUInt32LE(at + 20);
			const start = local + 30 + zip.readUInt16LE(local + 26) + zip.readUInt16LE(local + 28);
			const data = zip.subarray(start, start + size);
			return JSON.parse((method === 8 ? inflateRawSync(data) : data).toString('utf8'));
		}
		at += 46 + skip;
	}
	throw new Error('the sample has no workspace.json');
}

/** The newest sign-in or invite link emailed to an address. */
async function linkTo(email: string) {
	let link = '';
	await expect
		.poll(
			() => {
				const dir = join(DATA, 'mail');
				for (const f of readdirSync(dir).sort().reverse()) {
					const text = readFileSync(join(dir, f), 'utf8');
					const found = text.match(/https?:\/\/\S+token=[\w-]+/)?.[0];
					if (text.includes(`To: ${email}\r\n`) && found) return (link = found);
				}
				return '';
			},
			{ message: `an invitation to ${email}` }
		)
		.not.toBe('');
	return link;
}

/** A person invited into the workspace, who joins in a browser of their own. */
async function joins(browser: Browser, owner: Page, ws: string, p: Person) {
	await api(owner, 'POST', `/workspaces/${ws}/invites`, { email: p.email, role: p.role });
	const context = await browser.newContext({ storageState: { cookies: [], origins: [] } });
	const page = await context.newPage();
	await page.goto(await linkTo(p.email));
	await page.getByLabel('Your name').fill(p.name);
	await page.getByRole('button', { name: /^Join/ }).click();
	const arrives = p.role === 'guest' ? 'Projects' : 'Inbox';
	await expect(page.getByRole('heading', { level: 1, name: arrives, exact: true })).toBeVisible();
	return page;
}

/** A small, steady stream of numbers, so the same run makes the same work. */
function numbers(seed: number) {
	let x = seed;
	return () => {
		x = (x * 1103515245 + 12345) % 2147483648;
		return x / 2147483648;
	};
}

const day = (d: Date) => d.toISOString().slice(0, 10);

setup('make the workspace the documentation shows', async ({ page, browser }) => {
	setup.setTimeout(240_000);
	const sample = readSample();
	const exporter = sample.people[0];

	// The first account, from the one-time link the server logs.
	let link = '';
	await expect
		.poll(() => {
			const log = readFileSync(join(DATA, 'server.log'), 'utf8');
			link = log.match(/http:\/\/localhost:4177\/setup\?token=[\w-]+/)?.[0] ?? '';
			return link;
		})
		.not.toBe('');
	await page.goto(link);
	await page.getByLabel('Name').fill(exporter.name);
	await page.getByLabel('Email').fill(exporter.email);
	await page.getByRole('button', { name: 'Create account' }).click();
	await page.getByRole('button', { name: 'Skip for now' }).click();
	await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();

	// The sample, under its own IDs, so the rows below can be found by theirs.
	const upload = await page.request.post('/api/v1/imports', {
		headers: { Origin: SHOTS_SERVER },
		multipart: {
			ids: 'keep',
			file: { name: 'sample.zip', mimeType: 'application/zip', buffer: readFileSync(SAMPLE) }
		}
	});
	expect(upload.status(), await upload.text()).toBe(202);
	let ws = '';
	await expect
		.poll(
			async () => {
				const list = (await api(page, 'GET', '/me/imports')) as {
					status: string;
					workspaceId?: string;
					error?: string;
				}[];
				expect(list[0].status, list[0].error).not.toBe('failed');
				ws = list[0].status === 'ready' ? (list[0].workspaceId ?? '') : '';
				return ws;
			},
			{ timeout: 60_000 }
		)
		.not.toBe('');
	const w = `/workspaces/${ws}`;

	// Everyone else joins.
	const pages = new Map<string, Page>([[exporter.id, page]]);
	for (const p of sample.people.slice(1)) pages.set(p.id, await joins(browser, page, ws, p));
	const members = (await api(page, 'GET', `${w}/members`)) as { userId: string; name: string }[];
	const userOf = new Map(
		sample.people.map((p) => [p.id, members.find((m) => m.name === p.name)!.userId])
	);

	const rows = (entity: string) => sample.rows.filter((r) => r.entity === entity);
	const tasks = new Map(rows('task').map((r) => [r.id, r]));
	const projectOf = (taskId: string) => tasks.get(taskId)?.fields.project_id as string;

	// A guest gets the projects their comments and time are in.
	const others = (r: Row, field: string) =>
		r.fields[field] !== exporter.id && pages.has(r.fields[field] as string);
	const theirs = [
		...rows('comment').filter((r) => others(r, 'author_id')),
		...rows('time_entry').filter((r) => others(r, 'user_id')),
		...rows('project_time_entry').filter((r) => others(r, 'user_id'))
	];
	for (const p of sample.people.filter((p) => p.role === 'guest')) {
		const projects = new Set(
			theirs
				.filter((r) => (r.fields.author_id ?? r.fields.user_id) === p.id)
				.map((r) => (r.fields.project_id as string) ?? projectOf(r.fields.task_id as string))
		);
		for (const project of projects)
			await api(page, 'PUT', `${w}/projects/${project}/guests/${userOf.get(p.id)}`);
	}

	// Comments and time, each written again by whoever wrote it, in the order they were.
	const comments = rows('comment')
		.filter((r) => others(r, 'author_id') && !r.fields.deleted_at)
		.toSorted((a, b) => String(a.fields.created_at).localeCompare(String(b.fields.created_at)));
	for (const c of comments) {
		await api(page, 'DELETE', `${w}/comments/${c.id}`);
		await api(pages.get(c.fields.author_id as string)!, 'POST', `${w}/comments`, {
			task_id: c.fields.task_id,
			body: c.fields.body
		});
	}
	for (const t of rows('time_entry').filter((r) => others(r, 'user_id') && !r.fields.deleted_at)) {
		await api(page, 'DELETE', `${w}/time-entries/${t.id}`);
		await api(pages.get(t.fields.user_id as string)!, 'POST', `${w}/time-entries`, {
			task_id: t.fields.task_id,
			day: t.fields.day,
			minutes: t.fields.minutes,
			note: t.fields.note ?? ''
		});
	}
	for (const t of rows('project_time_entry').filter(
		(r) => others(r, 'user_id') && !r.fields.deleted_at
	)) {
		await api(page, 'DELETE', `${w}/project-time-entries/${t.id}`);
		await api(pages.get(t.fields.user_id as string)!, 'POST', `${w}/project-time-entries`, {
			project_id: t.fields.project_id,
			day: t.fields.day,
			minutes: t.fields.minutes,
			note: t.fields.note ?? ''
		});
	}

	// Work of their own: each member looks after a project or two, with some
	// of its open tasks theirs and time logged on them over the last eight weeks.
	const projects = new Map(rows('project').map((r) => [r.fields.name as string, r.id]));
	const byName = new Map(sample.people.map((p) => [p.name, p.id]));
	const looksAfter: [string, string[]][] = [
		['Tomasz Wolny', ['Clients', 'Hiring']],
		['Beth Alvarez', ['Orders', 'Marketing']],
		['Arjun Mehta', ['Roastery']]
	];
	const assigned = new Set(rows('task_assignee').map((r) => r.fields.task_id as string));
	const random = numbers(7);
	const today = new Date();
	for (const [name, names] of looksAfter) {
		const person = byName.get(name)!;
		const mine: string[] = [];
		for (const project of names.map((n) => projects.get(n)!)) {
			const open = [...tasks.values()].filter(
				(t) =>
					t.fields.project_id === project &&
					!t.fields.completed_at &&
					!t.fields.deleted_at &&
					!assigned.has(t.id)
			);
			for (const [i, t] of open.entries()) {
				if (i % 3 === 2) continue;
				await api(page, 'POST', `${w}/task-assignees`, {
					task_id: t.id,
					user_id: userOf.get(person)
				});
				mine.push(t.id);
			}
		}
		const as = pages.get(person)!;
		for (let week = 8; week >= 0; week--) {
			for (let n = 0; n < 3 && mine.length; n++) {
				const on = new Date(today.getTime() - (week * 7 + Math.floor(random() * 5)) * 86_400_000);
				if (on > today) continue;
				await api(as, 'POST', `${w}/time-entries`, {
					task_id: mine[Math.floor(random() * mine.length)],
					day: day(on),
					minutes: 30 + 15 * Math.floor(random() * 10),
					note: ''
				});
			}
		}
	}

	// Where the pictures go, by name.
	const named = (entity: string, field: string) =>
		Object.fromEntries(
			rows(entity)
				.filter((r) => !r.fields.deleted_at)
				.map((r) => [r.fields[field] as string, r.id])
		);
	await page.goto(`/?ws=${ws}`);
	await expect(page.locator('.sync.synced').first()).toBeAttached();
	// The sample suggests a workspace theme; answered, so its notice stays out of the pictures.
	await page.getByRole('button', { name: 'No thanks' }).click();
	await expect(page.getByRole('button', { name: 'No thanks' })).toBeHidden();
	for (const other of pages.values()) if (other !== page) await other.context().close();
	writeFileSync(
		SEED,
		JSON.stringify({
			ws,
			now: new Date().toISOString(),
			projects: named('project', 'name'),
			tasks: named('task', 'title'),
			pages: named('page', 'title'),
			views: named('saved_view', 'name')
		})
	);
	await page.context().storageState({ path: STATE });
});
