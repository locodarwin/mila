// CSV import: which column means what, how a file writes its values (a
// Dialect: any CSV file's, or an app's, such as ClickUp's in
// clickupimport.ts, Asana's in asanaimport.ts or Todoist's in
// todoistimport.ts), and the reader that turns rows into the dry run's
// planned tasks, through the checks every import shares (importplan.ts).
// The column screen is components/data/CsvImport.svelte, on the pages
// routes/(app)/settings/w/[ws]/data/csv and, for an app's CSV file, data/apps.
import { formatDate } from './dates';
import { MAX_ESTIMATE, parseDuration } from './duration';
import type { RepeatMode } from './recurrence';
import type { CustomField, FieldValue, Priority, StatusCategory } from './sync/entities';
import {
	CHECKLIST_TEXT,
	LIMITS,
	MAX_ALL_COMMENTS,
	MAX_CHECKLIST,
	MAX_NEW_LABELS,
	MAX_ROWS,
	blankTask,
	cleanText,
	clip,
	httpsUrl,
	linkParents,
	linkText,
	oneLine,
	planner,
	refKey,
	same,
	stripMentions,
	type Context,
	type Destination,
	type Plan,
	type PlannedTask,
	type Ref
} from './importplan';

export {
	MAX_ROWS,
	categoryOf,
	runImport,
	type Context,
	type Destination,
	type Plan,
	type PlannedTask,
	type Ref
} from './importplan';

/** What a column feeds. A custom field is "field:" and its ID. */
export type Target =
	| 'skip'
	| 'title'
	| 'notes'
	| 'project'
	| 'status'
	| 'priority'
	| 'start'
	| 'due'
	| 'dueTime'
	| 'completed'
	| 'labels'
	| 'assignees'
	| 'estimate'
	| 'checklist'
	| 'comments'
	| 'attachments'
	| 'link'
	| 'made'
	| 'type'
	| 'key'
	| 'parent'
	| 'space'
	| 'blockedBy'
	| 'blocking'
	| `field:${string}`;

export const TARGETS: { value: Target; label: string }[] = [
	{ value: 'skip', label: 'Leave out' },
	{ value: 'title', label: 'Title' },
	{ value: 'notes', label: 'Notes' },
	{ value: 'project', label: 'Project' },
	{ value: 'status', label: 'Status' },
	{ value: 'priority', label: 'Priority' },
	{ value: 'start', label: 'Start date' },
	{ value: 'due', label: 'Due date' },
	{ value: 'dueTime', label: 'Due time' },
	{ value: 'completed', label: 'Completed' },
	{ value: 'labels', label: 'Labels' },
	{ value: 'assignees', label: 'Assignees' },
	{ value: 'estimate', label: 'Estimate' },
	{ value: 'checklist', label: 'Checklist' },
	{ value: 'comments', label: 'Comments' },
	{ value: 'attachments', label: 'Files, as links' },
	{ value: 'link', label: 'Link back' },
	{ value: 'made', label: 'Made on' },
	{ value: 'type', label: 'Type' },
	{ value: 'key', label: 'ID, for subtasks' },
	{ value: 'parent', label: "Parent's ID" },
	{ value: 'space', label: "Project's group" },
	{ value: 'blockedBy', label: 'Blocked by' },
	{ value: 'blocking', label: 'Blocking' }
];

const squash = (s: string) => s.toLowerCase().replace(/[^a-z0-9]/g, '');

const HEADERS: Record<string, Target> = {
	title: 'title',
	name: 'title',
	task: 'title',
	taskname: 'title',
	summary: 'title',
	content: 'title',
	notes: 'notes',
	note: 'notes',
	description: 'notes',
	details: 'notes',
	body: 'notes',
	project: 'project',
	list: 'project',
	status: 'status',
	state: 'status',
	stage: 'status',
	priority: 'priority',
	start: 'start',
	startdate: 'start',
	due: 'due',
	duedate: 'due',
	deadline: 'due',
	duetime: 'dueTime',
	time: 'dueTime',
	completed: 'completed',
	completedat: 'completed',
	done: 'completed',
	labels: 'labels',
	label: 'labels',
	tags: 'labels',
	tag: 'labels',
	assignee: 'assignees',
	assignees: 'assignees',
	assignedto: 'assignees',
	owner: 'assignees',
	blockedby: 'blockedBy',
	waitsfor: 'blockedBy',
	blocking: 'blocking',
	blocks: 'blocking'
};

/** A comment as a file has it: who wrote it and when, where the file says. */
export interface ReadComment {
	author: string | null;
	at: Date | null;
	body: string;
}

