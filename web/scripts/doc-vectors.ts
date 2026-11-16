// Writes shared/doc-vectors.json from the editor's own answers
// (src/lib/docs/vectors.ts), keeping the Markdown the Go tests filled in
// for documents that are still there. Run it after upgrading Tiptap or
// Yjs, then `go test ./internal/ydoc -update` in server/.
//
//   npx tsx scripts/doc-vectors.ts
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { buildVectors } from '../src/lib/docs/vectors';

const path = new URL('../../shared/doc-vectors.json', import.meta.url).pathname;
const old: { render?: { json: unknown; md?: string }[] } = existsSync(path)
	? JSON.parse(readFileSync(path, 'utf8'))
	: {};
const kept = new Map((old.render ?? []).map((c) => [JSON.stringify(c.json), c.md]));
const next = buildVectors();
const render = next.render.map((c) => {
	const md = kept.get(JSON.stringify(c.json));
	return md === undefined ? c : { ...c, md };
});
writeFileSync(path, JSON.stringify({ ...next, render }, null, '\t') + '\n');
console.log(
	`wrote ${path}: ${next.parse.length} parse, ${render.length} render, ${next.yjs.length} yjs`
);
