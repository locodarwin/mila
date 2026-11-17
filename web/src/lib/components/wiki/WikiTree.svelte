<script module lang="ts">
	import { menu, type MenuSource } from '../../contextMenu.svelte';
	import { SvelteSet } from 'svelte/reactivity';

	// Pages folded away, remembered on this device, and the same in every tree
	// on the screen.
	const FOLDED = 'mila.wiki.folded';
	const folded = new SvelteSet<string>(read());
	function read(): string[] {
		try {
			const v = JSON.parse(localStorage.getItem(FOLDED) ?? '[]');
			return Array.isArray(v) ? v.filter((x) => typeof x === 'string') : [];
		} catch {
			return [];
		}
	}
</script>

<script lang="ts">
	// A wiki's pages as a tree: the main page at
	// the root, subpages under their parents. One Tab stop; the arrows walk
	// it and fold and unfold pages; Alt and the arrows move a page; a page
	// drags before, after or into another. The main page stays where it is.
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import FileText from '@lucide/svelte/icons/file-text';
	import House from '@lucide/svelte/icons/house';
	import { tick } from 'svelte';
	import { announce } from '../../announce.svelte';
	import { movePage } from '../../sync/data';
	import type { Page } from '../../sync/entities';

	interface Props {
		ws: string;
		/** The wiki's live pages, the main page among them or not. */
		pages: Page[];
		mainId: string;
		mainTitle: string;
		current: string;
		onopen: (pageId: string) => void;
		/** What the tree is called, for screen readers. */
		label: string;
		/** A page's right-click menu (Wiki.svelte). */
		menuOf?: (pageId: string) => MenuSource;
	}

	let { ws, pages, mainId, mainTitle, current, onopen, label, menuOf }: Props = $props();

	function fold(id: string, on: boolean) {
		if (on) folded.add(id);
		else folded.delete(id);
		try {
			localStorage.setItem(FOLDED, JSON.stringify([...folded].slice(-500)));
		} catch {
			// Kept for this visit only.
		}
	}

	const children = $derived.by(() => {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- made afresh by each derivation
		const out = new Map<string, Page[]>();
		for (const p of pages) {
			if (!p.parentId) continue;
			const list = out.get(p.parentId) ?? [];
			list.push(p);
			out.set(p.parentId, list);
		}
		return out;
	});
	const titleOf = (p: { id: string; title: string }) =>
		p.id === mainId ? mainTitle : p.title.trim() || 'Untitled';

	type Item = { id: string; title: string; depth: number; parent: string | null; kids: number };
	// The pages drawn, in order: the main page, then each page's subpages
	// under it unless it is folded.
	const items = $derived.by((): Item[] => {
		const out: Item[] = [];
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- made afresh by each derivation
		const seen = new Set<string>();
		const walk = (id: string, depth: number, parent: string | null) => {
			if (seen.has(id) || depth > 64) return;
			seen.add(id);
			const kids = children.get(id) ?? [];
			out.push({
				id,
				title: titleOf(pages.find((p) => p.id === id) ?? { id, title: '' }),
				depth,
				parent,
				kids: kids.length
			});
			if (!folded.has(id)) for (const k of kids) walk(k.id, depth + 1, id);
		};
		walk(mainId, 0, null);
		return out;
	});

	let root = $state<HTMLElement>();
	let focused = $state<string | null>(null);
	const active = $derived(
		items.some((i) => i.id === focused)
			? focused!
			: items.some((i) => i.id === current)
				? current
				: mainId
	);

	async function focusItem(id: string) {
		focused = id;
		await tick();
		root?.querySelector<HTMLElement>(`[data-page="${CSS.escape(id)}"]`)?.focus();
	}

	const siblingsOf = (id: string) => {
		const p = pages.find((x) => x.id === id);
		return p?.parentId ? (children.get(p.parentId) ?? []) : [];
	};

	/** Moves a page among its siblings, under the page above, or out a level. */
	async function move(id: string, how: 'up' | 'down' | 'in' | 'out') {
		if (id === mainId) return announce('The main page stays at the top.');
		const page = pages.find((p) => p.id === id);
		if (!page?.parentId) return;
		const sibs = siblingsOf(id);
		const i = sibs.findIndex((p) => p.id === id);
		const name = titleOf(page);
		if (how === 'up' || how === 'down') {
			const to = how === 'up' ? i - 1 : i + 1;
			if (to < 0 || to >= sibs.length) return;
			const others = sibs.filter((p) => p.id !== id);
			const before = others[to - 1]?.sortKey ?? null;
			const after = others[to]?.sortKey ?? null;
			await movePage(ws, id, page.parentId, before, after);
			announce(`Moved ${name} to ${to + 1} of ${sibs.length}`);
		} else if (how === 'in') {
			const above = sibs[i - 1];
			if (!above) return announce(`${name} has no page above it to go under.`);
			const kids = children.get(above.id) ?? [];
			fold(above.id, false);
			await movePage(ws, id, above.id, kids.at(-1)?.sortKey ?? null, null);
			announce(`Moved ${name} under ${titleOf(above)}`);
		} else {
			const parent = pages.find((p) => p.id === page.parentId);
			if (!parent?.parentId) return announce(`${name} is already at the top level.`);
			const theirs = siblingsOf(parent.id);
			const at = theirs.findIndex((p) => p.id === parent.id);
			await movePage(ws, id, parent.parentId, parent.sortKey, theirs[at + 1]?.sortKey ?? null);
			announce(`Moved ${name} out, after ${titleOf(parent)}`);
		}
		await focusItem(id);
	}

	function onkeydown(e: KeyboardEvent) {
		const el = (e.target as HTMLElement).closest<HTMLElement>('[data-page]');
		if (!el) return;
		const id = el.dataset.page!;
		const at = items.findIndex((i) => i.id === id);
		const item = items[at];
		if (!item) return;
		if (e.altKey && !e.ctrlKey && !e.metaKey) {
			const how = { ArrowUp: 'up', ArrowDown: 'down', ArrowRight: 'in', ArrowLeft: 'out' }[e.key];
			if (how) {
				e.preventDefault();
				void move(id, how as 'up' | 'down' | 'in' | 'out');
			}
			return;
		}
		if (e.ctrlKey || e.metaKey || e.altKey || e.shiftKey) return;
		const go = (i: number) => {
			e.preventDefault();
			const next = items[Math.max(0, Math.min(items.length - 1, i))];
			if (next) void focusItem(next.id);
		};
		switch (e.key) {
			case 'ArrowDown':
				return go(at + 1);
			case 'ArrowUp':
				return go(at - 1);
			case 'Home':
				return go(0);
			case 'End':
				return go(items.length - 1);
			case 'ArrowRight':
				e.preventDefault();
				if (item.kids && folded.has(id)) fold(id, false);
				else if (item.kids) go(at + 1);
				return;
			case 'ArrowLeft':
				e.preventDefault();
				if (item.kids && !folded.has(id)) fold(id, true);
				else if (item.parent) void focusItem(item.parent);
				return;
			case 'Enter':
			case ' ':
				e.preventDefault();
				onopen(id);
		}
	}

	// Dragging a page: before, after or into another, by where it is dropped.
	let drag = $state<{ id: string; x: number; y: number; moved: boolean } | null>(null);
	// A drag that ends over a page does not also open it.
	let dragged = false;
	let drop = $state<{ id: string; where: 'before' | 'after' | 'into' } | null>(null);

	/** Whether a page is the one dragged or below it, where it cannot go. */
	function within(id: string, of: string): boolean {
		for (let at: string | null = id; at; at = pages.find((p) => p.id === at)?.parentId ?? null)
			if (at === of) return true;
		return false;
	}

	function down(e: PointerEvent, id: string) {
		if (e.button !== 0 || id === mainId) return;
		drag = { id, x: e.clientX, y: e.clientY, moved: false };
	}
	function moveDrag(e: PointerEvent) {
		if (!drag) return;
		if (!drag.moved && Math.hypot(e.clientX - drag.x, e.clientY - drag.y) < 6) return;
		if (!drag.moved) {
			drag.moved = true;
			(e.currentTarget as HTMLElement).setPointerCapture?.(e.pointerId);
		}
		const target = document
			.elementsFromPoint(e.clientX, e.clientY)
			.map((el) => (el as HTMLElement).closest?.<HTMLElement>('[data-page]'))
			.find((el) => el && root?.contains(el));
		if (!target) {
			drop = null;
			return;
		}
		const id = target.dataset.page!;
		if (within(id, drag.id)) {
			drop = null;
			return;
		}
		const r = target.getBoundingClientRect();
		const y = (e.clientY - r.top) / r.height;
		const where = id === mainId ? 'into' : y < 0.3 ? 'before' : y > 0.7 ? 'after' : 'into';
		drop = { id, where };
	}
	async function up() {
		const d = drag;
		const target = drop;
		drag = null;
		drop = null;
		if (d?.moved) dragged = true;
		if (!d?.moved || !target) return;
		const page = pages.find((p) => p.id === d.id);
		const onto = pages.find((p) => p.id === target.id);
		if (!page) return;
		if (target.where === 'into') {
			const kids = (children.get(target.id) ?? []).filter((p) => p.id !== d.id);
			fold(target.id, false);
			await movePage(ws, d.id, target.id, kids.at(-1)?.sortKey ?? null, null);
			announce(`Moved ${titleOf(page)} under ${titleOf(onto ?? { id: target.id, title: '' })}`);
		} else if (onto?.parentId) {
			const sibs = (children.get(onto.parentId) ?? []).filter((p) => p.id !== d.id);
			const at = sibs.findIndex((p) => p.id === onto.id);
			const before = target.where === 'before' ? (sibs[at - 1]?.sortKey ?? null) : onto.sortKey;
			const after = target.where === 'before' ? onto.sortKey : (sibs[at + 1]?.sortKey ?? null);
			await movePage(ws, d.id, onto.parentId, before, after);
			announce(`Moved ${titleOf(page)} ${target.where} ${titleOf(onto)}`);
		}
	}
