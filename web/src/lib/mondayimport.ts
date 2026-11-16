// monday.com import: a board's Excel export (xlsx.ts reads it) made into a
// table for the CSV import's column screen and dry run (csvimport.ts), and
// how its values are written (a Dialect). The page loads this only when
// monday.com is chosen. A board's sheet is not a table of a row an item:
// its first rows are the board's name and description; then each group
// has a row of its name, a row of the board's column headings, its items,
// and a row of totals. Subitems, when the export has them, follow their
// item, a column further in under headings of their own that start
// "Subitems". Updates, when it has them, are a sheet of their own, each
// naming its item. `readMonday` makes one or more such files, a project
// each, into one table. The page is routes/(app)/settings/w/[ws]/data/apps.
import { ANY_CSV, type Dialect, type ReadComment, type Reshaped, type Target } from './csvimport';
import { jsonComments } from './jsoncomments';
import { readXlsx, type Sheet } from './xlsx';

const squash = (h: string) => h.toLowerCase().replace(/[^a-z0-9]/g, '');

// The table's own columns, before the board's. A board column with one of
// their names gets " (column)" after it.
const ID = 'Item ID';
const BOARD = 'Board';
const GROUP = 'Group';
const NAME = 'Name';
const PARENT = 'Parent item';
const UPDATES = 'Updates';
const OWN = [ID, BOARD, GROUP, NAME, PARENT, UPDATES];

/** What the table's headings and monday.com's usual column names feed. */
const HEADERS: Record<string, Target> = {
	itemid: 'key',
	board: 'project',
	group: 'labels',
	groupandtags: 'labels',
	name: 'title',
	parentitem: 'parent',
	updates: 'comments',
	owner: 'assignees',
	person: 'assignees',
	people: 'assignees',
	assignee: 'assignees',
	status: 'status',
	date: 'due',
	duedate: 'due',
	deadline: 'due',
	timelinestart: 'start',
	timelineend: 'due',
	files: 'attachments',
	priority: 'priority',
	notes: 'notes',
	lastupdated: 'skip',
	creationlog: 'skip'
};

// Board columns that mean one thing under different names (a subitem's
// Date and its item's Due date, one board's Person and another's Owner)
// share a column of the table.
const ROLES: Record<string, string> = {
	owner: 'people',
	person: 'people',
	people: 'people',
	assignee: 'people',
	date: 'due',
	duedate: 'due',
	deadline: 'due',
	status: 'status',
	priority: 'priority',
	notes: 'notes',
	files: 'files',
	timelinestart: 'start'
};

/** A board's file: its name and its sheets. */
export interface Workbook {
	name: string;
	sheets: Sheet[];
}

const filled = (row: string[] | undefined) => (row ?? []).filter((c) => c.trim()).length;
const firstFilled = (row: string[] | undefined) => (row ?? []).findIndex((c) => c.trim());

/** Where a row of column headings starts with Name (or, for subitems, Subitems then Name). */
function headingAt(row: string[] | undefined): number {
	const k = firstFilled(row);
	if (k < 0 || !row || filled(row) < 2) return -1;
	if (squash(row[k]) === 'name') return k;
	if (squash(row[k]) === 'subitems' && squash(row[k + 1] ?? '') === 'name') return k + 1;
	return -1;
}

// An updates sheet's columns, by the names they may have.
const UPDATE_COLUMNS = {
	id: ['itemid', 'pulseid'],
	item: ['itemname', 'item', 'name', 'pulsename'],
	body: ['updatecontent', 'update', 'updatetext', 'content', 'text', 'body'],
	author: ['user', 'createdby', 'author', 'creator', 'postedby', 'by'],
	at: ['createdat', 'created', 'date', 'postedat', 'time']
};

/**
 * An updates sheet's heading row and where each of its columns is; null for
 * another sheet. monday.com writes a title row first (the board's name and
 * "Updates"), so the headings are the first of the top rows that name an
 * update's text and its item.
 */
