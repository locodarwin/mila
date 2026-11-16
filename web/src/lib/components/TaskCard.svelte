<script lang="ts">
	import LabelChip from './LabelChip.svelte';
	import PriorityIcon from './PriorityIcon.svelte';
	import StatusIcon from './StatusIcon.svelte';
	import type { PaletteColor, Priority, StatusCategory } from '../theme/palette';

	interface Props {
		title: string;
		status: StatusCategory;
		priority: Priority;
		labels?: { name: string; color: PaletteColor }[];
		due?: string;
		lifted?: boolean;
	}

	let { title, status, priority, labels = [], due, lifted = false }: Props = $props();
</script>

<article class="card" class:lifted>
	<header>
		<StatusIcon category={status} />
		<h3>{title}</h3>
	</header>
	<!-- Dense mode hides secondary metadata until hover. -->
	<footer class="secondary">
		<PriorityIcon {priority} />
		{#each labels as label (label.name)}
			<LabelChip {...label} />
		{/each}
		{#if due}<span class="due tabular">{due}</span>{/if}
	</footer>
</article>

<style>
	.card {
		display: grid;
		gap: var(--space-2);
		padding: calc(var(--space-3) * var(--control-scale));
		border: var(--border-width) solid var(--card-border);
		border-radius: var(--card-radius);
		background: var(--card-bg);
		box-shadow: var(--card-shadow);
		transition:
			box-shadow var(--dur-panel) var(--ease),
			transform var(--dur-panel) var(--ease-spring);
	}
	.card:hover {
		border-color: var(--border-strong);
	}
	.lifted {
		box-shadow: var(--card-shadow-lifted);
		transform: rotate(calc(-1.5deg * var(--motion))) scale(calc(1 + 0.03 * var(--motion)));
	}
	header {
		display: flex;
		gap: var(--space-2);
		align-items: flex-start;
	}
	header :global(svg) {
		margin-top: 2px;
	}
	h3 {
		font-family: var(--font-ui);
		font-size: var(--text-md);
		font-weight: 550;
	}
	footer {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
		align-items: center;
	}
	.due {
		margin-left: auto;
		font-size: var(--text-xs);
		color: var(--row-meta-text);
	}
	:global([data-density='dense']) .secondary {
		display: none;
	}
	:global([data-density='dense']) .card:hover .secondary,
	:global([data-density='dense']) .card:focus-within .secondary {
		display: flex;
	}
</style>
