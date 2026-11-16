// Mentions in comments. The body stores each one as @[Name](user:<id>),
// which the server reads to notify people (internal/notify, the same
// pattern). While typing, the comment box shows plain "@Name" and turns
// it into that form on sending.
import { linkTarget } from './links';

/** Matches one stored mention: the name in group 1, the user ID in group 2. */
const MENTION = /@\[([^\]\n]{1,100})\]\(user:([A-Za-z0-9-]{1,64})\)/g;

export interface Person {
	userId: string;
	name: string;
	/** Their picture's path, where the list knows it. */
	avatar?: string | null;
}

export type Segment = { text: string } | { userId: string; name: string };

/** Splits a comment into text and mentions, in order. */
export function segments(body: string): Segment[] {
	const out: Segment[] = [];
	let at = 0;
	for (const m of body.matchAll(MENTION)) {
		if (m.index > at) out.push({ text: body.slice(at, m.index) });
		out.push({ name: m[1]!, userId: m[2]! });
		at = m.index + m[0].length;
	}
	if (at < body.length) out.push({ text: body.slice(at) });
	return out;
}

/** The users a comment mentions, each once. */
export function mentioned(body: string): string[] {
	const ids: string[] = [];
	for (const m of body.matchAll(MENTION)) if (!ids.includes(m[2]!)) ids.push(m[2]!);
	return ids;
}

/** A name as a mention can hold it: one line, no brackets, 100 characters. */
function clean(name: string): string {
	return (
		name
			.replace(/[[\]\n]/g, ' ')
			.trim()
			.slice(0, 100) || 'Someone'
	);
}

/** The stored form of a mention. */
export function token(p: Person): string {
	return `@[${clean(p.name)}](user:${p.userId})`;
}

/**
 * A stored body as the comment box shows it, with the people it mentions,
 * so sending it again keeps them.
 */
export function toDisplay(body: string): { text: string; picked: Person[] } {
	const picked: Person[] = [];
	const text = body.replace(MENTION, (_, name: string, userId: string) => {
		if (!picked.some((p) => p.userId === userId)) picked.push({ userId, name });
		return '@' + name;
	});
	return { text, picked };
}

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

/**
 * Turns "@Name" back into stored mentions: the people picked from the
 * list first, then anyone whose full name is typed out and is the only
 * member with that name. Longer names go first, so "@Dana Lee" is not
 * taken for "@Dana".
 */
export function toStored(text: string, picked: Person[], members: Person[] = []): string {
	const byName = new Map<string, Person>();
	for (const p of picked) if (!byName.has(clean(p.name))) byName.set(clean(p.name), p);
	const counts = new Map<string, number>();
	for (const m of members) counts.set(clean(m.name), (counts.get(clean(m.name)) ?? 0) + 1);
	for (const m of members) {
		const name = clean(m.name);
		if (!byName.has(name) && counts.get(name) === 1) byName.set(name, m);
	}
	if (byName.size === 0) return text;
	const names = [...byName.keys()].sort((a, b) => b.length - a.length);
	// A name counts after the start, a space or an opening bracket, and
	// before anything that cannot continue a name.
	const pattern = new RegExp(
		`(^|[\\s(])@(${names.map(escape).join('|')})(?![\\p{L}\\p{N}_-])`,
		'gu'
	);
	return text.replace(
		pattern,
		(_, before: string, name: string) => before + token(byName.get(name)!)
	);
}

/** A mention being typed: where its "@" is and what follows it. */
export interface Query {
	start: number;
	query: string;
}

/**
 * The mention being typed just before the caret, if any: an "@" at the
 * start or after a space or bracket, followed by up to 30 characters on
 * the same line.
 */
export function activeQuery(text: string, caret: number): Query | null {
	const line = text.slice(0, caret).split('\n').pop()!;
	const at = line.lastIndexOf('@');
	if (at < 0) return null;
	const before = line[at - 1];
	if (before !== undefined && !/[\s(]/.test(before)) return null;
	const query = line.slice(at + 1);
	if (query.length > 30 || /[@[\]]/.test(query) || /^\s/.test(query) || /\s{2}/.test(query))
		return null;
	return { start: caret - line.length + at, query };
}

/** The people whose name, or a word of it, starts with the query. */
export function matching<T extends Person>(people: T[], query: string, limit = 8): T[] {
	const q = query.trim().toLowerCase();
	const starts = (p: T) => p.name.toLowerCase().startsWith(q);
	const words = (p: T) =>
		p.name
			.toLowerCase()
			.split(/\s+/)
			.some((w) => w.startsWith(q));
	return [...people.filter(starts), ...people.filter((p) => !starts(p) && words(p))].slice(
		0,
		limit
	);
}

/** Puts a picked person in place of the query; returns the text and caret. */
export function insert(
	text: string,
	q: Query,
	caret: number,
	p: Person
): { text: string; caret: number } {
	const shown = '@' + clean(p.name) + ' ';
	const rest = text.slice(caret).replace(/^ /, '');
	return { text: text.slice(0, q.start) + shown + rest, caret: q.start + shown.length };
}

// References in comments: `/task` and `/wiki` in the comment box pick a task or
// a wiki page, stored as an ordinary Markdown link to its address (links.ts)
// and shown as [[Title]] while typing. Web addresses typed or pasted in become
// links when the comment shows.

/** Matches one stored reference: the label in group 1, the address in group 2. */
const REFERENCE = /\[([^\]\n]{1,200})\]\((\/[^)\s]*)\)/g;
const WEB_ADDRESS = /https?:\/\/[^\s<>]+/gi;

