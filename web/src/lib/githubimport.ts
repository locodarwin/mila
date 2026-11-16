// GitHub Issues import: the reader for the file `gh issue list --json`
// writes (GITHUB_COMMAND), turning each issue into a planned task through
// the checks every import shares (importplan.ts). The file is untrusted:
// anything not of the shape gh writes is left out or cut to size, never
// passed on as it came. The page is routes/(app)/settings/w/[ws]/data/apps.
import { formatDate } from './dates';
import {
	LIMITS,
	MAX_ALL_COMMENTS,
	MAX_NEW_LABELS,
	MAX_ROWS,
	blankTask,
	cleanText,
	isObject,
	oneLine,
	own,
	planner,
	refKey,
	type Context,
	type Destination,
	type Plan
} from './importplan';

/** What the person runs in the repository to make the file. */
export const GITHUB_COMMAND =
	'gh issue list -s all -L 5000 --json number,title,body,state,closedAt,createdAt,labels,assignees,milestone,comments,url > issues.json';

/** The largest file the reader takes. */
export const MAX_BYTES = 20 << 20;

/** More entries than this and the file is refused outright, not read one by one. */
const MAX_ENTRIES = MAX_ROWS * 4;
/** The most of each that one issue brings; gh writes far fewer. */
const MAX_LABELS = 100;
const MAX_ASSIGNEES = 20;
const MAX_COMMENTS = 1000;
export { MAX_ALL_COMMENTS, MAX_NEW_LABELS } from './importplan';

export interface Comment {
	author: string | null;
	/** An ISO time, or null when the file's could not be read. */
	at: string | null;
	body: string;
}

/** An issue as read, every value checked and cut to size. */
export interface Issue {
	/** Its place in the file, from 1. */
	line: number;
	number: number | null;
	title: string;
	body: string;
	closed: boolean;
	closedAt: string | null;
	createdAt: string | null;
	labels: string[];
	milestone: string | null;
	assignees: string[];
	comments: Comment[];
	url: string | null;
	/** What could not be read, and so does not come in. */
	problems: string[];
}

export interface IssueFile {
	issues: Issue[];
	/** Entries that are not issues at all, by their place in the file. */
	notIssues: number[];
	/** How many entries the file holds. */
	entries: number;
	/** owner/name, from the first issue's address. */
	repo: string | null;
	/** What each entry is called in the dry run, by its place in the file. */
	names: Map<number, string>;
}

/** A file that cannot be read, with what to tell the person. */
export class IssueFileError extends Error {}

const LOGIN = /^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$/;
const ISSUE_URL =
	/^https:\/\/[A-Za-z0-9.-]{1,253}(?::\d{1,5})?\/([A-Za-z0-9-]{1,39})\/([A-Za-z0-9._-]{1,100})\/issues\/(\d{1,9})$/;
const TIME = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/;

const text = cleanText;

function time(v: unknown): string | null {
	if (typeof v !== 'string' || !TIME.test(v)) return null;
	const at = new Date(v);
	return Number.isNaN(at.getTime()) ? null : at.toISOString();
}

function login(v: unknown): string | null {
	const name = isObject(v) ? own(v, 'login') : null;
	return typeof name === 'string' && LOGIN.test(name) ? name : null;
}

/** A list, or nothing and a problem said, at most `max` long. */
function list(o: Record<string, unknown>, key: string, max: number, problems: string[]) {
	const v = own(o, key);
	if (v === undefined || v === null) return [];
	if (!Array.isArray(v)) {
		problems.push(`Its ${key} could not be read.`);
		return [];
	}
	if (v.length > max) problems.push(`It has more ${key} than an import takes; ${max} come in.`);
	return v.slice(0, max);
}

/**
 * Reads the file gh writes: a list of issues, each checked field by field.
 * Throws IssueFileError for a file that is not such a list.
 */
export function readIssues(raw: string): IssueFile {
	let data: unknown;
	try {
		data = JSON.parse(raw);
	} catch {
		throw new IssueFileError(
			'That file is not JSON. Make it with the command above, then choose it.'
		);
	}
	if (!Array.isArray(data))
		throw new IssueFileError(
			'That file is not a list of issues. Make it with the command above, then choose it.'
		);
	if (data.length > MAX_ENTRIES)
		throw new IssueFileError(
			`That file holds ${data.length} entries; an import takes at most ${MAX_ROWS} issues.`
		);
	const out: IssueFile = {
		issues: [],
		notIssues: [],
		entries: data.length,
		repo: null,
		names: new Map()
	};
	data.forEach((entry: unknown, i) => {
		const line = i + 1;
		if (!isObject(entry)) {
			out.notIssues.push(line);
			out.names.set(line, `Entry ${line}`);
			return;
		}
		const problems: string[] = [];
		const n = own(entry, 'number');
		const number = Number.isSafeInteger(n) && (n as number) > 0 ? (n as number) : null;
		out.names.set(line, number ? `Issue #${number}` : `Entry ${line}`);
		const url = (() => {
			const u = own(entry, 'url');
			const m = typeof u === 'string' ? u.match(ISSUE_URL) : null;
			if (!m) return null;
			out.repo ??= `${m[1]}/${m[2]}`;
			return u as string;
		})();
		const state = own(entry, 'state');
		const closedAt = time(own(entry, 'closedAt'));
		let closed: boolean;
		if (typeof state === 'string' && /^(open|closed)$/i.test(state))
			closed = state.toUpperCase() === 'CLOSED';
		else {
			closed = closedAt !== null;
			problems.push(`Its state could not be read; it comes in ${closed ? 'done' : 'open'}.`);
		}
		const milestone = own(entry, 'milestone');
		out.issues.push({
			line,
			number,
			title: oneLine(text(own(entry, 'title'), LIMITS.title * 4), LIMITS.title),
			body: text(own(entry, 'body'), LIMITS.notes),
			closed,
			closedAt,
			createdAt: time(own(entry, 'createdAt')),
			labels: list(entry, 'labels', MAX_LABELS, problems)
				.map((l) => (isObject(l) ? oneLine(text(own(l, 'name'), 400), LIMITS.label) : ''))
				.filter(Boolean),
			milestone: isObject(milestone)
				? oneLine(text(own(milestone, 'title'), 400), LIMITS.label) || null
				: null,
			assignees: list(entry, 'assignees', MAX_ASSIGNEES, problems)
				.map(login)
				.filter((x): x is string => !!x),
			comments: list(entry, 'comments', MAX_COMMENTS, problems)
				.filter(isObject)
				.map((c) => ({
					author: login(own(c, 'author')),
					at: time(own(c, 'createdAt')),
					body: text(own(c, 'body'), LIMITS.comment).trim()
				}))
				.filter((c) => c.body),
			url,
			problems
		});
	});
	return out;
}

