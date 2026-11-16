// One polite live region for things that change without moving focus: a
// task moved with the keyboard, how many tasks a filter leaves, the
// connection going and coming back. Toasts have their own regions
// (Toaster.svelte); this is for what needs no toast.

class Announcer {
	message = $state('');
	#timer: ReturnType<typeof setTimeout> | undefined;

	/**
	 * Says something once. The region is emptied first, so the same words
	 * twice in a row (two moves to the same column) are read twice. A
	 * longer wait suits something said as someone types: only the last
	 * one is read.
	 */
	say(message: string, wait = 50) {
		clearTimeout(this.#timer);
		this.message = '';
		this.#timer = setTimeout(() => (this.message = message), wait);
	}
}

export const announcer = new Announcer();

/** Shorthand for announcer.say. */
export function announce(message: string, wait?: number) {
	announcer.say(message, wait);
}