function updatesLayout(s: Sheet) {
	for (let head = 0; head < Math.min(s.rows.length, 5); head++) {
		if (filled(s.rows[head]) < 2) continue;
		const keys = s.rows[head].map(squash);
		const find = (names: string[]) => {
			for (const n of names) {
				const i = keys.indexOf(n);
				if (i >= 0) return i;
			}
			return -1;
		};
		const cols = {
			id: find(UPDATE_COLUMNS.id),
			item: find(UPDATE_COLUMNS.item),
			body: find(UPDATE_COLUMNS.body),
			author: find(UPDATE_COLUMNS.author),
			at: find(UPDATE_COLUMNS.at)
		};
		if (cols.body >= 0 && (cols.id >= 0 || cols.item >= 0)) return { head, ...cols };
	}
	return null;
}

const MONTHS = ['jan', 'feb', 'mar', 'apr', 'may', 'jun', 'jul', 'aug', 'sep', 'oct', 'nov', 'dec'];

/**
 * When an update was written. monday.com writes "10/November/2026  03:38:40
 * PM", in UTC; anything else is read as a date the browser knows, a
 * trailing "UTC" included. Null when it is neither.
 */
export function updateTime(s: string): Date | null {
	const m = /^(\d{1,2})\/([a-z]+)\/(\d{4})\s+(\d{1,2}):(\d{2})(?::(\d{2}))?\s*([ap]m)?$/i.exec(
		s.trim()
	);
	if (m) {
		const [, day, name, year, h, min, sec, half] = m;
		const month = MONTHS.indexOf(name.slice(0, 3).toLowerCase());
		let hour = Number(h);
		if (half) {
			if (hour < 1 || hour > 12) return null;
			hour = (hour % 12) + (half.toLowerCase() === 'pm' ? 12 : 0);
		}
		const at = new Date(
			Date.UTC(Number(year), month, Number(day), hour, Number(min), Number(sec ?? 0))
		);
		const fits =
			month >= 0 &&
			hour < 24 &&
			Number(min) < 60 &&
			Number(sec ?? 0) < 60 &&
			at.getUTCDate() === Number(day);
		return fits ? at : null;
	}
	const when = Date.parse(s.replace(/ UTC$/, 'Z'));
	return Number.isNaN(when) ? null : new Date(when);
}

/**
 * An update's text, where monday.com wrote it as HTML, as plain text. A
 * tag's run stops at the next < as well as its >, and spaces are matched a
 * whole run at a time, so a long run of either with nothing to end it is
 * read once rather than from each of its characters in turn.
 */
