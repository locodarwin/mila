// The wiki's main pages (docs/PLAN.md section 3). A wiki's main page has
// an ID made from its project's, or for the workspace's wiki the
// workspace's, so whichever device first writes in a new wiki makes the
// same row any other would. server/internal/syncer/pages.go has the same.
import { xorId } from './entities';

const MAIN_PAGE_SPACE = '2f5c6c0e-8a9b-4c7e-9d3a-6b1e4f2a7c58';

/** The ID of a wiki's main page: its project's, or the workspace's. */
export function mainPageId(projectOrWorkspace: string): string {
	return xorId(projectOrWorkspace, MAIN_PAGE_SPACE);
}

/** The main page of a project's wiki, or the workspace's for null. */
export function wikiMain(ws: string, projectId: string | null): string {
	return mainPageId(projectId ?? ws);
}

/**
 * The wiki a main page heads, from its ID alone: the project's ID, null
 * for the workspace's, or undefined when the ID is no wiki's main page.
 * For a main page this device has yet to save.
 */
export function wikiOfMain(
	ws: string,
	pageId: string,
	projectIds: string[]
): string | null | undefined {
	if (pageId === wikiMain(ws, null)) return null;
	return projectIds.find((p) => wikiMain(ws, p) === pageId);
}
