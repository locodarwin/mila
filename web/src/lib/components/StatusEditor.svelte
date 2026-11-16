<script lang="ts">
	import Settings2 from '@lucide/svelte/icons/settings-2';
	import X from '@lucide/svelte/icons/x';
	import { createStatus, deleteStatus, updateStatus } from '../sync/data';
	import { engine } from '../sync/engine.svelte';
	import type { StatusCategory } from '../sync/entities';
	import { toasts } from '../toasts.svelte';
	import { anchored } from '../popover';
	import StatusIcon from './StatusIcon.svelte';

	let { ws, projectId }: { ws: string; projectId: string } = $props();
	const statuses = $derived(engine.replica(ws)?.statuses(projectId) ?? []);
	let name = $state('');
	let category = $state<StatusCategory>('todo');

	const categories: [StatusCategory, string][] = [
		['backlog', 'Backlog'],
		['todo', 'To do'],
		['in-progress', 'In progress'],
		['done', 'Done'],
		['canceled', 'Canceled']
	];

	function add(e: SubmitEvent) {
		e.preventDefault();
		if (name.trim()) void createStatus(ws, projectId, name, category);
		name = '';
	}

	async function remove(id: string) {
		try {
			await deleteStatus(ws, projectId, id);
		} catch (err) {
			toasts.error(err instanceof Error ? err.message : 'Could not delete the status.');
		}
	}
</script>

<button class="icon" type="button" popovertarget="status-editor" aria-label="Edit statuses">
	<Settings2 size={16} />
</button>

<div id="status-editor" popover class="menu editor" use:anchored>
	<p class="heading">Statuses</p>
	<ul aria-label="Statuses">
		{#each statuses as s (s.id)}
			<li>
				<StatusIcon category={s.category} />
				<input
					aria-label="Status name"
					value={s.name}
					onchange={(e) => {
						const n = e.currentTarget.value.trim();
						if (n && n !== s.name) void updateStatus(ws, s.id, { name: n });
					}}
				/>
				<select
					aria-label={`Kind of ${s.name}`}
					value={s.category}
					onchange={(e) =>
						void updateStatus(ws, s.id, { category: e.currentTarget.value as StatusCategory })}
				>
					{#each categories as [value, label] (value)}<option {value}>{label}</option>{/each}
				</select>
				<button
					type="button"
					aria-label={`Delete ${s.name}`}
					onclick={() => remove(s.id)}
					disabled={statuses.length < 2}
				>
					<X size={14} />
				</button>
			</li>
		{/each}
	</ul>
	<form onsubmit={add}>
		<input aria-label="New status" placeholder="New status" bind:value={name} />
		<select aria-label="Kind of new status" bind:value={category}>
			{#each categories as [value, label] (value)}<option {value}>{label}</option>{/each}
		</select>
	</form>
	<p class="hint">Deleting a status moves its tasks to another of the same kind.</p>
</div>

<style>
	.icon {
		display: grid;
		place-items: center;
		width: 30px;
		height: 30px;
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.icon:hover {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	.editor {
		width: min(420px, 92vw);
	}
	ul {
		display: grid;
		gap: var(--space-1);
		margin: 0;
		padding: 0;
		list-style: none;
	}
	li,
	form {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
	li input,
	form input {
		flex: 1;
		min-width: 0;
		margin: 0;
	}
	select {
		padding: var(--space-1);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-sm);
		background: var(--surface-0);
		font-size: var(--text-sm);
	}
	li button {
		display: grid;
		place-items: center;
		width: 24px;
		height: 24px;
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-muted);
		cursor: pointer;
	}
	li button:hover:not(:disabled) {
		background: var(--surface-2);
		color: var(--danger);
	}
	form {
		margin-top: var(--space-2);
	}
	.hint {
		margin-top: var(--space-2);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
</style>