export function plainText(s: string): string {
	if (!/<\/?[a-z][^<>]*>/i.test(s)) return s.trim();
	return s
		.replace(/<br\s*\/?>/gi, '\n')
		.replace(/<\/(p|div|li|h[1-6])>/gi, '\n')
		.replace(/<li[^<>]*>/gi, '- ')
		.replace(/<[^<>]*>/g, '')
		.replace(/&nbsp;/g, ' ')
		.replace(/&lt;/g, '<')
		.replace(/&gt;/g, '>')
		.replace(/&quot;/g, '"')
		.replace(/&#39;/g, "'")
		.replace(/&amp;/g, '&')
		.replace(/[ \t]+(\n?)/g, (run, end: string) => end || run)
		.replace(/\n{3,}/g, '\n\n')
		.trim();
}

/** A board's name from its file's, where the sheet gives none: monday.com adds a number. */
const nameFromFile = (name: string) =>
	name
		.replace(/\.xlsx$/i, '')
		.replace(/_\d{6,}$/, '')
		.replace(/_/g, ' ')
		.trim() || 'monday.com';

/**
 * Boards' files as one table: a row an item or subitem, in the project its
 * board is, labeled with its group (and any tags), a subitem's parent its
 * item, its updates gathered as JSON, then the board's own columns. Null
 * when a file holds no board.
 */
export function readMonday(books: Workbook[]): Reshaped | null {
	if (!books.length) return null;
	const several = books.length > 1;
	/** The board's columns, by squashed name, and where each is in the table. */
	const columns = new Map<string, number>();
	const headings: string[] = [...OWN];
	let tags = false;
	const out: string[][] = [];
	const where = ['', 'Headings'];
	const left: { reason: string; at: string }[] = [];
	const comments = new Map<number, ReadComment[]>();
	const said: string[] = [];
	let noUpdates = 0;
	let missingSubitems = 0;

	const columnFor = (heading: string): number => {
		const name = squash(heading);
		const key = Object.hasOwn(ROLES, name) ? `role:${ROLES[name]}` : name;
		const known = columns.get(key);
		if (known !== undefined) return known;
		const own = OWN.some((h) => squash(h) === name);
		headings.push(own ? `${heading} (column)` : heading);
		columns.set(key, headings.length - 1);
		return headings.length - 1;
	};

	for (const [bi, book] of books.entries()) {
		// The sheet named Updates, or else the first that reads as one.
		const titled = book.sheets.findIndex((s) => squash(s.name) === 'updates' && updatesLayout(s));
		const updatesAt = titled >= 0 ? titled : book.sheets.findIndex((s) => updatesLayout(s));
		const sheet = book.sheets.find(
			(s, i) => i !== updatesAt && s.rows.some((r) => headingAt(r) >= 0)
		);
		if (!sheet) return null;
		const rows = sheet.rows;
		const at = (sheetName: string | null, r: number) =>
			[several ? book.name : null, sheetName, `row ${r + 1}`]
				.filter(Boolean)
				.join(', ')
				.replace(/^row/, 'Row');

		// The board's name is its first row, before any group's headings.
		const firstHeading = rows.findIndex((r) => headingAt(r) >= 0);
		const title = rows.slice(0, firstHeading).findIndex((r) => filled(r));
		const board =
			title >= 0 ? rows[title][firstFilled(rows[title])].trim() : nameFromFile(book.name);

		/** A heading row's columns: where its names are, and each other column's place in the table. */
		type Layout = { name: number; id: number; subitems: number; cells: [number, number][] };
		const layout = (row: string[], name: number): Layout => {
			const l: Layout = { name, id: -1, subitems: -1, cells: [] };
			row.forEach((h, i) => {
				const key = squash(h);
				if (i <= name || !key) return;
				if (key.startsWith('itemid')) l.id = i;
				else if (key === 'subitems') l.subitems = i;
				else if (key === 'tags') {
					tags = true;
					l.cells.push([i, -1]);
				} else l.cells.push([i, columnFor(h)]);
			});
			return l;
		};

		let group = '';
		let items: Layout | null = null;
		let subitems: Layout | null = null;
		/** The last item read, its line and its key, for its subitems. */
		let item: { key: string; group: string; subitems: string; had: boolean } | null = null;
		const byName = new Map<string, number[]>();
		const byId = new Map<string, number>();

		const closeItem = () => {
			if (item && item.subitems && !item.had) missingSubitems++;
		};

		const add = (row: string[], r: number, l: Layout, parent: string, labels: string): string => {
			const line = out.length + 2;
			const id = (l.id >= 0 ? row[l.id]?.trim() : '') || `${bi + 1}-${r + 1}`;
			const cells: string[] = [];
			cells[0] = id;
			cells[1] = board;
			cells[3] = row[l.name].trim();
			cells[4] = parent;
			const tagged: string[] = [];
			for (const [i, col] of l.cells) {
				const v = (row[i] ?? '').trim();
				if (col < 0) {
					if (v) tagged.push(v);
				} else if (!cells[col]) cells[col] = v;
			}
			cells[2] = [labels, ...tagged].filter(Boolean).join(', ');
			// An item with a timeline and no date of its own is due when the timeline ends.
			const due = columns.get('role:due');
			const end = columns.get('timelineend');
			if (due !== undefined && end !== undefined && !cells[due] && cells[end])
				cells[due] = cells[end];
			out.push(cells);
			where.push(at(null, r));
			const name = cells[3].toLowerCase();
			byName.set(name, [...(byName.get(name) ?? []), line]);
			byId.set(id, line);
			return id;
		};

		for (let r = title + 1; r < rows.length; r++) {
			const row = rows[r] ?? [];
			if (!filled(row)) {
				subitems = null;
				continue;
			}
			const h = headingAt(row);
			if (h >= 0) {
				if (squash(row[h - 1] ?? '') === 'subitems' && items) subitems = layout(row, h);
				else {
					closeItem();
					item = null;
					items = layout(row, h);
					subitems = null;
				}
				continue;
			}
			// A group's name: a row of one cell, the group's headings next.
			const k = firstFilled(row);
			if (filled(row) === 1 && k === 0) {
				let next = r + 1;
				while (next < rows.length && !filled(rows[next])) next++;
				const nh = headingAt(rows[next]);
				if (nh >= 0 && squash(rows[next][nh - 1] ?? '') !== 'subitems') {
					closeItem();
					item = null;
					group = row[0]
						.trim()
						.split(',')
						.map((part) => part.trim())
						.join(' ');
					items = null;
					subitems = null;
					continue;
				}
			}
			if (subitems && item && k === subitems.name) {
				item.had = true;
				add(row, r, subitems, item.key, item.group);
				continue;
			}
			// A row with no name is the group's totals.
			if (!items || !(row[items.name] ?? '').trim()) continue;
			closeItem();
			subitems = null;
			const key = add(row, r, items, '', group);
			item = {
				key,
				group,
				subitems: items.subitems >= 0 ? (row[items.subitems] ?? '').trim() : '',
				had: false
			};
		}
		closeItem();

		// Updates, each on the item it names.
		const updates = updatesAt >= 0 ? book.sheets[updatesAt] : null;
		const u = updates ? updatesLayout(updates) : null;
		if (!updates || !u) {
			noUpdates++;
			continue;
		}
		for (let r = u.head + 1; r < updates.rows.length; r++) {
			const row = updates.rows[r] ?? [];
			const cell = (i: number) => (i >= 0 ? (row[i] ?? '').trim() : '');
			const body = plainText(cell(u.body));
			if (!body) continue;
			const place = at(updates.name, r);
			let line = byId.get(cell(u.id));
			if (line === undefined) {
				const named = byName.get(cell(u.item).toLowerCase()) ?? [];
				if (named.length > 1) {
					left.push({
						at: place,
						reason: `It is an update on "${cell(u.item)}", a name more than one item has, so it is left out.`
					});
					continue;
				}
				line = named[0];
			}
			if (line === undefined) {
				left.push({ at: place, reason: 'It is an update on an item that is not in the file.' });
				continue;
			}
			const list = comments.get(line) ?? [];
			list.push({
				author: cell(u.author) || null,
				at: cell(u.at) ? updateTime(cell(u.at)) : null,
				body
			});
			comments.set(line, list);
		}
	}

	if (tags) headings[2] = 'Group and tags';
	const width = headings.length;
	const table = out.map((cells) => Array.from({ length: width }, (_, i) => cells[i] ?? ''));
	for (const [line, list] of comments)
		table[line - 2][5] = JSON.stringify(
			list.map((c) => ({ author: c.author, at: c.at?.toISOString() ?? null, body: c.body }))
		);
	if (noUpdates)
		said.push(
			`${!several ? 'The file has' : noUpdates === books.length ? 'The files have' : `${noUpdates} of the files ${noUpdates === 1 ? 'has' : 'have'}`} no updates. If the board has some, export it again with updates ticked to bring them as comments`
		);
	if (missingSubitems)
		said.push(
			`${missingSubitems} ${missingSubitems === 1 ? 'item has subitems' : 'items have subitems'} that are not in the file. Export the board again with subitems ticked to bring them as subtasks`
		);
	const rows = [headings, ...table];
	return {
		rows,
		where: [...where, ...left.map((l) => l.at)],
		left: left.map((l, i) => ({ line: rows.length + 1 + i, reason: l.reason })),
		said
	};
}

/** Boards' Excel files read and made into one table; null when one holds no board. */
export async function readMondayFiles(files: File[]): Promise<Reshaped | null> {
	const books: Workbook[] = [];
	for (const f of files)
		books.push({ name: f.name, sheets: await readXlsx(await f.arrayBuffer()) });
	return readMonday(books);
}

/** A board's file, made into a table by `readMondayFiles`. */
export const MONDAY: Dialect = {
	...ANY_CSV,
	app: 'monday.com',
	headers: HEADERS,
	priority: (s) => (squash(s) === 'critical' ? 'urgent' : ANY_CSV.priority(s)),
	priorities: '(low, medium, high or critical)',
	comments: (s, problems) => jsonComments(s, problems),
	listStrangers: true,
	behind: "Updates on subitems are not in monday.com's file, so they stay in monday.com",
	readFiles: readMondayFiles
};
