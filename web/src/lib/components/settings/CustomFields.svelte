<script lang="ts">
	import { settingsWorkspace } from '../../settingsWorkspace.svelte';
	import Button from '#lib/components/Button.svelte';
	import LabelChip from '#lib/components/LabelChip.svelte';
	import { createField, deleteField, editFieldOptions, updateField } from '#lib/sync/data.js';
	import { engine } from '#lib/sync/engine.svelte.js';
	import { FIELD_TYPES, type CustomField, type FieldType } from '#lib/sync/entities.js';
	import { PALETTE, type PaletteColor } from '#lib/theme/palette.js';
	import { toasts } from '#lib/toasts.svelte.js';
	import { uuid } from '#lib/uuid.js';

	const ws = $derived(settingsWorkspace()?.id ?? '');
	const replica = $derived(engine.replica(ws));
	const fields = $derived(replica?.fields() ?? []);
	const projects = $derived(replica?.projects() ?? []);
	let name = $state('');
	let type = $state<FieldType>('text');
	let scope = $state('');

	const typeName = (t: FieldType) => FIELD_TYPES.find((f) => f.value === t)?.label ?? t;
	const scopeName = (f: CustomField) =>
		f.projectId ? (replica?.project(f.projectId)?.name ?? 'A deleted project') : 'Every project';

	function add(e: SubmitEvent) {
		e.preventDefault();
		if (!name.trim()) return;
		void createField(ws, name, type, scope || null);
		name = '';
	}

	function remove(f: CustomField) {
		// Undo takes back this delete, not whatever was edited after it.
		const { undo } = engine.track(() => deleteField(ws, f.id));
		toasts.show(`Deleted the field ${f.name}`, { onundo: () => void undo() });
	}

	function addOption(f: CustomField, e: SubmitEvent) {
		e.preventDefault();
		const input = (e.currentTarget as HTMLFormElement).elements.namedItem(
			'option'
		) as HTMLInputElement;
		const n = input.value.trim();
		if (!n) return;
		input.value = '';
		void editFieldOptions(ws, f.id, (options) => [
			...options,
			{ id: uuid(), name: n, color: PALETTE[options.length % PALETTE.length] }
		]);
	}
</script>

<section id="fields">
	<h2>Custom fields in {settingsWorkspace()?.name}</h2>
	<p class="hint">
		Fields add your own details to tasks, such as a cost or a client. A field can belong to every
		project or to one. Its type is fixed once it is made.
	</p>
	{#if fields.length}
		<ul class="fields" aria-label="Fields">
			{#each fields as f (f.id)}
				<li>
					<div class="row">
						<input
							class="grow"
							aria-label={`Name of ${f.name}`}
							value={f.name}
							onchange={(e) => {
								const n = e.currentTarget.value.trim();
								if (n && n !== f.name) void updateField(ws, f.id, { name: n });
							}}
						/>
						<span class="meta">{typeName(f.type)} · {scopeName(f)}</span>
						<Button title="Delete this field" size="sm" variant="danger" onclick={() => remove(f)}
							>Delete</Button
						>
					</div>
					{#if f.type === 'select' || f.type === 'multi_select'}
						<ul class="options" aria-label={`Choices for ${f.name}`}>
							{#each f.options as o (o.id)}
								<li>
									<LabelChip color={o.color as PaletteColor} name={o.name} />
									<input
										class="grow"
										aria-label={`Name of the choice ${o.name}`}
										value={o.name}
										onchange={(e) => {
											const n = e.currentTarget.value.trim();
											if (n)
												void editFieldOptions(ws, f.id, (options) =>
													options.map((x) => (x.id === o.id ? { ...x, name: n } : x))
												);
										}}
									/>
									<select
										aria-label={`Color of the choice ${o.name}`}
										value={o.color}
										onchange={(e) => {
											const color = e.currentTarget.value;
											void editFieldOptions(ws, f.id, (options) =>
												options.map((x) => (x.id === o.id ? { ...x, color } : x))
											);
										}}
									>
										{#each PALETTE as c (c)}<option value={c}>{c}</option>{/each}
									</select>
									<Button
										title="Remove this choice"
										size="sm"
										onclick={() =>
											void editFieldOptions(ws, f.id, (options) =>
												options.filter((x) => x.id !== o.id)
											)}>Remove</Button
									>
								</li>
							{/each}
							<li>
								<form class="inline-form" onsubmit={(e) => addOption(f, e)}>
									<input
										class="grow"
										name="option"
										aria-label={`New choice for ${f.name}`}
										placeholder="Add a choice"
									/>
								</form>
							</li>
						</ul>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
	<form class="inline-form" onsubmit={add}>
		<input class="grow" aria-label="New field" placeholder="New field" bind:value={name} />
		<select aria-label="Type of the new field" bind:value={type}>
			{#each FIELD_TYPES as t (t.value)}<option value={t.value}>{t.label}</option>{/each}
		</select>
		<select aria-label="Where the new field applies" bind:value={scope}>
			<option value="">Every project</option>
			{#each projects as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
		</select>
		<Button type="submit" variant="primary">Add field</Button>
	</form>
</section>

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
	.fields,
	.options {
		display: grid;
		gap: var(--space-2);
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.fields > li {
		display: grid;
		gap: var(--space-2);
		padding: var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
	}
	.row,
	.options li {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
		align-items: center;
	}
	.grow {
		flex: 1;
		min-width: 8rem;
	}
	.options {
		padding-left: var(--space-4);
	}
	.meta {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
</style>
