<script lang="ts">
	import { menu, taskActions } from '../../contextMenu.svelte';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import Checkbox from '../Checkbox.svelte';
	import { createTask, setTaskCompleted } from '../../sync/data';
	import type { Task } from '../../sync/entities';
	import { openTask } from '../../taskLink';

	let { ws, parent, subtasks }: { ws: string; parent: Task; subtasks: Task[] } = $props();
	let title = $state('');

	function add(e: SubmitEvent) {
		e.preventDefault();
		if (title.trim()) void createTask(ws, parent.projectId, title, { parentId: parent.id });
		title = '';
	}
</script>

<ul aria-label="Subtasks">
	{#each subtasks as t (t.id)}
		<li use:menu={taskActions(ws, t.id)}>
			<Checkbox
				label={t.title}
				hideLabel
				checked={t.completedAt !== null}
				onchange={(done) => void setTaskCompleted(ws, t, done)}
			/>
			<button type="button" class:done={t.completedAt !== null} onclick={() => openTask(t.id)}>
				{t.title}<ChevronRight size={14} />
			</button>
		</li>
	{/each}
</ul>
<form onsubmit={add}>
	<input aria-label="New subtask" placeholder="Add a subtask" bind:value={title} />
</form>

<style>
	ul {
		display: grid;
		gap: 2px;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	li {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
	button {
		display: flex;
		flex: 1;
		align-items: center;
		justify-content: space-between;
		min-width: 0;
		padding: var(--space-1) var(--space-2);
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		text-align: left;
		cursor: pointer;
	}
	button:hover {
		background: var(--surface-2);
	}
	button.done {
		color: var(--text-muted);
		text-decoration: line-through;
	}
	input {
		width: 100%;
		margin-top: var(--space-1);
		padding: var(--space-1) var(--space-2);
		border: var(--border-width) solid transparent;
		border-radius: var(--radius-sm);
		background: none;
	}
	input:hover {
		border-color: var(--border);
	}
	/* A border and a ring inside it, two pixels of accent in all. */
	input:focus-visible {
		border-color: var(--accent);
		box-shadow: inset 0 0 0 1px var(--accent);
	}
</style>
