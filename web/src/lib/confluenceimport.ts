// Importing from Confluence (docs/PLAN.md section 3, Importers): the files
// Confluence's own Export to Markdown writes, one page each, with its title
// as the first heading, its text and its tables, and no pictures, attached
// files or pages under it. Reading is pure; running makes the pages in the
// wiki, side by side under where they go, in title order.
import { createPage, ensureMainPage } from './sync/data';
import { engine } from './sync/engine.svelte';
import { keyBetween } from './sync/sortkey';
import { uuid } from './uuid';

/** The server's limits on a page (syncer/entities.go). */
const PAGE_TITLE = 200;
const PAGE_BODY = 1_000_000;
/** The most one file may be, and the most pages one import makes. */
export const MAX_FILE = 20 << 20;
export const MAX_PAGES = 2000;

/** A file chosen, as read. */
export interface ConfluenceFile {
	name: string;
	text: string;
}

/** A page a file would make. */
export interface ConfluencePage {
	file: string;
	title: string;
	body: string;
	words: number;
	tables: number;
	/** Links to Confluence pages, which keep going to Confluence. */
	confluenceLinks: number;
	/** Cut to the server's limit. */
	cut: boolean;
}

export type PagesGo = { kind: 'confluence' } | { kind: 'top' } | { kind: 'project'; id: string };

/** The first heading, which Confluence writes as the page's title. */
const TITLE = /^#[ \t]+([\s\S]+)$/;

/**
 * A heading's text without the spaces and #s that may close it, as
 * `(.+?)[ \t#]*$` takes it, in one pass: the pattern would try the run
 * from each of its characters in turn.
 */
function unclosed(s: string): string {
	let end = s.length;
	while (end > 1 && ' \t#'.includes(s[end - 1])) end--;
	return s.slice(0, end);
}

/** A table's separator row. */
const SEPARATOR = /^\|?[ \t]*:?-{3,}:?[ \t]*(\|[ \t]*:?-{3,}:?[ \t]*)*\|?[ \t]*$/;
// Control characters other than tabs and line ends, which every importer
// leaves out: they only hide or garble what is written.
// eslint-disable-next-line no-control-regex
const CONTROLS = /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/g;

/**
 * A link, and its address. The address stops at the next ( unless its
 * parentheses pair, so a line of `[](` never closed is read once rather
 * than from each to its end.
 */
const LINK = /!?\[(?:[^[\]\\]|\\.)*\]\(([^()\s]+(?:\([^()\s]*\)[^()\s]*)*)(?:\s+"[^"]*")?\)/g;

/** Whether an address is a page in Confluence, here or another site's. */
export function isConfluence(href: string): boolean {
	try {
		const u = new URL(href);
		return (
			u.hostname.endsWith('.atlassian.net') || /\/wiki\/(spaces|display|pages)\//.test(u.pathname)
		);
	} catch {
		return false;
	}
}

/** Reads one file as a page: its title, and the rest as its body. */
export function readPage(file: ConfluenceFile): ConfluencePage {
	const lines = file.text
		.replace(/^\uFEFF/, '')
		.replace(/\r\n?/g, '\n')
		.replace(CONTROLS, '')
		.split('\n');
	let title = '';
	const first = lines.findIndex((l) => l.trim() !== '');
	const heading = first >= 0 ? TITLE.exec(lines[first]) : null;
	if (heading) {
		title = unclosed(heading[1]).trim();
		lines.splice(first, 1);
	}
	if (!title) title = file.name.replace(/\.(md|markdown)$/i, '').replace(CONTROLS, '');
	title = title.replace(/\s+/g, ' ').trim().slice(0, PAGE_TITLE) || 'Untitled';
	// Confluence leaves runs of blank lines where pictures and macros were.
	let body = lines
		.join('\n')
		.replace(/\n{3,}/g, '\n\n')
		.trim();
	const cut = body.length > PAGE_BODY;
	if (cut) body = body.slice(0, PAGE_BODY);
	const words = body
		.replace(/\]\([^)\]]*\)/g, ']')
		.split(/[^\p{L}\p{N}']+/u)
		.filter(Boolean).length;
	const tables = lines.filter((l) => SEPARATOR.test(l.trim()) && l.includes('-')).length;
	let confluenceLinks = 0;
	for (const m of body.matchAll(LINK)) if (isConfluence(m[1])) confluenceLinks++;
	return { file: file.name, title, body, words, tables, confluenceLinks, cut };
}

export interface ConfluencePlan {
	pages: ConfluencePage[];
	/** Pages whose title is already in the wiki they go to, or twice among the files. */
	repeats: Set<string>;
	/** Files left out, and why. */
	skipped: { file: string; reason: string }[];
	over: boolean;
}

/**
 * What the files would make, in title order. `existing` is the titles
 * already in the wiki they go to, to say when a page would repeat one.
 */
export function planConfluence(
	files: ConfluenceFile[],
	existing: string[],
	skipped: { file: string; reason: string }[] = []
): ConfluencePlan {
	const pages = files
		.map(readPage)
		.sort((a, b) => a.title.localeCompare(b.title, undefined, { sensitivity: 'base' }));
	const key = (t: string) => t.toLocaleLowerCase();
	const seen = new Set(existing.map(key));
	const repeats = new Set<string>();
	for (const p of pages) {
		if (seen.has(key(p.title))) repeats.add(p.file);
		seen.add(key(p.title));
	}
	return { pages, repeats, skipped, over: pages.length > MAX_PAGES };
}

export interface ConfluenceMade {
	/** The pages made, the new Confluence page first when there is one. */
	pages: string[];
	/** The pages made at the top, which take the rest with them to the trash. */
	top: string[];
	/** Where they went, for a link to it. */
	root: { id: string; projectId: string | null };
}

/** Makes the plan's pages, side by side in title order, under where they go. */
export async function runConfluence(
	ws: string,
	p: ConfluencePlan,
	where: PagesGo,
	onProgress: (done: number, total: number) => void = () => {}
): Promise<ConfluenceMade> {
	const projectId = where.kind === 'project' ? where.id : null;
	const main = await ensureMainPage(ws, projectId);
	const root = where.kind === 'confluence' ? await createPage(ws, null, main, 'Confluence') : main;
	const made: string[] = where.kind === 'confluence' ? [root] : [];
	const top: string[] = where.kind === 'confluence' ? [root] : [];
	let last =
		engine
			.replica(ws)
			?.pages(projectId)
			.filter((x) => x.parentId === root)
			.at(-1)?.sortKey ?? null;
	await engine.bulkly(ws, async () => {
		for (const [n, page] of p.pages.entries()) {
			const id = uuid();
			last = keyBetween(last, null);
			await engine.mutate(
				ws,
				'page',
				id,
				{
					project_id: projectId,
					parent_id: root,
					title: page.title,
					body: page.body,
					sort_key: last,
					deleted_at: null
				},
				{ undoable: false }
			);
			made.push(id);
			if (where.kind !== 'confluence') top.push(id);
			onProgress(n + 1, p.pages.length);
		}
	});
	return { pages: made, top, root: { id: root, projectId } };
}
