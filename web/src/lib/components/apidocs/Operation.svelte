<script lang="ts">
	import {
		anchorOf,
		blocks,
		resolveRef,
		typeOf,
		type Operation,
		type Schema,
		type Spec
	} from '../../apidocs';
	import SchemaView from './SchemaView.svelte';
	import TryIt from './TryIt.svelte';

	interface Props {
		spec: Spec;
		op: Operation;
		/** Its parts are drawn only once it is opened: there are many of them. */
		open?: boolean;
	}

	let { spec, op, open = $bindable(false) }: Props = $props();

	const success = $derived(op.responses.find((r) => r.status.startsWith('2') && r.schema));
	const id = $derived(anchorOf(op));

	function about(s: Schema | undefined): string {
		return s ? (resolveRef(spec, s).description ?? '') : '';
	}
</script>

<details class="op" {id} bind:open>
	<summary>
		<span class="method">{op.method}</span>
		<code class="path">{op.path}</code>
		{#if op.summary}<span class="summary">{op.summary}</span>{/if}
	</summary>
	{#if open}
		<div class="body">
			{#each blocks(op.description) as b, i (i)}
				{#if b.kind === 'p'}
					<p>{b.text}</p>
				{:else}
					<ul>
						{#each b.items as item (item)}<li>{item}</li>{/each}
					</ul>
				{/if}
			{/each}
			{#if !op.auth}<p class="hint">Needs no sign-in.</p>{/if}

			{#if op.parameters.length}
				<h3>Parameters</h3>
				<dl class="params">
					{#each op.parameters as p (p.in + p.name)}
						<div>
							<dt>
								<code>{p.name}</code>
								<span class="hint">{p.in}{p.required ? ', required' : ''}</span>
								{#if p.schema}<span class="hint">{typeOf(spec, p.schema)}</span>{/if}
							</dt>
							{#if p.description || about(p.schema)}
								<dd>{p.description || about(p.schema)}</dd>
							{/if}
						</div>
					{/each}
				</dl>
			{/if}

			{#if op.body}
				<h3>Body</h3>
				{#if op.body.type !== 'application/json'}<p class="hint">{op.body.type}</p>{/if}
				{#if op.body.schema}<SchemaView {spec} schema={op.body.schema} />{/if}
			{/if}

			<h3>Responses</h3>
			<ul class="responses">
				{#each op.responses as r (r.status)}
					<li>
						<code>{r.status === 'default' ? 'Otherwise' : r.status}</code>
						{r.description}
						{#if r.schema}<span class="hint">{typeOf(spec, r.schema)}</span>{/if}
					</li>
				{/each}
			</ul>
			{#if success?.schema}
				<SchemaView {spec} schema={success.schema} />
			{/if}

			<h3>Try it</h3>
			<TryIt {spec} {op} />
		</div>
	{/if}
</details>

<style>
	.op {
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-0);
	}
	summary {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-1) var(--space-3);
		min-height: max(var(--tap-min), calc(36px * var(--control-scale)));
		padding: var(--space-2) var(--space-3);
		cursor: pointer;
		border-radius: var(--radius-md);
	}
	summary:hover {
		background: var(--surface-1);
	}
	.method {
		font-family: var(--font-mono);
		font-size: var(--text-xs);
		font-weight: 700;
		color: var(--accent-text);
	}
	.path {
		overflow-wrap: anywhere;
	}
	.summary {
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	.body {
		display: grid;
		gap: var(--space-3);
		padding: var(--space-2) var(--space-4) var(--space-4);
		border-top: var(--border-width) solid var(--border);
	}
	.body > p,
	.body > ul {
		max-width: 72ch;
	}
	ul {
		margin: 0;
		padding-left: var(--space-5);
	}
	h3 {
		margin-top: var(--space-2);
		font-size: var(--text-md);
	}
	.hint {
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	.params {
		display: grid;
		gap: var(--space-2);
		margin: 0;
	}
	.params dt {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
		align-items: baseline;
	}
	.params dd {
		margin: 0;
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.responses {
		display: grid;
		gap: var(--space-1);
		padding: 0;
		list-style: none;
	}
</style>
