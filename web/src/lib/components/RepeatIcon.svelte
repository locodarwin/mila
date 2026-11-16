<script lang="ts">
	import Repeat from '@lucide/svelte/icons/repeat';

	/** `label` says how it repeats; it is the icon's name and tooltip. */
	let { size = 12, label = 'Repeats' }: { size?: number; label?: string } = $props();
</script>

<span class="repeat" title={label} role="img" aria-label={label}
	><Repeat {size} aria-hidden="true" /></span
>

<style>
	.repeat {
		display: inline-flex;
		flex: none;
		color: var(--text-secondary);
	}
</style>
