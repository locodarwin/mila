<script lang="ts">
	// Choosing a saved view to show in a wiki page or a task's notes, or a
	// report view to show a chart of. Only shared views are offered: someone's
	// own would show everyone else an empty box. Loaded with the editor's tools
	// (DocTools).
	import { session } from '../../session.svelte';
	import { engine } from '../../sync/engine.svelte';
	import Announcer from '../Announcer.svelte';

	interface Props {
		ws: string;
		/** Whether the dialog is open; closing it sets this false. */
		open: boolean;
		/** Puts the view chosen in the document. */
		onpick: (id: string, name: string) => void;
		/** Views of tasks, or report views. */
		kind?: 'tasks' | 'report';
	}

	let { ws, open = $bindable(), onpick, kind = 'tasks' }: Props = $props();
	let dialog = $state<HTMLDialogElement>();
	const replica = $derived(engine.replica(ws));
	const views = $derived(
		(replica?.views(session.user?.id ?? '') ?? []).filter(
			(v) => v.ownerId === null && (v.type === 'report') === (kind === 'report')
		)
	);
	const where = (projectId: string | null) =>
		projectId ? (replica?.project(projectId)?.name ?? 'A project') : 'Across projects';

	$effect(() => {
		if (open && dialog && !dialog.open) dialog.showModal();
		else if (!open && dialog?.open) dialog.close();
	});

	function pick(id: string, name: string) {
		open = false;
		onpick(id, name);
	}
</script>

<dialog
	bind:this={dialog}
	class="picker"
	aria-labelledby="view-picker-title"
	onclose={() => (open = false)}
>
	{#if kind === 'report'}
		<h2 id="view-picker-title">Show a report here</h2>
		<p class="hint">
			One of its charts shows here, worked out for each person from the tasks they can see. Only
			shared report views can go in a page.
		</p>
	{:else}
		<h2 id="view-picker-title">Show a view here</h2>
		<p class="hint">
			Its tasks show as they are, to each person as much as they can see. Only shared views can go
			in a page.
		</p>
	{/if}
	{#if views.length}
		<ul aria-label={kind === 'report' ? 'Shared reports' : 'Shared views'}>
			{#each views as v (v.id)}
				<li>
					<button type="button" onclick={() => pick(v.id, v.name)}>
						<span class="name">{v.name || 'Untitled view'}</span>
						<span class="where">{where(v.projectId)}</span>
					</button>
				</li>
			{/each}
		</ul>
	{:else}
		<p class="hint">
			{kind === 'report'
				? 'There are no shared report views yet. On the Reports page, choose Save as view and share it, then put it here.'
				: 'There are no shared views yet. Save a view and share it, then put it here.'}
		</p>
	{/if}
	<div class="actions">
		<form method="dialog"><button>Cancel</button></form>
	</div>
	{#if open}<Announcer inDialog />{/if}
</dialog>

<style>
	.picker {
		width: min(480px, calc(100vw - 32px));
		max-height: 80vh;
		padding: var(--space-5);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	.picker::backdrop {
		/* With the value as written, for browsers whose ::backdrop does not inherit custom properties. */
		background: var(--scrim, oklch(0 0 0 / 0.25));
	}
	h2 {
		font-size: var(--text-xl);
	}
	.hint {
		margin-top: var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	ul {
		display: grid;
		gap: var(--space-1);
		margin: var(--space-4) 0 0;
		padding: 0;
		list-style: none;
	}
	li button {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: var(--space-3);
		width: 100%;
		min-height: max(var(--tap-min), 36px);
		padding: var(--space-1) var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-1);
		color: var(--text-primary);
		font-size: var(--text-sm);
		text-align: left;
		cursor: pointer;
	}
	li button:hover {
		border-color: var(--border-strong);
	}
	.name {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-weight: 600;
	}
	.where {
		flex: none;
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.actions {
		display: flex;
		justify-content: flex-end;
		margin-top: var(--space-4);
	}
</style>
