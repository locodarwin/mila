// The user's preferences: theme (a built-in, and the custom theme in use),
// dates, sidebar, start page, keyboard shortcuts and the rest.
// The server keeps them with the user (users.settings), so they follow the
// person to every device. This device keeps a copy in localStorage, which
// the app uses before it hears from the server and while offline, along
// with which preferences were changed here and not yet saved, and for whom:
// those win over the server's copy until they are, and every other one
// follows it.
import { api, ApiError, call, type Schemas } from './api/client';
import { valid as validBinding } from './keys/chord';

export type Preferences = Schemas['Preferences'];
export type SidebarItem = Schemas['SidebarItem'];
type Key = keyof Preferences;

export const SIDEBAR_ITEMS: SidebarItem[] = [
	'inbox',
	'today',
	'upcoming',
	'wiki',
	'time',
	'reports',
	'views',
	'projects'
];

export const DEFAULT_PREFERENCES: Preferences = {
	theme: 'standard',
	scheme: 'system',
	accentHue: null,
	density: 'comfortable',
	fontSize: 'm',
	weekStart: 'auto',
	dateFormat: 'auto',
	clock: 'auto',
	showCompleted: false,
	startPage: 'inbox',
	sidebarOrder: [...SIDEBAR_ITEMS],
	sidebarHidden: [],
	singleKeyShortcuts: true,
	shortcuts: {},
	customTheme: null,
	themeAnswers: [],
	taskPanel: 'boxes',
	wikiToolbar: true
};

// The choices of each preference that has a list of them, as the server
// allows them.
const CHOICES: Partial<Record<Key, readonly unknown[]>> = {
	theme: ['standard', 'calm', 'playful', 'contrast'],
	scheme: ['system', 'light', 'dark'],
	density: ['comfortable', 'dense'],
	fontSize: ['s', 'm', 'l', 'xl', 'xxl'],
	weekStart: ['auto', 'sunday', 'monday', 'saturday'],
	dateFormat: ['auto', 'month-day', 'day-month', 'iso'],
	clock: ['auto', '12', '24'],
	startPage: ['inbox', 'today', 'upcoming', 'projects'],
	taskPanel: ['boxes', 'summary']
};

function sidebarList(v: unknown): v is SidebarItem[] {
	return Array.isArray(v) && v.every((x, i) => SIDEBAR_ITEMS.includes(x) && v.indexOf(x) === i);
}

/** Whether a value is allowed for a preference. */
function valid(key: Key, v: unknown): boolean {
	const choices = CHOICES[key];
	if (choices) return choices.includes(v);
	switch (key) {
		case 'accentHue':
			return v === null || (Number.isInteger(v) && (v as number) >= 0 && (v as number) < 360);
		case 'customTheme':
			// Only the shape: a theme deleted, or not loaded yet, falls back to its base.
			return v === null || (typeof v === 'string' && THEME_ID.test(v));
		case 'themeAnswers':
			return (
				Array.isArray(v) &&
				v.length <= MAX_ANSWERS &&
				v.every((x) => typeof x === 'string' && THEME_ID.test(x))
			);
		case 'showCompleted':
		case 'singleKeyShortcuts':
		case 'wikiToolbar':
			return typeof v === 'boolean';
		case 'sidebarOrder':
		case 'sidebarHidden':
			return sidebarList(v);
		case 'shortcuts':
			// Only the shape: parsePreferences cleans it entry by entry.
			return !!v && typeof v === 'object' && !Array.isArray(v);
	}
	return false;
}

// The suggested workspace themes answered, kept (accounts/preferences.go).
const MAX_ANSWERS = 100;

const THEME_ID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

const MAX_SHORTCUTS = 200;
const MAX_BINDINGS = 4;
const SHORTCUT_ID = /^[a-z][a-z0-9.-]{0,47}$/;

/**
 * The shortcuts map with what the server would refuse taken out: entries
 * past 200, ids that are not ids, values that are not lists, and bindings
 * that are not bindings, repeated, or past the fourth. One bad binding
 * drops only itself, never the person's other keys; an entry left with
 * none goes, so the command has its defaults again.
 */
