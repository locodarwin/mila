// What people do with workspace themes: admins
// publish one of their own themes, update it from that theme, rename,
// suggest and unpublish; everyone uses one, duplicates it, and answers
// a suggestion. Loaded with Settings, Workspace themes, the theme picker, the
// palette's theme entries and the theme notice, never up front.
import { preferences } from '../preferences.svelte';
import { session } from '../session.svelte';
import { engine } from '../sync/engine.svelte';
import type { WorkspaceTheme } from '../sync/entities';
import { keyBetween } from '../sync/sortkey';
import { toasts } from '../toasts.svelte';
import { uuid } from '../uuid';
import { appearance } from './appearance.svelte';
import { floorsOf } from './custom';
import { cleanName, LIMITS, uniqueName, type ThemeDoc } from './document';
import { themes } from './themes.svelte';
import {
	MAX_WORKSPACE_THEMES,
	withAnswer,
	workspaceDoc,
	workspaceThemes,
	type Kept
} from './workspaceThemes.svelte';

const now = () => new Date().toISOString();
const me = () => session.user?.id ?? null;
const snapshot = (doc: ThemeDoc) => JSON.parse(JSON.stringify(doc)) as ThemeDoc;

/**
 * Why a theme cannot be published or used: below the floors (text or the
 * focus ring under 3:1 on the page), or unreadable. Null when it can.
 * A workspace's theme is checked on its own accent, as most will see it.
 */
export function cannotPublish(doc: ThemeDoc): string | null {
	return floorsOf(doc);
}

/** The body a workspace keeps: the document under the row's id and name. */
const bodyOf = (doc: ThemeDoc, id: string, name: string) => ({ ...snapshot(doc), id, name });

/**
 * Publishes a copy of one of the admin's own themes to a workspace, under
 * a name of its own there, suggested to everyone or not. Says what is
 * wrong instead.
 */
export async function publish(
	ws: string,
	doc: ThemeDoc,
	opts: { name: string; suggest: boolean }
): Promise<{ id: string } | { error: string }> {
	const replica = engine.replica(ws);
	if (!replica) return { error: 'That workspace is not loaded yet.' };
	const offered = replica.workspaceThemes();
	if (offered.length >= MAX_WORKSPACE_THEMES)
		return {
			error: `A workspace offers ${MAX_WORKSPACE_THEMES} themes at most. Unpublish one first.`
		};
	const name = cleanName(opts.name);
	if (!name || [...name].length > LIMITS.name)
		return { error: `A name is 1 to ${LIMITS.name} characters.` };
	const why = cannotPublish(doc);
	if (why) return { error: why };
	const id = uuid();
	const at = now();
	await engine.group(async () => {
		await engine.mutate(ws, 'workspace_theme', id, (r) => ({
			name,
			body: bodyOf(doc, id, name),
			source_id: doc.id,
			suggested_at: opts.suggest ? at : null,
			published_by: me(),
			published_at: at,
			sort_key: keyBetween(r.workspaceThemes().at(-1)?.sortKey ?? null, null),
			deleted_at: null
		}));
		if (opts.suggest) await stopOthers(ws, id);
	});
	// Whoever suggests a theme is never asked about it themselves.
	if (opts.suggest) answer(id);
	return { id };
}

/** Takes the suggestion off every theme but one. */
async function stopOthers(ws: string, keep: string | null) {
	for (const t of engine.replica(ws)?.workspaceThemes() ?? [])
		if (t.id !== keep && t.suggestedAt)
			await engine.mutate(ws, 'workspace_theme', t.id, { suggested_at: null });
}

/**
 * Replaces a workspace theme with the admin's own theme as it is now, for
 * everyone using it. Says why not instead.
 */
export async function updateFrom(
	ws: string,
	t: WorkspaceTheme,
	doc: ThemeDoc
): Promise<string | null> {
	const why = cannotPublish(doc);
	if (why) return why;
	await engine.mutate(ws, 'workspace_theme', t.id, {
		body: bodyOf(doc, t.id, t.name),
		source_id: doc.id,
		published_by: me(),
		published_at: now()
	});
	return null;
}

export async function rename(ws: string, t: WorkspaceTheme, name: string): Promise<string | null> {
	const clean = cleanName(name);
	if (!clean || [...clean].length > LIMITS.name) return `A name is 1 to ${LIMITS.name} characters.`;
	await engine.mutate(ws, 'workspace_theme', t.id, { name: clean });
	return null;
}

/** Suggests a theme to everyone, which stops suggesting any other; or stops. */
export async function suggest(ws: string, t: WorkspaceTheme, on: boolean) {
	await engine.group(async () => {
		await engine.mutate(ws, 'workspace_theme', t.id, { suggested_at: on ? now() : null });
		if (on) await stopOthers(ws, t.id);
	});
	if (on) answer(t.id);
}

/** Unpublishes a theme, with a toast to undo it. */
export function unpublish(ws: string, t: WorkspaceTheme) {
	const { undo } = engine.track(() =>
		engine.mutate(ws, 'workspace_theme', t.id, { deleted_at: now(), suggested_at: null })
	);
	toasts.show(`Unpublished ${t.name}. Anyone using it keeps it until they choose another.`, {
		onundo: () => void undo()
	});
}

/**
 * Uses a workspace's theme, unless it is unreadable (the floors). Returns
 * why not, or null once it is in use.
 */
export function use(ws: string, t: WorkspaceTheme): string | null {
	const doc = workspaceDoc(t);
	if (!doc) return 'That theme cannot be read.';
	const why = floorsOf(doc, { accentHue: preferences.current.accentHue });
	if (why) return why;
	const wsName = session.workspaces.find((w) => w.id === ws)?.name ?? '';
	workspaceThemes.keep(ws, wsName, doc, me());
	preferences.set({ customTheme: t.id, theme: doc.base });
	return null;
}

/**
 * Copies a theme among the person's own, under a name of its own there;
 * the copy no longer follows the workspace. Returns it, or why not.
 */
export function duplicate(doc: ThemeDoc): ThemeDoc | string {
	if (themes.list.length >= LIMITS.themes)
		return `You can keep ${LIMITS.themes} themes. Delete one first.`;
	const copy: ThemeDoc = {
		...snapshot(doc),
		id: uuid(),
		name: uniqueName(
			doc.name,
			themes.list.map((x) => x.name)
		)
	};
	themes.put(copy);
	return copy;
}

/** Records that the person answered a suggestion, so it is never asked again. */
export function answer(id: string) {
	preferences.set({ themeAnswers: withAnswer(preferences.current.themeAnswers, id) });
}

/** The theme in use is gone: keeps it as one of the person's own, still in use. */
export function saveCopy(kept: Kept): string | null {
	const copy = duplicate(kept.doc);
	if (typeof copy === 'string') return copy;
	preferences.set({ customTheme: copy.id, theme: copy.base });
	workspaceThemes.forget();
	return null;
}

/** The theme in use is gone: goes back to the built-in it was made on. */
export function switchToBase(kept: Kept) {
	appearance.set({ character: kept.doc.base });
	workspaceThemes.forget();
}
