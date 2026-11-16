// Workspace themes (docs/PLAN.md section 3): themes a workspace offers
// everyone in it, guests too, and never imposes. Each is a copy of one of
// an admin's own themes, synced as the workspace's workspace_theme rows.
//
// This is the part every page needs, small: reading a row as a theme
// document, and the workspace theme in use, kept on this device. Kept, it
// shows from the first frame, before the workspace has loaded and while
// another workspace is in view; and when the workspace stops offering it
// (unpublished, or the person left), it stays on screen, marked gone,
// until the person saves a copy or picks another. follow.svelte.ts keeps
// it up to date from the workspace's rows.
import type { WorkspaceTheme } from '../sync/entities';
import type { ThemeDoc } from './document';
import { looksLikeTheme } from './themes.svelte';

const STORAGE_KEY = 'mila.workspaceTheme';

/** The workspace themes one workspace offers at most. */
export const MAX_WORKSPACE_THEMES = 10;

/** The suggestions a person's answers are kept for (themeAnswers). */
export const MAX_ANSWERS = 100;

// Each row's document, made once per body: the follow effect reads them
// on every change in the workspace, and the same body is the same theme.
const docs = new WeakMap<object, ThemeDoc | null>();

/**
 * The theme document a workspace theme holds, under the row's id and
 * name, or null when it cannot be one. parseTheme reads it when drawn.
 */
export function workspaceDoc(t: Pick<WorkspaceTheme, 'id' | 'name' | 'body'>): ThemeDoc | null {
	const body = t.body;
	if (!body || typeof body !== 'object' || Array.isArray(body)) return null;
	const hit = docs.get(body);
	if (hit === null || (hit && hit.id === t.id && hit.name === t.name)) return hit;
	const doc = { ...(body as Record<string, unknown>), id: t.id, name: t.name };
	const out = looksLikeTheme(doc) ? doc : null;
	docs.set(body, out);
	return out;
}

/** The one suggested to everyone: the theme suggested last, if any. */
export function suggestion<T extends Pick<WorkspaceTheme, 'id' | 'suggestedAt'>>(
	list: readonly T[]
): T | undefined {
	let best: T | undefined;
	for (const t of list)
		if (t.suggestedAt && (!best || t.suggestedAt > (best.suggestedAt as string))) best = t;
	return best;
}

/** The answers with one more, oldest first, the oldest dropped past the limit. */
export function withAnswer(answers: readonly string[], id: string): string[] {
	return [...answers.filter((x) => x !== id), id].slice(-MAX_ANSWERS);
}

export interface Kept {
	/** The workspace that offers it. */
	ws: string;
	/** Its name, for saying where the theme comes from while it is not loaded. */
	wsName: string;
	doc: ThemeDoc;
	/** The workspace no longer offers it: unpublished, or the person left. */
	gone: boolean;
	/** Whose it is, so the next person to sign in here never gets it. */
	user: string | null;
}

function read(): Kept | null {
	try {
		const v = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? 'null') as Partial<Kept> | null;
		if (
			v &&
			typeof v.ws === 'string' &&
			typeof v.wsName === 'string' &&
			looksLikeTheme(v.doc) &&
			typeof v.gone === 'boolean'
		)
			return {
				ws: v.ws,
				wsName: v.wsName,
				doc: v.doc,
				gone: v.gone,
				user: typeof v.user === 'string' ? v.user : null
			};
	} catch {
		// unreadable: none kept
	}
	return null;
}

/** The themes the workspace in view offers, for the theme picker. */
export interface Offered {
	ws: string;
	wsName: string;
	list: WorkspaceTheme[];
}

class WorkspaceThemeState {
	/** The workspace theme in use, as this device last had it. */
	// Raw: replaced whole, never changed in place, so a document is the same object.
	kept = $state.raw<Kept | null>(read());

	/**
	 * What the workspace in view offers, set by the app layout
	 * (follow.svelte.ts), so the picker reads it without the sync engine.
	 */
	offered = $state.raw<Offered | null>(null);

	/** The kept theme by id, if it is that one. */
	get(id: string | null | undefined): ThemeDoc | undefined {
		const k = this.kept;
		return id && k && k.doc.id === id ? k.doc : undefined;
	}

	#write() {
		try {
			if (this.kept) localStorage.setItem(STORAGE_KEY, JSON.stringify(this.kept));
			else localStorage.removeItem(STORAGE_KEY);
		} catch {
			// private mode or storage blocked: kept for this visit
		}
	}

	/** Keeps a workspace's theme as the one in use, as the workspace has it now. */
	keep(ws: string, wsName: string, doc: ThemeDoc, user: string | null) {
		const k = this.kept;
		if (k && k.ws === ws && k.wsName === wsName && !k.gone && k.user === user) {
			if (k.doc === doc) return;
			// The same theme, as the workspace's own copy from now on.
			if (JSON.stringify(k.doc) === JSON.stringify(doc)) {
				this.kept = { ...k, doc };
				return;
			}
		}
		this.kept = { ws, wsName, doc, gone: false, user };
		this.#write();
	}

	/** Sets what the workspace in view offers, unless it is the same. */
	offer(next: Offered | null) {
		const now = this.offered;
		const same =
			now === next ||
			(!!now &&
				!!next &&
				now.ws === next.ws &&
				now.wsName === next.wsName &&
				now.list.length === next.list.length &&
				now.list.every((t, i) => {
					const u = next.list[i];
					return (
						t.id === u.id &&
						t.name === u.name &&
						t.body === u.body &&
						t.sourceId === u.sourceId &&
						t.suggestedAt === u.suggestedAt &&
						t.publishedBy === u.publishedBy &&
						t.publishedAt === u.publishedAt &&
						t.sortKey === u.sortKey
					);
				}));
		if (!same) this.offered = next;
	}

	/** The workspace no longer offers the kept theme; it stays on screen until another is chosen. */
	lose() {
		if (!this.kept || this.kept.gone) return;
		this.kept = { ...this.kept, gone: true };
		this.#write();
	}

	forget() {
		if (!this.kept) return;
		this.kept = null;
		this.#write();
	}

	/** A signed-in user: a theme kept for anyone else goes. */
	connect(userId: string) {
		if (this.kept && this.kept.user !== userId) this.forget();
	}
}

export const workspaceThemes = new WorkspaceThemeState();
