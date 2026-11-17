// Right-click menus: what can be done to a task, a view, a
// project or another thing, at the pointer, beside what has focus (Shift+F10 or the
// Menu key), or from a held finger as a sheet. Everything in one is somewhere else
// too. This is the small part every page loads: what's open, and the `menu` action
// that opens it. The menu (components/ContextMenu.svelte) and what goes in the task,
// view and project ones (menus.ts) load the first time one opens.
import type { CommandId } from './keys/commands';
import { selection } from './selection.svelte';

/** One thing to do, or a list of them a step in. */
export interface MenuAction {
	id: string;
	label: string;
	/** The command whose keys show beside it. */
	command?: CommandId;
	/**
	 * A choice that's on (a label the task has), with a check beside it; for
	 * several tasks, 'mixed' when only some have it, with a dash.
	 */
	checked?: boolean | 'mixed';
	/** Deleting: drawn apart, and last. */
	danger?: boolean;
	/** Stays open after a choice, for choosing several (labels, people). */
	keep?: boolean;
	run?: () => unknown;
	/** A list one step in: Priority, Move to. */
	sub?: () => MenuItem[] | Promise<MenuItem[]>;
}

export type MenuItem = MenuAction | 'separator';

export interface MenuSpec {
	/** What it acts on, said first: "Order seeds". */
	name: string;
	/** What kind of thing it is, for screen readers: "task actions". */
	kind: string;
	items: MenuItem[];
	/** Makes the items again after a choice that keeps it open, so its checks follow. */
	refresh?: () => MenuItem[];
}

/** What a menu is made from when it opens, so it always says how things are now. */
export type MenuSource = (anchor: HTMLElement) => MenuSpec | null | Promise<MenuSpec | null>;

export interface OpenMenu {
	spec: MenuSpec;
	/** Where: a point, for the pointer, or under the element, for the keyboard. */
	x: number;
	y: number;
	/** From a held finger: a sheet from the bottom. */
	sheet: boolean;
	/** Above the point rather than below it: from the selection bar, at the bottom. */
	above?: boolean;
	/** What it acts on, lit while open. */
	anchor: HTMLElement;
	/** Where focus goes back to: what had it, in what the menu acts on. */
	back: HTMLElement | null;
}

class ContextMenu {
	open = $state.raw<OpenMenu | null>(null);
	#asked = 0;

