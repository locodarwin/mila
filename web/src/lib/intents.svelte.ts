// What a right-click menu asks of a page it opens (contextMenu.svelte.ts): rename a
// project or view, share it, save it as a template. The page does it once it has
// drawn, as if the button there had been pressed, and the ask is used up.

export type IntentKind = 'project' | 'view' | 'task';

class Intents {
	#pending = $state.raw<{ kind: IntentKind; id: string; what: string } | null>(null);

	ask(kind: IntentKind, id: string, what: string) {
		this.#pending = { kind, id, what };
	}

	/** What's asked of this thing, once: the page that reads it does it. */
	take(kind: IntentKind, id: string | undefined): string | null {
		const p = this.#pending;
		if (!p || !id || p.kind !== kind || p.id !== id) return null;
		this.#pending = null;
		return p.what;
	}

	/** Read in an effect, so that it runs again when something is asked. */
	get pending() {
		return this.#pending;
	}
}

export const intents = new Intents();
