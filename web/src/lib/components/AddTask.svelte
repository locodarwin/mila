<script lang="ts">
	// The "Add a task" box at the top of a list: a task in the project, or in
	// the inbox (TaskList, and Inbox with a filter or display set).
	import Plus from '@lucide/svelte/icons/plus';
	import { createTask } from '../sync/data';
	import { toasts } from '../toasts.svelte';

	interface Props {
		ws: string;
		projectId: string | null;
	}

	let { ws, projectId }: Props = $props();
	let title = $state('');

	async function add(e: SubmitEvent) {
		e.preventDefault();
		const t = title.trim();
		if (!t) return;
		title = '';
		try {
			await createTask(ws, projectId, t);
		} catch (err) {
			toasts.error(err instanceof Error ? err.message : 'Could not add the task.');
		}
	}
</script>

<form class="add" onsubmit={add}>
	<Plus size={16} />
	<input aria-label="New task" placeholder="Add a task" bind:value={title} />
</form>

<style>
	.add {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		min-height: var(--row-height);
		padding: 0 var(--pad-x);
		border-bottom: 1px solid var(--row-divider);
		color: var(--text-muted);
	}
	.add input {
		flex: 1;
		min-width: 0;
		height: var(--row-height);
		border: none;
		background: none;
		color: var(--text-primary);
	}
	.add input::placeholder {
		color: var(--input-placeholder);
	}
	.add input:focus-visible {
		outline: none;
	}
	.add:focus-within {
		box-shadow: inset 2px 0 0 var(--accent);
	}
</style>
