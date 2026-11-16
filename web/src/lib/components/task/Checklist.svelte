<script lang="ts">
	import X from '@lucide/svelte/icons/x';
	import Checkbox from '../Checkbox.svelte';
	import { addChecklistItem, deleteChecklistItem, updateChecklistItem } from '../../sync/data';
	import type { ChecklistItem } from '../../sync/entities';

	let { ws, taskId, items }: { ws: string; taskId: string; items: ChecklistItem[] } = $props();
	let text = $state('');

	function add(e: SubmitEvent) {
		e.preventDefault();
		if (text.trim()) void addChecklistItem(ws, taskId, text);
		text = '';
	}
</script>

<ul class="items" aria-label="Checklist">
	{#each items as item (item.id)}
		<li>
			<Checkbox
				label={item.text}
				hideLabel
				checked={item.done}
				onchange={(done) => void updateChecklistItem(ws, item.id, { done })}
			/>
			<input
				class:done={item.done}
				aria-label="Checklist item"
				value={item.text}
				onchange={(e) => {
					const t = e.currentTarget.value.trim();
					if (t && t !== item.text) void updateChecklistItem(ws, item.id, { text: t });
				}}
			/>
			<button
				type="button"
				aria-label={`Remove ${item.text}`}
				onclick={() => void deleteChecklistItem(ws, item.id)}
			>
				<X size={14} />
			</button>
		</li>
	{/each}
</ul>
<form onsubmit={add}>
	<input aria-label="New checklist item" placeholder="Add an item" bind:value={text} />
</form>

<style>
	.items {
		display: grid;
		gap: 2px;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	li {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
	input {
		flex: 1;
		min-width: 0;
		padding: var(--space-1) var(--space-2);
		border: var(--border-width) solid transparent;
		border-radius: var(--radius-sm);
		background: none;
	}
	input:hover {
		border-color: var(--border);
	}
	/* A border and a ring inside it, two pixels of accent in all. */
	input:focus-visible {
		border-color: var(--accent);
		box-shadow: inset 0 0 0 1px var(--accent);
	}
	input.done {
		color: var(--text-muted);
		text-decoration: line-through;
	}
	form input {
		width: 100%;
		margin-top: var(--space-1);
	}
	button {
		display: grid;
		place-items: center;
		width: 24px;
		height: 24px;
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-muted);
		cursor: pointer;
	}
	button:hover {
		background: var(--surface-2);
		color: var(--danger);
	}
</style>