/** A due date written in words, as Todoist writes one: the day, any time, any repeat. */
export interface DueWords {
	due: string;
	time: string | null;
	repeat: { rule: string; mode: RepeatMode } | null;
	/** Whether it counts from the day the file was written ("in 6 days"), and so is read from today. */
	relative: boolean;
}

/** A file as chosen: its name and its rows, the first being its headings. */
export interface ChosenFile {
	name: string;
	rows: string[][];
}

/**
 * Files that are not one table of a row a task, made into one (Todoist's).
 * `where` names each row's place in the files, by its line in `rows` (the
 * headings being line 1), for the dry run; `left` is what was left out
 * before the table, with lines after its last.
 */
export interface Reshaped {
	rows: string[][];
	where: string[];
	left: { line: number; reason: string }[];
	/** More for the dry run's summary, about the files as a whole. */
	said?: string[];
}

/**
 * How a file writes its values. Readers get a cell's text, trimmed and
 * never blank; the ones of lists push what they could not read into
 * `problems`, which the dry run lists, rather than failing the row.
 */
export interface Dialect {
	/** The app whose file it is, for the dry run's words; null for any CSV file. */
	app: string | null;
	/** Its headings, squashed, and what they feed: tried before the usual names. */
	headers: Record<string, Target>;
	/** What else the file writes for an empty cell. */
	blank: (s: string) => boolean;
	notes: (s: string) => string;
	priority: (s: string) => Priority | undefined;
	/** The priorities it takes, for a warning: "(none, low, medium, high, urgent)". */
	priorities: string;
	/** A day as YYYY-MM-DD, or null when it cannot be read. */
	date: (s: string) => string | null;
	/** How a date should look, for a warning: "a date like 2026-11-03". */
	dateHint: string;
	/** A time of day as HH:MM; null for a date with no time; undefined when it cannot be read. */
	time: (s: string) => string | null | undefined;
	/** A moment as an ISO time, or null when it cannot be read. */
	instant: (s: string) => string | null;
	/** People or labels in one cell. */
	names: (s: string) => string[];
	comments: (s: string, problems: string[]) => ReadComment[];
	files: (s: string, problems: string[]) => { name: string; url: string }[];
	checklist: (s: string, problems: string[]) => { text: string; done: boolean }[];
	/** Minutes; null for none; undefined when it cannot be read. */
	estimate: (s: string) => number | null | undefined;
	/** The Type that is a task. */
	taskType: string;
	/** What a row of any other type does: left out, a task with a label named after its type, or a task. */
	otherTypes: 'skip' | 'label' | 'task';
	/** People named who are not here are listed once with their rows, not warned of row by row. */
	listStrangers: boolean;
	/** What the link back at the top of the notes says. */
	linkName: string;
	/** What stays behind, for the dry run's summary; null for nothing to say. */
	behind: string | null;
	/**
	 * The projects one cell names, where a task can be in several (Asana's):
	 * each row goes in the one the most rows name. One project by default.
	 */
	projects?: (s: string) => string[];
	/** A link back made from a task's ID, for a file with no link column of its own. */
	linkOf?: (key: string) => string | null;
	/**
	 * Whether the columns left out still come in, as a table of their names
	 * and values at the top of the notes (Notion's properties with nowhere
	 * else to go).
	 */
	restInNotes?: boolean;
	/**
	 * A due date in words, with any time and repeat, read in place of
	 * `date` for the due column; null when it cannot be read.
	 */
	dueWords?: (s: string, today: Date) => DueWords | null;
	/** The app's files made into one table; null when they are not its files. */
	reshape?: (files: ChosenFile[]) => Reshaped | null;
	/**
	 * For an app whose file is not CSV (monday.com's, Excel): its files read
	 * and made into one table, in place of `reshape`; null when they are not
	 * its files. It throws, with words for the person, when one cannot be read.
	 */
	readFiles?: (files: File[]) => Promise<Reshaped | null>;
}

/**
 * A first guess at what each column means, from its heading: the file's
 * own app's names, the usual names, Mila's own CSV export, and the names
 * of custom fields. A target other than a field is given to one column only.
 */
export function guessTargets(
	headers: string[],
	fields: CustomField[],
	dialect: Dialect = ANY_CSV
): Target[] {
	const used = new Set<Target>();
	return headers.map((h) => {
		const key = squash(h);
		const field = fields.find((f) => squash(f.name) === key);
		let t: Target = Object.hasOwn(dialect.headers, key)
			? dialect.headers[key]
			: field
				? `field:${field.id}`
				: (Object.hasOwn(HEADERS, key) && HEADERS[key]) || 'skip';
		if (t !== 'skip' && used.has(t)) t = 'skip';
		used.add(t);
		return t;
	});
}

