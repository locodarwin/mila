<script lang="ts">
	import { resolve } from '$app/paths';
	import { session } from '../session.svelte';
	import type { Schemas } from '../api/client';
	import Announcer from './Announcer.svelte';
	import Avatar from './task/Avatar.svelte';

	type Guest = Schemas['ProjectGuest'];

	interface Props {
		/** Whether the dialog is open; closing it sets this false. */
		open: boolean;
		projectName: string;
		/** The workspace's guests, or null while they load. */
		guests: Guest[] | null;
		/** Gives a guest the project, or takes it away. */
		onchange?: (guest: Guest, shared: boolean) => void;
		/** A guest whose change is on its way. */
		pending?: string | null;
	}

	let { open = $bindable(), projectName, guests, onchange, pending = null }: Props = $props();
	let dialog = $state<HTMLDialogElement>();

	$effect(() => {
		if (open && dialog && !dialog.open) dialog.showModal();
		else if (!open && dialog?.open) dialog.close();
	});
</script>

<dialog
	bind:this={dialog}
	class="share"
	aria-labelledby="share-title"
	onclose={() => (open = false)}
>
	<h2 id="share-title">Share {projectName}</h2>
	<p class="hint">
		Members see every project already. Guests see only the projects given to them here, and their
		devices pick the change up at once.
	</p>
	{#if guests === null}
		<p class="hint">Loading…</p>
	{:else if guests.length}
		<ul aria-label="Guests">
			{#each guests as g (g.userId)}
				<li>
					<label>
						<input
							type="checkbox"
							checked={g.shared}
							disabled={pending === g.userId}
							aria-label={`${g.name} can see ${projectName}`}
							onchange={(e) => onchange?.(g, e.currentTarget.checked)}
						/>
						<Avatar name={g.name} src={g.avatar} size={28} />
						<span class="who">
							{g.name}
							<span class="sub">{g.email}</span>
						</span>
					</label>
				</li>
			{/each}
		</ul>
	{:else}
		<p class="hint">
			This workspace has no guests yet. Invite someone as a guest in
			<!-- eslint-disable svelte/no-navigation-without-resolve -- resolved -->
			<a
				class="link"
				href={resolve('/(app)/settings/w/[ws]/people', { ws: session.workspace?.id ?? '' })}
				onclick={() => (open = false)}>Settings, People</a
			><!-- eslint-enable svelte/no-navigation-without-resolve -->, then give them this project
			here.
		</p>
	{/if}
	<form method="dialog"><button title="Close this">Done</button></form>
	{#if open}<Announcer inDialog />{/if}
</dialog>

<style>
	.share {
		width: min(460px, calc(100vw - 32px));
		max-height: 80vh;
		padding: var(--space-5);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	.share::backdrop {
		/* With the value as written, for browsers whose ::backdrop does not inherit custom properties. */
		background: var(--scrim, oklch(0 0 0 / 0.25));
	}
	h2 {
		font-size: var(--text-xl);
		overflow-wrap: anywhere;
	}
	.hint {
		margin-top: var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	ul {
		margin: var(--space-4) 0 0;
		padding: 0;
		list-style: none;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
	}
	li + li {
		border-top: 1px solid var(--border);
	}
	label {
		position: relative;
		display: flex;
		align-items: center;
		gap: var(--space-3);
		min-height: var(--row-height);
		padding: var(--pad-y) var(--space-4);
		cursor: pointer;
	}
	input {
		flex: none;
		width: max(var(--tap-min), 16px);
		height: max(var(--tap-min), 16px);
		margin: 0;
		accent-color: var(--accent);
	}
	.who {
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.sub {
		display: block;
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.link {
		color: var(--accent-text);
		font-weight: 550;
	}
	form {
		margin-top: var(--space-4);
		text-align: right;
	}
	button {
		min-height: var(--tap-min);
		padding: var(--space-1) var(--space-3);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		cursor: pointer;
	}
</style>
