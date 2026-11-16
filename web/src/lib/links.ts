// Links between pages and tasks (docs/PLAN.md section 3): ordinary
// Markdown links to an address in the app, a page's (pageHref in wiki.ts)
// or a task's, /?task=<id>. server/internal/syncer/links.go reads them
// the same way (LinkTarget).

const ID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** A task's address: it opens over whatever page the app is on. */
export const taskHref = (id: string) => `/?task=${id}`;

const appOrigin = () => (typeof location === 'undefined' ? 'http://mila.invalid' : location.origin);

/**
 * What a link's address points at, or null for an address that is not the
 * app's: a path in it, or a full address of the app's own origin (here).
 * The address is resolved as the browser would follow it, so `//host`,
 * `/\host` and a tab inside, which lead to another site, are not the app's,
 * though shown with a page's or task's title they would look it.
 */
export function linkTarget(
	href: string,
	here: string = appOrigin()
): { kind: 'page' | 'task'; id: string } | null {
	if (!href.startsWith('/') && !/^https?:/i.test(href)) return null;
	let u: URL;
	try {
		u = new URL(href, `${here}/`);
	} catch {
		return null;
	}
	if (u.origin !== here) return null;
	const page = u.searchParams.get('page');
	if (page && ID.test(page) && (u.pathname === '/wiki' || u.pathname.startsWith('/projects/')))
		return { kind: 'page', id: page };
	const task = u.searchParams.get('task');
	if (task && ID.test(task)) return { kind: 'task', id: task };
	return null;
}

/** The IDs of the pages and tasks a document's Markdown links to. */
export function linksIn(markdown: string): string[] {
	const out = new Set<string>();
	for (const m of markdown.matchAll(/\]\(\s*<?([^)\s>]+)/g)) {
		const t = linkTarget(m[1]);
		if (t) out.add(t.id);
	}
	return [...out];
}
