<script lang="ts">
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import { Growing, upTo } from '../growing.svelte';
	import { collapse } from '../motion';
	import { preferences } from '../preferences.svelte';
	import { engine } from '../sync/engine.svelte';
	import type { Status, Task } from '../sync/entities';
	import AddTask from './AddTask.svelte';
	import EmptyState from './EmptyState.svelte';
	import { taskKeys } from '../taskKeys';
	import StatusIcon from './StatusIcon.svelte';
	import TaskItem from './TaskItem.svelte';

	interface Props {
		ws: string;
		projectId: string | null;
		emptyTitle: string;
		emptyHint: string;
	}

	let { ws, projectId, emptyTitle, emptyHint }: Props = $props();

	const replica = $derived(engine.replica(ws));
	const tasks = $derived(replica?.tasks(projectId) ?? []);
	const open = $derived(tasks.filter((t) => t.completedAt === null));
	// In a project, open tasks are grouped under their statuses, in order;
	// a task whose status is gone joins the first group.
	const groups = $derived.by((): { status: Status | undefined; tasks: Task[] }[] => {
		if (!projectId || !replica) return [{ status: undefined, tasks: open }];
		const live = replica
			.statuses(projectId)
			.filter((s) => s.category !== 'done' && s.category !== 'canceled');
		const ids = new Set(live.map((s) => s.id));
		return live
			.map((status, i) => ({
				status,
				tasks: open.filter(
					(t) => t.statusId === status.id || (i === 0 && !ids.has(t.statusId ?? ''))
				)
			}))
			.filter((g) => g.tasks.length > 0);
	});
	const completed = $derived(tasks.filter((t) => t.completedAt !== null));
	// A long list is drawn a screenful at a time, as it is scrolled; the
	// completed tasks, when shown, count after the open ones.
	const growing = new Growing(() => open.length + (showCompleted ? completed.length : 0));
	const drawn = $derived(upTo(groups, growing.limit));
	const drawnCompleted = $derived(completed.slice(0, Math.max(0, growing.limit - open.length)));
	const statuses = $derived(
		projectId ? new Map((replica?.statuses(projectId) ?? []).map((s) => [s.id, s])) : new Map()
	);
	// Open or closed to begin with, as the user prefers; then as they toggle it.
	let showCompleted = $state(preferences.current.showCompleted);
</script>

<AddTask {ws} {projectId} />

{#if open.length === 0 && completed.length === 0}
	<EmptyState title={emptyTitle} hint={emptyHint} />
{:else}
	<div
		class="list"
		aria-label="Tasks"
		role="list"
		use:taskKeys={{
			ws,
			groups: () => groups.map((g) => ({ id: g.status?.id ?? 'all', tasks: g.tasks }))
		}}
	>
		{#each drawn as { group, shown } (group.status?.id ?? 'all')}
			{#if group.status && groups.length > 1}
				<!-- A heading in the list, so heading keys reach each status. -->
				<div class="group" role="listitem">
					<StatusIcon category={group.status.category} />
					<h2>{group.status.name}</h2>
					<span class="tabular"
						>{group.tasks.length}<span class="visually-hidden">
							{group.tasks.length === 1 ? 'task' : 'tasks'}</span
						></span
					>
				</div>
			{/if}
			{#each shown as task (task.id)}
				<!-- A row is focusable so the list can be driven from the keyboard;
				     taskKeys keeps one row in the tab order (a roving tabindex). -->
				<div role="listitem" data-task-id={task.id} class="focusable" transition:collapse>
					<TaskItem {ws} {task} status={task.statusId ? statuses.get(task.statusId) : undefined} />
				</div>
			{/each}
		{/each}
	</div>
	<div use:growing.more aria-hidden="true"></div>
	<!-- After every open task is drawn, so it stays below them. -->
	{#if completed.length && growing.limit >= open.length}
		<button
			class="completed-toggle"
			type="button"
			aria-expanded={showCompleted}
			onclick={() => (showCompleted = !showCompleted)}
		>
			<ChevronRight size={14} />Completed · {completed.length}
		</button>
		{#if showCompleted}
			<!-- One tab stop too, driven by the same keys (X reopens). -->
			<div
				class="list"
				aria-label="Completed tasks"
				role="list"
				transition:collapse
				use:taskKeys={{ ws, groups: () => [{ id: 'completed', tasks: completed }] }}
			>
				{#each drawnCompleted as task (task.id)}
					<div role="listitem" data-task-id={task.id} class="focusable" transition:collapse>
						<TaskItem
							{ws}
							{task}
							status={task.statusId ? statuses.get(task.statusId) : undefined}
						/>
					</div>
				{/each}
			</div>
			<div use:growing.more aria-hidden="true"></div>
		{/if}
	{/if}
{/if}

<style>
	.focusable:focus-visible {
		position: relative;
		z-index: 1;
		outline: none;
		box-shadow: inset 3px 0 0 var(--accent);
	}
	.focusable:focus-visible :global(.row) {
		background: var(--row-selected-bg);
	}
	.group {
		position: relative;
		display: flex;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-3) var(--pad-x) var(--space-1);
		font-size: var(--text-xs);
		font-weight: 650;
		color: var(--group-header-text);
	}
	.group h2 {
		font-family: var(--font-ui);
		font-size: inherit;
		font-weight: inherit;
	}
	.group span {
		font-weight: 500;
	}
	.completed-toggle {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		margin: var(--space-3) var(--space-3) 0;
		padding: var(--space-1) var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.completed-toggle[aria-expanded='true'] :global(svg) {
		transform: rotate(90deg);
	}
	.completed-toggle :global(svg) {
		transition: transform var(--dur-micro) var(--ease);
	}
	.completed-toggle:hover {
		background: var(--surface-2);
	}
</style>
