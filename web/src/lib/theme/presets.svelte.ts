// The themes Mila ships made from Omarchy's (omarchy.ts, written to
// presets/omarchy.json by scripts/omarchy-themes.mjs). Each is a theme
// document like a person's own, used the same way (the customTheme
// preference names it), but no one's to change: Settings, Appearance makes a
// copy to edit.
//
// This is the part every page loads, small: the list is its own file,
// loaded by the picker, Settings, Appearance and the palette, or when a
// device first finds one in use. The one in use is kept on the device,
// so it shows from the first frame; the list loading puts this version's
// copy in its place.
import { preferences } from '../preferences.svelte';
import type { Scheme } from './color';
import type { ThemeDoc } from './document';
import { isPreset } from './presetId';
import { looksLikeTheme } from './themes.svelte';

/** A theme as the app ships it: the scheme Omarchy has it in, or both. */
export interface Preset {
	slug: string;
	mode: Scheme | 'both';
	doc: ThemeDoc;
}

const STORAGE_KEY = 'mila.presetTheme';

function looksLikePreset(v: unknown): v is Preset {
	if (!v || typeof v !== 'object') return false;
	const p = v as Partial<Preset>;
	return (
		typeof p.slug === 'string' &&
		(p.mode === 'light' || p.mode === 'dark' || p.mode === 'both') &&
		looksLikeTheme(p.doc) &&
		isPreset(p.doc.id)
	);
}

function read(): Preset | null {
	try {
		const v: unknown = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? 'null');
		return looksLikePreset(v) ? v : null;
	} catch {
		return null;
	}
}

class PresetState {
	/** The one in use, as this device last had it. */
	// Raw: replaced whole, so a document stays the same object.
	kept = $state.raw<Preset | null>(read());

	/** Every one, once loaded. */
	list = $state.raw<Preset[]>([]);

	#loading: Promise<Preset[]> | null = null;

	/** One by id, from the list or the one kept. */
	get(id: string | null | undefined): ThemeDoc | undefined {
		if (!isPreset(id)) return undefined;
		return (this.list.find((p) => p.doc.id === id) ?? (this.kept?.doc.id === id ? this.kept : null))
			?.doc;
	}

	/** Which scheme Omarchy has a theme in. */
	modeOf(id: string): Preset['mode'] | undefined {
		return (this.list.find((p) => p.doc.id === id) ?? this.kept)?.mode;
	}

	/** Loads the list, once. */
	ensure(): Promise<Preset[]> {
		this.#loading ??= import('./presets/omarchy.json').then(
			(m) => (this.list = (m.default.themes as unknown[]).filter(looksLikePreset)),
			(err) => {
				// A new build is out and this one's file is gone: update.svelte.ts offers a reload.
				this.#loading = null;
				throw err;
			}
		);
		return this.#loading;
	}

	/**
	 * Follows the theme in use, in an effect: keeps this version's copy of
	 * a preset (one chosen on another device included), and lets go of it
	 * once another theme is in use.
	 */
	follow(id: string | null | undefined) {
		if (!isPreset(id)) {
			if (this.kept) this.#keep(null);
			return;
		}
		if (this.kept?.doc.id === id && this.list.length) return;
		void this.ensure()
			.then((list) => {
				const p = list.find((x) => x.doc.id === id);
				// One a newer version no longer ships: the base shows, as for a theme deleted.
				if (preferences.current.customTheme === id) this.#keep(p ?? null);
			})
			.catch(() => {});
	}

	/**
	 * Uses one, unless it would be unreadable (as themes.use). One Omarchy
	 * has in a single scheme switches to that scheme; the other still
	 * works. Returns why not, or null once it is in use.
	 */
	async use(id: string): Promise<string | null> {
		const list = await this.ensure();
		const p = list.find((x) => x.doc.id === id);
		if (!p) return 'That theme is gone.';
		const { floorsOf } = await import('./custom');
		const why = floorsOf(p.doc, { accentHue: preferences.current.accentHue });
		if (why) return why;
		this.#keep(p);
		preferences.set({
			customTheme: id,
			theme: p.doc.base,
			...(p.mode === 'both' ? {} : { scheme: p.mode })
		});
		return null;
	}

	#keep(p: Preset | null) {
		this.kept = p;
		try {
			if (p) localStorage.setItem(STORAGE_KEY, JSON.stringify(p));
			else localStorage.removeItem(STORAGE_KEY);
		} catch {
			// storage blocked: it loads with the list next time
		}
	}
}

export const presets = new PresetState();
