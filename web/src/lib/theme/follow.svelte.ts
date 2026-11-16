// Keeps the workspace theme in use in step with the workspace that offers
// it (workspaceThemes.svelte.ts), and works out the theme notice: the
// workspace's suggestion, or that the theme in use is no longer offered.
// The app layout runs both, and has the engine already; the sign-in page
// never loads this.
import { preferences } from '../preferences.svelte';
import { session } from '../session.svelte';
import { engine } from '../sync/engine.svelte';
import type { WorkspaceTheme } from '../sync/entities';
import { themes } from './themes.svelte';
import { suggestion, workspaceDoc, workspaceThemes, type Kept } from './workspaceThemes.svelte';

/**
 * Follows the theme in use, in an effect: when a workspace offers it,
 * keeps that workspace's copy, so an admin's update shows on the next
 * sync; when the workspace no longer does, marks it gone. A theme of the
 * person's own, or a built-in, lets go of any kept.
 */
export function followWorkspaceTheme() {
	const id = preferences.current.customTheme;
	const user = session.user?.id ?? null;
	const kept = workspaceThemes.kept;
	if (!id || !user || themes.get(id)) {
		workspaceThemes.forget();
		return;
	}
	if (kept && kept.doc.id !== id) workspaceThemes.forget();
	// The workspace kept with it first; on a device that never kept it,
	// whichever of the person's workspaces offers it.
	const list = session.workspaces;
	const first = list.find((w) => w.id === kept?.ws);
	for (const w of first ? [first, ...list.filter((x) => x !== first)] : list) {
		const t = engine.replica(w.id)?.workspaceTheme(id);
		const doc = t && workspaceDoc(t);
		if (doc) {
			workspaceThemes.keep(w.id, w.name, doc, user);
			return;
		}
	}
	if (!kept || kept.doc.id !== id || !list.length) return;
	// Left the workspace, or it was unpublished there (or never came: a
	// copy from the server that lacks it).
	if (!first || engine.hasCopy(kept.ws)) workspaceThemes.lose();
}

/** Whether a theme is one a workspace offers, which is no one's own to fetch. */
export function isWorkspaceTheme(id: string | null | undefined): boolean {
	if (!id) return false;
	return (
		!!workspaceThemes.get(id) ||
		session.workspaces.some((w) => engine.replica(w.id)?.workspaceTheme(id))
	);
}

/** Tells the picker what the workspace in view offers, in an effect. */
export function followOffered() {
	const w = session.workspace;
	const list = w && !w.isPersonal ? (engine.replica(w.id)?.workspaceThemes() ?? []) : [];
	workspaceThemes.offer(w && list.length ? { ws: w.id, wsName: w.name, list } : null);
}

export type ThemeNotice =
	| { kind: 'gone'; kept: Kept }
	| { kind: 'suggest'; ws: string; wsName: string; theme: WorkspaceTheme };

/**
 * What the theme notice says, if anything: the theme in use is gone, or
 * the workspace in view suggests one this person has not answered and is
 * not using.
 */
export function themeNotice(): ThemeNotice | null {
	const id = preferences.current.customTheme;
	const kept = workspaceThemes.kept;
	if (kept?.gone && kept.doc.id === id) return { kind: 'gone', kept };
	const w = session.workspace;
	if (!w || w.isPersonal) return null;
	const t = suggestion(engine.replica(w.id)?.workspaceThemes() ?? []);
	if (!t || t.id === id || preferences.current.themeAnswers.includes(t.id)) return null;
	return { kind: 'suggest', ws: w.id, wsName: w.name, theme: t };
}
