<script lang="ts">
	// One condition on a custom field in a saved view's filter: the field,
	// how it compares, and what with. lib/filter.ts says what each means.
	import Check from '@lucide/svelte/icons/check';
	import X from '@lucide/svelte/icons/x';
	import { FIELD_OPS } from '../../filter';
	import { session } from '../../session.svelte';
	import { localToday } from '../../sync/data';
	import type { CustomField, FieldCondition, FieldOp, FieldType } from '../../sync/entities';
	import { conditionChoices } from '../../viewDisplay';

	interface Props {
		ws: string;
		condition: FieldCondition;
		/** The fields a condition can be on, in order. */
		fields: CustomField[];
		/** Which condition this is, for names that tell rows apart. */
		n: number;
		/**
		 * A change, as how to work out the condition from itself as it stands
		 * when it is saved, so quick changes build on each other.
		 */
		onchange: (edit: (condition: FieldCondition) => FieldCondition) => void;
		onremove: () => void;
	}

	let { ws, condition, fields, n, onchange, onremove }: Props = $props();

	const field = $derived(fields.find((f) => f.id === condition.field));
	// A field of a type from a newer app has no operators here.
	const ops = $derived(field ? (FIELD_OPS[field.type] ?? []) : []);

	const OP_WORDS: Record<FieldOp, string> = {
		contains: 'contains',
		not_contains: 'does not contain',
		eq: 'is',
		lt: 'is less than',
		lte: 'is at most',
		gt: 'is more than',
		gte: 'is at least',
		on: 'is on',
		before: 'is before',
		after: 'is after',
		between: 'is between',
		any: 'is any of',
		all: 'has all of',
		none_of: 'is none of',
		checked: 'is checked',
		unchecked: 'is not checked',
		empty: 'is empty',
		not_empty: 'is not empty'
	};
	const words = (op: FieldOp, type: FieldType) =>
		type === 'multi_select' && op === 'any'
			? 'has any of'
			: type === 'multi_select' && op === 'none_of'
				? 'has none of'
				: OP_WORDS[op];

	/** What a new operator starts with, so the condition holds something sensible. */
	function startValue(op: FieldOp): unknown {
		if (op === 'between') return [0, 7];
		if (op === 'on' || op === 'before' || op === 'after') return 0;
		if (op === 'eq' || op === 'lt' || op === 'lte' || op === 'gt' || op === 'gte') return 0;
		return undefined;
	}

	function setField(id: string) {
		const f = fields.find((x) => x.id === id);
		const op = f && FIELD_OPS[f.type]?.[0];
		if (!op) return;
		onchange(() => ({ field: id, op, value: startValue(op) }));
	}

	/** What kind of value each operator takes; a change within a kind keeps the value. */
	const KIND: Record<FieldOp, string> = {
		contains: 'text',
		not_contains: 'text',
		eq: 'number',
		lt: 'number',
		lte: 'number',
		gt: 'number',
		gte: 'number',
		on: 'day',
		before: 'day',
		after: 'day',
		between: 'days',
		any: 'list',
		all: 'list',
		none_of: 'list',
		checked: 'none',
		unchecked: 'none',
		empty: 'none',
		not_empty: 'none'
	};

	function setOp(op: FieldOp) {
		onchange((c) => ({
			...c,
			op,
			value: KIND[op] === KIND[c.op] ? c.value : startValue(op)
		}));
	}

	const set = (value: unknown) => onchange((c) => ({ ...c, value }));

	const listOf = (v: unknown) => (Array.isArray(v) ? (v as string[]) : []);
	const chosen = $derived(listOf(condition.value));
	function toggle(id: string) {
		onchange((c) => {
			const now = listOf(c.value);
			return { ...c, value: now.includes(id) ? now.filter((x) => x !== id) : [...now, id] };
		});
	}
	const choices = $derived(
		field
			? conditionChoices(field, chosen, {
					me: session.user?.id ?? '',
					members: session.members(ws),
					nameOf: (id) => session.nameOf(ws, id)
				})
			: []
	);

	const DAYS = [
		{ value: '0', label: 'Today' },
		{ value: '1', label: 'Tomorrow' },
		{ value: '7', label: 'A week from now' },
		{ value: '-7', label: 'A week ago' },
		{ value: 'date', label: 'A date…' }
	];
	/** Which choice of the day menu a bound is. */
	const dayChoice = (bound: unknown) =>
		typeof bound === 'number' && DAYS.some((d) => d.value === String(bound))
			? String(bound)
			: 'date';
	const bounds = $derived(
		condition.op === 'between'
			? Array.isArray(condition.value)
				? condition.value
				: [0, 7]
			: [condition.value]
	);
	function setBound(i: number, bound: unknown) {
		onchange((c) => {
			if (c.op !== 'between') return { ...c, value: bound };
			const next = Array.isArray(c.value) ? [...c.value] : [0, 7];
			next[i] = bound;
			return { ...c, value: next };
		});
	}
