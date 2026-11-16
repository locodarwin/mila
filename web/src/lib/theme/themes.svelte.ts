// The person's custom themes. The server keeps them (/me/themes, one row
// each, so two devices editing different themes never undo each other);
// this device keeps a copy in localStorage for the moment before it hears
// from the server and while offline, with the themes changed or deleted
// here and not yet saved, and whose they are. Like the preferences, those
// win over the server's copy until they are saved; a whole theme is last
// writer wins.
//
// Themes are kept as stored documents and read with parseTheme only when
// drawn or edited (custom.ts, the editor), which keeps the registry out of
// the first load.
import { api, ApiError, call } from '../api/client';
import { preferences } from '../preferences.svelte';
import type { ThemeDoc } from './document';

const STORAGE_KEY = 'mila.themes';

/** Why the server would not take a theme it never had, kept until the theme changes. */
export interface NotSaved {
	/** The server's error code, such as too_many_themes. */
	code: string;
	message: string;
}

interface Stored {
	themes: ThemeDoc[];
	unsaved: string[];
	deleted: string[];
	notSaved: Record<string, NotSaved>;
	/** Whose the list is: the user last connected, kept after signing out so the next person never gets it. */
	user: string | null;
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

/** Whether something looks like a theme document at all; parseTheme does the rest. */
export function looksLikeTheme(v: unknown): v is ThemeDoc {
	if (!v || typeof v !== 'object' || Array.isArray(v)) return false;
	const t = v as Record<string, unknown>;
	return (
		t.mila === 'theme' &&
		typeof t.id === 'string' &&
		UUID.test(t.id) &&
		typeof t.name === 'string' &&
		typeof t.base === 'string'
	);
}

function read(): Stored {
	try {
		const v = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? 'null') as Partial<Stored> | null;
		if (v && typeof v === 'object') {
			const themes = Array.isArray(v.themes) ? v.themes.filter(looksLikeTheme) : [];
			const ids = (list: unknown) =>
				Array.isArray(list) ? list.filter((x): x is string => typeof x === 'string') : [];
			const notSaved: Record<string, NotSaved> = {};
			if (v.notSaved && typeof v.notSaved === 'object')
				for (const [id, why] of Object.entries(v.notSaved))
					if (UUID.test(id) && why && typeof why.message === 'string')
						notSaved[id] = { code: String(why.code), message: why.message };
			return {
				themes,
				unsaved: ids(v.unsaved),
				deleted: ids(v.deleted),
				notSaved,
				user: typeof v.user === 'string' ? v.user : null
			};
		}
	} catch {
		// unreadable: start with none
	}
	return { themes: [], unsaved: [], deleted: [], notSaved: {}, user: null };
}

/**
 * Whether the server turned the theme down (or it is not this person's),
 * rather than failing to take it for now (offline, restarting, busy).
 */
function refused(err: unknown): boolean {
	return err instanceof ApiError && [400, 404, 413, 422].includes(err.status);
}

/**
 * JSON with every object's keys in order. The server writes a theme's
 * keys sorted, the app in the order it made them; comparing (and the
 * appearance's build key) goes by this, so the same theme is the same.
 */
export function canonical(v: unknown): string {
	return JSON.stringify(v, (_key, value: unknown) =>
		value && typeof value === 'object' && !Array.isArray(value)
			? Object.fromEntries(
					Object.keys(value)
						.sort()
						.map((k) => [k, (value as Record<string, unknown>)[k]])
				)
			: value
	);
}

const same = (a: unknown, b: unknown) => canonical(a) === canonical(b);

/** How long to wait after a change before saving it: longer while the editor is open. */
export const SAVE_DELAY = { editing: 1000, idle: 500 };

class ThemesState {
	/** The person's themes, in the order they were made. */
	list = $state<ThemeDoc[]>([]);
	/** Whether the list has come from the server since the app opened. */
	loaded = $state(false);
	/** Whether the theme editor is open, which saves less often. */
	editing = false;
	/**
	 * Themes the server refused that it never had (past the limit, say):
	 * kept here, with why, until they change or are deleted, so nothing is
	 * lost; the manager shows them as not saved.
	 */
	notSaved = $state<Record<string, NotSaved>>({});

	#unsaved = new Set<string>();
	#deleted = new Set<string>();
	#owner: string | null = null;
	#user: string | null = null;
	#timers = new Map<string, ReturnType<typeof setTimeout>>();
	#busy = new Map<string, Promise<void>>();
	// Saving failed for now: wait for a reconnect or a new change.
	#waiting = false;
	#loading: Promise<void> | null = null;

	constructor() {
		const stored = read();
		this.list = stored.themes;
		this.#unsaved = new Set(stored.unsaved);
		this.#deleted = new Set(stored.deleted);
		this.notSaved = stored.notSaved;
		this.#owner = stored.user;
	}

	/** A theme by id, if this person has it. */
	get(id: string | null | undefined): ThemeDoc | undefined {
		return id ? this.list.find((t) => t.id === id) : undefined;
	}

