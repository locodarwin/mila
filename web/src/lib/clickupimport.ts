// ClickUp import: how ClickUp's CSV export writes its values (a Dialect for
// the CSV import's column screen and dry run, csvimport.ts), and which of
// its columns go where, matched before the person sees them. The file is
// untrusted: JSON in a cell is read for its own keys only, every value's
// type checked, and whatever cannot be read is said in the dry run rather
// than failing the row. The page is routes/(app)/settings/w/[ws]/data/apps.
import {
	nameFromUrl,
	readDate,
	readInstant,
	readTime,
	type Dialect,
	type ReadComment,
	type Target
} from './csvimport';
import { parseDuration } from './duration';
import { cleanText, httpsUrl, isObject, own } from './importplan';
import type { Priority } from './sync/entities';

/**
 * What a row whose Task Type is not "Task" does (ClickUp's Lead and Account,
 * and any custom type): 'skip' leaves it out, and the dry run lists it;
 * 'label' brings it in as a task with a label named after its type; 'task'
 * brings it in as any other task. Shawn chose to leave them out (2026-11-07).
 */
export const OTHER_TYPES: Dialect['otherTypes'] = 'skip';

/**
 * Times of day ClickUp gives a date that has no time: midnight, and 4 AM,
 * which its dates without a time carry. A due time at either is no due time.
 */
export const NO_TIME = ['00:00', '04:00'];

/** ClickUp's headings, squashed, and what they feed; any other column is left out. */
const HEADERS: Record<string, Target> = {
	taskid: 'key',
	tasklink: 'link',
	tasktype: 'type',
	taskname: 'title',
	taskcontent: 'notes',
	status: 'status',
	datecreated: 'made',
	datecreatedtext: 'skip',
	duedate: 'due',
	duedatetext: 'dueTime',
	startdate: 'start',
	startdatetext: 'skip',
	dateclosed: 'completed',
	datedone: 'completed',
	parentid: 'parent',
	subtasksids: 'skip',
	attachments: 'attachments',
	assignees: 'assignees',
	tags: 'labels',
	priority: 'priority',
	listname: 'project',
	foldernamepath: 'skip',
	spacename: 'space',
	timeestimated: 'estimate',
	timeestimatedtext: 'skip',
	checklists: 'checklist',
	comments: 'comments',
	assignedcomments: 'skip',
	timespent: 'skip',
	timespenttext: 'skip',
	rolleduptime: 'skip',
	rolleduptimetext: 'skip',
	homelocationid: 'skip',
	homelocation: 'skip',
	otherlocationids: 'skip',
	otherlocations: 'skip'
};

const PRIORITIES: Record<string, Priority> = {
	'1': 'urgent',
	'2': 'high',
	'3': 'medium',
	'4': 'low',
	urgent: 'urgent',
	high: 'high',
	normal: 'medium',
	low: 'low'
};

/** Milliseconds since 1970 as ClickUp writes its dates, from 1970 to the year 9999. */
function epoch(s: string): Date | null {
	if (!/^\d{1,15}$/.test(s)) return null;
	const n = Number(s);
	if (n >= 253402300800000) return null;
	const at = new Date(n);
	return Number.isNaN(at.getTime()) ? null : at;
}

const pad = (n: number) => String(n).padStart(2, '0');
const localDay = (at: Date) => `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}`;

/**
 * A moment as ClickUp writes it in words: "5/20/2026, 7:49:18 AM PDT", read
 * on this device's clock (the zone's name is not one the browser reads).
 */
