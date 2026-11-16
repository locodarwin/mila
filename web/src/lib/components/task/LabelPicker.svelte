<script lang="ts">
	import Plus from '@lucide/svelte/icons/plus';
	import LabelChip from '../LabelChip.svelte';
	import { createLabel, setTaskLabel } from '../../sync/data';
	import { canOrganize } from '../../roles';
	import { session } from '../../session.svelte';
	import type { Label } from '../../sync/entities';
	import type { PaletteColor } from '../../theme/palette';
	import { anchored } from '../../popover';
	import { uuid } from '../../uuid';

	interface Props {
		ws: string;
		taskId: string;
		all: Label[];
		on: Label[];
	}

	let { ws, taskId, all, on }: Props = $props();
	const id = `labels-${uuid()}`;
	const onIds = $derived(new Set(on.map((l) => l.id)));
	let name = $state('');

	async function create(e: SubmitEvent) {
		e.preventDefault();
		const n = name.trim();
		if (!n) return;
		name = '';
		const labelId = await createLabel(ws, n);
		await setTaskLabel(ws, taskId, labelId, true);
	}
</script>

<div class="labels">
	{#each on as label (label.id)}
		<LabelChip color={label.color as PaletteColor} name={label.name} />
	{/each}
	<button class="add" type="button" popovertarget={id} aria-label="Edit labels"
		><Plus size={14} /></button
	>
</div>

<div {id} popover class="menu" use:anchored>
	<p class="heading">Labels</p>
	{#each all as label (label.id)}
		<label class="option">
			<input
				type="checkbox"
				checked={onIds.has(label.id)}
				onchange={(e) => void setTaskLabel(ws, taskId, label.id, e.currentTarget.checked)}
			/>
			<LabelChip color={label.color as PaletteColor} name={label.name} />
		</label>
	{/each}
	{#if !session.workspace || canOrganize(session.workspace.role)}
		<form onsubmit={create}>
			<input aria-label="New label" placeholder="New label" bind:value={name} />
		</form>
	{/if}
</div>

<style>
	.labels {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1);
		align-items: center;
	}
	.add {
		display: grid;
		place-items: center;
		width: 24px;
		height: 24px;
		border: var(--border-width) dashed var(--border-strong);
		border-radius: var(--radius-full);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.add:hover {
		border-color: var(--accent);
		color: var(--text-primary);
	}
</style>
