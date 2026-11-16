// Touch gestures, as Svelte actions on pointer events: swiping a task row
// sideways (right completes, left schedules) and pulling the phone's task
// sheet down to close it. Only touch and pen start them; a mouse drags
// text or cards as usual. Neither is ever the only way to do something:
// the row has its checkbox and schedule button, the sheet a Close button.

/** Movement before a gesture picks an axis; under it, a tap is still a tap. */
export const SLOP = 10;
/** Touches this close to the screen's side edges belong to the OS's back gesture. */
export const EDGE = 20;

export type Swipe = 'right' | 'left' | null;

/**
 * Whether a horizontal swipe counts once the finger lifts: far enough
 * (a third of the row, at most 120px), or a quick flick past 40px.
 */
export function swipeOutcome(dx: number, velocity: number, width: number): Swipe {
	const far = Math.abs(dx) >= Math.min(width / 3, 120);
	const flick =
		Math.abs(dx) >= 40 && Math.abs(velocity) >= 0.5 && Math.sign(velocity) === Math.sign(dx);
	if (!far && !flick) return null;
	return dx > 0 ? 'right' : 'left';
}

/** How far the row follows the finger: one to one, then with growing resistance. */
export function rubberBand(dx: number, width: number): number {
	const limit = width * 0.45;
	const a = Math.abs(dx);
	if (a <= limit) return dx;
	return Math.sign(dx) * (limit + (a - limit) / (1 + (a - limit) / 80));
}

/** Whether a pull down the sheet closes it: a quarter of the screen, or a flick. */
export function pullCloses(dy: number, velocity: number, height: number): boolean {
	return dy >= height / 4 || (dy >= 40 && velocity >= 0.5);
}

function byTouch(e: PointerEvent): boolean {
	return e.pointerType === 'touch' || e.pointerType === 'pen';
}

/**
 * A click that ends a gesture would open the row's task; this eats it. Only
 * that one: the next touch, or a moment, and clicks count again.
 */
function swallowNextClick(node: HTMLElement) {
	const eat = (e: Event) => {
		e.preventDefault();
		e.stopPropagation();
		stop();
	};
	const stop = () => {
		node.removeEventListener('click', eat, { capture: true });
		node.removeEventListener('pointerdown', stop, { capture: true });
		clearTimeout(timer);
	};
	const timer = setTimeout(stop, 400);
	node.addEventListener('click', eat, { capture: true });
	node.addEventListener('pointerdown', stop, { capture: true });
}

export interface SwipeOptions {
	/** Swipes are off (a phone picking tasks to select). */
	off?: boolean;
	onright?: () => void;
	onleft?: () => void;
	/** Called as the row moves: its offset, and which way it would go if let go now. */
	onmove?: (dx: number, outcome: Swipe) => void;
}

/**
 * Lets a row be swiped sideways. The row itself (or what onmove moves) shows
 * the offset; the action works out the gesture: the axis is locked after a
 * few pixels, so a vertical drag scrolls the list as before, and touches at
 * the screen's edge are left to the OS.
 */