function cleanShortcuts(v: Record<string, unknown>): Preferences['shortcuts'] {
	const out: Preferences['shortcuts'] = {};
	let n = 0;
	for (const [id, keys] of Object.entries(v)) {
		if (n === MAX_SHORTCUTS) break;
		if (!SHORTCUT_ID.test(id) || !Array.isArray(keys)) continue;
		const good = keys.filter((b): b is string => typeof b === 'string' && validBinding(b));
		// Nothing usable left is not the same as no key: the defaults come back.
		if (keys.length && !good.length) continue;
		out[id] = [...new Set(good)].slice(0, MAX_BINDINGS);
		n++;
	}
	return out;
}

/** Preferences from anything: the defaults, with each valid value on top. */
export function parsePreferences(v: unknown): Preferences {
	const out: Record<string, unknown> = { ...DEFAULT_PREFERENCES };
	if (v && typeof v === 'object') {
		for (const key of Object.keys(DEFAULT_PREFERENCES) as Key[]) {
			const x = (v as Record<string, unknown>)[key];
			if (x !== undefined && valid(key, x)) out[key] = x;
		}
	}
	const p = out as unknown as Preferences;
	// Items an older order leaves out follow in their usual order.
	// An item the saved order leaves out, such as one added since, goes
	// just after the item before it in the first order, as the server has it.
	const order = [...p.sidebarOrder];
	SIDEBAR_ITEMS.forEach((item, i) => {
		if (order.includes(item)) return;
		const before = SIDEBAR_ITEMS.slice(0, i).findLast((x) => order.includes(x));
		order.splice(before ? order.indexOf(before) + 1 : 0, 0, item);
	});
	p.sidebarOrder = order;
	p.shortcuts = cleanShortcuts(p.shortcuts);
	return p;
}

const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

const STORAGE_KEY = 'mila.preferences';
// Before preferences synced, the appearance ones lived here alone.
const LEGACY_KEY = 'mila.appearance';

interface Stored {
	preferences: Preferences;
	unsaved: Key[];
	/** Whose unsaved changes they are; null for ones made before anyone signed in. */
	user: string | null;
}

function read(): Stored {
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (raw) {
			const v = JSON.parse(raw) as Partial<Stored>;
			const unsaved = (v.unsaved ?? []).filter((k) => k in DEFAULT_PREFERENCES);
			const user = typeof v.user === 'string' ? v.user : null;
			return { preferences: parsePreferences(v.preferences), unsaved, user };
		}
		// Appearance chosen on this device before it synced counts as a
		// change not yet saved, so the first sign-in keeps it.
		const legacy = JSON.parse(localStorage.getItem(LEGACY_KEY) ?? 'null');
		if (legacy && typeof legacy === 'object') {
			const preferences = parsePreferences({ ...legacy, theme: legacy.character });
			const unsaved = (['theme', 'scheme', 'accentHue', 'density', 'fontSize'] as Key[]).filter(
				(k) => !same(preferences[k], DEFAULT_PREFERENCES[k])
			);
			return { preferences, unsaved, user: null };
		}
	} catch {
		// unreadable: start from the defaults
	}
	return { preferences: { ...DEFAULT_PREFERENCES }, unsaved: [], user: null };
}

/**
 * Whether the server turned the values down, rather than failing to take
 * them for now (offline, restarting behind a proxy, busy, signed out).
 */
function refused(err: unknown): boolean {
	return err instanceof ApiError && (err.status === 400 || err.status === 422);
}

/** How long to wait after a change before saving it, so a slider saves once. */
const SAVE_DELAY = 500;

class PreferencesState {
	current = $state<Preferences>(DEFAULT_PREFERENCES);
	#unsaved = new Set<Key>();
	// Whose the unsaved changes are, so they never go to someone else's account.
	#owner: string | null = null;
	// The signed-in user, while connected.
	#user: string | null = null;
	#timer: ReturnType<typeof setTimeout> | undefined;
	#saving: Promise<void> | null = null;
	#refused = false;
	// A save failed for now: wait for a reconnect or a new change rather
	// than trying again and again.
	#waiting = false;

