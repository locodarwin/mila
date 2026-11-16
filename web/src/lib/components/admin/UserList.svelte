<script lang="ts">
	import { tick } from 'svelte';
	import type { Schemas } from '../../api/client';
	import { relative } from '../../time';
	import Button from '../Button.svelte';
	import Avatar from '../task/Avatar.svelte';

	type User = Schemas['AdminUser'];

	interface Props {
		users: User[];
		/** The admin looking, whose own rows say "you". */
		me?: string;
		/** Gives or takes admin rights. */
		onadmin?: (user: User, on: boolean) => void;
		/** Ends every session of the user; pressed twice. */
		onsignout?: (user: User) => void;
		/** Takes down the user's picture; done when the list has it gone. */
		onremovepicture?: (user: User) => void | Promise<void>;
	}

	let { users, me, onadmin, onsignout, onremovepicture }: Props = $props();
	let confirming = $state<string | null>(null);
	const admins = $derived(users.filter((u) => u.isInstanceAdmin).length);

	// The button pressed goes with the picture, so focus moves on to the
	// row's last control still on (Sign out everywhere, else the switch).
	async function removePicture(u: User, row: HTMLElement | null) {
		await onremovepicture?.(u);
		await tick();
		if (!row?.isConnected || row.contains(document.activeElement)) return;
		const left = [...row.querySelectorAll<HTMLElement>('button, input')].filter(
			(el) => !(el as HTMLButtonElement).disabled
		);
		left.at(-1)?.focus();
	}

	function signOut(u: User) {
		if (confirming !== u.id) {
			confirming = u.id;
			return;
		}
		confirming = null;
		onsignout?.(u);
	}
</script>

<ul class="list" aria-label="Accounts">
	{#each users as u (u.id)}
		<li>
			<Avatar name={u.name} src={u.avatar} size={28} />
			<span class="grow">
				{u.name}
				{#if u.id === me}<span class="badge">You</span>{/if}
				{#if u.isInstanceAdmin}<span class="badge">Admin</span>{/if}
				<span class="sub">
					{u.email} · joined {relative(u.createdAt)} ·
					{#if u.lastActiveAt}active {relative(u.lastActiveAt)}{:else}not active lately{/if}
					· {u.devices}
					{u.devices === 1 ? 'device' : 'devices'} signed in
				</span>
			</span>
			<label class="toggle">
				<input
					type="checkbox"
					checked={u.isInstanceAdmin}
					disabled={u.isInstanceAdmin && admins <= 1}
					aria-label={`Admin: ${u.name}`}
					onchange={(e) => onadmin?.(u, e.currentTarget.checked)}
				/>
				Admin
			</label>
			{#if u.avatar}
				<Button
					size="sm"
					variant="ghost"
					aria-label={`Remove picture: ${u.name}`}
					onclick={(e) => void removePicture(u, e.currentTarget.closest('li'))}
				>
					Remove picture
				</Button>
			{/if}
			<Button
				size="sm"
				variant="danger"
				title="Ends their sessions on every device, their tokens, feeds and email addresses, and removes their passkeys; they sign in again by email"
				aria-label={`${confirming === u.id ? 'Confirm sign out' : 'Sign out everywhere'}: ${u.name}`}
				onclick={() => signOut(u)}
			>
				{confirming === u.id ? 'Confirm sign out' : 'Sign out everywhere'}
			</Button>
		</li>
	{/each}
</ul>
{#if admins <= 1}
	<p class="sub">Mila always keeps one admin: make someone else an admin before stepping down.</p>
{/if}

<style>
	.list {
		margin: 0;
		padding: 0;
		list-style: none;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
	}
	li {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-3);
		min-height: var(--row-height);
		padding: var(--pad-y) var(--space-4);
	}
	li + li {
		border-top: 1px solid var(--border);
	}
	.grow {
		flex: 1;
		min-width: 200px;
		overflow-wrap: anywhere;
	}
	.sub {
		display: block;
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.badge {
		margin-left: var(--space-1);
		padding: 1px var(--space-2);
		border-radius: var(--radius-full);
		background: var(--accent-soft);
		font-size: var(--text-xs);
		font-weight: 600;
	}
	.toggle {
		position: relative;
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		min-height: var(--tap-min);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.toggle input {
		width: max(var(--tap-min), 16px);
		height: max(var(--tap-min), 16px);
		margin: 0;
		accent-color: var(--accent);
	}
	.toggle:has(input:disabled) {
		cursor: default;
		color: var(--text-secondary);
	}
</style>
