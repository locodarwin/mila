<script lang="ts">
	import Ban from '@lucide/svelte/icons/ban';

	// The Blocked mark: a task waiting for one
	// that is not done. The names say what it waits for.
	let { names }: { names: string[] } = $props();
	const label = $derived(`Blocked by ${names.join(', ')}`);
</script>

<span class="blocked" title={label}
	><Ban size={11} aria-hidden="true" /><span aria-hidden="true">Blocked</span><span
		class="visually-hidden">{label}</span
	></span
>

<style>
	.blocked {
		display: inline-flex;
		flex: none;
		align-items: center;
		gap: 3px;
		padding: 0 4px;
		border: var(--border-width) solid currentColor;
		border-radius: var(--radius-sm);
		color: var(--danger);
		font-size: var(--text-xs);
		font-weight: 600;
		line-height: 1.4;
		white-space: nowrap;
	}
</style>
