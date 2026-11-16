// Todoist import: a project's CSV export (Todoist's template file) made into
// a table for the CSV import's column screen and dry run (csvimport.ts), and
// how its values are written (a Dialect). Todoist's file is not a table of a
// row a task: each row has a TYPE; a section heads the tasks below it; a
// task's INDENT makes it a subtask of the task above with less; labels are
// written in the task's content (@label); and a note row is a comment on the
// task above it, its files as JSON. `readTodoist` makes one or more such
// files, a project each, into one table. The page is
// routes/(app)/settings/w/[ws]/data/apps.
import {
	ANY_CSV,
	readDate,
	type ChosenFile,
	type Dialect,
	type DueWords,
	type ReadComment,
	type Reshaped,
	type Target
} from './csvimport';
import { jsonComments } from './jsoncomments';
import { httpsUrl, linkText } from './importplan';
import { parseQuickAdd } from './quickadd';

/** The table's headings, and what each feeds. */
const COLUMNS: [string, Target][] = [
	['ID', 'key'],
	['Project', 'project'],
	['Section', 'status'],
	['Task', 'title'],
	['Labels', 'labels'],
	['Description', 'notes'],
	['Priority', 'priority'],
	['Parent', 'parent'],
	['Responsible', 'assignees'],
	['Date', 'due'],
	['Duration', 'estimate'],
	['Comments', 'comments']
];
const HEADERS: Record<string, Target> = Object.fromEntries(
	COLUMNS.map(([h, t]) => [h.toLowerCase(), t])
);

const squash = (h: string) => h.toLowerCase().replace(/[^a-z0-9]/g, '');

/** Whether a file's headings are Todoist's: a row's type, content and indent. */
export const isTodoist = (headers: string[]) => {
	const keys = new Set(headers.map(squash));
	return keys.has('type') && keys.has('content') && keys.has('indent');
};

/**
 * A person as Todoist writes one, "Dana (61395206)", by name. The number is
 * matched from its ( on, since a pattern starting at the spaces would try a
 * long run of them from each in turn; the trim takes the spaces before it.
 */
const personName = (s: string) => s.replace(/\(\d+\)\s*$/, '').trim();

// A label in a task's content: @ and a name, after a space or at the start
// (so an address, dana@example.com, is no label).
const LABEL = /(^|\s)@([^\s@]+)/g;

/**
 * Todoist's files as one table: a row a task, in the project its file is
 * named for, under the section above it, its parent the task above it with
 * less indent, its labels taken out of its content, its comments gathered
 * as JSON. Null when a file's headings are not Todoist's.
 */
export function readTodoist(files: ChosenFile[]): Reshaped | null {
	if (!files.length || !files.every((f) => f.rows.length && isTodoist(f.rows[0]))) return null;
	const rows: string[][] = [COLUMNS.map(([h]) => h)];
	const where = ['', 'Headings'];
	const left: { reason: string; at: string }[] = [];
	const comments = new Map<number, ReadComment[]>();
	const several = files.length > 1;
	files.forEach((f, fi) => {
		const index = new Map(f.rows[0].map((h, i) => [squash(h), i]));
		const project = f.name.replace(/\.csv$/i, '').trim() || 'Todoist';
		const at = (r: number) => (several ? `${f.name}, row ${r + 1}` : `Row ${r + 1}`);
		let section = '';
		/** The tasks above, by indent, that a deeper one is a subtask of. */
		let stack: { indent: number; id: string }[] = [];
		let last: number | null = null;
		f.rows.slice(1).forEach((row, n) => {
			const r = n + 1;
			const cell = (name: string) => (row[index.get(name) ?? -1] ?? '').trim();
			const type = cell('type').toLowerCase();
			const content = cell('content');
			if (!type && !row.some((c) => c.trim())) return;
			if (type === 'meta') return;
			if (type === 'section') {
				section = content;
				stack = [];
				last = null;
				return;
			}
			if (type === 'note') {
				if (last === null) {
					left.push({ at: at(r), reason: 'It is a comment on the project, not on a task.' });
					return;
				}
				const list = comments.get(last) ?? [];
				const when = Date.parse(cell('date'));
				list.push({
					author: personName(cell('author')) || null,
					at: Number.isNaN(when) ? null : new Date(when),
					body: content
				});
				comments.set(last, list);
				return;
			}
			if (type !== 'task') {
				left.push({ at: at(r), reason: `It is a row of type ${type}, not a task.` });
				return;
			}
			const indent = Math.max(1, Number.parseInt(cell('indent'), 10) || 1);
			while (stack.length && stack.at(-1)!.indent >= indent) stack.pop();
			const parent = indent > 1 ? (stack.at(-1)?.id ?? '') : '';
			const id = `${fi + 1}-${r + 1}`;
			stack.push({ indent, id });
			const labels: string[] = [];
			const title = content
				.replace(LABEL, (_, before: string, name: string) => {
					labels.push(name);
					return before;
				})
				.replace(/\s+/g, ' ')
				.trim();
			const date = cell('date');
			const deadline = cell('deadline');
			const description = cell('description');
			const minutes = Number(cell('duration'));
			const unit = cell('durationunit').toLowerCase();
			rows.push([
				id,
				project,
				section,
				title,
				labels.join(', '),
				// Mila has one due date: Todoist's date, or its deadline where it has none.
				date && deadline
					? [`Deadline in Todoist: ${deadline}`, description].filter(Boolean).join('\n\n')
					: description,
				cell('priority'),
				parent,
				personName(cell('responsible')),
				date || deadline,
				minutes > 0 && (unit === 'minute' || unit === '') ? `${minutes}m` : '',
				''
			]);
			where.push(at(r));
			last = rows.length - 1;
		});
	});
	for (const [i, list] of comments)
		rows[i][COLUMNS.length - 1] = JSON.stringify(
			list.map((c) => ({ author: c.author, at: c.at?.toISOString() ?? null, body: c.body }))
		);
	return {
		rows,
		where: [...where, ...left.map((l) => l.at)],
		left: left.map((l, i) => ({ line: rows.length + 1 + i, reason: l.reason }))
	};
}

