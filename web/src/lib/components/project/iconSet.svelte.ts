// The project icons, loaded the first time a project shows or picks one.
import type { Component } from 'svelte';

class IconSet {
	icons = $state<Record<string, Component> | null>(null);
	#loading: Promise<void> | null = null;

	load(): Promise<void> {
		this.#loading ??= import('./icons')
			.then((m) => {
				this.icons = m.PROJECT_ICONS;
			})
			.catch(() => {
				// offline before the icons were ever loaded: the dot stands in
				this.#loading = null;
			});
		return this.#loading;
	}

	/**
	 * The icon of a name, once loaded. Only the set's own names count, so a
	 * project named "lucide:__proto__" or "lucide:toString" shows the dot.
	 */
	get(name: string): Component | undefined {
		const icons = this.icons;
		return icons && Object.hasOwn(icons, name) ? icons[name] : undefined;
	}
}

export const iconSet = new IconSet();

/** The prefix of an icon that names a Lucide icon rather than an emoji. */
export const LUCIDE = 'lucide:';

/** An emoji typed into a field: its first character as people see it, or "". */
export function firstEmoji(text: string): string {
	const t = text.trim();
	if (!t) return '';
	const first = [...new Intl.Segmenter(undefined, { granularity: 'grapheme' }).segment(t)][0];
	const g = first?.segment ?? '';
	// Letters and digits are not icons; pictographs, flags and keycaps are.
	return /\p{Extended_Pictographic}|\p{Regional_Indicator}|⃣/u.test(g) && g.length <= 32 ? g : '';
}
