<script lang="ts">
	import Bell from '@lucide/svelte/icons/bell';

	/** `label` says what reminders the task has; it is the icon's name and tooltip. */
	let { size = 12, label = 'Has a reminder' }: { size?: number; label?: string } = $props();
</script>

<span class="reminder" title={label} role="img" aria-label={label}
	><Bell {size} aria-hidden="true" /></span
>

<style>
	.reminder {
		display: inline-flex;
		flex: none;
		color: var(--text-secondary);
	}
</style>
