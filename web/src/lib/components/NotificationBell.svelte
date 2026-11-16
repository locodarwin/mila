<script lang="ts">
	import Bell from '@lucide/svelte/icons/bell';
	import { layout } from '../layout.svelte';

	interface Props {
		unread: number;
		/** The id of the notification panel it opens. */
		target: string;
		/** A sidebar row with its name, or just the icon (the phone's top bar). */
		variant?: 'row' | 'icon';
	}

	let { unread, target, variant = 'row' }: Props = $props();
	const count = $derived(unread > 99 ? '99+' : String(unread));
</script>

<button
	class="bell {variant === 'icon' ? 'compact' : 'row'}"
	type="button"
	popovertarget={target}
	aria-label={unread ? `Notifications, ${unread} unread` : 'Notifications'}
	title={variant === 'row' && layout.collapsed ? 'Notifications' : undefined}
>
	<span class="icon"
		><Bell size={variant === 'icon' ? 20 : 16} />{#if unread}<span class="dot" aria-hidden="true"
			></span>{/if}</span
	>
	{#if variant === 'row'}<span class="name">Notifications</span>{/if}
	{#if unread}<span class="count" aria-hidden="true">{count}</span>{/if}
</button>

<style>
	.bell {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
		transition: background-color var(--dur-micro) var(--ease);
		anchor-name: --notification-bell;
	}
	.bell:hover {
		background: var(--surface-3);
		color: var(--text-primary);
	}
	.row {
		width: 100%;
		height: calc(32px * var(--control-scale));
		padding: 0 var(--space-2);
		white-space: nowrap;
	}
	.icon {
		position: relative;
		display: grid;
		place-items: center;
	}
	.compact {
		display: grid;
		place-items: center;
		position: relative;
		width: 40px;
		height: 40px;
		flex: none;
	}
	.count {
		min-width: 20px;
		margin-left: auto;
		padding: 0 var(--space-1);
		border-radius: var(--radius-full);
		background: var(--accent);
		color: var(--accent-contrast);
		font-size: var(--text-xs);
		font-weight: 650;
		line-height: 18px;
		text-align: center;
	}
	.compact .count {
		position: absolute;
		top: 2px;
		right: 0;
		margin: 0;
	}
	/* The dot shows the unread state when the count is hidden (a collapsed
	   sidebar). */
	.dot {
		display: none;
		position: absolute;
		top: -3px;
		right: -3px;
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--accent);
		box-shadow: 0 0 0 2px var(--surface-1);
	}
	:global(.collapsed) .row .dot {
		display: block;
	}
	:global(.collapsed) .row .count,
	:global(.collapsed) .row .name {
		display: none;
	}
</style>
