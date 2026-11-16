// Appearance: theme character or custom theme, light/dark scheme, accent,
// density and font size. The values are preferences
// (preferences.svelte.ts), which sync with the user; this applies them to
// the page. apply() also leaves what the pre-paint script in app.html
// needs under STORAGE_KEY, and the two must stay in step.
//
// A custom theme is drawn from a stylesheet built for it (custom.ts),
// which is kept with the rest for the pre-paint script, so the next load
// shows it from the first frame. Building one loads the registry and the
// serializer, so that happens only when nothing built is kept for it.

import { preferences } from '../preferences.svelte';
import { refreshMotion } from '../motion';
import { accentColors, pageColor, type AccentColors, type Scheme } from './color';
import type { ThemeDoc } from './document';
import { untrack } from 'svelte';
import { presets } from './presets.svelte';
import { canonical, themes } from './themes.svelte';
import { workspaceThemes } from './workspaceThemes.svelte';

export const CHARACTERS = ['standard', 'calm', 'playful', 'contrast'] as const;
export const SCHEMES = ['system', 'light', 'dark'] as const;
export const DENSITIES = ['comfortable', 'dense'] as const;
export const FONT_SIZES = ['s', 'm', 'l', 'xl', 'xxl'] as const;

export type Character = (typeof CHARACTERS)[number];
export type SchemeSetting = (typeof SCHEMES)[number];
export type Density = (typeof DENSITIES)[number];
export type FontSize = (typeof FONT_SIZES)[number];

export interface Appearance {
	character: Character;
	scheme: SchemeSetting;
	/** A custom accent hue, or null for the character's own. */
	accentHue: number | null;
	density: Density;
	fontSize: FontSize;
	/** The custom theme in use (its id), or null for the character alone. */
	customTheme: string | null;
}

/**
 * Raise when what a built theme's CSS means changes (tokens.css renames,
 * css.ts writes differently): kept CSS of another version is not used,
 * and the base theme shows for a frame until it is built again. The
 * pre-paint script in app.html checks the same number.
 */
export const CACHE_VERSION = 1;

/** A custom theme as built for the page, kept for the pre-paint script. */
export interface CustomCache {
	v: number;
	id: string;
	/** What it was built from (themeKey). */
	key: string;
	base: Character;
	css: string;
	/** The accent hue to set on <html>, or null to leave the theme's own. */
	hue: number | null;
	page: Record<Scheme, string>;
	accent: Record<Scheme, AccentColors>;
}

/** What the pre-paint script reads: the appearance, and the accent worked out for each scheme. */
export const STORAGE_KEY = 'mila.appearance';

export const DEFAULTS: Appearance = {
	character: 'standard',
	scheme: 'system',
	accentHue: null,
	density: 'comfortable',
	fontSize: 'm',
	customTheme: null
};

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

function oneOf<T extends string>(list: readonly T[], v: unknown, fallback: T): T {
	return list.includes(v as T) ? (v as T) : fallback;
}

/** Reads stored settings, ignoring anything missing or malformed. */
export function parse(raw: string | null): Appearance {
	let v: Record<string, unknown> = {};
	try {
		v = JSON.parse(raw ?? '{}') ?? {};
	} catch {
		// fall back to defaults
	}
	const hue =
		typeof v.accentHue === 'number' && v.accentHue >= 0 && v.accentHue < 360 ? v.accentHue : null;
	return {
		character: oneOf(CHARACTERS, v.character, DEFAULTS.character),
		scheme: oneOf(SCHEMES, v.scheme, DEFAULTS.scheme),
		accentHue: hue,
		density: oneOf(DENSITIES, v.density, DEFAULTS.density),
		fontSize: oneOf(FONT_SIZES, v.fontSize, DEFAULTS.fontSize),
		customTheme:
			typeof v.customTheme === 'string' && UUID.test(v.customTheme) ? v.customTheme : null
	};
}

/** The custom theme kept for the pre-paint script, if it is of this version. */
export function readCustom(raw: string | null): CustomCache | null {
	try {
		const c = (JSON.parse(raw ?? 'null') ?? {}).custom as CustomCache | undefined;
		if (c && c.v === CACHE_VERSION && typeof c.css === 'string' && typeof c.key === 'string')
			return c;
	} catch {
		// unreadable: build it again
	}
	return null;
}

function writeStorage(a: Appearance, accent: Record<Scheme, AccentColors>, custom?: CustomCache) {
	try {
		localStorage.setItem(STORAGE_KEY, JSON.stringify({ ...a, accent, custom }));
	} catch {
		// private mode or storage blocked: the page paints with the defaults first
	}
}

