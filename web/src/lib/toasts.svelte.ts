// Short messages at the bottom of the screen: confirmations, errors, and
// the undo offer that replaces confirmation dialogs.
//
// Each goes away on its own, but not while the pointer or keyboard focus
// is on the toasts (WCAG 2.2.1): reaching an Undo button takes time. A
// sticky one ("Update available, Reload") stays until it is acted on or
// closed.

/** A button on a toast other than Undo, such as Reload. */
export interface ToastAction {
	label: string;
	run: () => void;
}

export interface ToastItem {
	id: number;
	message: string;
	tone: 'info' | 'error';
	onundo?: () => void;
	action?: ToastAction;
	/** Stays until acted on; with closable, it can also be closed. */
	sticky?: boolean;
	closable?: boolean;
}

/** How long each kind stays: an offer to undo and an error are given longer to read. */
export const TOAST_MS = { info: 5000, undo: 8000, error: 10000 };

let next = 1;

interface Clock {
	left: number;
	started: number;
	timer?: ReturnType<typeof setTimeout>;
}

class Toasts {
	items = $state<ToastItem[]>([]);
	#clocks = new Map<number, Clock>();
	#paused = false;

	show(
		message: string,
		options: {
			tone?: 'info' | 'error';
			onundo?: () => void;
			action?: ToastAction;
			sticky?: boolean;
			closable?: boolean;
		} = {}
	): number {
		const item: ToastItem = {
			id: next++,
			message,
			tone: options.tone ?? 'info',
			onundo: options.onundo,
			action: options.action,
			sticky: options.sticky,
			closable: options.closable
		};
		this.items = [...this.items, item];
		if (item.sticky) return item.id;
		const ms =
			item.tone === 'error'
				? TOAST_MS.error
				: item.onundo || item.action
					? TOAST_MS.undo
					: TOAST_MS.info;
		const clock: Clock = { left: ms, started: Date.now() };
		this.#clocks.set(item.id, clock);
		if (!this.#paused) this.#run(item.id, clock);
		return item.id;
	}

	error(message: string) {
		this.show(message, { tone: 'error' });
	}

	dismiss(id: number) {
		clearTimeout(this.#clocks.get(id)?.timer);
		this.#clocks.delete(id);
		this.items = this.items.filter((t) => t.id !== id);
	}

	/** Stops every toast's countdown, while someone is reading or reaching one. */
	pause() {
		if (this.#paused) return;
		this.#paused = true;
		const now = Date.now();
		for (const clock of this.#clocks.values()) {
			clearTimeout(clock.timer);
			clock.timer = undefined;
			clock.left -= now - clock.started;
		}
	}

	/** Starts them again, each with the time it had left. */
	resume() {
		if (!this.#paused) return;
		this.#paused = false;
		for (const [id, clock] of this.#clocks) this.#run(id, clock);
	}

	#run(id: number, clock: Clock) {
		clock.started = Date.now();
		clock.timer = setTimeout(() => this.dismiss(id), Math.max(clock.left, 1000));
	}
}

export const toasts = new Toasts();
