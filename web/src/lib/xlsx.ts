// A small reader of Excel workbooks (.xlsx), for imports whose file is one
// (monday.com's, mondayimport.ts): each sheet's name and its cells as text,
// a row of strings a row. A workbook is a zip of XML; this reads the zip's
// directory and inflates the parts it needs (zip.ts), and reads their XML
// with a scanner of its own (no DOM, so it runs in tests too). Dates, which Excel keeps as numbers with a date
// format, come out as YYYY-MM-DD, with the time where there is one. The
// file is untrusted: its parts are inflated only up to a limit, sheets are
// held to a size, and control characters are taken out of the text.

import { inflate as inflateEntry, readZip, ZipError } from './zip';

export class XlsxError extends Error {}

const UNREADABLE = 'That file could not be read as an Excel workbook.';

/** No part is inflated past this, nor all of them together past twice it. */
const PART_LIMIT = 100 << 20;
const MAX_ROWS = 100_000;
const MAX_COLUMNS = 1_000;

export interface Sheet {
	name: string;
	rows: string[][];
}

/** The sheets of an Excel workbook, in order, each a grid of text. */
export async function readXlsx(
	data: ArrayBuffer | Uint8Array,
	{ limit = PART_LIMIT } = {}
): Promise<Sheet[]> {
	const bytes = data instanceof Uint8Array ? data : new Uint8Array(data);
	const zip = zipOf(bytes);
	let inflated = 0;
	const part = async (path: string): Promise<string | null> => {
		const entry = zip.get(path.replace(/^\/+/, ''));
		if (!entry) return null;
		const out = await inflate(bytes, entry, limit);
		inflated += out.length;
		if (inflated > 2 * limit) throw new XlsxError('That workbook is larger than an import takes.');
		return new TextDecoder().decode(out);
	};

	const workbook = await part('xl/workbook.xml');
	if (workbook === null) throw new XlsxError(UNREADABLE);
	const rels = relations((await part('xl/_rels/workbook.xml.rels')) ?? '', 'xl/');
	const ofType = (kind: string) =>
		[...rels.values()].find((r) => r.type.endsWith(`/${kind}`))?.target;

	let date1904 = false;
	const sheets: { name: string; target: string }[] = [];
	walk(workbook, {
		open(name, attrs) {
			if (name === 'workbookPr') date1904 = attrs.date1904 === '1' || attrs.date1904 === 'true';
			if (name !== 'sheet') return;
			const id = attrs['r:id'] ?? Object.entries(attrs).find(([k]) => k.endsWith(':id'))?.[1];
			const target = id ? rels.get(id)?.target : undefined;
			if (target) sheets.push({ name: clean(attrs.name ?? ''), target });
		}
	});

	const shared = sharedStrings(
		(await part(ofType('sharedStrings') ?? 'xl/sharedStrings.xml')) ?? ''
	);
	const dates = dateStyles((await part(ofType('styles') ?? 'xl/styles.xml')) ?? '');
	const out: Sheet[] = [];
	for (const s of sheets) {
		const xml = await part(s.target);
		if (xml !== null) out.push({ name: s.name, rows: sheetRows(xml, shared, dates, date1904) });
	}
	return out;
}

/** The zip's words as a workbook's. */
function why(err: unknown): never {
	if (!(err instanceof ZipError)) throw err;
	if (err.reason === 'password')
		throw new XlsxError('That workbook has a password, so it cannot be read.');
	if (err.reason === 'large') throw new XlsxError('That workbook is larger than an import takes.');
	throw new XlsxError(UNREADABLE);
}

function zipOf(bytes: Uint8Array) {
	try {
		return readZip(bytes);
	} catch (err) {
		why(err);
	}
}

async function inflate(...args: Parameters<typeof inflateEntry>) {
	try {
		return await inflateEntry(...args);
	} catch (err) {
		why(err);
	}
}

// The XML.

interface Visitor {
	open?(name: string, attrs: Record<string, string>, empty: boolean): void;
	close?(name: string): void;
	text?(s: string): void;
}

/**
 * Calls the visitor for each element opened and closed, and the text
 * between, in order. Names lose their namespace prefix (x:c is c), and
 * attributes keep theirs (r:id).
 */
