// Long lists are drawn a screenful at a time: the first rows at once, then
// more as the reader scrolls near the end of what is drawn. A view of a
// thousand tasks so opens, and closes, as fast as one of fifty (the budget
// for switching views is 50 ms, docs/PLAN.md section 13). Headings keep
// the full counts, so nobody is misled about what is further down.

import { untrack } from 'svelte';

/** Rows drawn at once: more than a tall screen holds. */
export const FIRST = 60;
/** Rows added each time the end of the drawn ones comes near. */
export const STEP = 60;

/**
 * How many rows of a list to draw. Make one in a component's script and
 * put `use:growing.more` on an empty element after the list; it grows by
 * STEP whenever that element comes within a screen of view, and never
 * shrinks, so an edit to the list never takes rows away from under the
 * reader.
 */
export class Growing {
	limit = $state(FIRST);
	#total: () => number;

	constructor(total: () => number) {
		this.#total = total;
	}

	/** The action for the element after the list. */
	more = (node: HTMLElement) => {
		if (typeof IntersectionObserver === 'undefined') {
			this.limit = Infinity;
			return;
		}
		let frame = 0;
		// An observer speaks only when the element comes into view or leaves
		// it, so while it stays in view (a tall screen, or the reader at the
		// end) it is asked again, once the rows just drawn are on the page.
		const again = () => {
			cancelAnimationFrame(frame);
			frame = requestAnimationFrame(() => {
				watch.unobserve(node);
				watch.observe(node);
			});
		};
		const watch = new IntersectionObserver(
			([entry]) => {
				if (!entry?.isIntersecting || this.limit >= this.#total()) return;
				this.limit += STEP;
				again();
			},
			// A screen ahead, measured in what scrolls: a margin on the
			// window would be cut off by the scrolling page inside it.
			{ root: scroller(node), rootMargin: '0px 0px 100% 0px' }
		);
		watch.observe(node);
		// The list grew past what is drawn (a task added, or one synced in):
		// if the end is in view, the new rows must be drawn too.
		$effect(() => {
			if (this.#total() > untrack(() => this.limit)) again();
		});
		return {
			destroy() {
				cancelAnimationFrame(frame);
				watch.disconnect();
			}
		};
	};
}

/** The nearest element that scrolls around a node, or null for the window. */
function scroller(node: HTMLElement): HTMLElement | null {
	for (let el = node.parentElement; el; el = el.parentElement) {
		const { overflowY } = getComputedStyle(el);
		if (overflowY === 'auto' || overflowY === 'scroll') return el;
	}
	return null;
}

/**
 * The rows of each group to draw when `limit` are drawn in all, in order:
 * groups past it are left out, and the one it falls in is cut short.
 */
export function upTo<G extends { tasks: unknown[] }>(
	groups: G[],
	limit: number
): { group: G; shown: G['tasks'] }[] {
	const out: { group: G; shown: G['tasks'] }[] = [];
	let left = limit;
	for (const group of groups) {
		if (left <= 0) break;
		out.push({
			group,
			shown: group.tasks.length <= left ? group.tasks : group.tasks.slice(0, left)
		});
		left -= group.tasks.length;
	}
	return out;
}
