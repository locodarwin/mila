// Tells the service worker which of Mermaid's files to leave out of what it
// keeps for offline use when it installs. Mermaid draws diagrams in pages and
// notes, and is several megabytes in all; the worker keeps its core and the
// kinds most used (flowchart, sequence, state, class and entity relationship)
// with the app, and each of the rest once it is first drawn
// (src/service-worker/index.ts). Run by `npm run build`, after vite build,
// since only the build's manifest says which hashed file holds what.

import { existsSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { brotliCompressSync, constants, gzipSync } from 'node:zlib';

const web = fileURLToPath(new URL('..', import.meta.url));
const build = join(web, 'build');
const manifest = JSON.parse(
	readFileSync(join(web, '.svelte-kit/output/client/.vite/manifest.json'), 'utf8')
);

const ENTRY = Object.keys(manifest).find((k) =>
	/node_modules\/mermaid\/dist\/mermaid\.core\.mjs$/.test(k)
);
if (!ENTRY) throw new Error('diagram-chunks.mjs: no Mermaid in the build');
// The kinds kept with the app, and the layout the flowchart draws with.
const COMMON =
	/node_modules\/mermaid\/dist\/chunks\/[^/]+\/(flowDiagram|sequenceDiagram|stateDiagram(-v2)?|classDiagram(-v2)?|erDiagram|dagre)-[A-Z0-9]+\.mjs$/;

/** Every module reached from these, by static imports and, if asked, dynamic ones. */
function reached(from, dynamic, stop = new Set()) {
	const seen = new Set();
	const next = [...from];
	while (next.length) {
		const k = next.pop();
		if (seen.has(k) || stop.has(k) || !manifest[k]) continue;
		seen.add(k);
		next.push(...(manifest[k].imports ?? []));
		if (dynamic) next.push(...(manifest[k].dynamicImports ?? []));
	}
	return seen;
}

const all = reached([ENTRY], true);
const kept = reached([ENTRY, ...Object.keys(manifest).filter((k) => COMMON.test(k))], false);
// Whatever the rest of the app reaches stays, Mermaid's or not.
const app = reached(
	Object.keys(manifest).filter((k) => manifest[k].isEntry),
	true,
	new Set([ENTRY])
);
const files = (keys) =>
	[...new Set([...keys].flatMap((k) => [manifest[k].file, ...(manifest[k].css ?? [])]))].sort();
const later = files([...all].filter((k) => !kept.has(k) && !app.has(k)));
const size = (list) => list.reduce((n, f) => n + statSync(join(build, f)).size, 0);
const keptFiles = files([...all].filter((k) => !later.includes(manifest[k].file)));

const swPath = join(build, 'service-worker.js');
const marker = 'mila:later';
const sw = readFileSync(swPath, 'utf8');
if (sw.split(marker).length !== 2) throw new Error(`service-worker.js has no single ${marker}`);
const patched = sw.replace(marker, later.join(','));
writeFileSync(swPath, patched);
// The precompressed copies are made again from the worker as it now is.
if (existsSync(swPath + '.br'))
	writeFileSync(
		swPath + '.br',
		brotliCompressSync(patched, { params: { [constants.BROTLI_PARAM_QUALITY]: 11 } })
	);
if (existsSync(swPath + '.gz')) writeFileSync(swPath + '.gz', gzipSync(patched, { level: 9 }));

const kb = (n) => `${Math.round(n / 1024)} KB`;
console.log(
	`diagram-chunks.mjs: ${keptFiles.length} of Mermaid's files (${kb(size(keptFiles))}) kept with the app, ${later.length} (${kb(size(later))}) when first drawn`
);