	constructor() {
		const stored = read();
		this.current = stored.preferences;
		this.#unsaved = new Set(stored.unsaved);
		this.#owner = stored.user;
	}

	#write() {
		try {
			const stored: Stored = {
				preferences: this.current,
				unsaved: [...this.#unsaved],
				user: this.#unsaved.size ? this.#owner : null
			};
			localStorage.setItem(STORAGE_KEY, JSON.stringify(stored));
		} catch {
			// private mode or storage blocked: preferences last for this visit
		}
	}

	/** Changes preferences here at once, and saves them for the user's other devices. */
	set(patch: Partial<Preferences>) {
		const next = parsePreferences({ ...this.current, ...patch });
		for (const key of Object.keys(patch) as Key[]) this.#unsaved.add(key);
		if (this.#user) this.#owner = this.#user;
		this.current = next;
		this.#waiting = false;
		this.#write();
		this.#schedule();
	}

	/**
	 * Takes the server's copy, keeping what was changed here and not yet
	 * saved. Copies only from the server: a cached one may be older than
	 * what this device already has.
	 */
	adopt(server: Preferences) {
		const next = parsePreferences(server);
		for (const key of this.#unsaved) (next as Record<Key, unknown>)[key] = this.current[key];
		if (!same(next, this.current)) this.current = next;
		this.#write();
	}

	/**
	 * Starts saving for a signed-in user, adopting their preferences when
	 * they come from the server (null when the app opened offline).
	 * Changes someone else left unsaved here are dropped, never sent to
	 * this user's account.
	 */
	connect(userId: string, server: Preferences | null) {
		if (this.#owner !== null && this.#owner !== userId) this.#unsaved.clear();
		this.#user = userId;
		this.#owner = userId;
		this.#waiting = false;
		if (server) this.adopt(server);
		else this.#write();
		this.#schedule(0);
	}

	/**
	 * Stops saving, and forgets what was not saved, which belonged to the
	 * user who signed out. The look stays, for the sign-in page.
	 */
	disconnect() {
		this.#user = null;
		this.#owner = null;
		this.#waiting = false;
		clearTimeout(this.#timer);
		this.#unsaved.clear();
		this.#write();
	}

	#schedule(delay = SAVE_DELAY) {
		clearTimeout(this.#timer);
		if (this.#user && this.#unsaved.size && !this.#waiting) {
			this.#timer = setTimeout(() => void this.save(), delay);
		}
	}

	/**
	 * Sends what changed here. Offline, or when the server cannot take it
	 * now, it stays unsaved until the next reconnect or change; a value the
	 * server refuses is dropped for the server's.
	 */
	async save(): Promise<void> {
		if (this.#saving) return this.#saving;
		if (!this.#user || !this.#unsaved.size) return;
		const keys = [...this.#unsaved];
		const sent = Object.fromEntries(keys.map((k) => [k, this.current[k]])) as Partial<Preferences>;
		this.#saving = (async () => {
			try {
				const saved = await call(api.PATCH('/me/settings/preferences', { body: sent }));
				// Changed again while this was on its way: that change still counts.
				for (const k of keys) if (same(this.current[k], sent[k])) this.#unsaved.delete(k);
				this.adopt(saved);
			} catch (err) {
				if (!refused(err)) {
					this.#waiting = true;
					return;
				}
				console.warn('Mila: preferences refused:', err instanceof Error ? err.message : err);
				for (const k of keys) this.#unsaved.delete(k);
				this.#write();
				this.#refused = true;
			} finally {
				this.#saving = null;
			}
		})();
		await this.#saving;
		this.#schedule();
		if (this.#refused) {
			this.#refused = false;
			await this.refresh();
		}
	}

	/** Saves anything unsaved, then takes the server's copy: on reconnect, and when told they changed. */
	async refresh() {
		if (!this.#user) return;
		this.#waiting = false;
		await this.save();
		try {
			const me = await call(api.GET('/me'));
			this.adopt(me.preferences);
		} catch {
			// offline: the next reconnect tries again
		}
	}
}

export const preferences = new PreferencesState();