/** A note's text, with each file Todoist writes in it ([[file {...}]]) as a link. */
export function noteBody(content: string, problems: string[]): string {
	const parts: string[] = [];
	let rest = content;
	for (;;) {
		const start = rest.indexOf('[[file ');
		if (start < 0) break;
		const open = start + '[[file '.length;
		const end = jsonEnd(rest, open);
		if (end < 0 || !rest.startsWith(']]', end)) break;
		parts.push(rest.slice(0, start));
		let file: unknown = null;
		try {
			file = JSON.parse(rest.slice(open, end));
		} catch {
			// An unreadable file is said below.
		}
		const f = (file ?? {}) as Record<string, unknown>;
		const url = httpsUrl(f.file_url);
		const name = typeof f.file_name === 'string' && f.file_name.trim() ? f.file_name : 'File';
		if (url) parts.push(`[${linkText(name)}](${url})`);
		else problems.push('A file in a comment has no web address, so it is left out.');
		rest = rest.slice(end + 2);
	}
	parts.push(rest);
	return parts
		.map((p) => p.trim())
		.filter(Boolean)
		.join('\n\n');
}

/** Where the JSON object starting at `from` ends, or -1. */
function jsonEnd(s: string, from: number): number {
	if (s[from] !== '{') return -1;
	let depth = 0;
	let quoted = false;
	for (let i = from; i < s.length; i++) {
		const c = s[i];
		if (quoted) {
			if (c === '\\') i++;
			else if (c === '"') quoted = false;
		} else if (c === '"') quoted = true;
		else if (c === '{') depth++;
		else if (c === '}' && --depth === 0) return i + 1;
	}
	return -1;
}

const MONTH = /\b(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\b/;

/**
 * A due date as Todoist writes it: a date (2026-11-14, Nov 14 2026), words
 * (tomorrow at 9am, in 6 days, next week) or a repeat (every monday, every!
 * 3 days for three days after each is done), read the way quick add reads
 * them. Null for anything not read whole.
 */
export function readTodoistDate(s: string, today: Date): DueWords | null {
	let text = s.trim().toLowerCase().replace(/\s+/g, ' ');
	if (/^\d{4}-\d{2}-\d{2}$/.test(text)) {
		const due = readDate(text);
		return due ? { due, time: null, repeat: null, relative: false } : null;
	}
	let mode: 'schedule' | 'completion' = 'schedule';
	if (text.startsWith('every!')) {
		mode = 'completion';
		text = `every${text.slice('every!'.length)}`;
	}
	// "every day starting nov 20": the date is the first one.
	text = text.replace(/\b(starting|from)\b/g, ' ');
	let year: string | null = null;
	text = text.replace(/,?\s*\b(\d{4})\b/, (_, y: string) => {
		year = y;
		return ' ';
	});
	const p = parseQuickAdd(text, { today, projects: [], labels: [], members: [] });
	const kinds = new Set<string>(['date', 'time', 'repeat']);
	if (p.title.trim() || !p.dueDate || p.tokens.some((t) => !kinds.has(t.kind))) return null;
	if (mode === 'completion' && !p.recurrence) return null;
	const due = year === null ? p.dueDate : readDate(`${year}${p.dueDate.slice(4)}`);
	if (!due) return null;
	return {
		due,
		time: p.dueTime ?? null,
		repeat: p.recurrence ? { rule: p.recurrence, mode } : null,
		relative: !p.recurrence && year === null && !MONTH.test(text) && !/\d+[/-]\d+/.test(text)
	};
}

const PRIORITIES: Record<string, 'urgent' | 'high' | 'medium' | 'none'> = {
	'1': 'urgent',
	'2': 'high',
	'3': 'medium',
	'4': 'none'
};

/** Todoist's project file, made into a table by `readTodoist`. */
export const TODOIST: Dialect = {
	...ANY_CSV,
	app: 'Todoist',
	headers: HEADERS,
	// Todoist's file writes p1, its highest, as 1, and p4, a task with no
	// priority, as 4.
	priority: (s) => {
		const key = s.toLowerCase().replace(/^p(?=[1-4]$)/, '');
		return Object.hasOwn(PRIORITIES, key) ? PRIORITIES[key] : ANY_CSV.priority(s);
	},
	priorities: "(1 to 4, Todoist's p1 to p4)",
	dateHint: 'a date Todoist writes, like tomorrow, in 6 days or every monday',
	dueWords: readTodoistDate,
	names: (s) => s.split(',').map(personName).filter(Boolean),
	comments: (s, problems) => jsonComments(s, problems, noteBody),
	listStrangers: true,
	behind: "Completed tasks and reminders are not in Todoist's file, so they stay in Todoist",
	reshape: readTodoist
};
