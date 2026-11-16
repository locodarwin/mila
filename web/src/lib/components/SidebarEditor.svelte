<script lang="ts">
	import ArrowDown from '@lucide/svelte/icons/arrow-down';
	import ArrowUp from '@lucide/svelte/icons/arrow-up';
	import { tick } from 'svelte';
	import { announce } from '../announce.svelte';
	import type { SidebarItem } from '../preferences.svelte';
	import Checkbox from './Checkbox.svelte';

	interface Props {
		order: SidebarItem[];
		hidden: SidebarItem[];
		onchange: (change: { order?: SidebarItem[]; hidden?: SidebarItem[] }) => void;
	}

	let { order, hidden, onchange }: Props = $props();

	const NAMES: Record<SidebarItem, string> = {
		inbox: 'Inbox',
		today: 'My Day',
		upcoming: 'Upcoming',
		wiki: 'Workspace Wiki',
		time: 'Time',
		reports: 'Reports',
		views: 'Views',
		projects: 'Projects'
	};

	let list = $state<HTMLElement>();

	/** Moves an item one place, keeping focus on the control that moved it. */
	async function move(item: SidebarItem, by: -1 | 1, control: 'up' | 'down' | 'show') {
		const i = order.indexOf(item);
		const j = i + by;
		if (j < 0 || j >= order.length) return;
		const next = [...order];
		[next[i], next[j]] = [next[j], next[i]];
		onchange({ order: next });
		announce(`Moved ${NAMES[item]} to ${j + 1} of ${order.length}`);
		await tick();
		const row = list?.querySelector<HTMLElement>(`[data-item="${item}"]`);
		const want =
			control === 'show'
				? row?.querySelector<HTMLElement>('input')
				: row?.querySelector<HTMLElement>(`[data-move="${control}"]`);
		// A button that can no longer move that way is disabled: focus the other.
		const target =
			want && !(want as HTMLButtonElement).disabled
				? want
				: row?.querySelector<HTMLElement>('[data-move]:not(:disabled)');
		target?.focus();
	}

	function keys(e: KeyboardEvent, item: SidebarItem) {
		if (!e.altKey || (e.key !== 'ArrowUp' && e.key !== 'ArrowDown')) return;
		e.preventDefault();
		const t = e.target as HTMLElement;
		const control = (t.dataset.move as 'up' | 'down' | undefined) ?? 'show';
		void move(item, e.key === 'ArrowUp' ? -1 : 1, control);
	}

	function show(item: SidebarItem, on: boolean) {
		onchange({ hidden: on ? hidden.filter((h) => h !== item) : [...hidden, item] });
	}
</script>

<ol class="items" aria-label="Sidebar items" bind:this={list}>
	{#each order as item, i (item)}
		<!-- Alt+arrows from any of the row's controls, which are what take focus. -->
		<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
		<li data-item={item} onkeydown={(e) => keys(e, item)}>
			<Checkbox
				label={`Show ${NAMES[item]}`}
				checked={!hidden.includes(item)}
				onchange={(on) => show(item, on)}
			/>
			<span class="moves">
				<button
					type="button"
					class="icon"
					data-move="up"
					aria-label={`Move ${NAMES[item]} up`}
					aria-keyshortcuts="Alt+ArrowUp"
					disabled={i === 0}
					onclick={() => move(item, -1, 'up')}
				>
					<ArrowUp size={14} />
				</button>
				<button
					type="button"
					class="icon"
					data-move="down"
					aria-label={`Move ${NAMES[item]} down`}
					aria-keyshortcuts="Alt+ArrowDown"
					disabled={i === order.length - 1}
					onclick={() => move(item, 1, 'down')}
				>
					<ArrowDown size={14} />
				</button>
			</span>
		</li>
	{/each}
</ol>

<style>
	.items {
		margin: 0;
		padding: 0;
		list-style: none;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
	}
	li {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-3);
		min-height: var(--row-height);
		padding: var(--space-1) var(--space-2) var(--space-1) var(--space-4);
	}
	li + li {
		border-top: var(--border-width) solid var(--border);
	}
	.moves {
		display: inline-flex;
		gap: var(--space-1);
	}
	.icon {
		display: grid;
		place-items: center;
		width: calc(28px * var(--ui-scale));
		height: calc(28px * var(--ui-scale));
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		color: var(--text-secondary);
		cursor: pointer;
	}
	.icon:hover:not(:disabled) {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	.icon:disabled {
		color: var(--text-muted);
		cursor: default;
		opacity: 0.5;
	}
</style>
