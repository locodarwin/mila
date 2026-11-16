// Popover menus open under the button that opens them (the element whose
// popovertarget names the menu) and stay on the screen. CSS anchor
// positioning does it where the browser has it (base.css, .menu[popover]);
// elsewhere the menu is placed from the button's position when it opens.

/** Where a menu of a size goes next to a button: below it, else above; never off screen. */
export function placeNear(
	trigger: { top: number; bottom: number; left: number; right: number },
	menu: { width: number; height: number },
	viewport: { width: number; height: number },
	{ gap = 4, margin = 8 }: { gap?: number; margin?: number } = {}
): { top: number; left: number } {
	let top = trigger.bottom + gap;
	const above = trigger.top - gap - menu.height;
	if (top + menu.height > viewport.height - margin && above >= margin) top = above;
	top = Math.max(margin, Math.min(top, viewport.height - menu.height - margin));
	// Lined up with the button's left edge, or its right edge near the
	// right side of the screen.
	let left = trigger.left;
	if (left + menu.width > viewport.width - margin) left = trigger.right - menu.width;
	left = Math.max(margin, Math.min(left, viewport.width - menu.width - margin));
	return { top, left };
}

const anchors = typeof CSS !== 'undefined' && CSS.supports?.('anchor-name: --a');
let next = 0;

/**
 * An action for a menu with `popover` and an id: anchors it to its button.
 * `docked`, when it says so, leaves the menu where its own CSS puts it (a
 * sheet along the bottom of a phone), clearing any place set before.
 */
export function anchored(menu: HTMLElement, options: { docked?: () => boolean } = {}) {
	const name = `--menu-${++next}`;
	menu.style.setProperty('position-anchor', name);

	function trigger(): HTMLElement | null {
		return document.querySelector<HTMLElement>(`[popovertarget="${CSS.escape(menu.id)}"]`);
	}

	// Named before it shows, so the first frame is already in place.
	function before(e: Event) {
		if ((e as ToggleEvent).newState !== 'open') return;
		trigger()?.style.setProperty('anchor-name', name);
	}

	function opened(e: Event) {
		if ((e as ToggleEvent).newState !== 'open') return;
		if (options.docked?.()) {
			menu.style.removeProperty('top');
			menu.style.removeProperty('left');
			return;
		}
		if (anchors) return;
		const t = trigger();
		if (!t) return;
		const { top, left } = placeNear(
			t.getBoundingClientRect(),
			{ width: menu.offsetWidth, height: menu.offsetHeight },
			{ width: innerWidth, height: innerHeight }
		);
		menu.style.top = `${top}px`;
		menu.style.left = `${left}px`;
	}

	menu.addEventListener('beforetoggle', before);
	menu.addEventListener('toggle', opened);
	return {
		destroy() {
			menu.removeEventListener('beforetoggle', before);
			menu.removeEventListener('toggle', opened);
		}
	};
}