export function walk(xml: string, v: Visitor) {
	let i = 0;
	const n = xml.length;
	while (i < n) {
		const lt = xml.indexOf('<', i);
		const stop = lt < 0 ? n : lt;
		if (stop > i) v.text?.(unescape(xml.slice(i, stop)));
		if (lt < 0) return;
		if (xml.startsWith('<![CDATA[', lt)) {
			const e = xml.indexOf(']]>', lt);
			if (e < 0) throw new XlsxError(UNREADABLE);
			v.text?.(xml.slice(lt + 9, e));
			i = e + 3;
			continue;
		}
		if (xml.startsWith('<!--', lt)) {
			const e = xml.indexOf('-->', lt);
			if (e < 0) throw new XlsxError(UNREADABLE);
			i = e + 3;
			continue;
		}
		if (xml[lt + 1] === '?' || xml[lt + 1] === '!') {
			const e = xml.indexOf('>', lt);
			if (e < 0) throw new XlsxError(UNREADABLE);
			i = e + 1;
			continue;
		}
		// The tag's end: the first > outside a quoted value.
		let gt = lt + 1;
		let quote = '';
		for (; gt < n; gt++) {
			const c = xml[gt];
			if (quote) {
				if (c === quote) quote = '';
			} else if (c === '"' || c === "'") quote = c;
			else if (c === '>') break;
		}
		if (gt >= n) throw new XlsxError(UNREADABLE);
		const body = xml.slice(lt + 1, gt);
		i = gt + 1;
		if (body.startsWith('/')) {
			v.close?.(local(body.slice(1).trim()));
			continue;
		}
		const empty = body.endsWith('/');
		const tag = empty ? body.slice(0, -1) : body;
		const name = local(tag.match(/^[^\s/>]+/)?.[0] ?? '');
		v.open?.(name, attributes(tag), empty);
		if (empty) v.close?.(name);
	}
}

const local = (name: string) => name.slice(name.indexOf(':') + 1);

const space = (c: string) => c === ' ' || c === '\t' || c === '\n' || c === '\r';

/**
 * A tag's attributes, read in one pass. A regular expression for them took
 * the square of the tag's length on a long run of letters with no =, which
 * a file of a few hundred bytes could hold.
 */
export function attributes(tag: string): Record<string, string> {
	const attrs: Record<string, string> = {};
	const n = tag.length;
	let i = 0;
	while (i < n && !space(tag[i])) i++; // the tag's name
	while (i < n) {
		while (i < n && space(tag[i])) i++;
		const from = i;
		while (i < n && !space(tag[i]) && tag[i] !== '=') i++;
		const key = tag.slice(from, i);
		while (i < n && space(tag[i])) i++;
		if (tag[i] !== '=') {
			if (i === from) i++; // nothing read: step past it
			continue;
		}
		i++;
		while (i < n && space(tag[i])) i++;
		const quote = tag[i];
		if (quote !== '"' && quote !== "'") continue;
		const end = tag.indexOf(quote, i + 1);
		if (end < 0) break;
		if (key) attrs[key] = unescape(tag.slice(i + 1, end));
		i = end + 1;
	}
	return attrs;
}

const ENTITIES: Record<string, string> = { lt: '<', gt: '>', amp: '&', quot: '"', apos: "'" };

function unescape(s: string): string {
	if (!s.includes('&')) return s;
	return s.replace(/&(#x[0-9a-fA-F]+|#\d+|[a-z]+);/g, (whole, ref: string) => {
		if (ref[0] !== '#') return ENTITIES[ref] ?? whole;
		const code = ref[1] === 'x' ? parseInt(ref.slice(2), 16) : parseInt(ref.slice(1), 10);
		return code > 0 && code <= 0x10ffff ? String.fromCodePoint(code) : '';
	});
}

// Control characters other than tabs and line ends.
// eslint-disable-next-line no-control-regex
const CONTROLS = /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/g;

/**
 * Text as a person wrote it: Excel's escapes for characters XML cannot hold
 * (_x000D_) undone, line ends made \n, and other control characters taken
 * out.
 */
function clean(s: string): string {
	return s
		.replace(/_x([0-9A-Fa-f]{4})_/g, (_, hex: string) => String.fromCharCode(parseInt(hex, 16)))
		.replace(/\r\n?/g, '\n')
		.replace(CONTROLS, '');
}

/** A workbook's relations: each ID's type and the part it names, from the workbook's folder. */
function relations(xml: string, base: string) {
	const out = new Map<string, { type: string; target: string }>();
	walk(xml, {
		open(name, a) {
			if (name !== 'Relationship' || !a.Id || !a.Target || a.TargetMode === 'External') return;
			out.set(a.Id, { type: a.Type ?? '', target: resolvePath(base, a.Target) });
		}
	});
	return out;
}

function resolvePath(base: string, target: string): string {
	const parts = (target.startsWith('/') ? target.slice(1) : base + target).split('/');
	const out: string[] = [];
	for (const p of parts) {
		if (p === '..') out.pop();
		else if (p && p !== '.') out.push(p);
	}
	return out.join('/');
}

/** The workbook's shared strings, in order: each <si>'s text, its runs joined. */
function sharedStrings(xml: string): string[] {
	const out: string[] = [];
	let current: string | null = null;
	let inText = false;
	let phonetic = 0;
	walk(xml, {
		open(name) {
			if (name === 'si') current = '';
			else if (name === 't') inText = true;
			else if (name === 'rPh') phonetic++;
		},
		close(name) {
			if (name === 'si' && current !== null) {
				out.push(clean(current));
				current = null;
			} else if (name === 't') inText = false;
			else if (name === 'rPh') phonetic--;
		},
		text(s) {
			if (inText && !phonetic && current !== null) current += s;
		}
	});
	return out;
}

// Excel's own formats that are dates or times.
const DATE_FORMATS = new Set([
	14, 15, 16, 17, 18, 19, 20, 21, 22, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 45, 46, 47, 50, 51,
	52, 53, 54, 55, 56, 57, 58
]);