function storedRaw(): string | null {
	try {
		return localStorage.getItem(STORAGE_KEY);
	} catch {
		return null;
	}
}

/**
 * A short fingerprint of what a built theme depends on: the document and
 * the person's accent hue. FNV-1a, which is plenty to tell versions apart.
 */
export function themeKey(doc: ThemeDoc, accentHue: number | null): string {
	// Keys in order, so the server's copy of a theme keys as this device's does.
	const text = `${CACHE_VERSION}|${accentHue}|${canonical(doc)}`;
	let h = 0x811c9dc5;
	for (let i = 0; i < text.length; i++) {
		h ^= text.charCodeAt(i);
		h = Math.imul(h, 0x01000193);
	}
	return (h >>> 0).toString(16) + text.length.toString(16);
}

/** The person's hue, unless the theme's accent is set by hand. */
const hueFor = (doc: ThemeDoc, accentHue: number | null) =>
	'manual' in doc.accent ? null : accentHue;

/** A preview for the style guide: settings, and a theme document to show instead of the person's. */
export type Preview = Partial<Appearance> & { doc?: ThemeDoc | null };

const darkQuery =
	typeof matchMedia === 'function' ? matchMedia('(prefers-color-scheme: dark)') : null;

class AppearanceState {
	// A look tried on the style guide: shown, never saved or synced.
	#preview = $state<Preview | null>(null);

