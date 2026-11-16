<script lang="ts">
	import Download from '@lucide/svelte/icons/download';
	import type { Schemas } from '../../api/client';
	import { relative } from '../../time';
	import Button from '../Button.svelte';
	import { fileSize, FORMAT_NAMES, STATUS_NAMES } from './format';

	interface Props {
		items: Schemas['Export'][];
		/** Removes an export; pressed twice, since it cannot be undone. */
		onremove?: (id: string) => void;
	}

	let { items, onremove }: Props = $props();
	let confirming = $state<string | null>(null);

	function remove(id: string) {
		if (confirming !== id) {
			confirming = id;
			return;
		}
		confirming = null;
		onremove?.(id);
	}
</script>

{#if items.length}
	<ul class="list" aria-label="Exports">
		{#each items as e (e.id)}
			{@const name = `${FORMAT_NAMES[e.format]} export of ${e.workspaceName}`}
			<li>
				<span class="grow">
					{name}
					<span class="sub">
						<span class="status {e.status}">{STATUS_NAMES[e.status]}</span>
						{#if e.status === 'ready'}
							· {fileSize(e.size)} · expires {relative(e.expiresAt)}
						{:else if e.status === 'failed'}
							· {e.error}
						{:else}
							· asked {relative(e.createdAt)}
						{/if}
					</span>
				</span>
				{#if e.status === 'ready'}
					<!-- A plain link: the browser downloads the zip with the session cookie. -->
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- an API download, not an app page -->
					<a class="download" href="/api/v1/exports/{e.id}/download" download={e.fileName}>
						<Download size={16} aria-hidden="true" />Download<span class="visually-hidden">
							{name}</span
						>
					</a>
				{/if}
				{#if e.status === 'ready' || e.status === 'failed'}
					<Button
						size="sm"
						variant="danger"
						aria-label={`${confirming === e.id ? 'Confirm delete' : 'Delete'} ${name}`}
						onclick={() => remove(e.id)}
					>
						{confirming === e.id ? 'Confirm delete' : 'Delete'}
					</Button>
				{/if}
			</li>
		{/each}
	</ul>
{:else}
	<p class="empty">No exports yet.</p>
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
		position: relative;
		flex: 1;
		min-width: 180px;
	}
	.sub {
		display: block;
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.status {
		font-weight: 600;
	}
	.status.failed {
		color: var(--danger);
	}
	.download {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		min-height: max(var(--tap-min), calc(28px * var(--control-scale)));
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
		font-weight: 550;
		text-decoration: none;
	}
	.download:hover {
		background: var(--surface-2);
	}
	.empty {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
</style>
