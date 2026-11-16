<script lang="ts">
	import LayoutTemplate from '@lucide/svelte/icons/layout-template';
	import { untrack } from 'svelte';
	import FileText from '@lucide/svelte/icons/file-text';
	import FolderKanban from '@lucide/svelte/icons/folder-kanban';
	import ListTodo from '@lucide/svelte/icons/list-todo';
	import { api, call, messageOf, type Schemas } from '#lib/api/client.js';
	import Button from '#lib/components/Button.svelte';
	import EmptyState from '#lib/components/EmptyState.svelte';
	import { session } from '#lib/session.svelte.js';
	import { restore, restorePage } from '#lib/sync/data.js';
	import { engine } from '#lib/sync/engine.svelte.js';
	import { relative } from '#lib/time.js';
	import { toasts } from '#lib/toasts.svelte.js';

	const ws = $derived(session.workspace?.id ?? '');
	let items = $state<Schemas['SyncRow'][] | null>(null);
	let error = $state('');

	async function load() {
		try {
			// Send this device's own deletes first, or they would be missing.
			await engine.flush();
			items = await call(
				api.GET('/workspaces/{workspaceId}/trash', { params: { path: { workspaceId: ws } } })
			);
		} catch (err) {
			error = messageOf(err);
		}
	}

	$effect(() => {
		// Only the workspace starts a load; what load reads must not.
		if (ws) untrack(() => void load());
	});

	// A page deleted with the page above it comes back with that one, so
	// only the page that was deleted is listed.
	const shown = $derived(
		(items ?? []).filter(
			(r) =>
				r.entity !== 'page' ||
				!(items ?? []).some((p) => p.entity === 'page' && p.id === r.fields.parent_id)
		)
	);
	const nameOf = (row: Schemas['SyncRow']) =>
		String(row.fields.name ?? row.fields.title ?? '').trim() || 'Untitled';

	async function bringBack(row: Schemas['SyncRow']) {
		if (row.entity === 'page') {
			// The device holds the deleted pages, so the subpages and the
			// pages above come back with this one.
			const replica = engine.replica(ws);
			for (const r of items ?? [])
				if (r.entity === 'page') replica?.remote('page', r.id, r.fields, r.clocks);
			await restorePage(ws, row.id);
			const back = new Set(
				(engine.replica(ws)?.pages(row.fields.project_id as string | null) ?? []).map((p) => p.id)
			);
			items = items?.filter((r) => !back.has(r.id)) ?? null;
		} else {
			await restore(ws, row);
			items = items?.filter((r) => r.id !== row.id) ?? null;
		}
		toasts.show(`Restored ${nameOf(row)}`);
	}
</script>

<svelte:head><title>Trash · Mila</title></svelte:head>

<header class="page"><h1>Trash</h1></header>
{#if error}
	<EmptyState title="The trash needs a connection" hint={error} />
{:else if items === null}
	<p class="loading">Loading…</p>
{:else if items.length === 0}
	<EmptyState
		title="The trash is empty"
		hint="Deleted projects, tasks, wiki pages and templates stay here for 30 days."
	/>
{:else}
	<p class="hint">
		Deleted projects, tasks, wiki pages and templates stay here for 30 days, then are gone for good.
	</p>
	<ul aria-label="Deleted items">
		{#each shown as row (row.id)}
			<li>
				{#if row.entity === 'project'}<FolderKanban
						size={16}
					/>{:else if row.entity === 'page'}<FileText
						size={16}
					/>{:else if row.entity === 'template'}<LayoutTemplate size={16} />{:else}<ListTodo
						size={16}
					/>{/if}
				<span class="name">{nameOf(row)}</span>
				<span class="when">deleted {relative(String(row.fields.deleted_at))}</span>
				<Button title="Bring this back from the trash" size="sm" onclick={() => bringBack(row)}
					>Restore</Button
				>
			</li>
		{/each}
	</ul>
{/if}

<style>
	.hint,
	.loading {
		padding: var(--space-3) var(--space-5);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	ul {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	li {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		min-height: var(--row-height);
		padding: var(--pad-y) var(--pad-x);
		border-bottom: 1px solid var(--border);
		color: var(--text-secondary);
	}
	.name {
		flex: 1;
		min-width: 0;
		color: var(--text-primary);
	}
	.when {
		font-size: var(--text-xs);
	}
</style>
