<script lang="ts">
	import { flatten, refName, resolveRef, typeOf, type Schema, type Spec } from '../../apidocs';
	import SchemaView from './SchemaView.svelte';

	interface Props {
		spec: Spec;
		schema: Schema;
		/** How deep this view is inside another; objects below 2 show by name. */
		depth?: number;
	}

	let { spec, schema, depth = 0 }: Props = $props();

	const flat = $derived(flatten(spec, schema));
	const name = $derived(refName(schema));

	/** The object a property holds, or holds a list of, to show inside it. */
	function inner(p: Schema): Schema | null {
		const s = resolveRef(spec, p);
		const target = s.type === 'array' && s.items ? s.items : p;
		const t = flatten(spec, target);
		return t.properties && Object.keys(t.properties).length ? target : null;
	}

	/** Limits and defaults, in words. */
	function notes(p: Schema): string[] {
		const s = resolveRef(spec, p);
		const out: string[] = [];
		if (s.default !== undefined) out.push(`Default ${JSON.stringify(s.default)}.`);
		if (s.minLength && s.maxLength) out.push(`${s.minLength} to ${s.maxLength} characters.`);
		else if (s.maxLength) out.push(`At most ${s.maxLength} characters.`);
		if (s.minimum !== undefined && s.maximum !== undefined)
			out.push(`From ${s.minimum} to ${s.maximum}.`);
		if (s.pattern) out.push(`Matches ${s.pattern}.`);
		return out;
	}
</script>

{#if flat.properties && Object.keys(flat.properties).length}
	<div class="schema">
		{#if name && depth === 0}
			<p class="name"><code>{name}</code></p>
		{/if}
		{#if flat.description && depth === 0}
			<p class="about">{flat.description}</p>
		{/if}
		{#if flat.minProperties}
			<p class="about">Give at least {flat.minProperties} of these.</p>
		{/if}
		<dl>
			{#each Object.entries(flat.properties) as [field, p] (field)}
				{@const s = resolveRef(spec, p)}
				{@const nested = depth < 1 ? inner(p) : null}
				<div class="field">
					<dt>
						<code class="key">{field}</code>
						<span class="type">{typeOf(spec, p)}</span>
						{#if flat.required?.includes(field) && !s.readOnly}
							<span class="flag">required</span>
						{/if}
						{#if s.readOnly}
							<span class="flag">read-only</span>
						{/if}
					</dt>
					<dd>
						{#if s.description}<span>{s.description}</span>{/if}
						{#each notes(p) as n (n)}<span>{n}</span>{/each}
						{#if nested}
							<SchemaView {spec} schema={nested} depth={depth + 1} />
						{/if}
					</dd>
				</div>
			{/each}
		</dl>
	</div>
{:else}
	<p class="plain"><span class="type">{typeOf(spec, schema)}</span></p>
{/if}

<style>
	.schema {
		display: grid;
		gap: var(--space-2);
	}
	.name {
		font-weight: 600;
	}
	.about {
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	dl {
		display: grid;
		margin: 0;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
	}
	.field {
		display: grid;
		gap: var(--space-1);
		padding: var(--space-2) var(--space-3);
	}
	.field + .field {
		border-top: var(--border-width) solid var(--border);
	}
	dt {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-2);
	}
	dd {
		display: grid;
		gap: var(--space-1);
		margin: 0;
		font-size: var(--text-sm);
		color: var(--text-secondary);
		overflow-wrap: anywhere;
	}
	dd :global(.schema) {
		margin-top: var(--space-1);
	}
	.key {
		font-weight: 600;
		color: var(--text-primary);
	}
	.type {
		font-size: var(--text-xs);
		color: var(--text-secondary);
		overflow-wrap: anywhere;
	}
	.flag {
		padding: 0 var(--space-1);
		border-radius: var(--radius-sm);
		background: var(--surface-2);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.plain {
		font-size: var(--text-sm);
	}
</style>