	current: Appearance = $derived.by(() => {
		const preview: Preview = { ...this.#preview };
		delete preview.doc;
		return {
			character: preferences.current.theme,
			scheme: preferences.current.scheme,
			accentHue: preferences.current.accentHue,
			density: preferences.current.density,
			fontSize: preferences.current.fontSize,
			customTheme: preferences.current.customTheme ?? null,
			...preview
		};
	});

	/**
	 * The custom theme shown, or null for a built-in: the one in use, the
	 * person's own, a workspace's or one made from Omarchy's, while this
	 * device has it (a theme
	 * deleted, or not loaded yet on a new device, falls back to its base,
	 * which `theme` holds).
	 */
	custom: ThemeDoc | null = $derived.by(() => {
		const p = this.#preview;
		if (p && 'doc' in p) return p.doc ?? null;
		if (p?.character) return null;
		const id = this.current.customTheme;
		return themes.get(id) ?? workspaceThemes.get(id) ?? presets.get(id) ?? null;
	});

	/**
	 * The id of a custom theme that was not shown because its text or focus
	 * ring would be unreadable (below 3:1 on the page); the last good look
	 * stays. The editor says so.
	 */
	notApplied = $state<string | null>(null);

	// The custom theme last built, kept so the next apply() reuses it.
	#built: CustomCache | null = readCustom(storedRaw());
	// Each build asked for; one finishing after a newer one was asked for is dropped.
	#ticket = 0;
	#systemDark = $state(darkQuery?.matches ?? false);

	/** The scheme in effect after resolving "system". */
	scheme: Scheme = $derived(
		this.current.scheme === 'system' ? (this.#systemDark ? 'dark' : 'light') : this.current.scheme
	);

	constructor() {
		darkQuery?.addEventListener('change', (e) => (this.#systemDark = e.matches));
	}

	/**
	 * Changes the appearance preferences. Choosing a character leaves any
	 * custom theme, so "Theme: Standard" always gets back to a built-in.
	 */
	set(patch: Partial<Appearance>) {
		const { character, ...rest } = patch;
		preferences.set(character ? { ...rest, theme: character, customTheme: null } : rest);
	}

	/**
	 * Shows a look on this page only, for the style guide: the user's
	 * preferences stay as they are. `doc` shows a theme document instead
	 * of the person's own. null goes back to them.
	 */
	preview(patch: Preview | null) {
		this.#preview = patch && { ...this.#preview, ...patch };
	}

	/**
	 * Writes the settings onto <html>, and keeps them for the next
	 * pre-paint unless they are only a preview.
	 */
	apply() {
		const root = document.documentElement;
		const a = this.current;
		const custom = this.custom;
		root.dataset.theme = custom ? custom.base : a.character;
		root.dataset.scheme = this.scheme;
		root.dataset.density = a.density;
		root.dataset.fontSize = a.fontSize;
		if (custom) {
			this.#applyCustom(custom, a);
			return;
		}
		this.#ticket++;
		this.#notApplied(null);
		this.#uninstall();
		this.#applyBuiltin(a, !!this.#preview);
	}

	/** Shows a built-in theme, the character `a` names, and keeps it for the pre-paint. */
	#applyBuiltin(a: Appearance, preview: boolean) {
		const root = document.documentElement;
		root.dataset.theme = a.character;
		// The character's default hue lives in CSS; read it back unless the
		// user picked one.
		if (a.accentHue === null) root.style.removeProperty('--accent-hue');
		else root.style.setProperty('--accent-hue', String(a.accentHue));
		const hue = Number(getComputedStyle(root).getPropertyValue('--accent-hue')) || 262;
		const accent = {
			light: accentColors(hue, 'light', a.character),
			dark: accentColors(hue, 'dark', a.character)
		};
		const now = accent[this.scheme];
		root.style.setProperty('--accent-l', String(now.l));
		root.style.setProperty('--accent-c', String(now.c));
		root.style.setProperty('--accent-text-l', String(now.textL));
		root.style.setProperty('--accent-text-c', String(now.textC));

		// The browser's own bars (and an installed app's title bar) take the
		// page's color, in the scheme shown rather than the OS's.
		const page = pageColor(a.character, this.scheme);
		for (const meta of document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]'))
			meta.content = page;
		refreshMotion();

		if (!preview) writeStorage(a, accent);
	}

	/** Sets notApplied, unless it already says that; apply() runs in an effect, which must not depend on it. */
	#notApplied(id: string | null, unless?: string) {
		untrack(() => {
			if (this.notApplied !== id && this.notApplied !== (unless ?? null)) this.notApplied = id;
		});
	}

	#applyCustom(doc: ThemeDoc, a: Appearance) {
		const key = themeKey(doc, a.accentHue);
		const kept = this.#built;
		// A refusal of another theme no longer applies.
		this.#notApplied(null, doc.id);
		if (kept && kept.id === doc.id && kept.key === key) {
			this.#ticket++;
			this.#notApplied(null);
			this.#install(kept);
			if (!this.#preview) writeStorage(a, kept.accent, kept);
			return;
		}
		// Meanwhile the last good look stays: an earlier version of this
		// theme, or what was shown before.
		const ticket = ++this.#ticket;
		const preview = !!this.#preview;
		void import('./custom')
			.then(({ build }) => {
				if (ticket !== this.#ticket) return;
				let built;
				try {
					built = build(doc, { accentHue: hueFor(doc, a.accentHue) });
				} catch (err) {
					// Unreadable: the base, as for a theme that is gone.
					console.warn('Mila: theme not shown:', err instanceof Error ? err.message : err);
					this.#uninstall();
					return;
				}
				if (!built.floors.light || !built.floors.dark) {
					this.notApplied = doc.id;
					// An earlier version of this theme stays; with none, its
					// base shows, and is what the next load paints.
					if (document.documentElement.dataset.customTheme !== doc.id) {
						this.#uninstall();
						this.#applyBuiltin({ ...a, character: doc.base }, preview);
					}
					return;
				}
				this.notApplied = null;
				const c: CustomCache = {
					v: CACHE_VERSION,
					id: doc.id,
					key,
					base: doc.base,
					css: built.css,
					hue: hueFor(doc, a.accentHue),
					page: built.page,
					accent: built.accent
				};
				this.#built = c;
				this.#install(c);
				if (!preview) writeStorage(a, c.accent, c);
			})
			.catch(() => {
				// The lazy part failed to load (a new build is out): update.svelte.ts offers a reload.
			});
	}

	/** Shows a built custom theme: its stylesheet, its accent and its page color. */
	#install(c: CustomCache) {
		const root = document.documentElement;
		let style = document.getElementById(STYLE_ID) as HTMLStyleElement | null;
		if (!style) {
			style = document.createElement('style');
			style.id = STYLE_ID;
			document.head.append(style);
		}
		if (style.textContent !== c.css) style.textContent = c.css;
		root.dataset.theme = c.base;
		root.dataset.customTheme = c.id;
		if (c.hue === null) root.style.removeProperty('--accent-hue');
		else root.style.setProperty('--accent-hue', String(c.hue));
		const now = c.accent[this.scheme];
		root.style.setProperty('--accent-l', String(now.l));
		root.style.setProperty('--accent-c', String(now.c));
		root.style.setProperty('--accent-text-l', String(now.textL));
		root.style.setProperty('--accent-text-c', String(now.textC));
		for (const meta of document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]'))
			meta.content = c.page[this.scheme];
		refreshMotion();
	}

	/** Goes back to the built-in theme alone. */
	#uninstall() {
		document.getElementById(STYLE_ID)?.remove();
		delete document.documentElement.dataset.customTheme;
	}
}

/** The custom theme's stylesheet, which the pre-paint script makes too. */
const STYLE_ID = 'mila-theme';

export const appearance = new AppearanceState();
