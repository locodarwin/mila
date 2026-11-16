<script lang="ts">
	import Undo2 from '@lucide/svelte/icons/undo-2';
	import X from '@lucide/svelte/icons/x';
	import type { ToastAction } from '../toasts.svelte';

	let {
		message,
		onundo,
		action,
		ondismiss,
		tone = 'info'
	}: {
		message: string;
		onundo?: () => void;
		/** A button of its own, such as Reload. */
		action?: ToastAction;
		/** Shown for a toast that stays until it is closed. */
		ondismiss?: () => void;
		tone?: 'info' | 'error';
	} = $props();
</script>

<div class="toast" class:error={tone === 'error'}>
	<span>{message}</span>
	{#if onundo}
		<button title="Undo this change" type="button" onclick={onundo}><Undo2 size={14} />Undo</button>
	{/if}
	{#if action}
		<button type="button" onclick={action.run}>{action.label}</button>
	{/if}
	{#if ondismiss}
		<button type="button" class="close" aria-label="Dismiss" onclick={ondismiss}
			><X size={14} /></button
		>
	{/if}
</div>

<style>
	.toast {
		display: inline-flex;
		align-items: center;
		gap: var(--space-4);
		padding: var(--space-2) var(--space-2) var(--space-2) var(--space-4);
		border-radius: var(--radius-lg);
		background: var(--toast-bg);
		color: var(--toast-text);
		box-shadow: var(--shadow-lg);
		font-size: var(--text-sm);
	}
	.error {
		background: var(--danger);
		color: var(--surface-0);
	}
	button {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		padding: var(--space-1) var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: transparent;
		color: inherit;
		font-weight: 650;
		cursor: pointer;
	}
	button:hover {
		background: color-mix(in oklch, var(--toast-text) 15%, transparent);
	}
	.close {
		margin-left: calc(-1 * var(--space-3));
		justify-content: center;
		min-width: max(var(--tap-min), 24px);
		min-height: max(var(--tap-min), 24px);
	}
</style>
