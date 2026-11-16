<script lang="ts">
	import type { Schemas } from '../../api/client';
	import { relative } from '../../time';
	import Button from '../Button.svelte';

	type Invite = Schemas['AdminInvite'];

	interface Props {
		invites: Invite[];
		/** Cancels an invitation; pressed twice. */
		oncancel?: (invite: Invite) => void;
	}

	let { invites, oncancel }: Props = $props();
	let confirming = $state<string | null>(null);

	function cancel(inv: Invite) {
		if (confirming !== inv.id) {
			confirming = inv.id;
			return;
		}
		confirming = null;
		oncancel?.(inv);
	}
</script>

{#if invites.length}
	<ul class="list" aria-label="Open invitations">
		{#each invites as inv (inv.id)}
			<li>
				<span class="grow">
					{inv.email}
					<span class="sub">
						{inv.role} in {inv.workspaceName} · invited by {inv.invitedByName}
						{relative(inv.createdAt)} · expires {relative(inv.expiresAt)}
					</span>
				</span>
				<Button
					size="sm"
					variant="danger"
					aria-label={`${confirming === inv.id ? 'Confirm cancel' : 'Cancel'}: the invitation to ${inv.email}`}
					onclick={() => cancel(inv)}
				>
					{confirming === inv.id ? 'Confirm cancel' : 'Cancel'}
				</Button>
			</li>
		{/each}
	</ul>
{:else}
	<p class="empty">No open invitations.</p>
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
	.empty {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
</style>
