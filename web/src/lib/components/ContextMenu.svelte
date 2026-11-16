<script lang="ts">
	// The right-click menu that's open (contextMenu.svelte.ts): at the pointer, or beside
	// what has focus, kept on screen; or, from a held finger, a sheet from the bottom. A
	// list one step in (Priority, Move to) takes the menu's place, with Back. The arrow
	// keys move, Right opens a list, Left goes back, Escape closes, and focus goes back
	// to what it acted on. Loaded the first time a menu opens.
	import Check from '@lucide/svelte/icons/check';
	import ChevronLeft from '@lucide/svelte/icons/chevron-left';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import Minus from '@lucide/svelte/icons/minus';
	import { tick } from 'svelte';
	import { announce } from '../announce.svelte';
	import {
		contextMenu,
		type MenuAction,
		type MenuItem,
		type OpenMenu
	} from '../contextMenu.svelte';
	import Kbd from './Kbd.svelte';

	/** inline: drawn in its place, for the style guide, and not taking focus. */
	let { open, inline = false }: { open: OpenMenu; inline?: boolean } = $props();

	type Level = { title: string | null; items: MenuItem[]; from: number };
	// Levels: the menu, then each list opened from it.
	let levels = $state<Level[]>([]);
	let box = $state<HTMLElement>();
	let place = $state({ left: 0, top: 0 });
	const level = $derived(levels.at(-1));

	$effect(() => {
		levels = [{ title: null, items: open.spec.items, from: 0 }];
		void tick().then(() => {
			fit();
			// A sheet takes focus as a whole, with no ring on a row nobody chose; the
			// arrow keys still start at its first item.
			if (inline) return;
			if (open.sheet) box?.focus();
			else focusAt(0);
		});
	});

	/** Keeps the menu on screen, at the point or as near it as fits. */
	function fit() {
		if (!box || open.sheet || inline) return;
		const w = box.offsetWidth;
		const h = box.offsetHeight;
		place = {
			left: Math.max(6, Math.min(open.x, innerWidth - w - 6)),
			top: Math.max(6, Math.min(open.above ? open.y - h - 6 : open.y, innerHeight - h - 6))
		};
	}

	function buttons() {
		return [...(box?.querySelectorAll<HTMLButtonElement>('[role^="menuitem"]') ?? [])];
	}
	function focusAt(i: number) {
		const b = buttons();
		b[Math.max(0, Math.min(i, b.length - 1))]?.focus();
	}

	async function choose(item: MenuAction, index: number) {
		if (item.sub) {
			const items = await item.sub();
			levels = [...levels, { title: item.label, items, from: index }];
			announce(`${item.label}, ${items.filter((i) => i !== 'separator').length} choices`);
			await tick();
			fit();
			focusAt(1);
			return;
		}
		// Closed first, so that an action that moves focus (Rename) keeps it there.
		if (!item.keep) {
			contextMenu.close(true);
			await item.run?.();
			return;
		}
		await item.run?.();
		// Its checks follow what changed: the menu is made again, at this level.
		const at = buttons().indexOf(document.activeElement as HTMLButtonElement);
		const top = { ...levels[0], items: open.spec.refresh?.() ?? levels[0].items };
		let items = top.items;
		const next: Level[] = [top];
		for (const l of levels.slice(1)) {
			const parent = items[l.from];
			items = parent !== 'separator' && parent?.sub ? await parent.sub() : l.items;
			next.push({ ...l, items });
		}
		levels = next;
		await tick();
		focusAt(at);
	}

	async function back() {
		if (levels.length < 2) {
			contextMenu.close(true);
			return;
		}
		const from = levels.at(-1)!.from;
		levels = levels.slice(0, -1);
		await tick();
		fit();
		focusAt(from);
	}

	function keys(e: KeyboardEvent) {
		const b = buttons();
		const i = b.indexOf(document.activeElement as HTMLButtonElement);
		switch (e.key) {
			case 'ArrowDown':
				e.preventDefault();
				focusAt((i + 1) % b.length);
				break;
			case 'ArrowUp':
				e.preventDefault();
				focusAt((i - 1 + b.length) % b.length);
				break;
			case 'Home':
				e.preventDefault();
				focusAt(0);
				break;
			case 'End':
				e.preventDefault();
				focusAt(b.length - 1);
				break;
			case 'ArrowRight':
				if (b[i]?.getAttribute('aria-haspopup')) {
					e.preventDefault();
					b[i].click();
				}
				break;
			case 'ArrowLeft':
				if (levels.length > 1) {
					e.preventDefault();
					void back();
				}
				break;
			case 'Escape':
				e.preventDefault();
				e.stopPropagation();
				if (levels.length > 1) void back();
				else contextMenu.close(true);
				break;
			case 'Tab':
				e.preventDefault();
				contextMenu.close(true);
				break;
		}
	}

	function outside(e: PointerEvent) {
		if (box && !box.contains(e.target as Node)) contextMenu.close();
	}
