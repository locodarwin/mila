// The initial-JavaScript budget: for every route,
// the scripts a first visit loads before the page shows (the entry, the
// root layout, the route's layouts and page, and everything they import
// statically) must come to less than 200 KB compressed. Lazy parts (the
// editor, dynamic imports) do not count. It reads the Vite manifest and
// SvelteKit's route table from the last `npm run build`, and measures
// the .gz copies the build wrote, which are what the server sends a
// browser without brotli (the .br copies are smaller still).
//
//   npm run build && npm run budget:size

import { existsSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { gzipSync } from 'node:zlib';

const BUDGET = 200 * 1024;
const root = new URL('..', import.meta.url).pathname;
const build = join(root, 'build');
const manifestPath = join(root, '.svelte-kit/output/client/.vite/manifest.json');
const appPath = join(root, '.svelte-kit/generated/build/client-optimized/app.js');

if (!existsSync(manifestPath) || !existsSync(appPath)) {
	console.error('No build to measure; run `npm run build` first.');
	process.exit(1);
}
const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));

// The route table: each route's page node and its layouts. The root
// layout (node 0) loads for every route.
const app = readFileSync(appPath, 'utf8');
const table = app.match(/export const dictionary = (\{[\s\S]*?\});/);
if (!table) {
	console.error(`No route table in ${appPath}; has SvelteKit changed its output?`);
	process.exit(1);
}
/** @type {Record<string, [number, number[]?]>} */
const dictionary = JSON.parse(table[1]);

const nodeKey = (n) => `.svelte-kit/generated/build/client-optimized/nodes/${n}.js`;
const entries = Object.keys(manifest).filter((k) =>
	manifest[k].file.startsWith('_app/immutable/entry/')
);

/** Every chunk loaded statically from these manifest keys. */
function closure(keys) {
	const seen = new Set();
	const stack = [...keys];
	while (stack.length) {
		const k = stack.pop();
		if (seen.has(k)) continue;
		if (!manifest[k]) throw new Error(`${k} is not in the Vite manifest`);
		seen.add(k);
		stack.push(...(manifest[k].imports ?? []));
	}
	return [...new Set([...seen].map((k) => manifest[k].file))];
}

function gzipped(file) {
	const gz = join(build, file + '.gz');
	if (existsSync(gz)) return statSync(gz).size;
	return gzipSync(readFileSync(join(build, file)), { level: 9 }).length;
}

function brotli(file) {
	const br = join(build, file + '.br');
	return existsSync(br) ? statSync(br).size : NaN;
}

const kb = (n) => (n / 1024).toFixed(1).padStart(6) + ' KB';
const rows = [];
for (const [route, [page, layouts = []]] of Object.entries(dictionary)) {
	const files = closure([...entries, nodeKey(0), ...layouts.map(nodeKey), nodeKey(page)]).filter(
		(f) => f.endsWith('.js')
	);
	const raw = files.reduce((n, f) => n + statSync(join(build, f)).size, 0);
	const gz = files.reduce((n, f) => n + gzipped(f), 0);
	const br = files.reduce((n, f) => n + brotli(f), 0);
	rows.push({ route, files: files.length, raw, gz, br });
}
rows.sort((a, b) => b.gz - a.gz);

console.log(`Initial JavaScript per route (budget ${kb(BUDGET).trim()} compressed)\n`);
console.log('    gzip   brotli      raw  files  route');
for (const r of rows) {
	const mark = r.gz >= BUDGET ? '  OVER' : '';
	console.log(
		`${kb(r.gz)} ${kb(r.br)} ${kb(r.raw)} ${String(r.files).padStart(5)}  ${r.route}${mark}`
	);
}
const over = rows.filter((r) => r.gz >= BUDGET);
if (over.length) {
	console.error(`\n${over.length} route(s) over the ${kb(BUDGET).trim()} budget.`);
	process.exit(1);
}
console.log(
	`\nLargest: ${rows[0].route} at ${kb(rows[0].gz).trim()} gzip, ${kb(BUDGET - rows[0].gz).trim()} under.`
);
