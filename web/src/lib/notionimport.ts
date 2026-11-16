// Notion import: an export of Notion's (Markdown & CSV, as a zip) brought
// into the wiki and as tasks. The page is routes/(app)/settings/w/[ws]/data/apps,
// which loads this only when Notion is chosen; its screen is
// components/data/NotionImport.svelte.
//
// Notion's export is a zip, sometimes of Part zips. A page is a Markdown
// file, `Title <id>.md`, its subpages in a folder beside it, `Title <id>/`,
// or, when the export was made without folders for subpages, everything at
// the top and each subpage a link on a line of its own in its page.
// A database is a CSV of the view's columns, `Name <id>.csv`, another of
// every property, `Name <id>_all.csv`, and a folder of a page a row, each
// starting with its title and a `Key: value` line a property. Links between
// pages are relative paths to their files, URL-encoded, and a page's images
// and files sit beside it. Callouts are written as <aside> and toggles as
// <details>.
//
// Reading makes each page's Markdown Mila's, with every link and file it
// can follow written as a marker (`mila-notion:<n>`, the nth reference) to
// be pointed at what the import makes; a link to a page not in the export
// becomes its text. Planning places the pages under one root and makes the
// databases brought as tasks one table for the CSV import's dry run
// (csvimport.ts, the `NOTION` dialect); running makes the tasks, then the
// pages, uploads the files, and last writes each page's body with its
// links pointed home.
import { parseCsv } from './csv';
import {
	ANY_CSV,
	guessTargets,
	plan as planTable,
	readDate,
	type Dialect,
	type Target
} from './csvimport';
import { LIMITS, MAX_ROWS, runImport, type Context, type Plan } from './importplan';
import { createPage, ensureMainPage } from './sync/data';
import { engine } from './sync/engine.svelte';
import type { CustomField } from './sync/entities';
import { keyBetween } from './sync/sortkey';
import { MAX_UPLOAD, IMAGE_TYPES, fileUrl, uploadFile } from './uploads';
import { uuid } from './uuid';
import { inflate, readZip, ZipError, type Entry } from './zip';

export class NotionError extends Error {}

/** The largest export taken, and the most a part of it may inflate to. */
export const MAX_EXPORT = 500 << 20;
const MAX_ENTRIES = 50_000;
/** The most a page's or database's file may hold. */
const MAX_TEXT = 20 << 20;
/** The server's limits: a page's title and body (syncer/entities.go). */
const PAGE_TITLE = 200;
const PAGE_BODY = 1_000_000;

const UNREADABLE = 'That file could not be read as a Notion export.';

function zipWords(err: unknown): never {
	if (!(err instanceof ZipError)) throw err;
	if (err.reason === 'password')
		throw new NotionError('That zip has a password, so it cannot be read.');
	if (err.reason === 'large') throw new NotionError('That export is larger than an import takes.');
	throw new NotionError(UNREADABLE);
}

/** A file in the export, by its path, read when needed, up to `limit` bytes. */
export interface NotionEntry {
	path: string;
	size: number;
	read: (limit?: number) => Promise<Uint8Array>;
}

/**
 * All an export's files may inflate to, read after read: sixteen times its
 * zip, but at least 64 MB and at most twice the largest export. Markdown
 * and CSV deflate about three to six times and pictures and files hardly
 * at all, and a large export's Part zips are read once more besides, so a
 * real export of text alone stays well inside it, while a zip a stranger
 * made to inflate a thousandfold stops there.
 */
const INFLATION = 16;
const MIN_INFLATED = 64 << 20;
const MAX_INFLATED = 2 * MAX_EXPORT;

/** The files of an export's zip, with those of any zips inside it. */
export async function openNotion(data: Uint8Array): Promise<NotionEntry[]> {
	if (data.length > MAX_EXPORT)
		throw new NotionError('That export is larger than an import takes.');
	const out: NotionEntry[] = [];
	// A running total: each file is held to its own limit, and all of them
	// together to the budget, since many small streams each inflating a
	// thousandfold would otherwise fill the tab's memory. Parts stored as
	// they are count too.
	const budget = Math.min(MAX_INFLATED, Math.max(MIN_INFLATED, INFLATION * data.length));
	let inflated = 0;
	const counted = async (bytes: Uint8Array, e: Entry, limit: number) => {
		const room = budget - inflated;
		try {
			const read = await inflate(bytes, e, Math.min(limit, room));
			inflated += read.length;
			return read;
		} catch (err) {
			if (err instanceof ZipError && err.reason === 'large' && room < limit)
				throw new NotionError('That export is larger than an import takes.');
			throw err;
		}
	};
	const add = (bytes: Uint8Array, zip: Map<string, Entry>) => {
		for (const [path, e] of zip) {
			if (path.endsWith('/')) continue;
			if (out.length >= MAX_ENTRIES)
				throw new NotionError('That export has more files than an import takes.');
			out.push({ path, size: e.unpacked, read: (limit = MAX_EXPORT) => counted(bytes, e, limit) });
		}
	};
	try {
		const top = readZip(data);
		const inner = [...top].filter(([p]) => /\.zip$/i.test(p));
		add(data, new Map([...top].filter(([p]) => !/\.zip$/i.test(p))));
		// Notion gives a large export as Part zips inside the one downloaded.
		for (const [, e] of inner) {
			const bytes = await counted(data, e, MAX_EXPORT);
			add(bytes, readZip(bytes));
		}
	} catch (err) {
		zipWords(err);
	}
	return out;
}