	/** Whether a theme has changes here the server has not taken yet. */
	unsaved(id: string): boolean {
		return this.#unsaved.has(id);
	}

	#write() {
		try {
			const stored: Stored = {
				themes: this.list,
				unsaved: [...this.#unsaved],
				deleted: [...this.#deleted],
				notSaved: this.notSaved,
				user: this.#owner
			};
			localStorage.setItem(STORAGE_KEY, JSON.stringify(stored));
		} catch {
			// private mode or storage blocked: themes last for this visit
		}
	}

	#replace(doc: ThemeDoc) {
		const i = this.list.findIndex((t) => t.id === doc.id);
		if (i === -1) this.list = [...this.list, doc];
		else if (!same(this.list[i], doc)) this.list = this.list.with(i, doc);
	}

	#drop(id: string) {
		if (this.list.some((t) => t.id === id)) this.list = this.list.filter((t) => t.id !== id);
		this.#saved(id);
	}

	/** Clears a theme's not-saved mark. */
	#saved(id: string) {
		if (Object.hasOwn(this.notSaved, id)) {
			const rest = { ...this.notSaved };
			delete rest[id];
			this.notSaved = rest;
		}
	}

	/**
	 * Adds or changes a theme here at once, and saves it for the person's
	 * other devices after a moment (so a slider saves once).
	 */
	put(doc: ThemeDoc) {
		const copy = $state.snapshot(doc) as ThemeDoc;
		this.#replace(copy);
		this.#saved(copy.id);
		this.#unsaved.add(copy.id);
		this.#deleted.delete(copy.id);
		if (this.#user) this.#owner = this.#user;
		this.#waiting = false;
		this.#write();
		this.#schedule(copy.id, this.editing ? SAVE_DELAY.editing : SAVE_DELAY.idle);
	}

	/**
	 * Deletes a theme, here at once and then on the server. Returns the
	 * undo for its toast, which puts the same theme back, id and all, so a
	 * theme in use is in use again.
	 */
	remove(id: string): () => void {
		const doc = this.get(id);
		this.#drop(id);
		this.#unsaved.delete(id);
		this.#deleted.add(id);
		if (this.#user) this.#owner = this.#user;
		this.#waiting = false;
		this.#write();
		this.#schedule(id, 0);
		return () => {
			if (doc) this.put(doc);
		};
	}

	/**
	 * Starts saving for a signed-in user. A list kept for anyone else (or
	 * for no one known) goes, with what they changed here and left unsaved,
	 * which is never sent to this user's account. The list is fetched now
	 * only if a custom theme is in use or some are cached; otherwise it
	 * waits until the picker or the manager asks (ensure), or until a
	 * custom theme comes into use (follow).
	 */
	connect(userId: string) {
		if (this.#owner !== userId) {
			this.#unsaved.clear();
			this.#deleted.clear();
			this.notSaved = {};
			this.list = [];
		}
		this.#user = userId;
		this.#owner = userId;
		this.#waiting = false;
		this.loaded = false;
		this.#write();
		if (preferences.current.customTheme || this.list.length || this.#deleted.size)
			void this.refresh();
	}

	/**
	 * Stops saving and forgets what was not sent. The list stays for the
	 * sign-in page's look, still marked as its owner's, so the next person
	 * to sign in here never gets it.
	 */
	disconnect() {
		this.#user = null;
		this.#waiting = false;
		for (const t of this.#timers.values()) clearTimeout(t);
		this.#timers.clear();
		this.#unsaved.clear();
		this.#deleted.clear();
		this.#write();
	}

	/**
	 * The app reconnected: anything changed meanwhile, here or elsewhere,
	 * is caught up, if the list matters on this device yet.
	 */
	reconnected() {
		if (this.loaded || preferences.current.customTheme || this.list.length) void this.refresh();
	}

	/**
	 * A custom theme came into use (from another device, through the
	 * preferences): fetches it if this device does not have it yet.
	 */
	follow(id: string | null | undefined) {
		if (id && this.#user && !this.get(id)) void this.onMessage(id);
	}

	/** Loads the list from the server if it has not come yet this visit. */
	ensure(): Promise<void> {
		if (this.loaded) return Promise.resolve();
		return this.refresh();
	}

	#schedule(id: string, delay: number) {
		clearTimeout(this.#timers.get(id));
		this.#timers.delete(id);
		if (!this.#user || this.#waiting) return;
		if (!this.#unsaved.has(id) && !this.#deleted.has(id)) return;
		this.#timers.set(
			id,
			setTimeout(() => {
				this.#timers.delete(id);
				void this.#send(id);
			}, delay)
		);
	}

	/** Sends one theme's change, or its deletion; one request per theme at a time. */
	async #send(id: string): Promise<void> {
		const busy = this.#busy.get(id);
		if (busy) {
			await busy;
			if (this.#unsaved.has(id) || this.#deleted.has(id)) return this.#send(id);
			return;
		}
		if (!this.#user) return;
		const work = this.#deleted.has(id) ? this.#sendDelete(id) : this.#sendPut(id);
		this.#busy.set(id, work);
		try {
			await work;
		} finally {
			this.#busy.delete(id);
		}
	}

	async #sendPut(id: string) {
		const doc = this.get(id);
		if (!doc || !this.#unsaved.has(id)) return;
		const sent = $state.snapshot(doc) as ThemeDoc;
		try {
			const saved = await call(
				api.PUT('/me/themes/{themeId}', { params: { path: { themeId: id } }, body: sent as never })
			);
			// Changed again while this was on its way: that change still counts.
			if (same(this.get(id), sent)) {
				this.#unsaved.delete(id);
				if (looksLikeTheme(saved)) this.#replace(saved);
			}
			this.#write();
		} catch (err) {
			if (!refused(err)) {
				this.#waiting = true;
				return;
			}
			this.#unsaved.delete(id);
			this.#write();
			// The server's copy, if it has one, wins; one it never had stays
			// here, marked with why, rather than being lost.
			if ((await this.#fetch(id)) === 'missing' && this.get(id)) {
				const e = err as ApiError;
				this.notSaved = { ...this.notSaved, [id]: { code: e.code, message: e.message } };
				this.#write();
			}
		}
	}

	/** Takes the server's copy of one theme; says whether it had one, or could not be asked. */
	async #fetch(id: string): Promise<'found' | 'missing' | 'offline'> {
		try {
			const theme = await call(
				api.GET('/me/themes/{themeId}', { params: { path: { themeId: id } } })
			);
			if (this.#unsaved.has(id) || this.#deleted.has(id)) return 'found';
			if (looksLikeTheme(theme) && theme.id === id) {
				this.#replace(theme as ThemeDoc);
				this.#saved(id);
			}
			this.#write();
			return 'found';
		} catch (err) {
			return err instanceof ApiError && err.status === 404 ? 'missing' : 'offline';
		}
	}

	async #sendDelete(id: string) {
		try {
			await call(api.DELETE('/me/themes/{themeId}', { params: { path: { themeId: id } } }));
		} catch (err) {
			if (!refused(err)) {
				this.#waiting = true;
				return;
			}
			// Gone already, or never this person's: nothing to delete.
		}
		this.#deleted.delete(id);
		this.#write();
	}

	/** Sends everything waiting, one theme after another. */
	async flush() {
		this.#waiting = false;
		for (const id of [...this.#unsaved, ...this.#deleted]) {
			clearTimeout(this.#timers.get(id));
			this.#timers.delete(id);
			await this.#send(id);
			if (this.#waiting) return;
		}
	}

	/**
	 * Saves anything waiting, then takes the server's list, keeping what was
	 * changed or deleted here and not yet saved: on connecting, reconnecting,
	 * and when the picker or manager first opens.
	 */
	refresh(): Promise<void> {
		if (!this.#user) return Promise.resolve();
		if (this.#loading) return this.#loading;
		this.#loading = (async () => {
			try {
				await this.flush();
				const { themes } = await call(api.GET('/me/themes'));
				const server = themes.filter(looksLikeTheme) as ThemeDoc[];
				const ids = new Set(server.map((t) => t.id));
				// Kept too: changes not sent yet, and themes the server refused
				// and never had. One the server has now loses its mark.
				const local = (id: string) => this.#unsaved.has(id) || Object.hasOwn(this.notSaved, id);
				const kept = this.list.filter((t) => local(t.id) && !ids.has(t.id));
				for (const id of ids) this.#saved(id);
				const next = server
					.filter((t) => !this.#deleted.has(t.id))
					.map((t) => (this.#unsaved.has(t.id) ? this.get(t.id)! : t));
				const list = [...next, ...kept];
				if (!same(list, this.list)) this.list = list;
				this.loaded = true;
				this.#write();
			} catch {
				// offline: the next reconnect tries again
			} finally {
				this.#loading = null;
			}
		})();
		return this.#loading;
	}

	/**
	 * Another device changed or deleted a theme (the `themes` message):
	 * takes the server's copy of that one, unless it was changed here and
	 * not yet saved, which wins.
	 */
	async onMessage(id: string) {
		if (!this.#user || !UUID.test(id) || this.#unsaved.has(id) || this.#deleted.has(id)) return;
		const found = await this.#fetch(id);
		// Deleted elsewhere; one the server never had stays, as not saved.
		if (found === 'missing' && !this.#unsaved.has(id) && !Object.hasOwn(this.notSaved, id)) {
			this.#drop(id);
			this.#write();
		}
	}

	/**
	 * Uses a theme for the app, unless it is unreadable: text or the focus
	 * ring below 3:1 on the page in either scheme (the floors). Returns why
	 * not, or null once it is in use. The palette and the picker both go
	 * through here, so nothing can switch to a theme no one can read.
	 */
	async use(id: string): Promise<string | null> {
		const doc = this.get(id);
		if (!doc) return 'That theme is gone.';
		const { floorsOf } = await import('./custom');
		const why = floorsOf(doc, { accentHue: preferences.current.accentHue });
		if (why) return why;
		preferences.set({ customTheme: id, theme: doc.base });
		return null;
	}
}

export const themes = new ThemesState();
