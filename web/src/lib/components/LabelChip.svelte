<script lang="ts">
	import type { PaletteColor } from '../theme/palette';

	let { color, name }: { color: PaletteColor; name: string } = $props();
</script>

<span
	class="chip"
	style:--chip={`var(--palette-${color})`}
	style:--chip-soft={`var(--palette-${color}-soft)`}
>
	<span class="dot" aria-hidden="true"></span>{name}
</span>

<style>
	.chip {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: calc(22px * var(--ui-scale));
		padding: 0 8px;
		border-radius: var(--radius-full);
		background: var(--chip-soft);
		color: var(--text-primary);
		font-size: var(--text-xs);
		font-weight: 550;
		white-space: nowrap;
	}
	.dot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--chip);
	}
</style>
