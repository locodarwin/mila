<script lang="ts">
	import type { SyncState } from '../sync/engine.svelte';

	// compact: the phone's top bar, with room only for a short word.
	let {
		state,
		pending = 0,
		compact = false
	}: { state: SyncState; pending?: number; compact?: boolean } = $props();

	const label = $derived(
		state === 'synced'
			? 'Synced'
			: state === 'syncing'
				? 'Syncing'
				: state === 'paused'
					? pending
						? `Update needed · ${pending} not sent`
						: 'Update needed'
					: pending
						? `Offline · ${pending} pending`
						: 'Offline'
	);
	// The phone's top bar has room for the count or the state, not both.
	const short = $derived(
		state === 'paused'
			? pending
				? `${pending} not sent`
				: 'Update needed'
			: pending
				? `${pending} pending`
				: 'Offline'
	);
</script>

<!-- The sidebar says what is happening in words as well as color, laid out
     as a row of its footer. The phone's top bar has room only for the dot
     when all is synced, and a short word otherwise. Changes are announced
     by the app layout. -->
<span class="sync {state}" class:compact title={label}>
	<span class="mark" aria-hidden="true"><span class="dot"></span></span>
	{#if compact && state === 'synced'}
		<span class="visually-hidden">All changes synced</span>
	{:else if compact}
		<span class="text" aria-hidden="true">{short}</span>
		<span class="visually-hidden">{label}</span>
	{:else}
		<span class="text">{label}</span>
	{/if}
</span>

<style>
	/* In the sidebar, a row like the footer's links: the dot in their icon
	   column, the words where theirs start. */
	.sync {
		position: relative;
		display: flex;
		align-items: center;
		gap: var(--space-3);
		height: calc(32px * var(--control-scale));
		font-size: var(--text-sm);
		color: var(--text-secondary);
		white-space: nowrap;
	}
	/* As wide as the footer links' icons (16px), so the dot centers on them. */
	.mark {
		display: grid;
		flex: none;
		place-items: center;
		width: 16px;
	}
	.compact {
		display: inline-flex;
		flex: none;
		gap: var(--space-2);
		height: auto;
		font-size: var(--text-xs);
	}
	.compact .mark {
		width: auto;
	}
	.compact.syncing .text {
		display: none;
	}
	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--status-done);
	}
	.syncing .dot {
		background: var(--accent);
		animation: pulse 1.2s var(--ease) infinite;
	}
	.offline .dot {
		background: var(--text-muted);
	}
	/* The server wants a newer app: nothing is sent until a reload. */
	.paused .dot {
		background: var(--danger);
	}
	@keyframes pulse {
		50% {
			opacity: 0.35;
			transform: scale(0.8);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.syncing .dot {
			animation: none;
		}
	}
</style>