</script>

<div
	class="condition"
	role="group"
	aria-label={`Condition ${n} on ${field?.name ?? 'a deleted field'}`}
>
	{#if field}
		<select
			aria-label={`Condition ${n} field`}
			value={field.id}
			onchange={(e) => setField(e.currentTarget.value)}
		>
			{#each fields as f (f.id)}<option value={f.id}>{f.name}</option>{/each}
		</select>
		<select
			aria-label={`${field.name} comparison`}
			value={condition.op}
			onchange={(e) => setOp(e.currentTarget.value as FieldOp)}
		>
			{#if !ops.includes(condition.op)}<option value={condition.op}>Not available</option>{/if}
			{#each ops as op (op)}<option value={op}>{words(op, field.type)}</option>{/each}
		</select>
		<span class="value">
			{#if condition.op === 'contains' || condition.op === 'not_contains'}
				<input
					type="text"
					aria-label={`${field.name} text`}
					placeholder="Words"
					value={typeof condition.value === 'string' ? condition.value : ''}
					onchange={(e) => set(e.currentTarget.value.trim() || undefined)}
				/>
			{:else if ['eq', 'lt', 'lte', 'gt', 'gte'].includes(condition.op)}
				<input
					type="number"
					step="any"
					aria-label={`${field.name} number`}
					value={typeof condition.value === 'number' ? condition.value : ''}
					onchange={(e) => {
						const v = e.currentTarget.valueAsNumber;
						set(Number.isFinite(v) ? v : 0);
					}}
				/>
			{:else if ['on', 'before', 'after', 'between'].includes(condition.op)}
				{#each bounds as bound, i (i)}
					{@const which = condition.op === 'between' ? (i === 0 ? ' from' : ' to') : ' date'}
					<select
						aria-label={`${field.name}${which}`}
						value={dayChoice(bound)}
						onchange={(e) => {
							const v = e.currentTarget.value;
							setBound(i, v === 'date' ? localToday() : Number(v));
						}}
					>
						{#each DAYS as d (d.value)}<option value={d.value}>{d.label}</option>{/each}
					</select>
					{#if dayChoice(bound) === 'date'}
						<input
							type="date"
							aria-label={`${field.name}${which}, exact`}
							value={typeof bound === 'string' ? bound : ''}
							onchange={(e) => e.currentTarget.value && setBound(i, e.currentTarget.value)}
						/>
					{/if}
					{#if condition.op === 'between' && i === 0}<span class="and">and</span>{/if}
				{/each}
			{:else if ['any', 'all', 'none_of'].includes(condition.op)}
				<span class="chips" role="group" aria-label={`${field.name} choices`}>
					{#each choices as c (c.id)}
						{@const on = chosen.includes(c.id)}
						<button type="button" aria-pressed={on} onclick={() => toggle(c.id)}
							>{#if on}<Check size={12} />{/if}{c.name}</button
						>
					{:else}
						<span class="none">No choices yet</span>
					{/each}
				</span>
			{/if}
		</span>
	{:else}
		<span class="gone">A deleted field</span>
	{/if}
	<button
		class="remove"
		data-remove-condition
		type="button"
		aria-label={`Remove ${field?.name ?? 'deleted field'} condition`}
		title="Remove this condition"
		onclick={onremove}><X size={14} /></button
	>
</div>

<style>
	.condition {
		position: relative;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
	.value {
		display: inline-flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
	select,
	input {
		min-height: max(var(--tap-min), 30px);
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
		font-size: var(--text-sm);
	}
	input::placeholder {
		color: var(--input-placeholder);
	}
	input[type='text'],
	input[type='number'] {
		width: 10rem;
	}
	.and,
	.gone,
	.none {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1);
	}
	.chips button {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		min-height: max(var(--tap-min), 24px);
		padding: 2px var(--space-2);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-full);
		background: var(--surface-0);
		color: var(--text-secondary);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.chips button[aria-pressed='true'] {
		border-color: var(--accent);
		background: var(--accent-soft);
		color: var(--text-primary);
	}
	.remove {
		display: grid;
		place-items: center;
		min-width: max(var(--tap-min), 28px);
		min-height: max(var(--tap-min), 28px);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.remove:hover {
		background: var(--surface-3);
		color: var(--text-primary);
	}
	@media (max-width: 720px) {
		.condition {
			display: grid;
			grid-template-columns: 1fr auto;
		}
		.condition > select,
		.value {
			grid-column: 1;
		}
		.remove {
			grid-column: 2;
			grid-row: 1;
		}
	}
</style>