// Reading.

/** What a link or file in a page points at. */
export type Ref =
	{ kind: 'page'; key: string } | { kind: 'db'; key: string } | { kind: 'file'; path: string };

/** A page of the export: a Markdown file, by its path without `.md`. */
export interface NotionPage {
	key: string;
	title: string;
	/** The page or database it is under, by key; null for the top. */
	parent: string | null;
	/** For a database's row, its properties, as the page lists them. */
	props: [string, string][];
	/** Mila's Markdown, with markers for links and files. */
	body: string;
	callouts: number;
	toggles: number;
	/** Links to pages not in the export, made text. */
	missing: number;
}

/** A database of the export: its rows, each with its page where it has one. */
export interface NotionDatabase {
	key: string;
	name: string;
	parent: string | null;
	headers: string[];
	rows: { cells: string[]; page: string | null }[];
}

export interface NotionExport {
	pages: Map<string, NotionPage>;
	databases: Map<string, NotionDatabase>;
	files: Map<string, NotionEntry>;
	refs: Ref[];
	/** What could not be read, by where it is. */
	problems: { at: string; reason: string }[];
}

const ID = /\s?([0-9a-f]{32})$/;
const stripId = (name: string) => name.replace(ID, '').trim();
const idOf = (name: string) => name.match(ID)?.[1] ?? null;
const base = (path: string) => path.slice(path.lastIndexOf('/') + 1);
/** A name the export gives only by a file's path, as other text is cleaned. */
const nameOf = (path: string) => stripId(base(path)).replace(CONTROLS, '');
const dirOf = (path: string) => (path.includes('/') ? path.slice(0, path.lastIndexOf('/')) : '');

/** A path made plain: no ./, and .. taken back. */
function normal(path: string): string {
	const out: string[] = [];
	for (const p of path.split('/')) {
		if (p === '..') out.pop();
		else if (p && p !== '.') out.push(p);
	}
	return out.join('/');
}

const decode = (s: string) => {
	try {
		return decodeURIComponent(s);
	} catch {
		return s;
	}
};

// Control characters other than tabs and line ends.
// eslint-disable-next-line no-control-regex
const CONTROLS = /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/g;

