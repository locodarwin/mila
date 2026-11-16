// Lighthouse CI for the first-load budgets (docs/PLAN.md section 13): the
// sign-in page, a signed-in inbox and a board, audited as a mid-range
// phone on 4G (Lighthouse's mobile preset: a Moto G class screen, its CPU
// slowed four times, the network simulated). It starts bin/mila on a
// generated workspace of its own, signs in, runs Lighthouse CI (fetched
// with npx, pinned, so its many dependencies stay out of the lockfile),
// checks the assertions in lighthouserc.json and keeps the reports in
// tmp/lighthouse/reports.
//
// The script budget, 200 KB, counts what a page needs before it draws:
// the scripts asked for before mila:ready (on the sign-in page, before its
// largest paint), read from each run's network requests and held here, as
// make budgets-size holds each route's first load. Once a page is idle the
// app fetches the command palette, the task panel and the schedule chooser
// (about 35 KB), so their first use is instant; Lighthouse counts those
// too, so its own script check in lighthouserc.json is only a guard on the
// whole, at 300 KB (Shawn's choice, 2026-11-09).
//
//   make build && node scripts/lighthouse.mjs      (or make lighthouse)
//
// Chrome is Playwright's Chromium unless CHROME_PATH names another.
//
// Mila is measured behind Caddy, as a deployment serves it: HTTPS and
// HTTP/2, which loads every script over one connection. Served straight
// from the binary, over HTTP/1.1, Chrome opens six connections and
// queues the rest, which costs a phone on slow 4G about half a second
// that no deployment pays. Caddy runs in podman or docker, with a
// certificate of its own that Chrome is told to accept; with neither, or
// with LIGHTHOUSE_DIRECT=1, the binary is measured on its own.

import { execFileSync, spawn, spawnSync } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { chromium } from '@playwright/test';

const LHCI = '@lhci/cli@0.15.1';
/** The script a page may need before it draws, in bytes (sent, compressed). */
const FIRST_LOAD_BUDGET = 200 * 1024;
const PORT = 4175;
const PROXY_PORT = 4176;
const DIRECT = `http://127.0.0.1:${PORT}`;
const runtime = process.env.LIGHTHOUSE_DIRECT
	? ''
	: ['podman', 'docker'].find((c) => spawnSync(c, ['--version'], { stdio: 'ignore' }).status === 0);
const BASE = runtime ? `https://localhost:${PROXY_PORT}` : `http://localhost:${PORT}`;
const web = new URL('..', import.meta.url).pathname;
const root = join(web, '..');
const mila = join(root, 'bin/mila');
const tmp = join(root, 'tmp/lighthouse');
const data = join(tmp, 'data');
const flags = [
	'--data-dir',
	data,
	'--base-url',
	BASE,
	'--log-format',
	'text',
	'--log-level',
	'warn'
];

if (!existsSync(mila)) {
	console.error('No bin/mila; run make build first.');
	process.exit(1);
}
rmSync(tmp, { recursive: true, force: true });
mkdirSync(tmp, { recursive: true });

// A workspace of the size a person keeps: first loads, not the 5,000-task
// first sync, which has a budget (and a test) of its own.
const generated = execFileSync(
	mila,
	[
		'dev',
		'generate',
		'--email',
		'perf@example.com',
		'--tasks',
		'500',
		'--board-cards',
		'100',
		'--seed',
		'1',
		...flags
	],
	{ encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }
);
const ws = generated.match(/^workspace\s+(\S+)/m)?.[1];
const board = generated.match(/projects\/([\w-]+)\?view=board/)?.[1];

const server = spawn(mila, ['serve', '--listen', `127.0.0.1:${PORT}`, ...flags], {
	stdio: ['ignore', 'ignore', 'inherit']
});
const CADDY = 'mila-lighthouse-caddy';
if (runtime) {
	spawnSync(runtime, ['rm', '-f', CADDY], { stdio: 'ignore' });
	// prettier-ignore
	const run = spawnSync(runtime, [
		'run', '-d', '--rm', '--name', CADDY, '--network', 'host', 'docker.io/library/caddy:2-alpine',
		'caddy', 'reverse-proxy', '--from', BASE, '--to', `127.0.0.1:${PORT}`,
		'--internal-certs', '--disable-redirects'
	], { stdio: ['ignore', 'ignore', 'inherit'] });
	if (run.status !== 0) {
		console.error(`Could not start Caddy with ${runtime}.`);
		server.kill();
		process.exit(1);
	}
}
const stop = () => {
	server.kill();
	if (runtime) spawnSync(runtime, ['rm', '-f', CADDY], { stdio: 'ignore' });
};
process.on('exit', stop);
console.log(runtime ? `Measuring ${BASE}, Caddy in ${runtime}` : `Measuring ${BASE}, no proxy`);

// Whether Caddy answers yet; curl, since fetch will not take its certificate.
const proxyUp = () =>
	spawnSync('curl', ['-skf', '-o', '/dev/null', `${BASE}/healthz`], { stdio: 'ignore' }).status ===
	0;

