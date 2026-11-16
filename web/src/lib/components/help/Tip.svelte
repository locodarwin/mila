<script lang="ts">
	import Lightbulb from '@lucide/svelte/icons/lightbulb';
	import type { Snippet } from 'svelte';

	// Something worth remembering, set apart from a help topic's text.
	let { children }: { children: Snippet } = $props();
</script>

<div class="tip" role="note">
	<Lightbulb size={16} />
	<p><span class="visually-hidden">Tip: </span>{@render children()}</p>
</div>

<style>
	.tip {
		position: relative;
		display: flex;
		gap: var(--space-3);
		padding: var(--space-3) var(--space-4);
		border-radius: var(--radius-md);
		background: var(--accent-soft);
	}
	.tip :global(svg) {
		flex: none;
		margin-top: 0.25em;
		color: var(--accent-text);
	}
</style>
