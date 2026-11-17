import { expect, test, type BrowserContext, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { BUDGET_DATA, BUDGET_SERVER } from '../../playwright.budgets.config';

// The browser's budgets, on the generated
// 5,000-task workspace (playwright.budgets.config.ts):
//
// - first sync: a first visit, signed in on a device with nothing stored,
//   is ready (the mila:ready mark, see web/src/lib/perf.ts) in < 2 s;
// - warm start: opening the app again on that device is ready in < 500 ms;
// - interactions: switching views, filtering (on a field too), sorting,
//   grouping and opening a task show their result within 50 ms of the
//   input (the median of five tries is held to the budget; every try is
//   reported);
// - dragging on the 500-card board holds 60 fps: the 95th percentile frame
//   lasts under 20 ms (a frame at 60 fps is 16.7 ms, plus the jitter of a
//   headless browser's clock) and no frame takes over 50 ms. Headless
//   Chromium draws no drag image, so this is the page's own work per frame
//   while dragover events stream in.

// Shared CI runners hold them at a multiple of the budget (MILA_BUDGET_SCALE,
// 2 in .github/workflows/ci.yml); here they are held to the budget itself.
const SCALE = Number(process.env.MILA_BUDGET_SCALE ?? 1);
const FIRST_SYNC = 2000 * SCALE;
const WARM_START = 500 * SCALE;
const INTERACTION = 50 * SCALE;
const FRAME_P95 = 20 * SCALE;
const LONG_FRAME = 50 * SCALE;

const root = join(import.meta.dirname, '../..');
const data = join(root, BUDGET_DATA);
const generated = readFileSync(join(data, 'generated.txt'), 'utf8');
const workspace = generated.match(/^workspace\s+(\S+)/m)?.[1] ?? '';
const boardProject = generated.match(/projects\/([\w-]+)\?view=board/)?.[1] ?? '';

/** Signs the generated workspace's owner in to a context, through a one-time link. */
async function signIn(context: BrowserContext) {
	const link = execFileSync(
		join(root, '../bin/mila'),
		[
			'user',
			'login-link',
			'perf@example.com',
			'--data-dir',
			join(data, 'data'),
			'--base-url',
			BUDGET_SERVER
		],
		{ encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }
	).trim();
	const token = new URL(link).searchParams.get('token');
	const res = await context.request.post('/api/v1/auth/magic-link/verify', {
		data: { token },
		headers: { Origin: BUDGET_SERVER }
	});
	expect(res.ok()).toBe(true);
	// Open the generated workspace rather than the owner's personal one.
	await context.addInitScript((ws) => localStorage.setItem('mila.workspace', ws), workspace);
}

/** Milliseconds from the start of this page load to a user-timing mark. */
async function markTime(page: Page, name: string): Promise<number> {
	await page.waitForFunction((n) => performance.getEntriesByName(n).length > 0, name, {
		timeout: 30_000
	});
	return page.evaluate((n) => performance.getEntriesByName(n)[0].startTime, name);
}

type Done = { selector: string; text?: string; min?: number; changed?: boolean };
// changed: the elements matching are other ones than before.

/**
 * Milliseconds from the input that starts an action (the pointer going
 * down, or a key) to the first frame showing its result: `done` names an
 * element that must be there (with the text, or at least `min` of them, or
 * other ones than before).
 */
async function timeTo(page: Page, act: () => Promise<unknown>, done: Done): Promise<number> {
	await settle(page);
	await page.evaluate((d) => {
		const w = window as unknown as { budget: { start: number; end: number } };
		w.budget = { start: 0, end: 0 };
		// What the matching elements are, by their task or card.
		const which = () =>
			[...document.querySelectorAll<HTMLElement>(d.selector)]
				.map((e) => e.dataset.taskId ?? e.dataset.card ?? '')
				.join();
		const before = which();
		const start = (e: Event) => {
			w.budget.start ||= e.timeStamp;
		};
		addEventListener('pointerdown', start, { capture: true, once: true });
		addEventListener('keydown', start, { capture: true, once: true });
		const there = () => {
			const all = [...document.querySelectorAll(d.selector)];
			if (d.text !== undefined) return all.some((e) => e.textContent?.trim() === d.text);
			if (d.min !== undefined) return all.length >= d.min;
			if (d.changed) return which() !== before;
			return all.length > 0;
		};
		const frame = (t: number) => {
			if (w.budget.start && there()) w.budget.end = t;
			else requestAnimationFrame(frame);
		};
		requestAnimationFrame(frame);
	}, done);
	await act();
	await page.waitForFunction(
		() => (window as unknown as { budget: { end: number } }).budget.end > 0
	);
	return page.evaluate(() => {
		const b = (window as unknown as { budget: { start: number; end: number } }).budget;
		return b.end - b.start;
	});
}

/** Waits for the page to stop changing: no rows drawn or taken away for ten frames. */
async function settle(page: Page) {
	await page.evaluate(
		() =>
			new Promise<void>((done) => {
				let last = -1;
				let still = 0;
				const frame = () => {
					const n = document.querySelectorAll('[data-task-id], [data-card]').length;
					still = n === last ? still + 1 : 0;
					last = n;
					if (still >= 10) done();
					else requestAnimationFrame(frame);
				};
				requestAnimationFrame(frame);
			})
	);
}

const median = (xs: number[]) => [...xs].sort((a, b) => a - b)[Math.floor(xs.length / 2)];
const quantile = (xs: number[], q: number) =>
	[...xs].sort((a, b) => a - b)[Math.min(xs.length - 1, Math.floor(xs.length * q))];
const fmt = (xs: number[]) => xs.map((x) => x.toFixed(0)).join(', ');

test.describe.serial('budgets', () => {
	test.setTimeout(120_000);
	let context: BrowserContext;
	let page: Page;
	const results: string[] = [];

	test.beforeAll(async ({ browser }) => {
		expect(workspace, 'the generated workspace').not.toBe('');
		context = await browser.newContext();
		await signIn(context);
		page = await context.newPage();
		if (process.env.BUDGET_NO_VT)
			await page.addInitScript(() => {
				(Document.prototype as unknown as { startViewTransition?: unknown }).startViewTransition =
					undefined;
			});
	});

	test.afterAll(async () => {
		console.log('\n' + results.join('\n'));
		await context.close();
	});

	test('first sync of 5,000 tasks', async () => {
		await page.goto('/');
		const ready = await markTime(page, 'mila:ready');
		const synced = await page.evaluate(
			(ws) =>
				performance
					.getEntriesByName('mila:synced')
					.find((m) => (m as PerformanceMark).detail?.ws === ws)?.startTime ?? -1,
			workspace
		);
		await expect(page.locator('[data-task-id]').first()).toBeVisible();
		results.push(`first sync: synced at ${synced.toFixed(0)} ms, ready at ${ready.toFixed(0)} ms`);
		expect(synced).toBeGreaterThan(0);
		expect(ready, 'first visit ready (ms)').toBeLessThan(FIRST_SYNC);
	});

	test('warm start', async () => {
		// The service worker holds the shell once it has taken over.
		await page.evaluate(() => navigator.serviceWorker.ready);
		const times: number[] = [];
		for (let i = 0; i < 5; i++) {
			await page.reload();
			times.push(await markTime(page, 'mila:ready'));
		}
		results.push(`warm start: ${fmt(times)} ms (median ${median(times).toFixed(0)})`);
		expect(median(times), 'warm start ready (ms)').toBeLessThan(WARM_START);
	});

	test('switching views, filtering and opening a task', async () => {
		const nav = page.locator('#sidebar');
		const measured: Record<string, number[]> = {};
		const record = (name: string, ms: number) => {
			(measured[name] ??= []).push(ms);
			if (process.env.BUDGET_VERBOSE) console.log(name, ms.toFixed(0));
		};

		await page.goto('/');
		await markTime(page, 'mila:ready');
		for (let i = 0; i < 5; i++) {
			record(
				'inbox to upcoming',
				await timeTo(page, () => nav.getByRole('link', { name: 'Upcoming' }).click(), {
					selector: 'h1',
					text: 'Upcoming'
				})
			);
			record(
				'upcoming to inbox',
				await timeTo(page, () => nav.getByRole('link', { name: 'Inbox' }).click(), {
					selector: 'h1',
					text: 'Inbox'
				})
			);
		}

		for (let i = 0; i < 5; i++) {
			const row = page.locator('#main [data-task-id]').nth(i * 3);
			const title = (await row.locator('.title').first().textContent())?.trim() ?? '';
			record(
				'opening a task',
				await timeTo(page, () => row.locator('.title').first().click(), {
					selector: 'aside[aria-label="Task details"] h2',
					text: title
				})
			);
			await page.keyboard.press('Escape');
			await expect(page.locator('aside[aria-label="Task details"] h2')).toHaveCount(0);
		}

		await page.goto(`/projects/${boardProject}`);
		await markTime(page, 'mila:ready');
		const views = page.getByRole('group', { name: 'View' });
		for (let i = 0; i < 5; i++) {
			record(
				'list to 500-card board',
				await timeTo(page, () => views.getByRole('button', { name: 'Board' }).click(), {
					selector: '[data-card]',
					min: 400
				})
			);
			record(
				'board to list',
				await timeTo(page, () => views.getByRole('button', { name: 'List' }).click(), {
					selector: '[data-card]',
					changed: true
				})
			);
		}

		await page.goto('/');
		await markTime(page, 'mila:ready');
		await nav.getByRole('link', { name: 'Open work' }).click();
		await page.getByRole('button', { name: /Filter/ }).click();
		const priority = page.getByRole('group', { name: 'Priority' });
		for (let i = 0; i < 5; i++) {
			for (const name of ['High', 'High']) {
				record(
					'filtering',
					await timeTo(page, () => priority.getByRole('button', { name }).click(), {
						selector: '#main [data-task-id]',
						changed: true
					})
				);
			}
		}

		// Sorting, grouping and a field condition on the same view. A native
		// select is driven by its arrow keys: timeTo starts on a key or a
		// pointer, and selectOption sends neither.
		await page.getByRole('button', { name: /Filter/ }).click();
		await page.getByRole('button', { name: 'Display' }).click();
		const display = page.getByRole('group', { name: 'Display' });
		await display.getByRole('button', { name: 'Add a sort' }).click();
		const sortBy = display.getByLabel('Sort by', { exact: true });
		await sortBy.selectOption('priority');
		const groupBy = display.getByLabel('Group by');
		const rows = { selector: '#main [data-task-id]', changed: true };
		for (let i = 0; i < 5; i++) {
			await sortBy.focus();
			record('sorting', await timeTo(page, () => page.keyboard.press('ArrowDown'), rows));
			record('sorting', await timeTo(page, () => page.keyboard.press('ArrowUp'), rows));
			await groupBy.focus();
			record('grouping', await timeTo(page, () => page.keyboard.press('ArrowDown'), rows));
			record('grouping', await timeTo(page, () => page.keyboard.press('ArrowUp'), rows));
		}
		await display.getByRole('button', { name: 'Remove sort by Priority' }).click();
		await page.getByRole('button', { name: 'Display' }).click();
		await page.getByRole('button', { name: /Filter/ }).click();
		const filter = page.getByRole('group', { name: 'Filter' });
		await filter.getByRole('button', { name: 'Add a field condition' }).click();
		await filter.getByLabel('Condition 1 field').selectOption({ label: 'Estimate' });
		const comparison = filter.getByLabel('Estimate comparison');
		await comparison.selectOption('not_empty');
		for (let i = 0; i < 5; i++) {
			await comparison.focus();
			record(
				'filtering on a field',
				await timeTo(page, () => page.keyboard.press('ArrowUp'), rows)
			);
			record(
				'filtering on a field',
				await timeTo(page, () => page.keyboard.press('ArrowDown'), rows)
			);
		}

		const over: string[] = [];
		for (const [name, times] of Object.entries(measured)) {
			results.push(`${name}: ${fmt(times)} ms (median ${median(times).toFixed(0)})`);
			if (median(times) >= INTERACTION) over.push(`${name} ${median(times).toFixed(0)} ms`);
		}
		expect(over, `interactions over ${INTERACTION} ms`).toEqual([]);
	});

	test('a long list draws more as it is scrolled, and counts all of it', async () => {
		await page.goto('/upcoming');
		await markTime(page, 'mila:ready');
		await settle(page);
		const rows = page.locator('#main [data-task-id]');
		const drawn = await rows.count();
		// Upcoming holds every open task with a date: well over a screenful.
		const counts = await page.locator('#main .heading .count').allInnerTexts();
		const total = counts.reduce((n, c) => n + parseInt(c, 10), 0);
		expect(drawn).toBeLessThan(total);
		// Scroll whatever holds the rows, found as Growing finds it: the list's
		// own body on Upcoming, not the page.
		for (let i = 0; i < 3; i++) {
			await rows.last().evaluate((row) => {
				for (let el = row.parentElement; el; el = el.parentElement) {
					const { overflowY } = getComputedStyle(el);
					if (overflowY === 'auto' || overflowY === 'scroll') {
						el.scrollTo(0, el.scrollHeight);
						return;
					}
				}
				throw new Error('nothing scrolls the rows');
			});
			await settle(page);
		}
		expect(await rows.count()).toBeGreaterThan(drawn);
	});

	test('dragging on the 500-card board', async () => {
		await page.goto(`/projects/${boardProject}?view=board`);
		await markTime(page, 'mila:ready');
		const columns = page.locator('.board .column');
		await expect(page.locator('[data-card]')).toHaveCount(500);
		const card = columns.nth(1).locator('[data-card]').nth(2);
		await card.scrollIntoViewIfNeeded();
		const from = (await card.boundingBox())!;

		// Count frames from here on: their spacing, and any long one.
		await page.evaluate(() => {
			const w = window as unknown as { frames: number[]; long: number[]; going: boolean };
			w.frames = [];
			w.long = [];
			w.going = true;
			new PerformanceObserver((list) => {
				for (const e of list.getEntries()) w.long.push(e.duration);
			}).observe({ type: 'long-animation-frame', buffered: false });
			let last = 0;
			const frame = (t: number) => {
				if (last) w.frames.push(t - last);
				last = t;
				if (w.going) requestAnimationFrame(frame);
			};
			requestAnimationFrame(frame);
		});

		// Whether a landing slot opened, so the drag is known to be real.
		await page.evaluate(() => {
			const w = window as unknown as { slots: number };
			w.slots = 0;
			new MutationObserver(() => {
				if (document.querySelector('.board .slot')) w.slots++;
			}).observe(document.body, { childList: true, subtree: true });
		});
		const frame = () => page.evaluate(() => new Promise(requestAnimationFrame));
		await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
		await page.mouse.down();
		// Through every column and down each one, as a hand would wander,
		// a frame between moves.
		const count = await columns.count();
		for (let c = 0; c < count; c++) {
			const box = (await columns.nth(c).boundingBox())!;
			for (let y = 0; y < 24; y++) {
				await page.mouse.move(box.x + box.width / 2, box.y + 60 + (y * (box.height - 120)) / 23);
				await frame();
			}
		}
		await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
		await frame();
		await page.mouse.up();
		expect(
			await page.evaluate(() => (window as unknown as { slots: number }).slots),
			'landing slots shown'
		).toBeGreaterThan(0);

		const { frames, long } = await page.evaluate(() => {
			const w = window as unknown as { frames: number[]; long: number[]; going: boolean };
			w.going = false;
			return { frames: w.frames, long: w.long };
		});
		const p95 = quantile(frames, 0.95);
		const worst = Math.max(0, ...long);
		results.push(
			`board drag: ${frames.length} frames, p95 ${p95.toFixed(1)} ms, longest ${Math.max(...frames).toFixed(1)} ms, ${long.length} long animation frames (worst ${worst.toFixed(0)} ms)`
		);
		expect(frames.length).toBeGreaterThan(60);
		expect(p95, 'p95 frame (ms)').toBeLessThan(FRAME_P95);
		expect(worst, 'longest animation frame (ms)').toBeLessThan(LONG_FRAME);
	});
});
