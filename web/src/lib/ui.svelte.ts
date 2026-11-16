// Which overlay is open: quick add, the command palette, or the shortcut
// help. One at a time.
class UI {
	overlay = $state<null | 'quickadd' | 'palette' | 'help'>(null);
	/** A task whose Log time form should open, asked for from the palette. */
	logTime = $state<string | null>(null);

	open(which: 'quickadd' | 'palette' | 'help') {
		this.overlay = which;
	}

	close() {
		this.overlay = null;
	}
}

export const ui = new UI();

/** True when a key press belongs to whatever the person is typing in. */
export function typing(e: KeyboardEvent): boolean {
	const t = e.target as HTMLElement | null;
	return Boolean(t?.closest('input, textarea, select, [contenteditable="true"]'));
}