const PRIORITIES: Record<string, Priority> = {
	none: 'none',
	normal: 'none',
	low: 'low',
	medium: 'medium',
	med: 'medium',
	high: 'high',
	urgent: 'urgent',
	p1: 'urgent',
	p2: 'high',
	p3: 'medium',
	p4: 'none'
};

const TRUE = new Set(['yes', 'y', 'true', 'x', '1', 'done', 'completed', 'complete', '✓']);
const FALSE = new Set(['', 'no', 'n', 'false', '0', 'open', 'todo']);

/** A date as YYYY-MM-DD, from 2026-11-03, 2026/11/3 or an ISO date and time. */
export function readDate(s: string): string | null {
	const m = s.trim().match(/^(\d{4})[-/](\d{1,2})[-/](\d{1,2})(?:[T ].*)?$/);
	if (!m) return null;
	const [y, mo, d] = [+m[1], +m[2], +m[3]];
	const date = new Date(Date.UTC(y, mo - 1, d));
	if (date.getUTCMonth() !== mo - 1 || date.getUTCDate() !== d) return null;
	return `${m[1]}-${String(mo).padStart(2, '0')}-${String(d).padStart(2, '0')}`;
}

/** A time of day as HH:MM, from 14:30, 9:05, 2:30 pm, 2pm or 14:30:00. */
export function readTime(s: string): string | null {
	const m = s.trim().match(/^(\d{1,2})(?::(\d{2}))?(?::\d{2})?\s*([ap]\.?m\.?)?$/i);
	if (!m || (m[2] === undefined && !m[3])) return null;
	let h = +m[1];
	const min = +(m[2] ?? 0);
	const half = m[3]?.[0].toLowerCase();
	if (half && (h < 1 || h > 12)) return null;
	if (half === 'p' && h < 12) h += 12;
	if (half === 'a' && h === 12) h = 0;
	if (h > 23 || min > 59) return null;
	return `${String(h).padStart(2, '0')}:${String(min).padStart(2, '0')}`;
}

/** A moment from an ISO time, or noon on a date like 2026-11-03. */
export function readInstant(s: string): string | null {
	if (s.length > 10 && !Number.isNaN(Date.parse(s))) {
		const at = new Date(s);
		return Number.isNaN(at.getTime()) ? null : at.toISOString();
	}
	const date = readDate(s);
	return date ? new Date(`${date}T12:00:00`).toISOString() : null;
}

const split = (s: string) =>
	s
		.split(/[;,]/)
		.map((x) => x.trim())
		.filter(Boolean);

/** Any CSV file: the usual ways a spreadsheet writes things. */
export const ANY_CSV: Dialect = {
	app: null,
	headers: {},
	blank: () => false,
	notes: (s) => s,
	priority: (s) => {
		const key = squash(s);
		return Object.hasOwn(PRIORITIES, key) ? PRIORITIES[key] : undefined;
	},
	priorities: '(none, low, medium, high, urgent)',
	date: readDate,
	dateHint: 'a date like 2026-11-03',
	time: (s) => readTime(s) ?? undefined,
	instant: readInstant,
	names: split,
	// A cell is one comment, by nobody in particular.
	comments: (s) => [{ author: null, at: null, body: s }],
	files: (s, problems) => {
		const out: { name: string; url: string }[] = [];
		for (const part of s.split(/[\s,;]+/).filter(Boolean)) {
			const url = httpsUrl(part);
			if (!url) {
				problems.push('A file in it is not a web address, so it is left out.');
				continue;
			}
			out.push({ name: nameFromUrl(url), url });
		}
		return out;
	},
	checklist: (s) =>
		s
			.split(/\n|;/)
			.map((x) => x.trim())
			.filter(Boolean)
			.map((text) => {
				// The rest is any character, as one that . leaves out (a line
				// separator) would have the spaces before it tried one by one.
				const m = text.match(/^(?:[-*]\s*)?\[([ xX])\]\s*([\s\S]*)$/);
				return m ? { text: m[2], done: m[1] !== ' ' } : { text, done: false };
			}),
	estimate: (s) => parseDuration(s) ?? undefined,
	taskType: 'Task',
	otherTypes: 'label',
	listStrangers: false,
	linkName: 'Where it came from',
	behind: null
};

/** The last part of a file's address, as its name. */
export function nameFromUrl(url: string): string {
	const last = new URL(url).pathname.split('/').filter(Boolean).at(-1) ?? '';
	try {
		return decodeURIComponent(last) || url;
	} catch {
		return last || url;
	}
}

const MAX_FILES = 100;
const MAX_TASK_COMMENTS = 1000;

export interface PlanOptions {
	/** How the file writes its values; any CSV file's by default. */
	dialect?: Dialect;
	/**
	 * Whether rows go in the project their Project column names (the
	 * destination taking the rest), or all of them to the destination.
	 */
	byColumn?: boolean;
	/** How a comment's date is written; the person's date style by default. */
	date?: (at: Date) => string;
	/** The day dates in words count from; now by default. */
	today?: Date;
}