let status = 1;
try {
	for (let i = 0; ; i++) {
		try {
			if ((await fetch(`${DIRECT}/healthz`)).ok && (!runtime || proxyUp())) break;
		} catch {
			// not up yet
		}
		if (i > 100) throw new Error('the server did not start');
		await new Promise((r) => setTimeout(r, 100));
	}

	// Signed in through a one-time link; Lighthouse sends the cookie.
	const link = execFileSync(mila, ['user', 'login-link', 'perf@example.com', ...flags], {
		encoding: 'utf8',
		stdio: ['ignore', 'pipe', 'ignore']
	}).trim();
	const res = await fetch(`${DIRECT}/api/v1/auth/magic-link/verify`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json', Origin: BASE },
		body: JSON.stringify({ token: new URL(link).searchParams.get('token') })
	});
	const session = res.headers
		.getSetCookie()
		.map((c) => c.split(';')[0])
		.find((c) => c.startsWith('mila_session='));
	if (!res.ok || !session) throw new Error(`sign-in failed: ${res.status}`);

	const chromePath = process.env.CHROME_PATH || chromium.executablePath();
	const base = JSON.parse(readFileSync(join(web, 'lighthouserc.json'), 'utf8'));
	const collect = (name, urls, extraHeaders, additive = false) => {
		const rc = structuredClone(base);
		rc.ci.collect = {
			...rc.ci.collect,
			url: urls,
			chromePath,
			settings: {
				...rc.ci.collect.settings,
				...(runtime
					? { chromeFlags: `${rc.ci.collect.settings.chromeFlags} --ignore-certificate-errors` }
					: {}),
				...(extraHeaders ? { extraHeaders } : {})
			}
		};
		const file = join(tmp, `${name}.json`);
		writeFileSync(file, JSON.stringify(rc));
		return lhci(['collect', `--config=${file}`, ...(additive ? ['--additive'] : [])]);
	};
	const lhci = (args) =>
		spawnSync('npx', ['--yes', '-p', LHCI, 'lhci', ...args], { cwd: tmp, stdio: 'inherit' }).status;

	// The sign-in page signed out; the app's pages signed in, in the
	// generated workspace (?ws= switches to it, as links from emails do).
	if (collect('signed-out', [`${BASE}/login`]) !== 0) throw new Error('lighthouse failed');
	const signedIn = [`${BASE}/?ws=${ws}`, `${BASE}/projects/${board}?view=board&ws=${ws}`];
	if (collect('signed-in', signedIn, { Cookie: session }, true) !== 0)
		throw new Error('lighthouse failed');

	// What the runs measured, beside the assertions' verdict, and the
	// first load's scripts by page, for the script budget.
	const dir = join(tmp, '.lighthouseci');
	const firstLoads = new Map();
	for (const f of readdirSync(dir).filter((f) => f.startsWith('lhr-') && f.endsWith('.json'))) {
		const lhr = JSON.parse(readFileSync(join(dir, f), 'utf8'));
		const a = lhr.audits;
		const ms = (id) => Math.round(a[id]?.numericValue ?? NaN);
		const script = a['resource-summary']?.details?.items?.find((i) => i.resourceType === 'script');
		const ready = a['user-timings']?.details?.items?.find((i) => i.name === 'mila:ready');
		// Unthrottled times, as the run observed them, like mila:ready's.
		const drawn =
			ready?.startTime ?? a.metrics?.details?.items?.[0]?.observedLargestContentfulPaint;
		const first = (a['network-requests']?.details?.items ?? [])
			.filter(
				(r) => r.resourceType === 'Script' && drawn !== undefined && r.networkRequestTime <= drawn
			)
			.reduce((n, r) => n + (r.transferSize ?? 0), 0);
		const page = new URL(lhr.finalDisplayedUrl).pathname;
		firstLoads.set(page, [...(firstLoads.get(page) ?? []), first]);
		console.log(
			`${page.padEnd(48)} FCP ${ms('first-contentful-paint')} ms, LCP ${ms('largest-contentful-paint')} ms, TBT ${ms('total-blocking-time')} ms, CLS ${a['cumulative-layout-shift']?.numericValue?.toFixed(3)}, script ${(first / 1024).toFixed(0)} KB first, ${((script?.transferSize ?? 0) / 1024).toFixed(0)} KB in all${ready ? `, mila:ready ${Math.round(ready.startTime)} ms unthrottled` : ''}`
		);
	}

	status = lhci(['assert', `--config=${join(web, 'lighthouserc.json')}`]);
	for (const [page, sizes] of firstLoads) {
		const median = sizes.sort((x, y) => x - y)[Math.floor(sizes.length / 2)];
		if (median > FIRST_LOAD_BUDGET) {
			console.error(
				`${page}: ${(median / 1024).toFixed(0)} KB of script before the page draws, budget ${FIRST_LOAD_BUDGET / 1024} KB`
			);
			status = 1;
		}
	}
	lhci(['upload', '--target=filesystem', `--outputDir=${join(tmp, 'reports')}`]);
} catch (err) {
	console.error(err instanceof Error ? err.message : err);
} finally {
	stop();
}
process.exit(status);
