<script lang="ts">
	import type { Priority } from '../theme/palette';

	let { priority, size = 16 }: { priority: Priority; size?: number } = $props();

	const bars: Record<Priority, number> = { none: 0, low: 1, medium: 2, high: 3, urgent: 0 };
	const names: Record<Priority, string> = {
		none: 'No priority',
		low: 'Low priority',
		medium: 'Medium priority',
		high: 'High priority',
		urgent: 'Urgent'
	};
</script>

<!-- Bars fill from the left like a signal meter; urgent is its own shape. -->
<svg
	class="priority {priority}"
	width={size}
	height={size}
	viewBox="0 0 16 16"
	role="img"
	aria-label={names[priority]}
>
	{#if priority === 'urgent'}
		<rect x="1.5" y="1.5" width="13" height="13" rx="3" class="solid" />
		<path d="M8 4.5v4.2M8 11.3v.2" class="mark" />
	{:else if priority === 'none'}
		<path d="M3 8h1M7.5 8h1M12 8h1" class="dots" />
	{:else}
		{#each [0, 1, 2] as i (i)}
			<rect
				x={2 + i * 4.5}
				y={10 - i * 3}
				width="3"
				height={4 + i * 3}
				rx="1"
				class:on={i < bars[priority]}
			/>
		{/each}
	{/if}
</svg>

<style>
	.priority {
		flex: none;
	}
	rect {
		fill: var(--border-strong);
	}
	rect.on,
	.solid {
		fill: currentColor;
	}
	.dots {
		stroke: currentColor;
		stroke-width: 1.6;
		stroke-linecap: round;
	}
	.mark {
		stroke: var(--surface-0);
		stroke-width: 2;
		stroke-linecap: round;
	}
	.none {
		color: var(--priority-none);
	}
	.low {
		color: var(--priority-low);
	}
	.medium {
		color: var(--priority-medium);
	}
	.high {
		color: var(--priority-high);
	}
	.urgent {
		color: var(--priority-urgent);
	}
</style>
