// Where a page's comment threads sit in its text: what
// the panel beside the page and the editor's highlights share. Small, since the
// page loads it; the highlights themselves are commentMarks.ts, which loads with
// the editor, and the panel is PageComments.svelte.

/** A thread to draw in the text, from its row. */
export interface Pin {
	id: string;
	anchor: string;
	quote: string;
}

/** Words picked for a new thread, until its first comment is sent or dropped. */
export interface Draft {
	anchor: string;
	quote: string;
}

/** The thread being written: a highlight like the others, drawn under this ID. */
export const DRAFT = 'draft';

/** A transaction with this meta set draws the highlights again. */
export const REDRAW_PINS = 'redrawCommentPins';

export class CommentPins {
	/** The open threads to draw. */
	pins = $state.raw<Pin[]>([]);
	/** The thread picked, in the panel and the text alike. */
	active = $state<string | null>(null);
	draft = $state.raw<Draft | null>(null);
	/** The threads found in the text, in the order they come. */
	order = $state.raw<string[]>([]);
	/** The threads whose words are gone from the text. */
	// eslint-disable-next-line svelte/prefer-svelte-reactivity -- replaced whole, never changed
	gone = $state.raw<ReadonlySet<string>>(new Set());
	/** Whether the text has been drawn, so order and gone can be believed. */
	ready = $state(false);

	/** Starts a thread on the selection; set while the editor is open. */
	begin: (() => boolean) | null = null;
	/** Brings a thread's words into view; set while the editor is open. */
	reveal: ((id: string) => void) | null = null;

	/** Opens the panel, for a thread picked in the text or one being begun. */
	onshow: () => void;

	constructor(onshow: () => void = () => {}) {
		this.onshow = onshow;
	}

	/** Forgets what belongs to the page shown, for another. */
	reset() {
		this.active = null;
		this.draft = null;
		this.order = [];
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- replaced whole, never changed
		this.gone = new Set();
		this.ready = false;
	}
}