const article = (word: string) => (/^[aeiou]/i.test(word) ? 'an' : 'a');

/**
 * The dry run: what importing the rows (the first one being the headings)
 * would make, with the targets chosen for each column. Nothing changes.
 */
/** A table cell's text: one line, its bars escaped. */
const tableCell = (s: string) =>
	oneLine(cleanText(s, 2000), 500).replace(/\\/g, '\\\\').replace(/\|/g, '\\|');

/** The columns left out of a row, with values, as a Markdown table of name and value. */
function restTable(headers: string[], targets: Target[], row: string[]): string {
	const lines = targets.flatMap((t, i) => {
		const v = (row[i] ?? '').trim();
		const name = (headers[i] ?? '').trim();
		return t === 'skip' && v && name ? [`| ${tableCell(name)} | ${tableCell(v)} |`] : [];
	});
	return lines.length ? ['| Property | Value |', '| --- | --- |', ...lines].join('\n') : '';
}

export function plan(
	rows: string[][],
	targets: Target[],
	destination: Destination,
	ctx: Context,
	opts: PlanOptions = {}
): Plan {
	const d = opts.dialect ?? ANY_CSV;
	const byColumn = opts.byColumn ?? true;
	const write = opts.date ?? ((at: Date) => formatDate(at, { year: true }));
	// Written once a day, not once a comment: formatting is slow.
	const days = new Map<string, string>();
	const date = (at: Date) => {
		const key = `${at.getFullYear()}-${at.getMonth()}-${at.getDate()}`;
		let s = days.get(key);
		if (s === undefined) days.set(key, (s = write(at)));
		return s;
	};
	const p = planner(destination, ctx);
	const out = p.out;
	const col = (t: Target) => targets.indexOf(t);
	/** A cell as written, trimmed; what the file writes for empty is empty. */
	const cell = (row: string[], t: Target) => {
		const i = col(t);
		const v = i < 0 ? '' : (row[i] ?? '').trim();
		return t !== 'title' && t !== 'notes' && v && d.blank(v) ? '' : v;
	};
	/** A name of one line from a cell, cut to `max`. */
	const word = (row: string[], t: Target, max: number) =>
		oneLine(cleanText(cell(row, t), max * 4), max);
	const warn = p.warn;
	const now = new Date().toISOString();
	const today = opts.today ?? new Date();

	const fieldCols = targets
		.map((t, i) => ({ t, i }))
		.filter(({ t }) => t.startsWith('field:'))
		.map(({ t, i }) => ({ i, field: ctx.fields.find((f) => `field:${f.id}` === t) }))
		.filter((x): x is { i: number; field: CustomField } => !!x.field);

	const data = rows.slice(1);
	if (data.length > MAX_ROWS) {
		for (let n = MAX_ROWS; n < data.length; n++)
			p.skip(n + 2, `An import takes at most ${MAX_ROWS} rows.`);
	}

	// A cell naming several projects (a task in more than one of Asana's)
	// puts the row in the one the most rows name: the project exported.
	const several = byColumn && d.projects ? d.projects : null;
	const projectName1 = (x: string) => oneLine(cleanText(x, LIMITS.project * 4), LIMITS.project);
	const rowsNaming = new Map<string, number>();
	if (several)
		for (const row of data.slice(0, MAX_ROWS))
			for (const name of new Set(
				several(cell(row, 'project')).map((x) => projectName1(x).toLowerCase())
			))
				if (name) rowsNaming.set(name, (rowsNaming.get(name) ?? 0) + 1);
	const listOf = (row: string[]) => {
		if (!byColumn) return '';
		if (!several) return word(row, 'project', LIMITS.project);
		let best = '';
		let most = 0;
		for (const name of several(cell(row, 'project')).map(projectName1)) {
			const count = rowsNaming.get(name.toLowerCase()) ?? 0;
			if (name && count > most) [best, most] = [name, count];
		}
		return best;
	};

	// First, which rows come in: each with a title, once (by its ID), and
	// of a type that comes in.
	interface Entry {
		row: string[];
		line: number;
		title: string;
		/** Its ID in the file, or one made for it here (never a file's). */
		key: string | null;
		/** Its ID as the file has it. */
		id: string | null;
		parent: string | null;
		list: string;
		space: string;
		type: string | null;
	}
	const entries: Entry[] = [];
	const seen = new Map<string, number>();
	const leftOut = new Set<string>();
	const otherTypes = new Map<string, { name: string; count: number }>();
	const leftOutTitles = new Set<string>();
	data.slice(0, MAX_ROWS).forEach((row, n) => {
		const line = n + 2;
		const key = word(row, 'key', 100) || null;
		const title = oneLine(cleanText(cell(row, 'title'), LIMITS.title * 4), LIMITS.title);
		if (!title) {
			p.skip(line, 'It has no title.');
			if (key !== null && !seen.has(key)) leftOut.add(key);
			return;
		}
		if (key !== null && seen.has(key)) {
			p.skip(line, `It is the same task as row ${seen.get(key)}.`);
			return;
		}
		const named = word(row, 'type', LIMITS.label);
		const type = named && !same(named, d.taskType) ? named : null;
		if (type && d.otherTypes === 'skip') {
			const app = d.app ? `${d.app} ` : '';
			p.skip(line, `It is ${article(app || type)} ${app}${type}, not a task.`);
			const counted = otherTypes.get(type.toLowerCase());
			if (counted) counted.count++;
			else otherTypes.set(type.toLowerCase(), { name: type, count: 1 });
			if (key !== null) leftOut.add(key);
			leftOutTitles.add(title.toLowerCase());
			return;
		}
		if (key !== null) {
			seen.set(key, line);
			leftOut.delete(key);
		}
		entries.push({
			row,
			line,
			title,
			key,
			id: key,
			parent: oneLine(cleanText(cell(row, 'parent'), LIMITS.title * 4), LIMITS.title) || null,
			list: listOf(row),
			space: byColumn ? word(row, 'space', LIMITS.project) : '',
			type
		});
	});
	out.otherTypes = [...otherTypes.values()];

	// Projects by name; where two groups (ClickUp's spaces) each have one of
	// the same name, the group's name tells them apart.
	const groups = new Map<string, Set<string>>();
	for (const e of entries)
		if (e.list) {
			const g = groups.get(e.list.toLowerCase()) ?? new Set();
			g.add(e.space.toLowerCase());
			groups.set(e.list.toLowerCase(), g);
		}
	const projectName = (e: Entry) =>
		e.list && e.space && (groups.get(e.list.toLowerCase())?.size ?? 0) > 1
			? clip(`${e.list} (${e.space})`, LIMITS.project)
			: e.list;

	// A parent or a dependency named by its title rather than its ID
	// (Asana's Parent task): the one task of that title, where only one has
	// it. Tasks with no ID get one here, which no file's can be (cleanText
	// leaves out control characters).
	const keys = new Set(entries.flatMap((e) => (e.key === null ? [] : [e.key])));
	const byTitle = new Map<string, Entry[]>();
	for (const e of entries) {
		const k = e.title.toLowerCase();
		const list = byTitle.get(k);
		if (list) list.push(e);
		else byTitle.set(k, [e]);
		e.key ??= `\u0001${e.line}`;
	}
	for (const e of entries) {
		if (e.parent === null || keys.has(e.parent)) continue;
		const named = (byTitle.get(e.parent.toLowerCase()) ?? []).filter((x) => x !== e);
		if (named.length === 1) e.parent = named[0].key;
		else if (named.length > 1) {
			warn(e.line, "More than one task has its parent task's name, so it comes in on its own.");
			e.parent = null;
		} else if (leftOutTitles.has(e.parent.toLowerCase())) leftOut.add(e.parent);
	}

	// Subtasks keep their parents where the parent comes in too, and go in
	// the project of the task at the top.
	const parents = linkParents(entries, leftOut);
	const byLine = new Map(entries.map((e) => [e.line, e]));
	const projects = new Map<number, Ref | null>();
	const projectOf = (top: Entry): Ref | null => {
		if (projects.has(top.line)) return projects.get(top.line)!;
		const named = projectName(top);
		const r = named ? p.projectByName(named) : p.fallbackRef();
		projects.set(top.line, r);
		return r;
	};

	let comments = 0;
	const planned = new Map<string, PlannedTask>();
	for (const e of entries) {
		const { row, line } = e;
		const parentage = parents.get(line)!;
		if (parentage.problem) warn(line, parentage.problem);
		const project = projectOf(byLine.get(parentage.top) ?? e);
		const t: PlannedTask = blankTask(line, e.title, project);
		t.key = e.key;
		t.parent = parentage.parent;
		const priority = cell(row, 'priority');
		if (priority) {
			const level = d.priority(priority);
			if (level) t.priority = level;
			else warn(line, `"${priority}" is not a priority ${d.priorities}.`);
		}
		for (const [target, key] of [
			['start', 'start'],
			['due', 'due']
		] as const) {
			const v = cell(row, target);
			if (!v) continue;
			if (target === 'due' && d.dueWords) {
				const words = d.dueWords(v, today);
				if (!words) warn(line, `"${v}" is not ${d.dateHint}.`);
				else {
					[t.due, t.dueTime, t.repeat] = [words.due, words.time, words.repeat];
					if (words.relative) out.fromToday.push(line);
				}
				continue;
			}
			const day = d.date(v);
			if (day) t[key] = day;
			else warn(line, `"${v}" is not ${d.dateHint}.`);
		}
		const time = cell(row, 'dueTime');
		if (time) {
			const v = d.time(time);
			if (v && t.due) t.dueTime = v;
			else if (v) warn(line, 'A due time needs a due date.');
			else if (v === undefined) warn(line, `"${time}" is not a time like 14:30.`);
		}
		const made = cell(row, 'made');
		if (made) {
			const at = d.instant(made);
			if (at) t.madeAt = at;
			else warn(line, `"${made}" is not ${d.dateHint}.`);
		}
		const completed = cell(row, 'completed');
		if (completed) {
			const at = d.instant(completed);
			if (at) t.completedAt = at;
			else if (TRUE.has(completed.toLowerCase())) {
				t.completedAt = now;
				out.completedNow.push(line);
			} else if (!FALSE.has(completed.toLowerCase()))
				warn(line, `"${completed}" does not say whether it is completed.`);
		}
		const status = cell(row, 'status');
		if (status && !project) {
			warn(line, 'Tasks in the inbox have no status; it is left out.');
		} else if (project) {
			const chosen = status ? p.statusRef(project, status) : null;
			const done = chosen?.category === 'done' || chosen?.category === 'canceled';
			if (done && !t.completedAt) {
				t.completedAt = now;
				out.completedNow.push(line);
			}
			if (chosen && (!t.completedAt || done)) t.status = chosen.ref;
			else {
				// As a new task gets: the first open status, or a done one if it is completed.
				const want: StatusCategory = t.completedAt ? 'done' : 'todo';
				const list = p.statusesOf(project);
				const pick = list.find((s) => s.category === want) ?? list[0];
				t.status = pick?.ref ?? null;
			}
		}
		const labelNames = d.names(cell(row, 'labels')).map((n) => oneLine(cleanText(n, 400), 60));
		if (e.type && d.otherTypes === 'label') labelNames.push(e.type);
		for (const name of labelNames) {
			if (!name) continue;
			let r = p.knownLabel(name);
			if (!r && out.newLabels.length >= MAX_NEW_LABELS) {
				warn(line, `The label ${name} is left out: an import makes at most ${MAX_NEW_LABELS}.`);
				continue;
			}
			r ??= p.labelRef(clip(name, 60));
			if (!t.labels.some((l) => refKey(l) === refKey(r))) t.labels.push(r);
		}
		const people = d.names(cell(row, 'assignees')).map((n) => oneLine(cleanText(n, 400), 100));
		for (const who of people.filter(Boolean)) {
			const id = p.member(who);
			if (!id && d.listStrangers) p.stranger(who, line);
			else if (!id) warn(line, `${who} is not a member of this workspace.`);
			else if (!p.reaches(id, t.project)) warn(line, `${who} cannot reach this task's project.`);
			else if (!t.assignees.includes(id)) t.assignees.push(id);
		}
		if (d.listStrangers && people.some(Boolean) && !t.assignees.length) out.unassigned.push(line);

		const problems: string[] = [];
		const estimate = cell(row, 'estimate');
		if (estimate) {
			const minutes = d.estimate(estimate);
			if (minutes === undefined) warn(line, `"${estimate}" is not a length of time like 1h 30m.`);
			else if (minutes !== null && minutes > MAX_ESTIMATE) {
				warn(line, `Its estimate is more than ${MAX_ESTIMATE / 60} hours; it comes in as that.`);
				t.estimate = MAX_ESTIMATE;
			} else if (minutes !== null && minutes > 0) t.estimate = Math.round(minutes);
		}
		const checklist = cell(row, 'checklist');
		if (checklist) {
			const items = d
				.checklist(checklist, problems)
				.map((i) => ({
					text: oneLine(cleanText(i.text, CHECKLIST_TEXT * 4), CHECKLIST_TEXT),
					done: i.done
				}))
				.filter((i) => i.text);
			if (items.length > MAX_CHECKLIST)
				problems.push(
					`It has more checklist items than an import takes; ${MAX_CHECKLIST} come in.`
				);
			t.checklist = items.slice(0, MAX_CHECKLIST);
		}
		const said = cell(row, 'comments');
		if (said) {
			let read = d.comments(said, problems);
			if (read.length > MAX_TASK_COMMENTS) {
				problems.push(`It has more comments than an import takes; ${MAX_TASK_COMMENTS} come in.`);
				read = read.slice(0, MAX_TASK_COMMENTS);
			}
			for (const c of read) {
				const body = cleanText(c.body, LIMITS.comment).trim();
				if (!body) continue;
				if (comments >= MAX_ALL_COMMENTS) {
					warn(line, `Its comments are left out: an import makes at most ${MAX_ALL_COMMENTS}.`);
					break;
				}
				comments++;
				const by = c.author ? authorName(c.author, ctx) : null;
				// The author's name is the file's too, so no mention in it.
				const head =
					by || c.at
						? stripMentions(`Originally by ${by ?? 'someone'}${c.at ? `, ${date(c.at)}` : ''}\n\n`)
						: '';
				t.comments.push(`${head}${body}`.slice(0, LIMITS.comment));
			}
		}
		const files = cell(row, 'attachments');
		const kept = files ? d.files(files, problems).slice(0, MAX_FILES) : [];
		out.links += kept.length;
		const linkCell = cell(row, 'link');
		const fromKey = !linkCell && e.id !== null && d.linkOf ? d.linkOf(e.id) : null;
		const link = linkCell ? httpsUrl(cleanText(linkCell, 4000)) : httpsUrl(fromKey);
		if (linkCell && !link) warn(line, 'Its link back is not a web address, so it is left out.');
		const head = link
			? `[${d.linkName}](${link})${t.madeAt ? `, made ${date(new Date(t.madeAt))}` : ''}`
			: '';
		const tail = kept.length
			? `**Files**\n\n${kept.map((f) => `- [${linkText(f.name)}](${f.url})`).join('\n')}`
			: '';
		const rest = d.restInNotes ? restTable(rows[0], targets, row) : '';
		const room = LIMITS.notes - head.length - rest.length - tail.length - 6;
		const body = cleanText(d.notes(cell(row, 'notes')), Math.max(0, room)).trim();
		t.notes = [head, rest, body, tail].filter(Boolean).join('\n\n').slice(0, LIMITS.notes);
		for (const problem of new Set(problems)) warn(line, problem);

		for (const { i, field } of fieldCols) {
			const raw = cleanText((row[i] ?? '').trim(), 4000);
			if (!raw || d.blank(raw)) continue;
			if (field.projectId && !(project && 'id' in project && project.id === field.projectId)) {
				warn(line, `${field.name} belongs to another project.`);
				continue;
			}
			const value = fieldValue(field, raw, out, p.member);
			if (value === undefined) warn(line, `"${raw}" does not fit ${field.name}.`);
			else t.values.push({ fieldId: field.id, value });
		}
		p.add(t);
		planned.set(e.key!, t);
	}

	// Dependencies between tasks that both come in, named by ID or title,
	// with none that would make a loop, which the server would refuse.
	const waits = new Map<string, Set<string>>();
	/** Whether `from` waits for `to`, directly or down a chain. */
	const reaches = (from: string, to: string) => {
		const seen = new Set([from]);
		const queue = [from];
		while (queue.length) {
			for (const next of waits.get(queue.shift()!) ?? []) {
				if (next === to) return true;
				if (seen.has(next)) continue;
				seen.add(next);
				queue.push(next);
			}
		}
		return false;
	};
	/** A task by its ID or title: null for none, undefined for a title several have. */
	const refOf = (s: string): string | null | undefined => {
		if (keys.has(s)) return s;
		const named = byTitle.get(s.toLowerCase()) ?? [];
		return named.length === 1 ? named[0].key : named.length ? undefined : null;
	};
	for (const e of entries) {
		const problems = new Set<string>();
		const depend = (waiter: string, blocker: string) => {
			if (waiter === blocker) return problems.add('It waits for itself, so that is left out.');
			const list = waits.get(waiter) ?? new Set<string>();
			if (list.has(blocker)) return;
			if (reaches(blocker, waiter))
				return problems.add('A dependency in it would make a loop, so it is left out.');
			list.add(blocker);
			waits.set(waiter, list);
			planned.get(waiter)!.blockedBy.push(blocker);
		};
		for (const [target, missing] of [
			['blockedBy', 'It waits for a task that is not coming in, so that dependency is left out.'],
			['blocking', 'A task waiting for it is not coming in, so that dependency is left out.']
		] as const) {
			const raw = cleanText(cell(e.row, target), 100_000);
			if (!raw) continue;
			const whole = refOf(oneLine(raw, LIMITS.title));
			const named = whole
				? [whole]
				: raw
						.split(/[,;\n]/)
						.map((x) => oneLine(x, LIMITS.title))
						.filter(Boolean)
						.slice(0, 1000)
						.map(refOf);
			for (const other of named) {
				if (other === undefined)
					problems.add('More than one task has the name it gives, so that dependency is left out.');
				else if (other === null) problems.add(missing);
				else if (target === 'blockedBy') depend(e.key!, other);
				else depend(other, e.key!);
			}
		}
		for (const problem of problems) warn(e.line, problem);
	}
	return out;
}

