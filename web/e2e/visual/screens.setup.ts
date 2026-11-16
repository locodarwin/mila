import { expect, test as setup, type Page } from '@playwright/test';
import { readFileSync, writeFileSync } from 'node:fs';
import { DATA, DAY, SEED, STATE, TODAY } from './screens';

/** A date so many days from the fixed today. */
const day = (n: number) => new Date(TODAY.getTime() + n * DAY).toISOString().slice(0, 10);
/** A moment so many days from the fixed today: when a task was made, for Reports. */
const madeAt = (n: number) => new Date(TODAY.getTime() + n * DAY).toISOString();

async function api(page: Page, method: string, path: string, body?: unknown) {
	const out = await page.evaluate(
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
	expect(out.status, `${method} ${path}: ${JSON.stringify(out.body)}`).toBeLessThan(300);
	return out.body;
}

// One workspace for every main screen's screenshot. Nothing here depends on
// the day it runs: the app's clock is fixed at TODAY, and these dates with it.
setup('make the workspace the screens show', async ({ page }) => {
	let link = '';
	await expect
		.poll(() => {
			const log = readFileSync(`${DATA}/server.log`, 'utf8');
			link = log.match(/http:\/\/localhost:4176\/setup\?token=[\w-]+/)?.[0] ?? '';
			return link;
		})
		.not.toBe('');
	await page.goto(link);
	await page.getByLabel('Name').fill('Sam Rivera');
	await page.getByLabel('Email').fill('sam@example.com');
	await page.getByRole('button', { name: 'Create account' }).click();
	await page.getByRole('button', { name: 'Skip for now' }).click();
	await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();

	const { id: ws } = await api(page, 'POST', '/workspaces', { name: 'Foundry House' });
	const make = async (entity: string, body: unknown) =>
		(await api(page, 'POST', `/workspaces/${ws}/${entity}`, body)).id as string;

	const project = await make('projects', { name: 'Wholesale', color: 'blue', sort_key: 'a0' });
	await make('projects', { name: 'Holiday blends', color: 'green', sort_key: 'a1' });
	const [todo, doing, done] = await Promise.all(
		[
			['To do', 'todo', 'a0'],
			['Doing', 'in-progress', 'a1'],
			['Done', 'done', 'a2']
		].map(([name, category, sort_key]) =>
			make('statuses', { project_id: project, name, category, sort_key })
		)
	);
	const urgent = await make('labels', { name: 'Urgent', color: 'red', sort_key: 'a0' });
	const sampling = await make('labels', { name: 'Sampling', color: 'purple', sort_key: 'a1' });

	const task = (
		sort_key: string,
		title: string,
		status_id: string | undefined,
		fields: Record<string, unknown> = {}
	) => make('tasks', { project_id: project, status_id, sort_key, title, ...fields });

	const samples = await task('a0', 'Send samples to Lantern Cafe', doing, {
		priority: 'high',
		start_date: day(-2),
		due_date: day(1),
		estimate_minutes: 90,
		notes: 'Two coffees, with the roast date on each bag.',
		made_at: madeAt(-12)
	});
	await make('checklist-items', {
		task_id: samples,
		text: 'Roast Foundry House',
		done: true,
		sort_key: 'a0'
	});
	await make('checklist-items', { task_id: samples, text: 'Print the brew guide', sort_key: 'a1' });
	await make('task-labels', { task_id: samples, label_id: urgent });
	await make('task-labels', { task_id: samples, label_id: sampling });
	await make('tasks', {
		project_id: project,
		status_id: todo,
		parent_id: samples,
		sort_key: 'a0',
		title: 'Book the follow-up visit',
		due_date: day(4),
		made_at: madeAt(-6)
	});

	const terms = await task('a1', 'Agree terms with Harbor Street Espresso', todo, {
		priority: 'medium',
		start_date: day(1),
		due_date: day(6),
		estimate_minutes: 30,
		made_at: madeAt(-20)
	});
	const order = await task('a2', 'Set up the first standing order', todo, {
		start_date: day(7),
		due_date: day(12),
		made_at: madeAt(-20)
	});
	await make('task-dependencies', { task_id: order, blocker_id: terms });
	await task('a3', 'Check the Tuesday delivery route', todo, {
		priority: 'low',
		due_date: day(-1),
		made_at: madeAt(-30)
	});
	await task('a4', 'Review the price list', doing, {
		due_date: day(0),
		start_date: day(-5),
		made_at: madeAt(-9)
	});
	await task('a5', 'Service the sample grinder', done, {
		completed_at: TODAY.toISOString(),
		due_date: day(-3),
		made_at: madeAt(-16)
	});

	for (const [sort_key, title, fields] of [
		['a0', 'Call the green coffee broker', { priority: 'high', due_date: day(0) }],
		['a1', 'Order more 250 g bags', { due_date: day(2) }],
		['a2', 'Read the new water filter manual', {}]
	] as const) {
		await make('tasks', { sort_key, title, ...fields, made_at: madeAt(-3) });
	}

	// A wiki's main page comes with its first visit.
	await page.goto(`/projects/${project}?ws=${ws}&tab=wiki`);
	let main = '';
	await expect
		.poll(async () => {
			const out = await api(page, 'GET', `/workspaces/${ws}/pages?project_id=${project}`);
			const rows = (out.items ?? out.rows ?? out) as { id: string; parent_id: string | null }[];
			main = rows.find((p) => !p.parent_id)?.id ?? '';
			return main;
		})
		.not.toBe('');
	await api(page, 'PATCH', `/workspaces/${ws}/pages/${main}`, {
		body: [
			'Every new wholesale cafe goes through these steps before its first standing order.',
			'',
			'## Before the first call',
			'',
			'Look up the weekly volume and what the cafe pours now.',
			'',
			'### Delivery',
			'',
			'Anything past the Tuesday loop needs a courier quote.',
			'',
			'## Terms',
			'',
			'| Weekly volume | Terms | Delivery |',
			'| --- | --- | --- |',
			'| Under 10 kg | Prepay | Courier |',
			'| 10 to 40 kg | Net 15 | Weekly route |',
			'',
			'## Sampling',
			'',
			'- [x] Roast date on every bag',
			'- [ ] A one-page brew guide'
		].join('\n')
	});
	await make('pages', {
		project_id: project,
		parent_id: main,
		title: 'Delivery routes',
		body: 'The Tuesday loop.',
		sort_key: 'a0'
	});

	await page.goto(`/?ws=${ws}`);
	await expect(page.locator('.sync.synced').first()).toBeAttached();
	writeFileSync(SEED, JSON.stringify({ ws, project, task: samples }));
	await page.context().storageState({ path: STATE });
});