export function swipe(node: HTMLElement, options: SwipeOptions) {
	let opts = options;
	let start: { x: number; y: number; id: number } | null = null;
	let locked = false;
	let last = { x: 0, t: 0, v: 0 };
	let dx = 0;

	node.style.touchAction = 'pan-y';

	function reset() {
		start = null;
		locked = false;
		dx = 0;
		opts.onmove?.(0, null);
	}

	function down(e: PointerEvent) {
		if (opts.off || !byTouch(e) || !e.isPrimary) return;
		if (e.clientX < EDGE || e.clientX > innerWidth - EDGE) return;
		start = { x: e.clientX, y: e.clientY, id: e.pointerId };
		last = { x: e.clientX, t: e.timeStamp, v: 0 };
	}

	function move(e: PointerEvent) {
		if (!start || e.pointerId !== start.id) return;
		const mx = e.clientX - start.x;
		const my = e.clientY - start.y;
		if (!locked) {
			if (Math.abs(mx) < SLOP && Math.abs(my) < SLOP) return;
			if (Math.abs(my) >= Math.abs(mx)) {
				start = null; // a scroll
				return;
			}
			locked = true;
			try {
				node.setPointerCapture(e.pointerId);
			} catch {
				// a synthetic pointer: the events keep coming here anyway
			}
		}
		const dt = e.timeStamp - last.t;
		if (dt > 0) last = { x: e.clientX, t: e.timeStamp, v: (e.clientX - last.x) / dt };
		dx = mx;
		const width = node.offsetWidth || innerWidth;
		opts.onmove?.(rubberBand(dx, width), swipeOutcome(dx, last.v, width));
	}

	function up(e: PointerEvent) {
		if (!start || e.pointerId !== start.id) return;
		if (locked) {
			swallowNextClick(node);
			const outcome = swipeOutcome(dx, last.v, node.offsetWidth || innerWidth);
			reset();
			if (outcome === 'right') opts.onright?.();
			if (outcome === 'left') opts.onleft?.();
			return;
		}
		start = null;
	}

	function cancel(e: PointerEvent) {
		if (start && e.pointerId === start.id) reset();
	}

	node.addEventListener('pointerdown', down);
	node.addEventListener('pointermove', move);
	node.addEventListener('pointerup', up);
	node.addEventListener('pointercancel', cancel);
	return {
		update(next: SwipeOptions) {
			opts = next;
		},
		destroy() {
			node.removeEventListener('pointerdown', down);
			node.removeEventListener('pointermove', move);
			node.removeEventListener('pointerup', up);
			node.removeEventListener('pointercancel', cancel);
		}
	};
}

export interface PullOptions {
	/** Where a pull may start: a selector inside the node (the sheet's header). */
	handle: string;
	onclose: () => void;
}

/**
 * Lets the phone's task sheet be pulled down to close. The sheet follows
 * the finger; let go far enough and it closes from where it is, or else it
 * springs back.
 */
export function pullDown(node: HTMLElement, options: PullOptions) {
	let opts = options;
	let start: { y: number; x: number; id: number } | null = null;
	let pulling = false;
	let last = { y: 0, t: 0, v: 0 };
	let dy = 0;

	function down(e: PointerEvent) {
		if (!byTouch(e) || !e.isPrimary) return;
		if (!(e.target as Element | null)?.closest(opts.handle)) return;
		start = { y: e.clientY, x: e.clientX, id: e.pointerId };
		last = { y: e.clientY, t: e.timeStamp, v: 0 };
	}

	function move(e: PointerEvent) {
		if (!start || e.pointerId !== start.id) return;
		const my = e.clientY - start.y;
		if (!pulling) {
			if (Math.abs(my) < SLOP && Math.abs(e.clientX - start.x) < SLOP) return;
			if (my <= 0 || Math.abs(e.clientX - start.x) > my) {
				start = null;
				return;
			}
			pulling = true;
			node.style.transition = 'none';
			try {
				node.setPointerCapture(e.pointerId);
			} catch {
				// a synthetic pointer
			}
		}
		const dt = e.timeStamp - last.t;
		if (dt > 0) last = { y: e.clientY, t: e.timeStamp, v: (e.clientY - last.y) / dt };
		dy = Math.max(0, my);
		node.style.transform = `translateY(${dy}px)`;
	}

	function up(e: PointerEvent) {
		if (!start || e.pointerId !== start.id) return;
		start = null;
		if (!pulling) return;
		pulling = false;
		swallowNextClick(node);
		node.style.transition = '';
		if (pullCloses(dy, last.v, node.offsetHeight || innerHeight)) {
			// The closing slide carries on from here (fly keeps the transform).
			opts.onclose();
		} else {
			node.style.transform = '';
		}
		dy = 0;
	}

	function cancel(e: PointerEvent) {
		if (!start || e.pointerId !== start.id) return;
		start = null;
		pulling = false;
		node.style.transition = '';
		node.style.transform = '';
	}

	node.addEventListener('pointerdown', down);
	node.addEventListener('pointermove', move);
	node.addEventListener('pointerup', up);
	node.addEventListener('pointercancel', cancel);
	return {
		update(next: PullOptions) {
			opts = next;
		},
		destroy() {
			node.removeEventListener('pointerdown', down);
			node.removeEventListener('pointermove', move);
			node.removeEventListener('pointerup', up);
			node.removeEventListener('pointercancel', cancel);
		}
	};
}