/** An export's pages, databases and files, its pages' Markdown made Mila's. */
export async function readNotion(entries: NotionEntry[]): Promise<NotionExport> {
	const files = new Map(entries.map((e) => [normal(e.path), e]));
	const text = async (e: NotionEntry) => {
		if (e.size > MAX_TEXT) return null;
		// The size the zip claims is the file's word, so the read holds to
		// the limit too: a page past it is left out as one claiming more is.
		let bytes: Uint8Array;
		try {
			bytes = await e.read(MAX_TEXT);
		} catch (err) {
			if (err instanceof ZipError && err.reason === 'large') return null;
			throw err;
		}
		return new TextDecoder()
			.decode(bytes)
			.replace(/^\uFEFF/, '')
			.replace(/\r\n?/g, '\n')
			.replace(CONTROLS, '');
	};
	const problems: NotionExport['problems'] = [];

	// The databases first, so a row's page knows its properties' names.
	const databases = new Map<string, NotionDatabase>();
	const csvs = [...files.keys()].filter((p) => /\.csv$/i.test(p));
	for (const path of csvs) {
		const key = path.replace(/(_all)?\.csv$/i, '');
		const all = /_all\.csv$/i.test(path);
		// The file of every property wins over the view's.
		if (databases.has(key) && !all) continue;
		if (!all && files.has(`${key}_all.csv`)) continue;
		const t = await text(files.get(path)!);
		if (t === null) {
			problems.push({ at: base(path), reason: 'It is larger than an import takes.' });
			continue;
		}
		const rows = parseCsv(t).filter((r) => r.some((c) => c.trim()));
		databases.set(key, {
			key,
			name: nameOf(key) || 'Untitled',
			parent: null,
			headers: rows[0] ?? ['Name'],
			rows: rows.slice(1).map((cells) => ({ cells, page: null }))
		});
	}

	const pageKeys = [...files.keys()].filter((p) => /\.md$/i.test(p)).map((p) => p.slice(0, -3));
	const isPage = new Set(pageKeys);
	// A folder may be named without the ID its page's or database's file has
	// (Projects/ beside Projects <id>.csv), as newer exports write them.
	const alias = new Map<string, string | null>();
	for (const k of [...pageKeys, ...databases.keys()]) {
		const plain = normal(`${dirOf(k)}/${stripId(base(k))}`);
		if (plain !== k) alias.set(plain, alias.has(plain) ? null : k);
	}
	/** The page or database a folder is named for, by ID or by name alone. */
	const named = (dir: string): string | null =>
		isPage.has(dir) || databases.has(dir) ? dir : (alias.get(dir) ?? null);
	/** What a folder is the subpages of: the page or database of its name, or the folder above's. */
	const owner = (dir: string): string | null => {
		for (let d = dir; d; d = dirOf(d)) {
			const k = named(d);
			if (k) return k;
		}
		return null;
	};
	for (const db of databases.values()) db.parent = owner(dirOf(db.key));

	// Every page and database by Notion's ID, for links written as notion.so addresses.
	const byId = new Map<string, Ref>();
	for (const k of pageKeys) {
		const id = idOf(base(k));
		if (id) byId.set(id, { kind: 'page', key: k });
	}
	for (const k of databases.keys()) {
		const id = idOf(base(k));
		if (id) byId.set(id, { kind: 'db', key: k });
	}

	const refs: Ref[] = [];
	const refIndex = new Map<string, number>();
	const ref = (r: Ref) => {
		const id = r.kind === 'file' ? `file:${r.path}` : `${r.kind}:${r.key}`;
		let n = refIndex.get(id);
		if (n === undefined) {
			n = refs.push(r) - 1;
			refIndex.set(id, n);
		}
		return n;
	};

	const pages = new Map<string, NotionPage>();
	for (const key of pageKeys) {
		const t = await text(files.get(`${key}.md`)!);
		const parent = owner(dirOf(key));
		const db = parent && databases.has(parent) ? databases.get(parent)! : null;
		if (t === null) {
			problems.push({ at: `${base(key)}.md`, reason: 'It is larger than an import takes.' });
			continue;
		}
		const resolve = (href: string): Ref | null => {
			if (/^[a-z][a-z0-9+.-]*:/i.test(href)) {
				const id = /notion\.(so|site)\b/i.test(href)
					? href.replace(/[?#].*$/, '').match(/([0-9a-f]{32})$/)?.[1]
					: undefined;
				return id ? (byId.get(id) ?? null) : null;
			}
			const path = normal(`${dirOf(key)}/${decode(href.replace(/[?#].*$/, ''))}`);
			if (/\.md$/i.test(path) && isPage.has(path.slice(0, -3)))
				return { kind: 'page', key: path.slice(0, -3) };
			if (/\.csv$/i.test(path)) {
				const k = path.replace(/(_all)?\.csv$/i, '');
				if (databases.has(k)) return { kind: 'db', key: k };
			}
			if (files.has(path) && !/\.(md|csv)$/i.test(path)) return { kind: 'file', path };
			return null;
		};
		const page = convertPage(t, {
			fallbackTitle: nameOf(key) || 'Untitled',
			known: db ? new Set(db.headers.map((h) => h.trim())) : null,
			resolve,
			ref
		});
		pages.set(key, { key, parent, ...page });
	}

	// An export made without folders for subpages has every page at the
	// top, and only its links say which page is under which: a subpage is
	// a link on a line of its own in the page it is on. The first page to
	// link to one so, in the export's order, is its parent, never one of
	// its own subpages; a database's file linked so is placed the same way.
	// What has no parent yet is above a page only when it is the top of the
	// page's chain, so each page keeps the top it was last found under
	// (taken up to date as it is asked), rather than walking up the chain
	// for each link, which a long one makes slow.
	if (![...files.keys()].some((p) => p.includes('/'))) {
		const above = new Map<string, string>();
		const topOf = (key: string) => {
			let top = key;
			while (above.has(top)) top = above.get(top)!;
			for (let k = key; k !== top;) {
				const next = above.get(k)!;
				above.set(k, top);
				k = next;
			}
			return top;
		};
		for (const p of pages.values())
			for (const line of p.body.split('\n')) {
				const m = line.match(ALONE);
				const r = m ? refs[+m[1]] : undefined;
				if (!r || r.kind === 'file' || r.key === p.key) continue;
				const child = r.kind === 'page' ? pages.get(r.key) : databases.get(r.key);
				if (!child || child.parent !== null) continue;
				if (topOf(p.key) === r.key) continue;
				child.parent = p.key;
				above.set(r.key, p.key);
			}
	}

	// Each database's rows, with their pages: by title, in order; a page
	// with no row of its own is a row of what it lists.
	for (const db of databases.values()) {
		const free = new Map<string, string[]>();
		for (const p of pages.values())
			if (p.parent === db.key) free.set(p.title, [...(free.get(p.title) ?? []), p.key]);
		for (const row of db.rows) {
			const title = (row.cells[0] ?? '').trim() || 'Untitled';
			const list = free.get(title);
			if (list?.length) row.page = list.shift()!;
		}
		for (const list of free.values())
			for (const key of list) {
				const p = pages.get(key)!;
				const props = new Map(p.props);
				db.rows.push({
					cells: db.headers.map((h, i) => (i === 0 ? p.title : (props.get(h.trim()) ?? ''))),
					page: key
				});
			}
	}
	return { pages, databases, files, refs, problems };
}

/** Mila's name for an image Notion links: shown in the page where the server shows it. */
const isImage = (path: string) =>
	/\.(png|jpe?g|gif|webp)$/i.test(path) &&
	IMAGE_TYPES.includes(
		`image/${path
			.match(/\.(\w+)$/)![1]
			.toLowerCase()
			.replace('jpg', 'jpeg')}`
	);

/** A line that is one link, to what a marker names, and nothing else. */
const ALONE = /^\s*\[[^\]]*\]\(mila-notion:(\d+)\)\s*$/;

const LINK =
	/(!?)\[((?:[^[\]\\]|\\.)*)\]\(\s*<?([^()\s<>]*(?:\([^()\s]*\)[^()\s<>]*)*)>?(?:\s+"[^"]*")?\s*\)/g;

/**
 * Tags Notion writes that have nothing to show but their text. A tag's run
 * stops at the next < as well as its > (and a toggle's title, below, at the
 * next <summary>), so a line of tags never closed is read once rather than
 * from each of them to its end.
 */
const PLAIN_TAGS =
	/<\/?(span|div|p|u|mark|figure|figcaption|sup|sub|del|ins|font|center|small|big|strong|em|b|i|code|s)\b[^<>]*>/gi;

/** A heading's text without the #s that may close it, as `(.+?)\s*#*$` takes it, in one pass. */
function unclosed(s: string): string {
	let end = s.length;
	while (end > 1 && s[end - 1] === '#') end--;
	return s.slice(0, end);
}

/**
 * A page's Markdown made Mila's: its title taken from its heading, a row's
 * properties from the lines below it, callouts as note boxes (without the
 * icon Notion draws as a picture), toggles as their heading and contents,
 * and links and files as markers.
 */
export function convertPage(
	md: string,
	o: {
		fallbackTitle: string;
		/** A row's property names, so a body that starts "Note: ..." is not one. */
		known: Set<string> | null;
		resolve: (href: string) => Ref | null;
		ref: (r: Ref) => number;
	}
) {
	const lines = md.split('\n');
	let i = 0;
	while (i < lines.length && !lines[i].trim()) i++;
	let title = o.fallbackTitle;
	const heading = lines[i]?.match(/^#\s+([\s\S]+)$/);
	if (heading) {
		title = unclosed(heading[1]).trim();
		i++;
	}
	title = title.replace(/\s+/g, ' ').slice(0, PAGE_TITLE) || 'Untitled';
	while (i < lines.length && !lines[i].trim()) i++;
	const props: [string, string][] = [];
	if (o.known)
		for (; i < lines.length; i++) {
			const m = lines[i].match(/^([^:\n]{1,100}):\s?(.*)$/);
			if (!m || !o.known.has(m[1].trim())) break;
			props.push([m[1].trim(), m[2].trim()]);
		}

	let callouts = 0;
	let toggles = 0;
	let missing = 0;
	const out: string[] = [];
	let fence: string | null = null;
	let aside: string[] | null = null;
	const link = (line: string) =>
		line.replace(LINK, (whole, bang: string, text: string, href: string) => {
			if (!href || href.startsWith('#')) return whole;
			// A marker the export already held is the file's words, not one
			// of ours: its text alone.
			if (/^mila-notion:/i.test(href)) return text;
			const r = o.resolve(href);
			if (!r) {
				if (/^[a-z][a-z0-9+.-]*:/i.test(href) && !/notion\.(so|site)\b/i.test(href)) return whole;
				// A Notion page that is not in the export: its text.
				missing++;
				return bang ? text : text || decode(base(href)).replace(/\.md$/i, '');
			}
			const n = o.ref(r);
			if (bang && !(r.kind === 'file' && isImage(r.path)))
				return `[${text || 'File'}](mila-notion:${n})`;
			return `${bang}[${text}](mila-notion:${n})`;
		});
	const tidy = (line: string) =>
		link(
			line
				.replace(/<br\s*\/?>/gi, '  ')
				.replace(/<summary>((?:[^<]|<(?!\/?summary>))*)<\/summary>/gi, (_, s: string) => {
					toggles++;
					return `**${s.trim()}**\n`;
				})
				.replace(/<\/?details\b[^<>]*>/gi, '')
				.replace(PLAIN_TAGS, '')
		);
	for (; i < lines.length; i++) {
		const line = lines[i];
		const f = line.match(/^\s*(`{3,}|~{3,})/);
		if (fence) {
			out.push(line);
			if (f && f[1][0] === fence[0] && f[1].length >= fence.length) fence = null;
			continue;
		}
		if (f) {
			fence = f[1];
			out.push(line);
			continue;
		}
		if (/^\s*<aside\b[^>]*>\s*$/i.test(line)) {
			aside = [];
			continue;
		}
		if (aside && /^\s*<\/aside>\s*$/i.test(line)) {
			callouts++;
			const body = aside.join('\n').trim().split('\n');
			out.push('> [!NOTE]', ...body.map((l) => (l.trim() ? `> ${l}` : '>')));
			aside = null;
			continue;
		}
		// A callout's icon, when it is not an emoji, is a picture from
		// Notion's own site on the callout's first line: left out.
		if (aside && !aside.some((l) => l.trim()) && /^\s*<img\b[^>]*>\s*$/i.test(line)) continue;
		if (aside) aside.push(tidy(line));
		else out.push(tidy(line));
	}
	if (aside) out.push(...aside);
	const body = out
		.join('\n')
		.replace(/\n{3,}/g, '\n\n')
		.trim();
	return { title, props, body, callouts, toggles, missing };
}

// Dates.

const MONTHS = ['jan', 'feb', 'mar', 'apr', 'may', 'jun', 'jul', 'aug', 'sep', 'oct', 'nov', 'dec'];
const pad = (n: number) => String(n).padStart(2, '0');

/**
 * A date as Notion writes one, by the person's setting: April 9, 2025,
 * 04/09/2025, 09/04/2025 (read as day first only where the month cannot
 * be), 2025/04/09 or 2025-04-09, any time after it left off; a range's
 * start. Null for anything else.
 */
export function readNotionDate(s: string): string | null {
	// A range's start, found by the arrow and the spaces either side of it alone.
	const arrow = s.search(/\s→\s/);
	const v = (arrow < 0 ? s : s.slice(0, arrow)).trim();
	const iso = readDate(v);
	if (iso) return iso;
	let y: number, m: number, d: number;
	const num = v.match(/^(\d{1,2})\/(\d{1,2})\/(\d{4})\b/);
	const words = v.match(/^([A-Za-z]{3,})\.?\s+(\d{1,2}),?\s+(\d{4})\b/);
	const dayFirst = v.match(/^(\d{1,2})\s+([A-Za-z]{3,})\.?,?\s+(\d{4})\b/);
	if (num) {
		[m, d, y] = [+num[1], +num[2], +num[3]];
		if (m > 12) [m, d] = [d, m];
	} else if (words || dayFirst) {
		const [name, day, year] = words
			? [words[1], words[2], words[3]]
			: [dayFirst![2], dayFirst![1], dayFirst![3]];
		m = MONTHS.indexOf(name.slice(0, 3).toLowerCase()) + 1;
		if (!m) return null;
		[d, y] = [+day, +year];
	} else return null;
	return readDate(`${y}-${pad(m)}-${pad(d)}`);
}

/** A moment as Notion writes one ("April 9, 2025 3:00 PM"), or an ISO time. */
function readNotionInstant(s: string): string | null {
	const iso = ANY_CSV.instant(s);
	if (iso) return iso;
	const day = readNotionDate(s);
	if (!day) return null;
	const t = s.match(/(\d{1,2}):(\d{2})\s*([AP]M)?/i);
	let h = t ? +t[1] : 0;
	if (t?.[3]) h = (h % 12) + (/p/i.test(t[3]) ? 12 : 0);
	const at = new Date(`${day}T${pad(h)}:${t ? t[2] : '00'}:00`);
	return Number.isNaN(at.getTime()) ? null : at.toISOString();
}

// The table of the databases brought as tasks.

const KEY = 'Notion ID';
const DATABASE = 'Database';
const NAME = 'Name';
const PAGE = 'Page';
const OWN = [KEY, DATABASE, NAME, PAGE];

const squash = (h: string) => h.toLowerCase().replace(/[^a-z0-9]/g, '');

const HEADERS: Record<string, Target> = {
	notionid: 'key',
	database: 'project',
	name: 'title',
	page: 'notes',
	enddate: 'due',
	date: 'due',
	dates: 'due',
	duedate: 'due',
	due: 'due',
	person: 'assignees',
	people: 'assignees',
	owner: 'assignees',
	assignedto: 'assignees',
	multiselect: 'labels',
	created: 'made',
	createdtime: 'made',
	createdat: 'made'
};

/** Notion's databases: properties by name, dates as Notion writes them, the rest in the notes. */
export const NOTION: Dialect = {
	...ANY_CSV,
	app: 'Notion',
	headers: HEADERS,
	date: readNotionDate,
	dateHint: 'a date like April 9, 2025 or 04/09/2025',
	instant: readNotionInstant,
	// Notion's task databases offer Critical above High.
	priority: (s) => (squash(s) === 'critical' ? 'urgent' : ANY_CSV.priority(s)),
	priorities: '(low, medium, high or critical)',
	listStrangers: true,
	restInNotes: true,
	behind: "Comments and page history are not in Notion's export, so they stay in Notion"
};

/**
 * The databases brought as tasks, as one table: a row a database's row,
 * its key, its database (the project it goes in), its title, its page,
 * then every property, those of one name in one column.
 */
export function taskTable(exp: NotionExport, dbs: string[]): string[][] {
	const headings = [...OWN];
	const columns = new Map<string, number>();
	const rows: string[][] = [];
	for (const key of dbs) {
		const db = exp.databases.get(key);
		if (!db) continue;
		const at = db.headers.map((h, i) => {
			if (i === 0) return 2;
			const name = h.trim() || `Column ${i + 1}`;
			const k = squash(name);
			let c = columns.get(k);
			if (c === undefined) {
				headings.push(OWN.some((o) => squash(o) === k) ? `${name} (property)` : name);
				c = headings.length - 1;
				columns.set(k, c);
			}
			return c;
		});
		db.rows.forEach((row, n) => {
			const cells: string[] = [rowKey(key, row.page, n), db.name, '', ''];
			row.cells.forEach((v, i) => {
				if (i < at.length) cells[at[i]] = v;
			});
			cells[2] = (cells[2] ?? '').trim() || 'Untitled';
			cells[3] = row.page ? (exp.pages.get(row.page)?.body ?? '') : '';
			rows.push(cells);
		});
	}
	return [
		headings,
		...rows.map((r) => Array.from({ length: headings.length }, (_, i) => r[i] ?? ''))
	];
}

/**
 * A database row's key in the task table: its page's Notion ID, or, for a
 * row with no page, its database's and its place.
 */
export const rowKey = (db: string, page: string | null, n: number) =>
	(page ? (idOf(base(page)) ?? page) : null) ?? `${idOf(base(db)) ?? db}#${n + 1}`;

// Planning.

export type PagesGo = { kind: 'notion' } | { kind: 'top' } | { kind: 'project'; id: string };

export interface NotionChoices {
	where: PagesGo;
	/** How each database comes in, by key: tasks unless said. */
	as: Record<string, 'tasks' | 'pages'>;
	/** The task table's columns, when changed from the guess. */
	targets?: Target[];
}

/** A page to make: under another planned page by key, or under the import's root. */
export interface PlannedPage {
	key: string;
	title: string;
	parent: string | null;
	body: string;
}

export interface NotionPlan {
	pages: PlannedPage[];
	/** The databases brought as tasks, through the CSV import's dry run. */
	table: string[][];
	targets: Target[];
	tasks: Plan | null;
	/** The rows made tasks, by page key, for links to them. */
	taskRows: Map<string, string>;
	/** Each database brought as tasks, and the project its rows go in. */
	taskDbs: string[];
	uploads: { path: string; name: string; owner: { kind: 'page' | 'task'; key: string } }[];
	tooLarge: string[];
	images: number;
	links: number;
	missing: number;
	callouts: number;
	toggles: number;
	problems: { at: string; reason: string }[];
	/** More pages and tasks than an import makes. */
	over: boolean;
}

const MARKER = /mila-notion:(\d+)/g;

/** A database row's properties as a table, for the top of its page. */
function propsTable(props: [string, string][]): string {
	const cell = (s: string) =>
		s.replace(/\s+/g, ' ').trim().slice(0, 500).replace(/\\/g, '\\\\').replace(/\|/g, '\\|');
	const rows = props.filter(([, v]) => v.trim());
	return rows.length
		? [
				'| Property | Value |',
				'| --- | --- |',
				...rows.map(([k, v]) => `| ${cell(k)} | ${cell(v)} |`)
			].join('\n')
		: '';
}

/** What an export would make, as chosen: the pages, the tasks, the files. */
export function planNotion(
	exp: NotionExport,
	choices: NotionChoices,
	ctx: Context,
	fields: CustomField[] = []
): NotionPlan {
	const asTasks = (db: string) => (choices.as[db] ?? 'tasks') === 'tasks';
	const taskDbs = [...exp.databases.keys()].filter(asTasks);
	const taskRows = new Map<string, string>();
	for (const k of taskDbs)
		exp.databases.get(k)!.rows.forEach((r, n) => {
			if (r.page) taskRows.set(r.page, rowKey(k, r.page, n));
		});

	const table = taskTable(exp, taskDbs);
	const guessed = guessTargets(table[0], fields, NOTION);
	const targets =
		choices.targets && choices.targets.length === table[0].length ? choices.targets : guessed;
	const tasks =
		table.length > 1
			? planTable(table, targets, { kind: 'new', name: 'Notion' }, ctx, {
					dialect: NOTION,
					byColumn: true
				})
			: null;

	// The pages, each under the page or database it was under in Notion. A
	// page under a row that is now a task goes under that row's database's
	// place; a database brought as pages is a page of its own.
	const pages: PlannedPage[] = [];
	const placeOf = (key: string | null): string | null => {
		while (key !== null) {
			if (exp.pages.has(key) && !taskRows.has(key)) return key;
			if (exp.databases.has(key) && !asTasks(key)) return `db:${key}`;
			key = exp.pages.get(key)?.parent ?? exp.databases.get(key)?.parent ?? null;
		}
		return null;
	};
	// In the export's order, a database's rows in the database's.
	const order = new Map([...exp.files.keys()].map((path, i) => [path, i]));
	const at = new Map<string, number>();
	const asPage = (p: NotionPage): PlannedPage => ({
		key: p.key,
		title: p.title,
		parent: placeOf(p.parent),
		body: [propsTable(p.props), p.body].filter(Boolean).join('\n\n')
	});
	const rowPages = new Set<string>();
	for (const db of exp.databases.values()) {
		if (asTasks(db.key)) continue;
		const first = order.get(`${db.key}_all.csv`) ?? order.get(`${db.key}.csv`) ?? 0;
		pages.push({
			key: `db:${db.key}`,
			title: db.name.slice(0, PAGE_TITLE),
			parent: placeOf(db.parent),
			body: ''
		});
		at.set(`db:${db.key}`, first);
		db.rows.forEach((row, n) => {
			const page = row.page ? exp.pages.get(row.page) : undefined;
			if (page) {
				rowPages.add(page.key);
				pages.push({ ...asPage(page), parent: `db:${db.key}` });
				at.set(page.key, first + (n + 1) / 1e6);
				return;
			}
			const props = db.headers
				.slice(1)
				.map((h, i): [string, string] => [h.trim(), row.cells[i + 1] ?? '']);
			const key = `row:${db.key}#${n + 1}`;
			pages.push({
				key,
				title: (row.cells[0] ?? '').trim().slice(0, PAGE_TITLE) || 'Untitled',
				parent: `db:${db.key}`,
				body: propsTable(props)
			});
			at.set(key, first + (n + 1) / 1e6);
		});
	}
	for (const p of exp.pages.values()) {
		if (taskRows.has(p.key) || rowPages.has(p.key)) continue;
		pages.push(asPage(p));
		at.set(p.key, order.get(`${p.key}.md`) ?? 0);
	}
	pages.sort((a, b) => (at.get(a.key) ?? 0) - (at.get(b.key) ?? 0));
	// Parents before their subpages.
	const planned = new Set(pages.map((p) => p.key));
	const ordered: PlannedPage[] = [];
	const done = new Set<string>();
	const visit = (p: PlannedPage, depth = 0) => {
		if (done.has(p.key) || depth > 200) return;
		if (p.parent && planned.has(p.parent) && !done.has(p.parent))
			visit(
				pages.find((x) => x.key === p.parent)!,
				depth + 1
			);
		done.add(p.key);
		ordered.push(p);
	};
	for (const p of pages) visit(p);

	// The files the pages and tasks point at, each uploaded to the first that does.
	const uploads: NotionPlan['uploads'] = [];
	const tooLarge: string[] = [];
	const seen = new Set<string>();
	let images = 0;
	let links = 0;
	const docs: { kind: 'page' | 'task'; key: string; body: string }[] = [
		...ordered.map((p) => ({ kind: 'page' as const, key: p.key, body: p.body })),
		...[...taskRows].map(([page, task]) => ({
			kind: 'task' as const,
			key: task,
			body: exp.pages.get(page)?.body ?? ''
		}))
	];
	for (const d of docs)
		for (const m of d.body.matchAll(MARKER)) {
			const r = exp.refs[+m[1]];
			if (!r) continue;
			if (r.kind !== 'file') {
				links++;
				continue;
			}
			if (seen.has(r.path)) continue;
			seen.add(r.path);
			const e = exp.files.get(r.path)!;
			if (e.size > MAX_UPLOAD) tooLarge.push(base(r.path));
			else {
				uploads.push({
					path: r.path,
					name: base(r.path).replace(CONTROLS, ''),
					owner: { kind: d.kind, key: d.key }
				});
				if (isImage(r.path)) images++;
			}
		}

	const kept = [...exp.pages.values()];
	const total = ordered.length + (tasks?.tasks.length ?? 0);
	return {
		pages: ordered,
		table,
		targets,
		tasks,
		taskRows,
		taskDbs,
		uploads,
		tooLarge,
		images,
		links,
		missing: kept.reduce((n, p) => n + p.missing, 0),
		callouts: kept.reduce((n, p) => n + p.callouts, 0),
		toggles: kept.reduce((n, p) => n + p.toggles, 0),
		problems: exp.problems,
		over: total > MAX_ROWS
	};
}

// Running.

export interface NotionMade {
	/** The pages made, the root first where the import made one. */
	pages: string[];
	/** The pages made at the top, which take the rest with them to the trash. */
	top: string[];
	tasks: string[];
	/** The page everything went under, and its project, for a link to it. */
	root: { id: string; projectId: string | null };
	/** Files that could not be uploaded. */
	failed: string[];
}

/**
 * Makes what a plan says: the tasks, then the pages, then the files, then
 * the pages' bodies and the tasks' notes with their links pointed at what
 * was made. `href` makes a page's address (wiki.ts's pageHref), `projectHref`
 * a project's.
 */
export async function runNotion(
	ws: string,
	exp: NotionExport,
	p: NotionPlan,
	where: PagesGo,
	links: {
		page: (page: { id: string; projectId: string | null }) => string;
		project: (id: string) => string;
		task: (id: string) => string;
	},
	onProgress: (done: number, total: number) => void = () => {}
): Promise<NotionMade> {
	const total = p.pages.length + (p.tasks?.tasks.length ?? 0) + p.uploads.length + 1;
	let done = 0;
	const step = () => onProgress(++done, total);

	// The tasks, with each row's task by its page.
	const taskIds = new Map<string, string>();
	const tasks = p.tasks ? await runImport(ws, p.tasks, (n) => onProgress(n, total), taskIds) : [];
	done = tasks.length;
	const projectOf = new Map<string, string>();
	for (const db of p.taskDbs) {
		const first = exp.databases
			.get(db)!
			.rows.map((r, n) => rowKey(db, r.page, n))
			.find((k) => taskIds.has(k));
		const t = first ? engine.replica(ws)?.task(taskIds.get(first)!) : undefined;
		if (t?.projectId) projectOf.set(db, t.projectId);
	}

	// The pages, under the root.
	const projectId = where.kind === 'project' ? where.id : null;
	const main = await ensureMainPage(ws, projectId);
	const root = where.kind === 'notion' ? await createPage(ws, null, main, 'Notion') : main;
	const pageIds = new Map<string, string>();
	const made: string[] = where.kind === 'notion' ? [root] : [];
	const top: string[] = where.kind === 'notion' ? [root] : [];
	await engine.bulkly(ws, async () => {
		const last = new Map<string, string | null>();
		for (const page of p.pages) {
			const parent = page.parent ? (pageIds.get(page.parent) ?? root) : root;
			if (!last.has(parent))
				last.set(
					parent,
					engine
						.replica(ws)
						?.pages(projectId)
						.filter((x) => x.parentId === parent)
						.at(-1)?.sortKey ?? null
				);
			const sortKey = keyBetween(last.get(parent) ?? null, null);
			last.set(parent, sortKey);
			const id = uuid();
			pageIds.set(page.key, id);
			made.push(id);
			if (parent === root && where.kind !== 'notion') top.push(id);
			await engine.mutate(
				ws,
				'page',
				id,
				{
					project_id: projectId,
					parent_id: parent,
					title: page.title,
					body: '',
					sort_key: sortKey,
					deleted_at: null
				},
				{ undoable: false }
			);
			step();
		}
	});

	// The files, each to the page or task that first points at it.
	const fileIds = new Map<string, string>();
	const failed: string[] = [];
	for (const u of p.uploads) {
		const id = u.owner.kind === 'page' ? pageIds.get(u.owner.key) : taskIds.get(u.owner.key);
		const entry = exp.files.get(u.path);
		if (id && entry)
			try {
				const bytes = await entry.read(MAX_UPLOAD);
				const file = new File([bytes as BlobPart], u.name, { type: mimeOf(u.name) });
				fileIds.set(u.path, await uploadFile(ws, { kind: u.owner.kind, id }, file));
			} catch {
				failed.push(u.name);
			}
		step();
	}

	// The bodies, their links pointed home.
	const href = (r: Ref): string | null => {
		if (r.kind === 'file') {
			const id = fileIds.get(r.path);
			return id ? fileUrl(ws, id) : null;
		}
		if (r.kind === 'db') {
			const page = pageIds.get(`db:${r.key}`);
			if (page) return links.page({ id: page, projectId });
			const project = projectOf.get(r.key);
			return project ? links.project(project) : null;
		}
		const page = pageIds.get(r.key);
		if (page) return links.page({ id: page, projectId });
		const task = taskIds.get(p.taskRows.get(r.key) ?? '');
		return task ? links.task(task) : null;
	};
	const point = (body: string) =>
		body.replace(
			/(!?)\[((?:[^[\]\\]|\\.)*)\]\(mila-notion:(\d+)\)/g,
			(_, bang: string, text: string, n: string) => {
				const r = exp.refs[+n];
				const to = r ? href(r) : null;
				return to ? `${bang}[${text}](${to})` : text;
			}
		);
	await engine.bulkly(ws, async () => {
		for (const page of p.pages) {
			const id = pageIds.get(page.key);
			if (id && page.body)
				await engine.mutate(
					ws,
					'page',
					id,
					{ body: point(page.body).slice(0, PAGE_BODY) },
					{ undoable: false }
				);
		}
		for (const id of taskIds.values()) {
			const t = engine.replica(ws)?.task(id);
			if (t && t.notes.includes('mila-notion:'))
				await engine.mutate(
					ws,
					'task',
					id,
					{ notes: point(t.notes).slice(0, LIMITS.notes) },
					{ undoable: false }
				);
		}
	});
	step();
	return { pages: made, top, tasks, root: { id: root, projectId }, failed };
}

const MIME: Record<string, string> = {
	png: 'image/png',
	jpg: 'image/jpeg',
	jpeg: 'image/jpeg',
	gif: 'image/gif',
	webp: 'image/webp',
	svg: 'image/svg+xml',
	pdf: 'application/pdf',
	txt: 'text/plain',
	csv: 'text/csv',
	mp4: 'video/mp4',
	mov: 'video/quicktime',
	mp3: 'audio/mpeg',
	zip: 'application/zip',
	docx: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
	xlsx: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
	pptx: 'application/vnd.openxmlformats-officedocument.presentationml.presentation'
};
const mimeOf = (name: string) =>
	MIME[name.match(/\.(\w+)$/)?.[1]?.toLowerCase() ?? ''] ?? 'application/octet-stream';
