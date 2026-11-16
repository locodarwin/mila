<script lang="ts">
	// Someone's API tokens, each with what it can do, where, and when it was
	// last used.
	import KeyRound from '@lucide/svelte/icons/key-round';
	import type { Schemas } from '../../api/client';
	import { formatDate } from '../../dates';
	import { relative } from '../../time';
	import Button from '../Button.svelte';

	type Token = Schemas['ApiToken'];

	interface Props {
		tokens: Token[];
		/** A workspace's name by its ID. */
		workspaceName: (id: string) => string;
		onrevoke: (token: Token) => void;
	}

	let { tokens, workspaceName, onrevoke }: Props = $props();
	let confirming = $state<string | null>(null);

	function revoke(t: Token) {
		if (confirming === t.id) {
			confirming = null;
			onrevoke(t);
		} else confirming = t.id;
	}

	const expired = (t: Token) => Boolean(t.expiresAt) && new Date(t.expiresAt!) <= new Date();

	function ending(t: Token): string {
		if (!t.expiresAt) return 'never expires';
		return expired(t) ? 'expired' : `expires ${formatDate(t.expiresAt.slice(0, 10))}`;
	}
</script>

{#if tokens.length}
	<ul class="rows" aria-label="API tokens">
		{#each tokens as t (t.id)}
			<li>
				<KeyRound size={16} />
				<span class="grow">
					{t.name}
					{#if expired(t)}<span class="expired">Expired</span>{/if}
					<span class="sub">
						{t.app ? 'An app, for its tools only · ' : ''}{t.canWrite
							? 'Read and write'
							: 'Read only'} · {t.workspaceId
							? workspaceName(t.workspaceId)
							: 'All your workspaces'} · {t.lastUsedAt
							? `last used ${relative(t.lastUsedAt)}`
							: 'not used yet'} · {ending(t)}
					</span>
				</span>
				<Button
					size="sm"
					variant="danger"
					title="Stop this token working, at once"
					aria-label={`${confirming === t.id ? 'Confirm revoke' : 'Revoke'} ${t.name}`}
					onclick={() => revoke(t)}
				>
					{confirming === t.id ? 'Confirm revoke' : 'Revoke'}
				</Button>
			</li>
		{/each}
	</ul>
{:else}
	<p class="hint">You have no API tokens.</p>
{/if}

<style>
	/* As Settings' other rows, wherever this is shown. */
	.rows {
		display: grid;
		gap: var(--space-2);
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.rows li {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}
	.grow {
		flex: 1;
		min-width: 0;
	}
	.sub {
		display: block;
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.expired {
		margin-left: var(--space-1);
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--danger);
	}
</style>