/** Who wrote a comment, in words: a person here by their name, someone else by the first part of their address. */
function authorName(author: string, ctx: Context): string {
	const who = oneLine(cleanText(author, 200), 100);
	const here = ctx.members.find((m) => same(m.email, who) || same(m.name, who));
	if (here) return here.name;
	const at = who.indexOf('@');
	return at > 0 ? who.slice(0, at) : who;
}

/**
 * What the dry run says the import brings besides tasks, in a line each:
 * what the summary adds to the counts the plan shows itself.
 */
export function summaryLines(p: Plan, targets: Target[], d: Dialect = ANY_CSV): string[] {
	const lines: string[] = [];
	const n = (count: number, one: string, many: string) => `${count} ${count === 1 ? one : many}`;
	if (p.otherTypes.length) {
		const total = p.otherTypes.reduce((s, t) => s + t.count, 0);
		const kinds = p.otherTypes.map((t) => `${t.count} of type ${t.name}`).join(', ');
		lines.push(
			`${n(total, 'row is', 'rows are')} not ${total === 1 ? 'a task' : 'tasks'}${d.app ? ` in ${d.app}` : ''}, so ${total === 1 ? 'it is' : 'they are'} left out: ${kinds}`
		);
	}
	const subtasks = p.tasks.filter((t) => t.parent !== null).length;
	if (subtasks) lines.push(`${n(subtasks, 'subtask', 'subtasks')}, each under its parent task`);
	const comments = p.tasks.reduce((s, t) => s + t.comments.length, 0);
	if (comments)
		lines.push(
			`${n(comments, 'comment', 'comments')}${d.app ? ', each starting with who wrote it and when' : ''}`
		);
	const items = p.tasks.reduce((s, t) => s + t.checklist.length, 0);
	if (items) lines.push(n(items, 'checklist item', 'checklist items'));
	const estimates = p.tasks.filter((t) => t.estimate !== null).length;
	if (estimates) lines.push(`Estimates on ${n(estimates, 'task', 'tasks')}`);
	if (p.links)
		lines.push(
			`${n(p.links, 'file', 'files')} kept as links at the end of the notes; the files themselves stay in ${d.app ?? 'the other app'}`
		);
	const repeats = p.tasks.filter((t) => t.repeat !== null).length;
	if (repeats)
		lines.push(
			`${n(repeats, 'task repeats', 'tasks repeat')}, from the next day ${repeats === 1 ? 'it repeats' : 'each repeats'} on`
		);
	if (p.fromToday.length)
		lines.push(
			`${n(p.fromToday.length, 'due date is', 'due dates are')} written from the day the file was made, like "in 6 days", so ${p.fromToday.length === 1 ? 'it counts' : 'they count'} from today`
		);
	const dependencies = p.tasks.reduce((s, t) => s + t.blockedBy.length, 0);
	if (dependencies)
		lines.push(`${n(dependencies, 'dependency', 'dependencies')} between the tasks`);
	if (
		(targets.includes('link') || (d.linkOf && targets.includes('key'))) &&
		p.tasks.some((t) => t.notes.startsWith(`[${d.linkName}](`))
	)
		lines.push(`Each task's notes start with a link back to it${d.app ? ` in ${d.app}` : ''}`);
	if (p.completedNow.length)
		lines.push(
			`${n(p.completedNow.length, 'task is', 'tasks are')} done with no date for it in the file, so ${p.completedNow.length === 1 ? 'it comes' : 'they come'} in done as of today`
		);
	if (d.behind) lines.push(d.behind);
	return lines;
}

function fieldValue(
	field: CustomField,
	raw: string,
	out: Plan,
	member: (s: string) => string | null
): FieldValue | Ref | Ref[] | undefined {
	const option = (name: string): Ref => {
		name = clip(name, 60);
		const found = field.options.find((o) => same(o.name, name));
		if (found) return { id: found.id };
		const made = out.newOptions.find((o) => o.fieldId === field.id && same(o.name, name));
		if (!made) out.newOptions.push({ fieldId: field.id, name });
		return { make: `option:${field.id}|${made?.name ?? name}` };
	};
	switch (field.type) {
		case 'number': {
			const n = Number(raw.replace(/,(?=\d{3}\b)/g, ''));
			return Number.isFinite(n) ? n : undefined;
		}
		case 'date':
			return readDate(raw) ?? undefined;
		case 'checkbox':
			return TRUE.has(raw.toLowerCase()) ? true : FALSE.has(raw.toLowerCase()) ? false : undefined;
		case 'select':
			return option(raw);
		case 'multi_select':
			return split(raw).map(option);
		case 'user':
			return member(raw) ?? undefined;
		default:
			return raw.slice(0, 2000);
	}
}