/** Which cell styles are dates, by their place in the styles' cellXfs. */
function dateStyles(xml: string): boolean[] {
	const custom = new Map<number, boolean>();
	const out: boolean[] = [];
	let inXfs = false;
	walk(xml, {
		open(name, a) {
			if (name === 'numFmt' && a.numFmtId) {
				// A format is a date's if, outside quotes, colors and escapes, it has d, m, y, h or s.
				const code = (a.formatCode ?? '')
					.replace(/"[^"]*"/g, '')
					.replace(/\[[^\]]*\]/g, '')
					.replace(/\\./g, '');
				custom.set(Number(a.numFmtId), /[dmyhs]/i.test(code));
			} else if (name === 'cellXfs') inXfs = true;
			else if (name === 'xf' && inXfs) {
				const id = Number(a.numFmtId ?? 0);
				out.push(custom.get(id) ?? DATE_FORMATS.has(id));
			}
		},
		close(name) {
			if (name === 'cellXfs') inXfs = false;
		}
	});
	return out;
}

/** A column's place from a cell's reference: B5 is 1. */
function columnOf(ref: string): number {
	let n = 0;
	for (const c of ref) {
		const code = c.toUpperCase().charCodeAt(0);
		if (code < 65 || code > 90) break;
		n = n * 26 + code - 64;
	}
	return n - 1;
}

const DAY = 86_400_000;

/** An Excel day number as YYYY-MM-DD, with HH:MM (and :SS) where it has a time. */
export function excelDate(serial: number, date1904 = false): string | null {
	if (!Number.isFinite(serial) || serial < 0 || serial > 2_958_465) return null;
	// Day 1 is 1900-01-01, and Excel counts a 29 February 1900 that never was.
	const base = date1904
		? Date.UTC(1904, 0, 1)
		: serial < 60
			? Date.UTC(1899, 11, 31)
			: Date.UTC(1899, 11, 30);
	const at = new Date(base + Math.round((serial * DAY) / 1000) * 1000);
	const pad = (n: number) => String(n).padStart(2, '0');
	const day = `${at.getUTCFullYear()}-${pad(at.getUTCMonth() + 1)}-${pad(at.getUTCDate())}`;
	const h = at.getUTCHours();
	const m = at.getUTCMinutes();
	const s = at.getUTCSeconds();
	if (!h && !m && !s) return day;
	return `${day} ${pad(h)}:${pad(m)}${s ? `:${pad(s)}` : ''}`;
}

/** A sheet's cells as text, a row of them a row, the gaps empty. */
function sheetRows(xml: string, shared: string[], dates: boolean[], date1904: boolean): string[][] {
	const rows: string[][] = [];
	let row = -1;
	let next = 0;
	let cell: { col: number; type: string; style: number; value: string } | null = null;
	let inValue = false;
	let inline = false;
	let inText = false;
	let phonetic = 0;
	const big = () => new XlsxError('That sheet is larger than an import takes.');
	walk(xml, {
		open(name, a) {
			if (name === 'row') {
				row = a.r ? Number(a.r) - 1 : row + 1;
				if (!Number.isInteger(row) || row < 0) throw new XlsxError(UNREADABLE);
				if (row >= MAX_ROWS) throw big();
				next = 0;
			} else if (name === 'c') {
				const col = a.r ? columnOf(a.r) : next;
				if (col < 0) throw new XlsxError(UNREADABLE);
				if (col >= MAX_COLUMNS) throw big();
				next = col + 1;
				cell = { col, type: a.t ?? 'n', style: Number(a.s ?? 0), value: '' };
			} else if (name === 'v') inValue = true;
			else if (name === 'is') inline = true;
			else if (name === 't') inText = true;
			else if (name === 'rPh') phonetic++;
		},
		close(name) {
			if (name === 'v') inValue = false;
			else if (name === 'is') inline = false;
			else if (name === 't') inText = false;
			else if (name === 'rPh') phonetic--;
			else if (name === 'c' && cell) {
				const c = cell;
				cell = null;
				const text = cellText(c, shared, dates, date1904);
				if (!text || row < 0) return;
				while (rows.length <= row) rows.push([]);
				const r = rows[row];
				while (r.length < c.col) r.push('');
				r[c.col] = text;
			}
		},
		text(s) {
			if (!cell) return;
			if (inValue || (inline && inText && !phonetic)) cell.value += s;
		}
	});
	return rows;
}

function cellText(
	c: { type: string; style: number; value: string },
	shared: string[],
	dates: boolean[],
	date1904: boolean
): string {
	const v = c.value;
	switch (c.type) {
		case 's':
			return shared[Number(v)] ?? '';
		case 'str':
		case 'inlineStr':
			return clean(v);
		case 'b':
			return v.trim() === '1' ? 'TRUE' : v.trim() ? 'FALSE' : '';
		case 'e':
			return '';
		case 'd':
			return clean(v).trim();
		default: {
			const t = v.trim();
			if (!t) return '';
			return (dates[c.style] && excelDate(Number(t), date1904)) || t;
		}
	}
}
