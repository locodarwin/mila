// The theme editor's undo and redo: every version of the theme while the
// editor is open. A change is one step, however it was made: a slider
// moved shows each position at once (live) but becomes one step when it
// is let go (final), and only final versions are saved. Versions are
// compared as canonical JSON, so the server's copy of the same theme is
// the same.
import { canonical } from './themes.svelte';
import type { ThemeDoc } from './document';

/** How many steps back undo can go. */
export const DEPTH = 200;

export class ThemeHistory {
	/** The theme as it is now, live changes included. */
	doc = $state.raw<ThemeDoc>(null!);
	#past = $state.raw<ThemeDoc[]>([]);
	#future = $state.raw<ThemeDoc[]>([]);
	// The version before a live change, which its final change makes a step of.
	#from: ThemeDoc | null = null;
	readonly #opened: ThemeDoc;
	readonly #save: (doc: ThemeDoc) => void;

	/** save: called with each final version. */
	constructor(doc: ThemeDoc, save: (doc: ThemeDoc) => void) {
		this.doc = doc;
		this.#opened = doc;
		this.#save = save;
	}

	get canUndo(): boolean {
		return this.#past.length > 0 || this.#from !== null;
	}

	get canRedo(): boolean {
		return this.#future.length > 0;
	}

	/** Whether the theme differs from when the editor opened. */
	get changed(): boolean {
		return canonical(this.doc) !== canonical(this.#opened);
	}

	/** Whether a live change is under way (a slider held). */
	get live(): boolean {
		return this.#from !== null;
	}

	/**
	 * A new version. Final: a step of its own (with any live change before
	 * it), saved. Live: shown at once, not saved or a step yet.
	 */
	change(next: ThemeDoc, final = true) {
		this.#from ??= this.doc;
		this.doc = next;
		if (!final) return;
		const from = this.#from;
		this.#from = null;
		if (canonical(from) === canonical(next)) {
			// Moved and put back: nothing to keep, but a live change still
			// shows the saved version again.
			this.doc = from;
			return;
		}
		this.#past = [...this.#past, from].slice(-DEPTH);
		this.#future = [];
		this.#save(next);
	}

	/** Ends a live change where it stands, as a step (focus left a slider mid-drag). */
	settle() {
		if (this.#from) this.change(this.doc);
	}

	/** Drops a live change: back to the version before it, which is still the one saved. */
	cancel() {
		if (!this.#from) return;
		this.doc = this.#from;
		this.#from = null;
	}

	undo(): boolean {
		this.settle();
		const prev = this.#past.at(-1);
		if (!prev) return false;
		this.#past = this.#past.slice(0, -1);
		this.#future = [...this.#future, this.doc];
		this.doc = prev;
		this.#save(prev);
		return true;
	}

	redo(): boolean {
		this.settle();
		const next = this.#future.at(-1);
		if (!next) return false;
		this.#future = this.#future.slice(0, -1);
		this.#past = [...this.#past, this.doc];
		this.doc = next;
		this.#save(next);
		return true;
	}

	/** Back to the theme as it was when the editor opened, as one step undo takes back. */
	revert() {
		this.settle();
		this.change(this.#opened);
	}

	/**
	 * A version from elsewhere (another device saved it): shown, and a step
	 * undo can take back, but not saved again. Ignored mid-change.
	 */
	external(doc: ThemeDoc) {
		if (this.#from || canonical(doc) === canonical(this.doc)) return;
		this.#past = [...this.#past, this.doc].slice(-DEPTH);
		this.#future = [];
		this.doc = doc;
	}
}