</script>

<!-- The tree's keys are on the list; an item's click opens it, as Enter does. -->
<ul
	class="tree"
	role="tree"
	aria-label={label}
	bind:this={root}
	{onkeydown}
	class:dragging={drag?.moved}
>
	{#each items as item (item.id)}
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<li
			role="treeitem"
			aria-level={item.depth + 1}
			aria-expanded={item.kids ? !folded.has(item.id) : undefined}
			aria-selected={item.id === current}
			aria-current={item.id === current ? 'page' : undefined}
			data-page={item.id}
			use:menu={menuOf?.(item.id) ?? (() => null)}
			tabindex={item.id === active ? 0 : -1}
			class="item"
			class:on={item.id === current}
			class:lifted={drag?.moved && drag.id === item.id}
			class:into={drop?.id === item.id && drop.where === 'into'}
			class:before={drop?.id === item.id && drop.where === 'before'}
			class:after={drop?.id === item.id && drop.where === 'after'}
			style:--depth={item.depth}
			onfocus={() => (focused = item.id)}
			onclick={() => {
				if (dragged) dragged = false;
				else onopen(item.id);
			}}
			onpointerdown={(e) => down(e, item.id)}
			onpointermove={moveDrag}
			onpointerup={up}
			onpointercancel={() => {
				drag = null;
				drop = null;
			}}
		>
			{#if item.kids}
				<!-- The mouse's way to fold; Left and Right do it from the keyboard. -->
				<span
					class="twisty"
					aria-hidden="true"
					title="Fold or unfold"
					onclick={(e) => {
						e.stopPropagation();
						fold(item.id, !folded.has(item.id));
					}}
					onpointerdown={(e) => e.stopPropagation()}
				>
					<span class:closed={folded.has(item.id)}><ChevronDown size={14} /></span>
				</span>
			{:else}<span class="gap"></span>{/if}
			{#if item.id === mainId}<House size={14} />{:else}<FileText size={14} />{/if}
			<span class="title">{item.title}</span>
		</li>
	{/each}
</ul>

<style>
	.tree {
		display: grid;
		gap: 1px;
		padding: 0 var(--space-2);
		list-style: none;
	}
	.tree.dragging {
		user-select: none;
	}
	.item {
		position: relative;
		display: flex;
		align-items: center;
		gap: var(--space-1);
		min-height: max(var(--tap-min), calc(28px * var(--control-scale)));
		padding: 0 var(--space-2) 0 calc(var(--space-1) + var(--depth) * 14px);
		border-radius: var(--radius-md);
		color: var(--text-secondary);
		font-size: var(--text-sm);
		cursor: pointer;
		touch-action: none;
	}
	.item:hover {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	.item.on {
		background: var(--accent-soft);
		color: var(--text-primary);
		font-weight: 550;
	}
	.item:focus-visible {
		outline: none;
		box-shadow: inset 0 0 0 2px var(--accent);
	}
	.item.lifted {
		opacity: 0.45;
	}
	.item.into {
		box-shadow: inset 0 0 0 2px var(--accent);
	}
	.item.before::before,
	.item.after::after {
		content: '';
		position: absolute;
		left: calc(var(--space-1) + var(--depth) * 14px);
		right: 0;
		height: 2px;
		border-radius: 1px;
		background: var(--accent);
	}
	.item.before::before {
		top: -1px;
	}
	.item.after::after {
		bottom: -1px;
	}
	.item :global(svg) {
		flex: none;
	}
	.twisty,
	.gap {
		flex: none;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 18px;
		height: 18px;
		padding: 0;
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: inherit;
		cursor: pointer;
	}
	.twisty:hover {
		background: var(--surface-3);
	}
	.twisty .closed {
		display: inline-flex;
		transform: rotate(-90deg);
	}
	.title {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
