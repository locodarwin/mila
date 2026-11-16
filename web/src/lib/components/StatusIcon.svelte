<script lang="ts">
	import type { StatusCategory } from '../theme/palette';

	let { category, size = 16 }: { category: StatusCategory; size?: number } = $props();

	const names: Record<StatusCategory, string> = {
		backlog: 'Backlog',
		todo: 'To do',
		'in-progress': 'In progress',
		done: 'Done',
		canceled: 'Canceled'
	};
</script>

<!-- Shape carries the meaning as well as color: dashed, empty, half, full, crossed. -->
<svg
	class="status {category}"
	width={size}
	height={size}
	viewBox="0 0 16 16"
	role="img"
	aria-label={names[category]}
>
	{#if category === 'backlog'}
		<circle cx="8" cy="8" r="6" stroke-dasharray="2.4 2" />
	{:else if category === 'todo'}
		<circle cx="8" cy="8" r="6" />
	{:else if category === 'in-progress'}
		<circle cx="8" cy="8" r="6" />
		<path class="fill" d="M8 4a4 4 0 0 1 0 8z" />
	{:else if category === 'done'}
		<circle class="fill" cx="8" cy="8" r="6.75" />
		<path class="mark" d="M5.2 8.2l1.9 1.9 3.7-4" />
	{:else}
		<circle cx="8" cy="8" r="6" />
		<path d="M5.8 5.8l4.4 4.4M10.2 5.8l-4.4 4.4" />
	{/if}
</svg>

<style>
	.status {
		flex: none;
		fill: none;
		stroke: currentColor;
		stroke-width: 1.5;
		stroke-linecap: round;
	}
	.fill {
		fill: currentColor;
		stroke: none;
	}
	.mark {
		stroke: var(--surface-0);
		stroke-width: 1.8;
		stroke-linejoin: round;
	}
	.backlog {
		color: var(--status-backlog);
	}
	.todo {
		color: var(--status-todo);
	}
	.in-progress {
		color: var(--status-in-progress);
	}
	.done {
		color: var(--status-done);
	}
	.canceled {
		color: var(--status-canceled);
	}
</style>