/** The repository's own name, for a new project: "mila" from locodarwin/mila. */
export const repoName = (repo: string | null) => repo?.split('/')[1] ?? null;

export interface IssueOptions {
	/** How a comment's date is written; the person's date style by default. */
	date?: (at: Date) => string;
}

/**
 * The dry run for a file of issues: each issue a task in the place chosen,
 * open ones To do and closed ones Done since they closed, the body as notes
 * after a link back to the issue, labels and the milestone as labels,
 * assignees matched to people here by their GitHub login, and comments
 * starting with who wrote them and when. Nothing changes.
 */
export function planIssues(
	file: IssueFile,
	destination: Destination,
	ctx: Context,
	opts: IssueOptions = {}
): Plan {
	const write = opts.date ?? ((at: Date) => formatDate(at, { year: true }));
	// Written once a day, not once a comment: formatting is slow, and the
	// dry run is made again as the place chosen changes.
	const days = new Map<string, string>();
	const date = (at: Date) => {
		const key = `${at.getFullYear()}-${at.getMonth()}-${at.getDate()}`;
		let s = days.get(key);
		if (s === undefined) days.set(key, (s = write(at)));
		return s;
	};
	const p = planner(destination, ctx);
	const { out } = p;
	const now = new Date().toISOString();
	let comments = 0;
	for (const line of file.notIssues) p.skip(line, 'It is not an issue.');
	for (const [n, issue] of file.issues.entries()) {
		const { line } = issue;
		if (n >= MAX_ROWS) {
			p.skip(line, `An import takes at most ${MAX_ROWS} issues.`);
			continue;
		}
		if (!issue.title) {
			p.skip(line, 'It has no title.');
			continue;
		}
		for (const problem of issue.problems) p.warn(line, problem);
		const project = p.fallbackRef();
		const t = blankTask(line, issue.title, project);
		t.notes = notesOf(issue, date);
		// When it was opened, not imported: reports count it from then.
		t.madeAt = issue.createdAt;
		if (issue.closed) {
			t.completedAt = issue.closedAt ?? now;
			if (!issue.closedAt) p.warn(line, 'It has no closing time; it comes in done today.');
		}
		if (project)
			t.status = p.statusIn(
				project,
				issue.closed ? 'done' : 'todo',
				issue.closed ? 'Done' : 'To do'
			);
		// Names were cut to size as the file was read.
		for (const name of [...issue.labels, ...(issue.milestone ? [issue.milestone] : [])]) {
			let r = p.knownLabel(name);
			if (!r && out.newLabels.length >= MAX_NEW_LABELS) {
				p.warn(line, `The label ${name} is left out: an import makes at most ${MAX_NEW_LABELS}.`);
				continue;
			}
			r ??= p.labelRef(name);
			if (!t.labels.some((l) => refKey(l) === refKey(r))) t.labels.push(r);
		}
		for (const who of issue.assignees) {
			const id = p.member(who);
			if (!id) p.stranger(who, line);
			else if (!p.reaches(id, project)) p.warn(line, `${who} cannot reach this task's project.`);
			else if (!t.assignees.includes(id)) t.assignees.push(id);
		}
		if (issue.assignees.length && !t.assignees.length) out.unassigned.push(line);
		for (const c of issue.comments) {
			if (comments >= MAX_ALL_COMMENTS) {
				p.warn(line, `Its comments are left out: an import makes at most ${MAX_ALL_COMMENTS}.`);
				break;
			}
			comments++;
			const by = `Originally by ${c.author ?? 'someone'}${c.at ? `, ${date(new Date(c.at))}` : ''}`;
			t.comments.push(`${by}\n\n${c.body}`.slice(0, LIMITS.comment));
		}
		p.add(t);
	}
	return out;
}

/** The issue's body, after a line that links back to it. */
function notesOf(issue: Issue, date: (at: Date) => string): string {
	const opened = issue.createdAt ? `, opened ${date(new Date(issue.createdAt))}` : '';
	const name = issue.number ? `GitHub issue #${issue.number}` : 'GitHub issue';
	const head = issue.url ? `[${name}](${issue.url})${opened}` : '';
	const body = issue.body.trim();
	const notes = head && body ? `${head}\n\n${body}` : head || body;
	return notes.slice(0, LIMITS.notes);
}
