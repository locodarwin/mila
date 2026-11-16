<script lang="ts">
	import { anchorOf, type Group, type Operation } from '../../apidocs';

	interface Props {
		groups: Group[];
	}

	let { groups }: Props = $props();

	// Open beside the page on a wide screen; on a phone, folded above it.
	let open = $state(typeof matchMedia !== 'function' || matchMedia('(min-width: 900px)').matches);

	$effect(() => {
		const wide = matchMedia('(min-width: 900px)');
		const follow = () => {
			if (wide.matches) open = true;
		};
		wide.addEventListener('change', follow);
		return () => wide.removeEventListener('change', follow);
	});

	/** Opens the operation picked and puts focus on it, then lets the link scroll there. */
	function pick(op: Operation) {
		const target = document.getElementById(anchorOf(op));
		if (target instanceof HTMLDetailsElement) target.open = true;
		requestAnimationFrame(() => target?.querySelector('summary')?.focus({ preventScroll: true }));
		if (!matchMedia('(min-width: 900px)').matches) open = false;
	}
</script>

<nav class="list" aria-label="Endpoints">
	<details bind:open>
		<summary>Endpoints</summary>
		{#each groups as g (g.tag)}
			<p class="tag" id="nav-{g.tag}">{g.tag}</p>
			<ul aria-labelledby="nav-{g.tag}">
				{#each g.operations as op (op.id)}
					<li>
						<a href="#{anchorOf(op)}" onclick={() => pick(op)}>
							<span class="method">{op.method}</span>
							<span class="label">{op.summary || op.path}</span>
						</a>
					</li>
				{/each}
			</ul>
		{/each}
	</details>
</nav>

<style>
	.list {
		font-size: var(--text-sm);
	}
	summary {
		display: flex;
		align-items: center;
		min-height: max(var(--tap-min), calc(32px * var(--control-scale)));
		padding: 0 var(--space-2);
		font-weight: 600;
		cursor: pointer;
	}
	.tag {
		margin: var(--space-3) var(--space-2) var(--space-1);
		font-size: var(--text-xs);
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--text-secondary);
	}
	ul {
		display: grid;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	a {
		display: flex;
		align-items: baseline;
		gap: var(--space-2);
		min-height: max(var(--tap-min), calc(28px * var(--control-scale)));
		padding: var(--space-1) var(--space-2);
		border-radius: var(--radius-sm);
		color: var(--text-primary);
	}
	a:hover {
		background: var(--surface-2);
	}
	.method {
		flex: none;
		width: 3.6em;
		font-family: var(--font-mono);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.label {
		overflow-wrap: anywhere;
	}
	@media (min-width: 900px) {
		summary {
			display: none;
		}
	}
</style>