function words(s: string): Date | null {
	const m = s.match(
		/^(\d{1,2})\/(\d{1,2})\/(\d{4})(?:,?\s+(\d{1,2}):(\d{2})(?::(\d{2}))?\s*([AP])\.?M\.?)?/i
	);
	if (!m) return null;
	const [mo, d, y] = [+m[1], +m[2], +m[3]];
	let h = m[4] === undefined ? 12 : +m[4];
	if (m[7]) {
		if (h < 1 || h > 12) return null;
		if (m[7].toUpperCase() === 'P' && h < 12) h += 12;
		if (m[7].toUpperCase() === 'A' && h === 12) h = 0;
	}
	const at = new Date(y, mo - 1, d, h, +(m[5] ?? 0), +(m[6] ?? 0));
	if (at.getMonth() !== mo - 1 || at.getDate() !== d || h > 23 || +(m[5] ?? 0) > 59) return null;
	return at;
}

function json(s: string): unknown {
	try {
		return JSON.parse(s);
	} catch {
		return undefined;
	}
}

const firstString = (o: Record<string, unknown>, keys: string[]): string | null => {
	for (const k of keys) {
		const v = own(o, k);
		if (typeof v === 'string' && v.trim()) return v;
	}
	return null;
};

const MAX_NAMES = 100;

