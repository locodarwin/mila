<script lang="ts">
	import sizes from '../../help/shots.json';

	// A picture of the app in a help topic, in the scheme the person has. Made by
	// `make shots`; loaded when read, and never kept for offline by the service worker.
	interface Props {
		name: keyof typeof sizes;
		/** What the picture shows, for anyone who can't see it, or is offline. */
		alt: string;
		/** At once rather than when scrolled to: the style guide's screenshots. */
		eager?: boolean;
	}

	// How many image pixels `make shots` takes for each of the page's.
	const SCALE = 1.5;

	let { name, alt, eager = false }: Props = $props();
	const size = $derived(sizes[name]);
	const loading = $derived(eager ? 'eager' : 'lazy');
</script>

<figure>
	{#each ['light', 'dark'] as scheme (scheme)}
		<img
			class={scheme}
			src="/help-images/{name}-{scheme}.webp"
			{alt}
			width={size[0]}
			height={size[1]}
			style:max-width="{size[0] / SCALE}px"
			{loading}
			decoding="async"
		/>
	{/each}
</figure>

<style>
	figure {
		margin: 0;
	}
	img {
		display: block;
		width: 100%;
		height: auto;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
	}
	.dark,
	:global([data-scheme='dark']) .light {
		display: none;
	}
	:global([data-scheme='dark']) .dark {
		display: block;
	}
</style>