</script>

<svelte:window
	onpointerdown={outside}
	onresize={() => contextMenu.close()}
	onblur={() => contextMenu.close()}
/>

<!-- In a landmark of its own while open, as everything on a page is. -->
<section class="host" aria-label="Menu">
	{#if open.sheet}<div class="scrim" aria-hidden="true"></div>{/if}
	<div
		class="cmenu"
		class:sheet={open.sheet}
		class:inline
		role="menu"
		tabindex="-1"
		aria-label={`${open.spec.name}, ${open.spec.kind}`}
		bind:this={box}
		style:left={open.sheet ? undefined : `${place.left}px`}
		style:top={open.sheet ? undefined : `${place.top}px`}
		onkeydown={keys}
		oncontextmenu={(e) => e.preventDefault()}
	>
		{#if open.sheet}
			<div class="grab" aria-hidden="true"></div>
			<p class="what" aria-hidden="true">{open.spec.name}</p>
		{/if}
		{#if level}
			{#if level.title}
				<button type="button" class="item back" role="menuitem" onclick={() => void back()}>
					<span class="mark"><ChevronLeft size={15} /></span>
					<span class="label">{level.title}</span>
					<span class="visually-hidden">, back</span>
				</button>
				<div class="sep" role="separator"></div>
			{/if}
			{#each level.items as item, i (item === 'separator' ? `sep-${i}` : item.id)}
				{#if item === 'separator'}
					<div class="sep" role="separator"></div>
				{:else}
					<button
						type="button"
						class="item"
						class:danger={item.danger}
						role={item.checked === undefined ? 'menuitem' : 'menuitemcheckbox'}
						aria-checked={item.checked === undefined ? undefined : item.checked}
						aria-haspopup={item.sub ? 'menu' : undefined}
						onclick={() => void choose(item, i)}
					>
						<span class="mark"
							>{#if item.checked === 'mixed'}<Minus size={14} />{:else if item.checked}<Check
									size={14}
								/>{/if}</span
						>
						<span class="label">{item.label}</span>
						<span class="end">
							{#if item.sub}<ChevronRight size={15} />{:else if item.command && !open.sheet}<Kbd
									command={item.command}
								/>{/if}
						</span>
					</button>
				{/if}
			{/each}
		{/if}
	</div>
</section>

<style>
	.cmenu {
		position: fixed;
		z-index: 60;
		min-width: 230px;
		max-width: min(320px, calc(100vw - 12px));
		max-height: calc(100vh - 12px);
		overflow-y: auto;
		padding: 4px;
		border: var(--border-width) solid var(--menu-border);
		border-radius: var(--menu-radius);
		background: var(--menu-bg);
		box-shadow: var(--shadow-lg);
		font-size: var(--text-sm);
		user-select: none;
		outline: none;
	}
	.cmenu.inline {
		position: relative;
		width: max-content;
	}
	.cmenu.sheet {
		left: 0;
		right: 0;
		bottom: 0;
		max-width: none;
		max-height: 70vh;
		padding: var(--space-2) var(--space-3) calc(var(--space-4) + env(safe-area-inset-bottom));
		border-radius: var(--radius-lg) var(--radius-lg) 0 0;
		font-size: var(--text-md);
	}
	.scrim {
		position: fixed;
		inset: 0;
		z-index: 59;
		background: var(--scrim-sheet);
	}
	.grab {
		width: 36px;
		height: 4px;
		margin: 2px auto var(--space-2);
		border-radius: 2px;
		background: var(--border-strong);
	}
	.what {
		margin: 0;
		padding: 0 var(--space-2) var(--space-1);
		font-size: var(--text-xs);
		color: var(--text-secondary);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.item {
		display: grid;
		grid-template-columns: 18px minmax(0, 1fr) auto;
		align-items: center;
		gap: var(--space-2);
		width: 100%;
		min-height: max(var(--tap-min), 30px);
		padding: 0 var(--space-2);
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-primary);
		font: inherit;
		text-align: left;
		cursor: pointer;
	}
	.item:hover,
	.item:focus-visible {
		background: var(--accent-soft);
		color: var(--accent-text);
		outline: none;
	}
	.item.danger {
		color: var(--danger);
	}
	.mark {
		display: inline-flex;
		color: var(--accent-text);
	}
	.label {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.end {
		display: inline-flex;
		color: var(--text-muted);
		font-size: var(--text-xs);
	}
	.sep {
		height: 1px;
		margin: 4px 6px;
		background: var(--border);
	}
	.sheet .item {
		min-height: max(var(--tap-min), 44px);
	}
	@media (forced-colors: active) {
		.item:focus-visible {
			outline: 2px solid Highlight;
		}
	}
</style>
