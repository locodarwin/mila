<script lang="ts">
	import { pictures } from '../../pictures.svelte';

	// label: what a screen reader hears ("Assigned to Ann") where the
	// avatar stands alone; left out where the name is written beside it.
	// src: the person's picture, drawn over the initials once it loads;
	// any failure (offline and not cached, signed out, removed) leaves the
	// initials until pictures are tried again.
	let {
		name,
		size = 22,
		label,
		src = null
	}: { name: string; size?: number; label?: string; src?: string | null } = $props();
	const initials = $derived(
		name
			.split(/\s+/)
			.filter(Boolean)
			.slice(0, 2)
			.map((w) => w[0]!.toUpperCase())
			.join('') || '?'
	);
	// A stable color per name from the label palette.
	const colors = ['blue', 'green', 'purple', 'orange', 'teal', 'pink', 'indigo', 'red'];
	const color = $derived(
		colors[[...name].reduce((h, c) => (h * 31 + c.charCodeAt(0)) >>> 0, 7) % colors.length]
	);
	// A failed picture is tried again in the next round (pictures.svelte.ts).
	let failed = $state<{ src: string; round: number } | null>(null);
	let loadedSrc = $state<string | null>(null);
	const picture = $derived(
		src && !(failed?.src === src && failed.round === pictures.round) ? src : null
	);
</script>

<span
	class="avatar"
	class:pictured={picture}
	class:loaded={picture && loadedSrc === picture}
	title={name}
	role={label ? 'img' : undefined}
	aria-label={label}
	aria-hidden={label ? undefined : 'true'}
	style:width={`${size}px`}
	style:height={`${size}px`}
	style:--chip-soft={`var(--palette-${color}-soft)`}
	style:font-size={size >= 48 ? `${Math.round(size * 0.38)}px` : undefined}
	>{initials}{#if picture}<img
			src={picture}
			alt=""
			width={size}
			height={size}
			decoding="async"
			draggable="false"
			onload={() => (loadedSrc = picture)}
			onerror={() => (failed = picture ? { src: picture, round: pictures.round } : null)}
		/>{/if}</span
>

<style>
	.avatar {
		display: inline-grid;
		flex: none;
		place-items: center;
		border-radius: 50%;
		background: var(--chip-soft);
		color: var(--text-primary);
		font-size: calc(10px * var(--ui-scale));
		font-weight: 700;
	}
	.pictured {
		position: relative;
	}
	.loaded {
		color: transparent;
	}
	img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		border-radius: 50%;
		object-fit: cover;
	}
</style>
