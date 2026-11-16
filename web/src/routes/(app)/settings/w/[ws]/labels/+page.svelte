<script lang="ts">
	import { settingsWorkspace } from '#lib/settingsWorkspace.svelte.js';
	import Button from '#lib/components/Button.svelte';
	import LabelChip from '#lib/components/LabelChip.svelte';
	import CustomFields from '#lib/components/settings/CustomFields.svelte';
	import { createLabel, deleteLabel, updateLabel } from '#lib/sync/data.js';
	import { engine } from '#lib/sync/engine.svelte.js';
	import { PALETTE, type PaletteColor } from '#lib/theme/palette.js';
	import { toasts } from '#lib/toasts.svelte.js';

	const ws = $derived(settingsWorkspace()?.id ?? '');
	const labels = $derived(engine.replica(ws)?.labels() ?? []);
	let name = $state('');
	let color = $state<PaletteColor>('blue');

	function add(e: SubmitEvent) {
		e.preventDefault();
		if (!name.trim()) return;
		void createLabel(ws, name, color);
		name = '';
	}

	function remove(id: string, label: string) {
		// Undo takes back this delete, not whatever was edited after it.
		const { undo } = engine.track(() => deleteLabel(ws, id));
		toasts.show(`Deleted the label ${label}`, { onundo: () => void undo() });
	}
</script>

<svelte:head><title>Labels and fields · Mila</title></svelte:head>

<section>
	<h2>Labels in {settingsWorkspace()?.name}</h2>
	<p class="hint">
		Labels belong to the whole workspace, so one label can mark tasks in any project.
	</p>
	{#if labels.length}
		<ul class="rows" aria-label="Labels">
			{#each labels as l (l.id)}
				<li>
					<LabelChip color={l.color as PaletteColor} name={l.name} />
					<input
						class="grow"
						aria-label={`Name of ${l.name}`}
						value={l.name}
						onchange={(e) => {
							const n = e.currentTarget.value.trim();
							if (n && n !== l.name) void updateLabel(ws, l.id, { name: n });
						}}
					/>
					<select
						aria-label={`Color of ${l.name}`}
						value={l.color}
						onchange={(e) => void updateLabel(ws, l.id, { color: e.currentTarget.value })}
					>
						{#each PALETTE as c (c)}<option value={c}>{c}</option>{/each}
					</select>
					<Button
						title="Delete this label"
						size="sm"
						variant="danger"
						onclick={() => remove(l.id, l.name)}>Delete</Button
					>
				</li>
			{/each}
		</ul>
	{/if}
	<form class="inline-form" onsubmit={add}>
		<input class="grow" aria-label="New label" placeholder="New label" bind:value={name} />
		<select aria-label="Color of the new label" bind:value={color}>
			{#each PALETTE as c (c)}<option value={c}>{c}</option>{/each}
		</select>
		<Button type="submit" variant="primary">Add label</Button>
	</form>
</section>

<CustomFields />

<style>
	input,
	select {
		height: 30px;
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
	}
	input::placeholder {
		color: var(--input-placeholder);
	}
</style>