/** What separates a page's title from a section of it, in a reference's label. */
export const SECTION_MARK = ' › ';

export interface Reference {
	label: string;
	href: string;
}

const refLabel = (label: string) =>
	label
		.replace(/[[\]\n]/g, ' ')
		.trim()
		.slice(0, 200) || 'Untitled';

/** The text shown for a reference while typing. */
export const shownReference = (label: string) => `[[${refLabel(label)}]]`;

/** A stored body with its references as the comment box shows them, and those references. */
export function referencesToDisplay(body: string): { text: string; refs: Reference[] } {
	const refs: Reference[] = [];
	const text = body.replace(REFERENCE, (all, label: string, href: string) => {
		if (!linkTarget(href)) return all;
		if (!refs.some((r) => r.href === href)) refs.push({ label, href });
		return shownReference(label);
	});
	return { text, refs };
}

/** Turns the [[Title]] of each reference picked into its stored form. */
export function referencesToStored(text: string, refs: Reference[]): string {
	return refs.reduce(
		(out, r) => out.split(shownReference(r.label)).join(`[${refLabel(r.label)}](${r.href})`),
		text
	);
}

/** A slash command being typed just before the caret, if any. */
export interface SlashQuery {
	start: number;
	/** The command as typed so far. */
	word: string;
	/** What follows a whole command and a space: what to search for. */
	query: string | null;
}

export const SLASH_COMMANDS = [
	{ word: 'task', label: 'Refer to a task' },
	{ word: 'wiki', label: 'Refer to a wiki page' }
] as const;

/**
 * The slash command before the caret: a "/" at the start or after a space, so
 * an address or "and/or" is no command, then a command or the start of one.
 */
export function activeSlash(text: string, caret: number): SlashQuery | null {
	const line = text.slice(0, caret).split('\n').pop()!;
	const m = /(^|\s)\/([a-z]*)(?: ([^\n]{0,60}))?$/i.exec(line);
	if (!m) return null;
	const word = m[2]!.toLowerCase();
	const known = SLASH_COMMANDS.some((c) => c.word === word);
	if (m[3] !== undefined ? !known : !SLASH_COMMANDS.some((c) => c.word.startsWith(word)))
		return null;
	const at = m.index + m[1]!.length;
	return { start: caret - line.length + at, word, query: m[3] ?? null };
}

/** The headings of a page's Markdown, in order, leaving out code blocks. */
export function headingsIn(markdown: string | null | undefined): string[] {
	const out: string[] = [];
	let fenced = false;
	for (const line of (markdown ?? '').split('\n')) {
		if (/^\s*(```|~~~)/.test(line)) fenced = !fenced;
		const m = !fenced && /^#{1,6}\s+(.+?)\s*#*\s*$/.exec(line);
		if (m) {
			const text = m[1]!.replace(/\[([^\]]*)\]\([^)]*\)/g, '$1').replace(/[*_`~]/g, '');
			if (text) out.push(text);
		}
	}
	return out;
}

/** A heading as it goes in an address, after the "#". */
export function slugOf(heading: string): string {
	return heading
		.toLowerCase()
		.replace(/[^\p{L}\p{N}]+/gu, '-')
		.replace(/^-+|-+$/g, '');
}

export type CommentPart =
	| { text: string }
	| { userId: string; name: string }
	| { label: string; href: string; target: { kind: 'page' | 'task'; id: string } }
	| { url: string };

/** Splits a stored comment into what it shows: text, mentions, references and web links. */
export function commentParts(body: string): CommentPart[] {
	const out: CommentPart[] = [];
	for (const seg of segments(body)) {
		if (!('text' in seg)) {
			out.push(seg);
			continue;
		}
		let at = 0;
		const add = (to: number) => {
			if (to > at) out.push({ text: seg.text.slice(at, to) });
		};
		const found = [
			...[...seg.text.matchAll(REFERENCE)].map((m) => ({ m, ref: true })),
			...[...seg.text.matchAll(WEB_ADDRESS)].map((m) => ({ m, ref: false }))
		].sort((a, b) => a.m.index - b.m.index);
		for (const { m, ref } of found) {
			if (m.index < at) continue;
			if (ref) {
				const target = linkTarget(m[2]!);
				if (!target) continue;
				add(m.index);
				out.push({ label: m[1]!, href: m[2]!, target });
				at = m.index + m[0].length;
			} else {
				const url = trimAddress(m[0]);
				add(m.index);
				out.push({ url });
				at = m.index + url.length;
			}
		}
		add(seg.text.length);
	}
	return out;
}

/** An address without the punctuation that follows it in a sentence. */
function trimAddress(url: string): string {
	let out = url.replace(/[.,;:!?'"]+$/, '');
	while (out.endsWith(')') && out.split(')').length > out.split('(').length) {
		out = out.slice(0, -1).replace(/[.,;:!?'"]+$/, '');
	}
	return out;
}
