<script lang="ts">
	// A custom field's value on a row or card, read-only. What it shows is
	// said in words too ("Size: Large"), for screen readers.
	import Check from '@lucide/svelte/icons/check';
	import { formatDate } from '../../dates';
	import { session } from '../../session.svelte';
	import type { CustomField, FieldValue } from '../../sync/entities';
	import Avatar from '../task/Avatar.svelte';

	let { ws, field, value }: { ws: string; field: CustomField; value: FieldValue | undefined } =
		$props();

	const options = $derived(
		field.type === 'select' || field.type === 'multi_select'
			? (Array.isArray(value) ? value : [value]).flatMap((id) => {
					const o = field.options.find((x) => x.id === id);
					return o ? [o] : [];
				})
			: []
	);
	const host = $derived.by(() => {
		if (typeof value !== 'string') return '';
		try {
			return new URL(value).hostname || value;
		} catch {
			return value;
		}
	});
	const person = $derived(
		field.type === 'user' && typeof value === 'string' ? session.nameOf(ws, value) : ''
	);
	/** The value in words. */
	const said = $derived.by(() => {
		switch (field.type) {
			case 'select':
			case 'multi_select':
				return options.map((o) => o.name).join(', ');
			case 'checkbox':
				return value === true ? 'checked' : 'not checked';
			case 'date':
				return typeof value === 'string' ? formatDate(value) : '';
			case 'user':
				return person;
			case 'url':
				return host;
			default:
				return value === null || value === undefined ? '' : String(value);
		}
	});
</script>

{#if said && !(field.type === 'checkbox' && value !== true)}
	<span class="field" title={`${field.name}: ${said}`}>
		<span class="visually-hidden">{field.name}: {said}</span>
		<span class="shown" aria-hidden="true">
			{#if field.type === 'select' || field.type === 'multi_select'}
				{#each options as o (o.id)}<span
						class="option"
						style:--chip-soft={`var(--palette-${o.color}-soft)`}>{o.name}</span
					>{/each}
			{:else if field.type === 'checkbox'}
				<Check size={12} />{field.name}
			{:else if field.type === 'user'}
				<Avatar name={person} size={18} />
			{:else if field.type === 'number'}
				<span class="tabular">{said}</span>
			{:else}
				<span class="text">{said}</span>
			{/if}
		</span>
	</span>
{/if}

<style>
	.field {
		position: relative;
		display: inline-flex;
		align-items: center;
		min-width: 0;
		max-width: 12rem;
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.shown {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		min-width: 0;
	}
	.option {
		height: calc(20px * var(--ui-scale));
		padding: 0 6px;
		border-radius: var(--radius-full);
		background: var(--chip-soft);
		color: var(--text-primary);
		font-weight: 550;
		line-height: calc(20px * var(--ui-scale));
		white-space: nowrap;
	}
	.text {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