/** "[Ann Lee, Bo Chen]", as ClickUp writes people, or a JSON list of names or people. */
function names(s: string): string[] {
	let inner = s;
	if (/^\[[\s\S]*\]$/.test(s)) {
		const v = json(s);
		if (Array.isArray(v))
			return v
				.slice(0, MAX_NAMES)
				.map((x) =>
					typeof x === 'string'
						? x
						: isObject(x)
							? (firstString(x, ['username', 'name', 'email']) ?? '')
							: ''
				)
				.map((x) => x.trim())
				.filter(Boolean);
		inner = s.slice(1, -1);
	}
	return inner
		.split(',')
		.map((x) => x.trim().replace(/^["']|["']$/g, ''))
		.filter(Boolean)
		.slice(0, MAX_NAMES);
}

/** "One comment could not be read and is left out." */
const unread = (n: number, one: string, many: string) =>
	n === 1
		? `One ${one} could not be read and is left out.`
		: `${n} ${many} could not be read and are left out.`;

function comments(s: string, problems: string[]): ReadComment[] {
	const v = json(s);
	if (!Array.isArray(v)) {
		problems.push('Its comments could not be read.');
		return [];
	}
	const out: ReadComment[] = [];
	let odd = 0;
	for (const c of v) {
		const text = isObject(c) ? (own(c, 'text') ?? own(c, 'comment_text')) : undefined;
		if (!isObject(c) || typeof text !== 'string') {
			odd++;
			continue;
		}
		const by = own(c, 'by');
		const at = own(c, 'date');
		out.push({
			author: typeof by === 'string' && by.trim() ? by : null,
			at:
				typeof at === 'string'
					? (words(at.trim()) ?? epoch(at.trim()))
					: typeof at === 'number'
						? epoch(String(at))
						: null,
			body: text
		});
	}
	if (odd) problems.push(unread(odd, 'comment', 'comments'));
	return out;
}

function files(s: string, problems: string[]): { name: string; url: string }[] {
	const v = json(s);
	if (!Array.isArray(v)) {
		problems.push('Its attachments could not be read.');
		return [];
	}
	const out: { name: string; url: string }[] = [];
	let odd = 0;
	for (const f of v) {
		const url = isObject(f) ? httpsUrl(own(f, 'url')) : null;
		if (!isObject(f) || !url) {
			odd++;
			continue;
		}
		const title = own(f, 'title');
		const name = typeof title === 'string' ? cleanText(title, 400).trim() : '';
		out.push({ name: name || nameFromUrl(url), url });
	}
	if (odd) problems.push(unread(odd, 'attachment', 'attachments'));
	return out;
}

const DONE = new Set(['true', 'yes', 'done', 'resolved', 'checked', 'complete', 'completed', '1']);
const isDone = (v: unknown) =>
	v === true || v === 1 || (typeof v === 'string' && DONE.has(v.trim().toLowerCase()));
const ticked = (x: Record<string, unknown>) =>
	['resolved', 'done', 'checked', 'completed', 'is_checked'].some((k) => isDone(own(x, k)));

/**
 * Checklists as ClickUp's export writes them, a checklist's name to its
 * items, or as its API does, a list of checklists with items: each item a
 * name or an object with one, ticked or not.
 */
function checklist(s: string, problems: string[]): { text: string; done: boolean }[] {
	const v = json(s);
	const out: { text: string; done: boolean }[] = [];
	let odd = 0;
	const item = (x: unknown, depth: number) => {
		if (out.length > 1000) return;
		if (typeof x === 'string') {
			if (x.trim()) out.push({ text: x, done: false });
			return;
		}
		if (!isObject(x)) {
			odd++;
			return;
		}
		const text = firstString(x, ['name', 'text', 'title', 'item', 'content', 'label']);
		const items = own(x, 'items');
		if (Array.isArray(items) && depth < 3) {
			// A checklist of its own: its items, and a nested one's under them.
			if (text && depth > 0) out.push({ text, done: ticked(x) });
			for (const i of items) item(i, depth + 1);
			return;
		}
		if (!text) {
			odd++;
			return;
		}
		out.push({
			text,
			done: ticked(x)
		});
	};
	if (Array.isArray(v)) for (const x of v) item(x, 0);
	else if (isObject(v))
		for (const k of Object.keys(v)) {
			const list = own(v, k);
			if (Array.isArray(list)) for (const x of list) item(x, 1);
			else odd++;
		}
	else {
		problems.push('Its checklists could not be read.');
		return [];
	}
	if (odd) problems.push(unread(odd, 'checklist item', 'checklist items'));
	return out;
}

/** ClickUp's CSV export. */
export const CLICKUP: Dialect = {
	app: 'ClickUp',
	headers: HEADERS,
	// ClickUp writes null, NaN or "" for nothing.
	blank: (s) => /^(null|nan|undefined|""|'')$/i.test(s),
	// Line breaks in a task's text come as \n.
	notes: (s) => s.replace(/\\r\\n|\\n|\\r/g, '\n').replace(/\\t/g, '\t'),
	priority: (s) => {
		const key = s.trim().toLowerCase();
		return Object.hasOwn(PRIORITIES, key) ? PRIORITIES[key] : undefined;
	},
	priorities: '(1 urgent, 2 high, 3 normal, 4 low)',
	date: (s) => {
		const at = epoch(s) ?? words(s);
		return at ? localDay(at) : readDate(s);
	},
	dateHint: 'a date',
	time: (s) => {
		const at = epoch(s);
		const m = s.match(/\b(\d{1,2}:\d{2}(?::\d{2})?\s*[AP]\.?M\.?)/i);
		const t = at ? `${pad(at.getHours())}:${pad(at.getMinutes())}` : m ? readTime(m[1]) : null;
		if (t) return NO_TIME.includes(t) ? null : t;
		// A date alone has no time.
		return words(s) || readDate(s) ? null : undefined;
	},
	instant: (s) => {
		const at = epoch(s) ?? words(s);
		return at ? at.toISOString() : readInstant(s);
	},
	names,
	comments,
	files,
	checklist,
	estimate: (s) => {
		// Milliseconds, as the file writes them; or words, from its Text column.
		if (/^\d+(\.\d+)?$/.test(s)) return Math.round(Number(s) / 60_000);
		return parseDuration(s) ?? undefined;
	},
	taskType: 'Task',
	otherTypes: OTHER_TYPES,
	listStrangers: true,
	linkName: 'ClickUp task',
	behind: 'Time tracked in ClickUp is left out'
};

/** Whether a file's headings are ClickUp's: its own names for a task's ID and title. */
export const isClickUp = (headers: string[]) => {
	const keys = new Set(headers.map((h) => h.toLowerCase().replace(/[^a-z0-9]/g, '')));
	return keys.has('taskid') && keys.has('taskname');
};
