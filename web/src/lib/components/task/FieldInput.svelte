<script lang="ts">
	import ExternalLink from '@lucide/svelte/icons/external-link';
	import Checkbox from '../Checkbox.svelte';
	import { session } from '../../session.svelte';
	import { setFieldValue } from '../../sync/data';
	import type { CustomField, FieldValue } from '../../sync/entities';

	let {
		ws,
		taskId,
		field,
		value
	}: { ws: string; taskId: string; field: CustomField; value: FieldValue | undefined } = $props();

	const set = (v: FieldValue) => void setFieldValue(ws, taskId, field.id, v);
	const chosen = $derived(Array.isArray(value) ? value : []);
	const safeUrl = $derived(
		typeof value === 'string' && /^https?:\/\//i.test(value) ? value : undefined
	);
</script>

{#if field.type === 'checkbox'}
	<Checkbox label={field.name} hideLabel checked={value === true} onchange={(on) => set(on)} />
{:else if field.type === 'select'}
	<select
		aria-label={field.name}
		value={typeof value === 'string' ? value : ''}
		onchange={(e) => set(e.currentTarget.value || null)}
	>
		<option value="">None</option>
		{#each field.options as o (o.id)}<option value={o.id}>{o.name}</option>{/each}
	</select>
{:else if field.type === 'multi_select'}
	<div class="choices" role="group" aria-label={field.name}>
		{#each field.options as o (o.id)}
			{@const on = chosen.includes(o.id)}
			<button
				type="button"
				class="choice"
				aria-pressed={on}
				style:--chip={`var(--palette-${o.color})`}
				onclick={() => set(on ? chosen.filter((x) => x !== o.id) : [...chosen, o.id])}
				>{o.name}</button
			>
		{:else}
			<span class="none">No choices yet; add them in Settings, Labels and fields.</span>
		{/each}
	</div>
{:else if field.type === 'user'}
	<select
		aria-label={field.name}
		value={typeof value === 'string' ? value : ''}
		onchange={(e) => set(e.currentTarget.value || null)}
	>
		<option value="">Nobody</option>
		{#each session.members(ws) as m (m.userId)}<option value={m.userId}>{m.name}</option>{/each}
		<!-- Someone who is not a member now stays named, until it is changed. -->
		{#if typeof value === 'string' && value && !session.memberName(ws, value)}
			<option {value}>{session.nameOf(ws, value)}</option>
		{/if}
	</select>
{:else if field.type === 'number'}
	<input
		type="number"
		step="any"
		aria-label={field.name}
		value={typeof value === 'number' ? value : ''}
		onchange={(e) => {
			const n = e.currentTarget.valueAsNumber;
			set(Number.isFinite(n) ? n : null);
		}}
	/>
{:else if field.type === 'date'}
	<input
		type="date"
		aria-label={field.name}
		value={typeof value === 'string' ? value : ''}
		onchange={(e) => set(e.currentTarget.value || null)}
	/>
{:else}
	<span class="text">
		<input
			type={field.type === 'url' ? 'url' : 'text'}
			aria-label={field.name}
			placeholder={field.type === 'url' ? 'https://' : 'Empty'}
			value={typeof value === 'string' ? value : ''}
			onchange={(e) => set(e.currentTarget.value.trim() || null)}
		/>
		{#if field.type === 'url' && safeUrl}
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- a link out of the app -->
			<a href={safeUrl} target="_blank" rel="noopener noreferrer" aria-label={`Open ${field.name}`}
				><ExternalLink size={14} /></a
			>
		{/if}
	</span>
{/if}

<style>
	select,
	input {
		max-width: 100%;
		text-overflow: ellipsis;
		height: 30px;
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
		font-size: var(--text-sm);
	}
	input::placeholder {
		color: var(--input-placeholder);
	}
	select:hover,
	input:hover {
		border-color: var(--border-strong);
	}
	.text {
		display: flex;
		gap: var(--space-2);
		align-items: center;
	}
	.text input {
		flex: 1;
		min-width: 0;
	}
	.text a {
		color: var(--text-secondary);
	}
	.choices {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1);
	}
	.choice {
		padding: 2px var(--space-2);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-full);
		background: var(--surface-0);
		color: var(--text-secondary);
		font-size: var(--text-xs);
		cursor: pointer;
	}
	.choice[aria-pressed='true'] {
		border-color: var(--chip);
		background: color-mix(in oklch, var(--chip) 18%, var(--surface-0));
		color: var(--text-primary);
	}
	.none {
		font-size: var(--text-sm);
		color: var(--text-muted);
	}
</style>
