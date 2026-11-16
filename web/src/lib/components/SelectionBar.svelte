<script lang="ts">
	// The bar that changes the selected tasks (docs/PLAN.md section 3): floating at
	// the bottom of the page on a computer; on a phone in place of the tabs, with a
	// strip at the top for the count, All and Done. Each button's list is a menu
	// (manyActions.ts), above it, or a sheet on a phone. Loaded once something is
	// selected, in the app layout.
	import CalendarClock from '@lucide/svelte/icons/calendar-clock';
	import Check from '@lucide/svelte/icons/check';
	import Ellipsis from '@lucide/svelte/icons/ellipsis';
	import FolderInput from '@lucide/svelte/icons/folder-input';
	import X from '@lucide/svelte/icons/x';
	import { announce } from '../announce.svelte';
	import { contextMenu, type MenuItem, type MenuSpec } from '../contextMenu.svelte';
	import {
		clearSelection,
		completeMany,
		listMenu,
		listOf,
		offered,
		scheduleMany,
		selectedTasks,
		type ListId
	} from '../manyActions';
	import { formatDate } from '../dates';
	import { sayCount, selection } from '../selection.svelte';
	import { localToday } from '../sync/data';

	import type { Task } from '../sync/entities';

	let {
		ws,
		phone,
		inline = false,
		sample
	}: {
		ws: string;
		phone: boolean;
		/** Drawn in its place, for the style guide. */
		inline?: boolean;
		/** Tasks to show the bar for, without selecting them (the style guide). */
		sample?: Task[];
	} = $props();

	const tasks = $derived(sample ?? selectedTasks(ws));
	const count = $derived(tasks.length);
	const reopen = $derived(count > 0 && tasks.every((t) => t.completedAt !== null));
	const can = $derived(offered(ws, tasks));
	const lists = $derived(
		(['schedule', 'priority', 'status', 'labels', 'assign', 'move', 'more'] as ListId[]).filter(
			(w) => can[w]
		)
	);
	const NAMES: Record<ListId, string> = {
		schedule: 'Schedule',
		priority: 'Priority',
		status: 'Status',
		labels: 'Labels',
		assign: 'Assign',
		move: 'Move to',
		more: 'More'
	};

	let bar = $state<HTMLElement>();
	let place = $state<{ left: number; width: number } | null>(null);
	let height = $state(0);

	// Over the page, not the sidebar or the open task: placed on #main as it is.
	$effect(() => {
		const main = document.getElementById('main');
		if (!main || phone || inline) return;
		const fit = () => {
			const r = main.getBoundingClientRect();
			place = { left: r.left, width: r.width };
		};
		fit();
		const watch = new ResizeObserver(fit);
		watch.observe(main);
		addEventListener('resize', fit);
		return () => {
			watch.disconnect();
			removeEventListener('resize', fit);
		};
	});

	// Toasts rise above the bar while it shows.
	$effect(() => {
		if (inline) return;
		const root = document.documentElement;
		root.style.setProperty('--selection-bar', `${height}px`);
		return () => root.style.removeProperty('--selection-bar');
	});

	/** On a phone, More holds what the bar has no room for. */
	function phoneMore(): MenuSpec {
		const items: MenuItem[] = (['priority', 'status', 'labels', 'assign'] as ListId[])
			.filter((w) => can[w])
			.map((w) => ({ id: w, label: NAMES[w], sub: () => listOf(ws, w) }));
		return {
			name: `${count} tasks`,
			kind: 'selection actions',
			items: [...items, 'separator', ...listOf(ws, 'more')]
		};
	}

	function open(which: ListId, button: HTMLElement) {
		if (contextMenu.open?.anchor === button) {
			contextMenu.close(true);
			return;
		}
		const r = button.getBoundingClientRect();
		void contextMenu.show(
			() => (phone && which === 'more' ? phoneMore() : listMenu(ws, which)),
			button,
			{ x: r.left, y: r.top, above: true },
			phone
		);
	}

	function clear() {
		clearSelection();
	}

	function all() {
		selection.all(ws, selection.everything());
		announce(sayCount());
	}

	// One stop for Tab: the arrows go between the bar's buttons.
	function keys(e: KeyboardEvent) {
		const buttons = [...(bar?.querySelectorAll<HTMLButtonElement>('button') ?? [])];
		const i = buttons.indexOf(document.activeElement as HTMLButtonElement);
		if (i < 0) return;
		const to =
			e.key === 'ArrowRight'
				? (i + 1) % buttons.length
				: e.key === 'ArrowLeft'
					? (i - 1 + buttons.length) % buttons.length
					: e.key === 'Home'
						? 0
						: e.key === 'End'
							? buttons.length - 1
							: -1;
		if (to >= 0) {
			e.preventDefault();
			buttons.forEach((b, j) => (b.tabIndex = j === to ? 0 : -1));
			buttons[to].focus();
		} else if (e.key === 'Escape' && !contextMenu.open) {
			e.preventDefault();
			clear();
		}
	}

	// Escape clears the selection from anywhere but a field, a menu, a dialog or the open task.
	function escape(e: KeyboardEvent) {
		if (inline || e.key !== 'Escape' || e.defaultPrevented || contextMenu.open || selection.askDate)
			return;
		const t = e.target as Element | null;
		if (t?.closest('input, textarea, select, [contenteditable="true"], dialog, #task-panel'))
			return;
		e.preventDefault();
		clear();
	}

	// Pick a date: one due date for all of them.
	let date = $state(localToday());
	let dateInput = $state<HTMLInputElement>();
	$effect(() => {
		if (selection.askDate) {
			date = localToday();
			queueMicrotask(() => dateInput?.focus());
		}
	});
	function setDate(e: SubmitEvent) {
		e.preventDefault();
		selection.askDate = false;
		if (date) void scheduleMany(ws, tasks, date, formatDate(date));
	}
