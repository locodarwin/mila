<script lang="ts">
	import { PALETTE } from '../theme/palette';
	import { iconSet, LUCIDE } from './project/iconSet.svelte';

	interface Props {
		/** An emoji, "lucide:<name>", or "" for the plain color dot. */
		icon: string;
		color: string;
		size?: number;
	}

	let { icon, color, size = 16 }: Props = $props();

	const lucide = $derived(icon?.startsWith(LUCIDE) ? icon.slice(LUCIDE.length) : null);
	const Icon = $derived(lucide ? iconSet.get(lucide) : undefined);
	const tone = $derived((PALETTE as readonly string[]).includes(color) ? color : 'blue');

	$effect(() => {
		if (lucide) void iconSet.load();
	});
</script>

<!-- Shown beside the project's name, which says everything it does. -->
<span
	class="project-icon"
	style:--project-color={`var(--palette-${tone})`}
	style:--icon-size={`${size}px`}
	aria-hidden="true"
>
	{#if Icon}
		<Icon {size} />
	{:else if icon && !lucide}
		<span class="emoji">{icon}</span>
	{:else}
		<span class="dot"></span>
	{/if}
</span>

<style>
	.project-icon {
		display: inline-grid;
		flex: none;
		place-items: center;
		width: calc(var(--icon-size) * var(--ui-scale));
		height: calc(var(--icon-size) * var(--ui-scale));
		color: var(--project-color);
	}
	.project-icon :global(svg) {
		width: 100%;
		height: 100%;
	}
	.emoji {
		font-size: calc(var(--icon-size) * var(--ui-scale) * 0.9);
		line-height: 1;
	}
	.dot {
		width: 50%;
		height: 50%;
		border-radius: 50%;
		background: var(--project-color);
	}
</style>
