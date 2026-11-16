// The `[[` menu (docs/PLAN.md section 3): typing `[[` in a task's notes or
// a wiki page offers pages and tasks to link to, or a new subpage, and
// narrows them as more is typed.
import type { Editor } from '@tiptap/core';
import { taskHref } from '../links';
import type { Page, Project, Task } from '../sync/entities';
import { pageHref, pageTitle } from '../wiki';

/** Something a link can point at. */
export interface LinkChoice {
	kind: 'page' | 'task';
	id: string;
	title: string;
	href: string;
	/** Where it is: the page's wiki, or the task's project. */
	where: string;
}

/**
 * The `[[` being typed before the caret, with what follows it so far: the
 * menu is open while this is found. from and to are the positions the
 * link replaces.
 */
export function linkAt(editor: Editor): { query: string; from: number; to: number } | null {
	const { selection } = editor.state;
	if (!selection.empty) return null;
	const $from = selection.$from;
	if (!$from.parent.isTextblock || $from.parent.type.name === 'codeBlock') return null;
	const before = $from.parent.textBetween(0, $from.parentOffset, undefined, '￼');
	const m = /\[\[([^[\]\n￼]{0,60})$/.exec(before);
	if (!m) return null;
	return { query: m[1], from: $from.pos - m[0].length, to: $from.pos };
}

/** How well a title answers a query: lower is better, null for not at all. */
function rank(title: string, q: string): number | null {
	const t = title.toLowerCase();
	if (!q) return 2;
	if (t.startsWith(q)) return 0;
	if (t.split(/\s+/).some((w) => w.startsWith(q))) return 1;
	return t.includes(q) ? 2 : null;
}

/**
 * The pages and tasks a query finds, best first: at most six of each,
 * leaving out the document's own page or task.
 */
export function linkChoices(
	ws: string,
	query: string,
	pages: Page[],
	tasks: Task[],
	projectOf: (id: string | null) => Project | undefined,
	except: string
): LinkChoice[] {
	const q = query.trim().toLowerCase();
	const best = <T>(items: { item: T; title: string }[]) =>
		items
			.map((x) => ({ ...x, r: rank(x.title, q) }))
			.filter((x) => x.r !== null)
			.sort((a, b) => a.r! - b.r! || a.title.localeCompare(b.title))
			.slice(0, 6);
	const pageChoices = best(
		pages
			.filter((p) => p.id !== except)
			.map((p) => ({ item: p, title: pageTitle(ws, p, projectOf(p.projectId)) }))
	).map(({ item, title }): LinkChoice => ({
		kind: 'page',
		id: item.id,
		title,
		href: pageHref(item),
		where: projectOf(item.projectId)?.name ?? 'Workspace Wiki'
	}));
	const taskChoices = best(
		tasks.filter((t) => t.id !== except).map((t) => ({ item: t, title: t.title }))
	).map(({ item, title }): LinkChoice => ({
		kind: 'task',
		id: item.id,
		title,
		href: taskHref(item.id),
		where: projectOf(item.projectId)?.name ?? 'Inbox'
	}));
	return [...pageChoices, ...taskChoices];
}
