// Searching help: topic and section titles, their keywords and summaries,
// and the names of the keyboard commands, which lead to the shortcut
// reference. The palette loads this only once something is typed in it.
import { COMMANDS, type CommandId } from '../keys/commands';
import { score } from '../search';
import { HELP } from './index';

export interface HelpHit {
	/** Where it goes: a topic, a section of one, or a row of the shortcuts. */
	topic: string;
	section?: string;
	title: string;
	/** What it is part of: the topic's title, or "Keyboard shortcuts". */
	context: string;
	/** For a command: its id, so its current keys can be shown. */
	command?: CommandId;
	s: number;
}

/** The slug that stands for the shortcut reference in a hit. */
export const SHORTCUTS = 'shortcuts';

/**
 * The best of the title, the keywords and half the summary; a query of
 * several words may also find them spread across all three.
 */
function best(q: string, title: string, keywords: string[] = [], summary = ''): number {
	return Math.max(
		score(title, q),
		...keywords.map((k) => score(k, q)),
		score(summary, q) / 2,
		/\s/.test(q) ? score([title, ...keywords, summary].join(' '), q) : 0
	);
}

/** The shortcut reference itself, by what people call it and changing its keys. */
const REFERENCE = {
	title: 'Keyboard shortcuts',
	keywords: ['keyboard', 'keys', 'hotkeys', 'change shortcut', 'change keys', 'remap', 'rebind'],
	summary: 'Every shortcut, with your keys; change them in Settings, Keyboard.'
};

/** What matches a query, best first and in index order on ties, at most 20. */
export function searchHelp(query: string): HelpHit[] {
	const q = query.trim().toLowerCase();
	if (!q) return [];
	const hits: HelpHit[] = [];
	// A whole topic counts for more than one of its sections.
	for (const t of HELP) {
		const s = best(q, t.title, t.keywords, t.summary);
		if (s) hits.push({ topic: t.slug, title: t.title, context: 'Help', s: s + 10 });
		for (const sec of t.sections) {
			const ss = best(q, sec.title, sec.keywords);
			if (ss)
				hits.push({
					topic: t.slug,
					section: sec.id,
					title: sec.title,
					context: t.title,
					s: ss + 5
				});
		}
	}
	const ref = best(q, REFERENCE.title, REFERENCE.keywords, REFERENCE.summary);
	if (ref) hits.push({ topic: SHORTCUTS, title: REFERENCE.title, context: 'Help', s: ref + 10 });
	for (const c of COMMANDS) {
		const s = score(c.name, q);
		if (s)
			hits.push({
				topic: SHORTCUTS,
				section: `keys-${c.scope}`,
				title: c.name,
				context: 'Keyboard shortcuts',
				command: c.id,
				s
			});
	}
	// A stable sort keeps the index's order when scores tie.
	return hits.sort((a, b) => b.s - a.s).slice(0, 20);
}
