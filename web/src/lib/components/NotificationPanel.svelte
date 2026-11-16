<script lang="ts">
	import { resolve } from '$app/paths';
	import X from '@lucide/svelte/icons/x';
	import { tick } from 'svelte';
	import { announce } from '../announce.svelte';
	import { focusOnShow } from '../focus';
	import type { Notification } from '../notifications.svelte';
	import { relative } from '../time';
	import Avatar from './task/Avatar.svelte';

	interface Props {
		id: string;
		items: Notification[];
		unread: number;
		hasMore?: boolean;
		/** Shown in the page instead of as a popover (the style guide). */
		inline?: boolean;
		/** One was picked; its link then opens the task. */
		onopen?: (n: Notification) => void;
		onreadall?: () => void;
		onclear?: (n: Notification) => void;
		onclearall?: () => void;
		onmore?: () => void;
		/** The popover opened. */
		onshow?: () => void;
	}

	let {
		id,
		items,
		unread,
		hasMore = false,
		inline = false,
		onopen,
		onreadall,
		onclear,
		onclearall,
		onmore,
		onshow
	}: Props = $props();
	let panel = $state<HTMLElement>();
	let heading = $state<HTMLElement>();
	let clearAllButton = $state<HTMLElement>();
	/** Clear all asks once, in place: there is no taking it back. */
	let confirming = $state(false);

	// The server's links are app paths ("/?task=…&ws=…").
	const href = (n: Notification) => resolve('/(app)') + n.url.replace(/^\//, '');

	function open(n: Notification) {
		onopen?.(n);
		if (!inline) panel?.hidePopover();
	}

	// Focus stays in the list, on the next one, as the one it was on goes.
	async function clearOne(n: Notification, i: number) {
		onclear?.(n);
		announce('Notification cleared');
		await tick();
		const left = panel?.querySelectorAll<HTMLElement>('.clear');
		(left?.[Math.min(i, left.length - 1)] ?? heading)?.focus();
	}

	async function clearAll() {
		confirming = false;
		onclearall?.();
		announce('All notifications cleared');
		await tick();
		heading?.focus();
	}

	async function keep() {
		confirming = false;
		await tick();
		clearAllButton?.focus();
	}
</script>

<div
	{id}
	class="panel"
	class:inline
	popover={inline ? undefined : 'auto'}
	role="dialog"
	aria-labelledby="{id}-title"
	bind:this={panel}
	ontoggle={(e) => {
		if ((e as ToggleEvent).newState === 'open') onshow?.();
		else confirming = false;
	}}
>
	<header>
		<h2 id="{id}-title" tabindex="-1" bind:this={heading}>Notifications</h2>
		{#if confirming}
			<div class="actions" role="group" aria-label="Clear all notifications?">
				<span class="ask">Clear them all?</span>
				<button
					title="Clear every notification, for good"
					type="button"
					class="link"
					onclick={clearAll}>Clear</button
				>
				<button
					title="Keep your notifications"
					type="button"
					class="link"
					use:focusOnShow
					onclick={keep}>Keep</button
				>
			</div>
		{:else if items.length}
			<div class="actions">
				{#if unread}
					<button
						title="Mark every notification as read"
						type="button"
						class="link"
						onclick={() => onreadall?.()}>Mark all read</button
					>
				{/if}
				<button
					title="Clear every notification"
					type="button"
					class="link"
					bind:this={clearAllButton}
					onclick={() => (confirming = true)}>Clear all</button
				>
			</div>
		{/if}
	</header>
	{#if items.length === 0}
		<p class="empty">
			Nothing yet. Reminders, due tasks, mentions, tasks assigned to you and comments on your tasks
			show up here.
		</p>
	{:else}
		<ul>
			{#each items as n, i (n.id)}
				<li>
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- an app path the server made, resolved in href() -->
					<a href={href(n)} class:unread={!n.readAt} onclick={() => open(n)}>
						<Avatar name={n.actorName ?? 'Mila'} src={n.actorAvatar} size={28} />
						<span class="text">
							<span class="title"
								>{#if !n.readAt}<span class="visually-hidden">Unread: </span>{/if}{n.title}</span
							>
							{#if n.body}<span class="body">{n.body}</span>{/if}
							<span class="time">{relative(n.createdAt)}</span>
						</span>
						{#if !n.readAt}<span class="dot" aria-hidden="true"></span>{/if}
					</a>
					<button
						class="clear"
						type="button"
						title="Clear this notification"
						aria-label="Clear: {n.title}"
						onclick={() => clearOne(n, i)}
					>
						<X size={14} />
					</button>
				</li>
			{/each}
		</ul>
		{#if hasMore}
			<button
				title="Show older notifications"
				type="button"
				class="link more"
				onclick={() => onmore?.()}>Show older</button
			>
		{/if}
	{/if}
</div>

<style>
	.panel {
		position: fixed;
		inset: auto;
		top: var(--space-2);
		left: calc(var(--sidebar-width, 248px) + var(--space-2));
		width: min(380px, calc(100vw - 2 * var(--space-4)));
		max-height: min(560px, calc(100dvh - 2 * var(--space-4)));
		margin: 0;
		padding: 0;
		overflow: auto;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface-0);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	.panel.inline {
		position: static;
		box-shadow: var(--shadow-md);
	}
	@media (max-width: 720px) {
		.panel:not(.inline) {
			top: calc(56px + env(safe-area-inset-top));
			right: var(--space-2);
			left: var(--space-2);
			width: auto;
			max-height: calc(
				100dvh - 56px - var(--tabbar-height) - var(--space-5) - env(safe-area-inset-top) -
					env(safe-area-inset-bottom)
			);
		}
	}
	header {
		position: sticky;
		top: 0;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-3);
		padding: var(--space-3) var(--space-4);
		border-bottom: var(--border-width) solid var(--border);
		background: var(--surface-0);
	}
	h2 {
		font-size: var(--text-md);
	}
	.link {
		padding: var(--space-1);
		border: none;
		background: none;
		color: var(--accent-text);
		font-size: var(--text-sm);
		font-weight: 550;
		cursor: pointer;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: flex-end;
		gap: var(--space-3);
	}
	.ask {
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	h2:focus {
		outline: none;
	}
	.link:disabled {
		color: var(--text-secondary);
		cursor: default;
	}
	.link:not(:disabled):hover {
		text-decoration: underline;
	}
	.empty {
		padding: var(--space-5) var(--space-4);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	ul {
		margin: 0;
		padding: var(--space-1);
		list-style: none;
	}
	li {
		position: relative;
	}
	.clear {
		position: absolute;
		top: var(--space-1);
		right: var(--space-1);
		display: grid;
		place-items: center;
		width: max(var(--tap-min), 28px);
		height: max(var(--tap-min), 28px);
		padding: 0;
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-muted);
		cursor: pointer;
	}
	.clear:hover {
		background: var(--surface-3);
		color: var(--text-primary);
	}
	/* With a mouse it shows on the row; a finger has no hover to bring it out. */
	@media (hover: hover) {
		.clear {
			opacity: 0;
		}
		li:hover .clear,
		.clear:focus-visible {
			opacity: 1;
		}
	}
	a {
		display: flex;
		align-items: flex-start;
		gap: var(--space-3);
		padding: var(--space-2) calc(var(--space-2) + max(var(--tap-min), 28px)) var(--space-2)
			var(--space-3);
		border-radius: var(--radius-md);
		color: var(--text-secondary);
	}
	a:hover {
		background: var(--surface-2);
	}
	.text {
		display: grid;
		flex: 1;
		gap: 2px;
		min-width: 0;
	}
	.title {
		color: var(--text-primary);
		font-size: var(--text-sm);
		overflow-wrap: anywhere;
	}
	.unread .title {
		font-weight: 600;
	}
	.body {
		display: -webkit-box;
		overflow: hidden;
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		font-size: var(--text-sm);
		overflow-wrap: anywhere;
	}
	.time {
		font-size: var(--text-xs);
	}
	.dot {
		flex: none;
		width: 8px;
		height: 8px;
		margin-top: var(--space-2);
		border-radius: 50%;
		background: var(--accent);
	}
	.more {
		display: block;
		margin: 0 auto var(--space-2);
	}
</style>