	async show(
		source: MenuSource,
		anchor: HTMLElement,
		at: { x: number; y: number; above?: boolean },
		sheet: boolean
	) {
		const asked = ++this.#asked;
		const spec = await source(anchor);
		if (asked !== this.#asked || !spec || !spec.items.some((i) => i !== 'separator')) return;
		const had = document.activeElement;
		const back = had instanceof HTMLElement && anchor.contains(had) ? had : null;
		this.close();
		anchor.classList.add('menu-lit');
		this.open = { spec, x: at.x, y: at.y, above: at.above, sheet, anchor, back };
	}

	/** Closes the menu; back says whether focus goes back to what it acted on. */
	close(back = false) {
		const was = this.open;
		if (!was) return;
		was.anchor.classList.remove('menu-lit');
		this.open = null;
		if (back && was.back?.isConnected) was.back.focus({ preventScroll: true });
	}
}

export const contextMenu = new ContextMenu();

/** How long a finger is held without moving, in ms, before a menu comes up. */
export const HOLD_MS = 500;
/** How far it may move meanwhile, in px, and still be held rather than a swipe or a scroll. */
const HOLD_SLOP = 8;

/** Text and fields keep the browser's own menu: spelling, cut, copy and paste. */
function inText(target: EventTarget | null, node: HTMLElement): boolean {
	const el = target instanceof Element ? target : null;
	const field = el?.closest('input, textarea, select, [contenteditable="true"]');
	return !!field && node.contains(field) && field !== node;
}

/**
 * Gives an element a right-click menu, with or without Shift. Text and fields
 * inside keep the browser's. Shift+F10 or the Menu key
 * opens it when the element, or a button in it, has focus; a finger held still opens it as
 * a sheet.
 */
export function menu(node: HTMLElement, source: MenuSource) {
	let current = source;
	let timer: ReturnType<typeof setTimeout> | undefined;
	let start: { x: number; y: number } | null = null;
	let held = 0;

	const open = (at: { x: number; y: number }, sheet: boolean) =>
		void contextMenu.show(current, node, at, sheet);

	function contextmenu(e: MouseEvent) {
		if (inText(e.target, node)) return;
		// Inner things with menus of their own have theirs (a label on a task).
		if ((e.target as Element | null)?.closest('[data-menu]') !== node) return;
		e.preventDefault();
		e.stopPropagation();
		// A held finger fires this too on some phones; the hold has opened it already.
		if (Date.now() - held < 1000) return;
		open({ x: e.clientX, y: e.clientY }, false);
	}
	function keydown(e: KeyboardEvent) {
		// From the thing itself or a button in it (a row's title), not from text in it.
		if (inText(e.target, node) || (e.target as Element | null)?.closest('[data-menu]') !== node)
			return;
		if (
			(e.key === 'F10' && e.shiftKey && !e.ctrlKey && !e.altKey && !e.metaKey) ||
			e.key === 'ContextMenu'
		) {
			e.preventDefault();
			e.stopPropagation();
			const r = node.getBoundingClientRect();
			open({ x: r.left + Math.min(24, r.width / 2), y: r.bottom }, false);
		}
	}
	function cancel() {
		clearTimeout(timer);
		timer = undefined;
		start = null;
	}
	function down(e: PointerEvent) {
		if (e.pointerType !== 'touch' || inText(e.target, node)) return;
		if ((e.target as Element | null)?.closest('[data-menu]') !== node) return;
		start = { x: e.clientX, y: e.clientY };
		timer = setTimeout(() => {
			timer = undefined;
			held = Date.now();
			navigator.vibrate?.(10);
			// The finger lifting would be a tap on what it held: it isn't.
			const swallow = (c: Event) => {
				c.preventDefault();
				c.stopPropagation();
			};
			node.addEventListener('click', swallow, { capture: true, once: true });
			setTimeout(() => node.removeEventListener('click', swallow, { capture: true }), 600);
			open({ x: start?.x ?? 0, y: start?.y ?? 0 }, true);
		}, HOLD_MS);
	}
	function move(e: PointerEvent) {
		if (start && Math.hypot(e.clientX - start.x, e.clientY - start.y) > HOLD_SLOP) cancel();
	}

	node.dataset.menu = '';
	node.style.setProperty('-webkit-touch-callout', 'none');
	node.addEventListener('contextmenu', contextmenu);
	node.addEventListener('keydown', keydown);
	node.addEventListener('pointerdown', down);
	node.addEventListener('pointermove', move);
	node.addEventListener('pointerup', cancel);
	node.addEventListener('pointercancel', cancel);
	node.addEventListener('scroll', cancel, { capture: true, passive: true });
	return {
		update(next: MenuSource) {
			current = next;
		},
		destroy() {
			cancel();
			delete node.dataset.menu;
			node.removeEventListener('contextmenu', contextmenu);
			node.removeEventListener('keydown', keydown);
			node.removeEventListener('pointerdown', down);
			node.removeEventListener('pointermove', move);
			node.removeEventListener('pointerup', cancel);
			node.removeEventListener('pointercancel', cancel);
			node.removeEventListener('scroll', cancel, { capture: true });
			if (contextMenu.open?.anchor === node) contextMenu.close();
		}
	};
}

/**
 * The menus of tasks, views and projects, made from menus.ts when one opens.
 * A task that is selected with others has the selection's menu
 * (manyActions.ts), acting on all of them.
 */
export const taskActions =
	(ws: string, id: string): MenuSource =>
	(anchor) =>
		selection.has(ws, id) && selection.shown().length > 1
			? import('./manyActions').then((m) => m.manyMenu(ws))
			: import('./menus').then((m) => m.taskMenu(ws, id, anchor));
export const viewActions =
	(ws: string, id: string): MenuSource =>
	() =>
		import('./menus').then((m) => m.viewMenu(ws, id));
export const projectActions =
	(ws: string, id: string): MenuSource =>
	() =>
		import('./menus').then((m) => m.projectMenu(ws, id));
