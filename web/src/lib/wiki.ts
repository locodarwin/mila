// Where wiki pages live in the app, and what they are called. The workspace's
// wiki is /wiki; a project's is its Project Wiki tab.
import { resolve } from '$app/paths';
import { wikiMain } from './sync/pages';
import type { Page, Project } from './sync/entities';

/** A page's address: the workspace's wiki, or its project's Project Wiki tab. */
export function pageHref(page: { id: string; projectId: string | null }): string {
	return page.projectId
		? `${resolve('/(app)/projects/[id]', { id: page.projectId })}?tab=wiki&page=${page.id}`
		: `${resolve('/(app)/wiki')}?page=${page.id}`;
}

/** A wiki's name: its project's, or "Workspace Wiki" for the workspace's. */
export function wikiName(project: Project | undefined | null): string {
	return project?.name ?? 'Workspace Wiki';
}

/** What a page is called: a main page by its wiki, others by their title. */
export function pageTitle(ws: string, page: Page, project?: Project | null): string {
	if (page.id === wikiMain(ws, page.projectId)) return wikiName(project);
	return page.title.trim() || 'Untitled';
}