</script>

<svelte:window onkeydown={escape} />

{#if phone}
	<div class="strip" class:inline role="group" aria-label="Selection">
		<span class="count" aria-live="polite">{count} selected</span>
		<button type="button" class="link" onclick={all}>All</button>
		<button type="button" class="link" onclick={clear}>Done</button>
	</div>
{/if}

<div
	class="bar"
	class:phone
	class:inline
	role="toolbar"
	aria-label={`${count} ${count === 1 ? 'task' : 'tasks'} selected`}
	tabindex="-1"
	bind:this={bar}
	bind:offsetHeight={height}
	style:left={place && !phone ? `${place.left}px` : undefined}
	style:width={place && !phone ? `${place.width}px` : undefined}
	onkeydown={keys}
>
	<div class="inner">
		{#if !phone}<span class="count">{count} selected</span>{/if}
		<button
			type="button"
			class="act"
			tabindex="0"
			disabled={!count}
			onclick={() => void completeMany(ws, tasks)}
			>{#if phone}<Check size={20} />{/if}<span>{reopen ? 'Reopen' : 'Complete'}</span></button
		>
		{#each phone ? lists.filter((w) => w === 'schedule' || w === 'move' || w === 'more') : lists as which (which)}
			<button
				type="button"
				class="act"
				tabindex="-1"
				disabled={!count}
				aria-haspopup="menu"
				onclick={(e) => open(which, e.currentTarget)}
				>{#if phone}{#if which === 'schedule'}<CalendarClock
							size={20}
						/>{:else if which === 'move'}<FolderInput size={20} />{:else}<Ellipsis
							size={20}
						/>{/if}{/if}<span>{NAMES[which]}</span></button
			>
		{/each}
		{#if !phone}
			<button
				type="button"
				class="act clear"
				tabindex="-1"
				aria-label="Clear the selection"
				title="Clear the selection"
				onclick={clear}><X size={16} /></button
			>
		{/if}
	</div>
	{#if selection.askDate}
		<form class="date" onsubmit={setDate}>
			<label for="selection-date">Due date for {count} {count === 1 ? 'task' : 'tasks'}</label>
			<input
				id="selection-date"
				type="date"
				bind:value={date}
				bind:this={dateInput}
				onkeydown={(e) => {
					if (e.key === 'Escape') {
						e.preventDefault();
						selection.askDate = false;
					}
				}}
			/>
			<button type="submit" class="act primary">Set</button>
			<button type="button" class="act" onclick={() => (selection.askDate = false)}>Cancel</button>
		</form>
	{/if}
</div>

<style>
	.bar {
		position: fixed;
		bottom: calc(var(--space-4) + env(safe-area-inset-bottom));
		z-index: 40;
		display: grid;
		justify-items: center;
		gap: var(--space-2);
		pointer-events: none;
	}
	.inner,
	.date {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: center;
		gap: 2px;
		max-width: calc(100% - 2 * var(--space-3));
		padding: 4px;
		border: var(--border-width) solid var(--menu-border);
		border-radius: var(--menu-radius);
		background: var(--menu-bg);
		box-shadow: var(--shadow-lg);
		font-size: var(--text-sm);
		pointer-events: auto;
	}
	.date {
		order: -1;
		gap: var(--space-2);
		padding: var(--space-2) var(--space-3);
	}
	.date input {
		min-height: max(var(--tap-min), calc(30px * var(--ui-scale)));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-md);
		background: var(--input-bg);
		color: var(--text-primary);
	}
	.count {
		padding: 0 var(--space-3) 0 var(--space-2);
		margin-right: 2px;
		border-right: var(--border-width) solid var(--border);
		color: var(--accent-text);
		font-weight: 650;
		white-space: nowrap;
	}
	.act {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-height: max(var(--tap-min), calc(30px * var(--ui-scale)));
		padding: 0 var(--space-3);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-primary);
		white-space: nowrap;
		cursor: pointer;
	}
	.act:hover:not(:disabled),
	.act:global(.menu-lit) {
		background: var(--surface-2);
	}
	.act:disabled {
		color: var(--text-muted);
		cursor: default;
	}
	.act.primary {
		background: var(--button-primary-bg);
		color: var(--button-primary-text);
	}
	.clear {
		color: var(--text-secondary);
	}
	/* On a phone: in place of the tabs, four buttons a thumb can reach. */
	.bar.phone {
		inset: auto 0 0 0;
		z-index: 45;
		gap: 0;
	}
	.phone .inner {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(0, 1fr));
		width: 100%;
		max-width: none;
		min-height: var(--tabbar-height);
		padding: 0 0 env(safe-area-inset-bottom);
		border-width: var(--border-width) 0 0;
		border-radius: 0;
		box-shadow: none;
	}
	.phone .act {
		flex-direction: column;
		justify-content: center;
		gap: 2px;
		min-height: var(--tabbar-height);
		padding: 0 var(--space-1);
		border-radius: 0;
		color: var(--accent-text);
		font-size: var(--text-xs);
	}
	.phone .act span {
		color: var(--text-secondary);
	}
	.phone .date {
		width: calc(100% - 2 * var(--space-3));
		margin-bottom: var(--space-2);
	}
	/* In its place: the style guide's sample. */
	.bar.inline,
	.strip.inline {
		position: relative;
		inset: auto;
		bottom: auto;
		z-index: auto;
	}
	.strip {
		position: fixed;
		inset: 0 0 auto 0;
		z-index: 45;
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: calc(48px + env(safe-area-inset-top));
		padding: env(safe-area-inset-top) var(--space-3) 0;
		border-bottom: var(--border-width) solid var(--border);
		background: var(--surface-0);
	}
	.strip .count {
		margin-right: auto;
		padding: 0;
		border: none;
		color: var(--text-primary);
	}
	.link {
		min-height: var(--tap-min);
		min-width: var(--tap-min);
		padding: 0 var(--space-2);
		border: none;
		background: none;
		color: var(--accent-text);
		font-weight: 600;
		cursor: pointer;
	}
</style>
