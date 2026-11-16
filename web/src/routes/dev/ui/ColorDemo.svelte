<script lang="ts">
	// Every color token the registry has, by group: what a theme can
	// change. Loaded apart, with the registry, so the style guide stays
	// inside its script budget.
	import { GROUPS, TOKENS } from '#lib/theme/tokens.js';

	const groups = GROUPS.map((g) => ({
		...g,
		colors: TOKENS.filter((t) => t.group === g.id && t.type === 'color')
	})).filter((g) => g.colors.length);
</script>

{#each groups as g (g.id)}
	<h3>{g.label}</h3>
	<div class="swatches">
		{#each g.colors as t (t.name)}
			<figure>
				<div
					class="swatch"
					style:background-image={`linear-gradient(var(--${t.name}), var(--${t.name}))`}
				></div>
				<figcaption>{t.name}</figcaption>
			</figure>
		{/each}
	</div>
{/each}

<style>
	h3 {
		font-size: var(--text-md);
	}
	.swatches {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
		gap: var(--space-3);
	}
	figure {
		margin: 0;
	}
	.swatch {
		height: 40px;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		/* See-through tokens (the scrims) show over the page's raised surface. */
		background-color: var(--surface-1);
	}
	figcaption {
		margin-top: var(--space-1);
		font-family: var(--font-mono);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
</style>
