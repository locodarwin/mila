<script lang="ts">
	import Plus from '@lucide/svelte/icons/plus';
	import X from '@lucide/svelte/icons/x';
	import StatusIcon from '../StatusIcon.svelte';
	import { announce } from '../../announce.svelte';
	import { formatDate } from '../../dates';
	import { beginOf, blockerChoices, conflicts, finishOf } from '../../dependencies';
	import { dependencyProblem, setDependency } from '../../sync/data';
	import type { Task } from '../../sync/entities';
	import type { Replica } from '../../sync/replica.svelte';
	import { openTask } from '../../taskLink';
	import { focusOnShow } from '../../focus';
	import { toasts } from '../../toasts.svelte';
	import { uuid } from '../../uuid';

	// A task's Blocked by and Blocking, the two parts of its Dependencies section
	// (docs/PLAN.md section 3).
	let { ws, replica, task }: { ws: string; replica: Replica; task: Task } = $props();

	const blockers = $derived(replica.blockers(task.id));
	const waiting = $derived(replica.waiting(task.id));
	const late = $derived(blockers.filter((b) => conflicts(b, task)));

	const id = `deps-${uuid()}`;
	let adding = $state(false);
	let query = $state('');
	let active = $state(0);
	const choices = $derived(
		adding
			? blockerChoices(query, replica.allTasks(), new Set([task.id, ...blockers.map((b) => b.id)]))
			: []
	);
	// The chosen one stays in view as the arrows move past the list's edge.
	$effect(() => {
		const at = choices[active];
		if (at) document.getElementById(`${id}-opt-${at.id}`)?.scrollIntoView({ block: 'nearest' });
	});
	let addButton = $state<HTMLButtonElement>();

	function categoryOf(t: Task) {
		return replica.status(t.statusId)?.category ?? (t.completedAt ? 'done' : 'todo');
	}

	function when(t: Task): string {
		if (t.completedAt) return 'Done';
		if (t.dueDate) return `Due ${formatDate(t.dueDate)}`;
		if (t.startDate) return `Starts ${formatDate(t.startDate)}`;
		return 'No dates';
	}

	function conflictNote(b: Task): string {
		const start = beginOf(task)!;
		const end = finishOf(b)!;
		const verb = task.startDate ? 'Starts' : 'Due';
		const rel = start === end ? 'the day' : 'before';
		return `${verb} ${formatDate(start)}, ${rel} ${b.title} is due (${formatDate(end)}).`;
	}

	async function pick(t: Task) {
		const problem = dependencyProblem(replica, task.id, t.id);
		if (problem) {
			toasts.error(problem);
			return;
		}
		await setDependency(ws, task.id, t.id, true);
		announce(`${task.title} now waits for ${t.title}.`);
		close();
	}

	function close() {
		adding = false;
		query = '';
		active = 0;
		queueMicrotask(() => addButton?.focus());
	}

	async function stop(blocker: Task, waiter: Task) {
		await setDependency(ws, waiter.id, blocker.id, false);
		toasts.show(`${waiter.title} no longer waits for ${blocker.title}`, {
			onundo: () => void setDependency(ws, waiter.id, blocker.id, true)
		});
	}

	function keydown(e: KeyboardEvent) {
		if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
			e.preventDefault();
			if (!choices.length) return;
			active = (active + (e.key === 'ArrowDown' ? 1 : choices.length - 1)) % choices.length;
		} else if (e.key === 'Enter') {
			e.preventDefault();
			const t = choices[active];
			if (t) void pick(t);
		} else if (e.key === 'Escape') {
			// The task panel stays open.
			e.preventDefault();
			close();
		}
	}
</script>

{#snippet row(t: Task, remove: () => void, removeLabel: string)}
	<li>
		<StatusIcon category={categoryOf(t)} size={14} />
		<button type="button" class="open" class:done={t.completedAt} onclick={() => openTask(t.id)}
			>{t.title}</button
		>
		<span class="when">{when(t)}</span>
		<button type="button" class="icon" aria-label={removeLabel} title={removeLabel} onclick={remove}
			><X size={14} /></button
		>
	</li>
{/snippet}

<section class="deps" aria-labelledby="{id}-blocked">
	<h4 id="{id}-blocked">Blocked by</h4>
	<div class="what">
		{#if blockers.length}
			<ul>
				{#each blockers as b (b.id)}
					{@render row(b, () => void stop(b, task), `Stop waiting for ${b.title}`)}
				{/each}
			</ul>
		{:else}
			<p class="none">Nothing. It can start any time.</p>
		{/if}
		{#each late as b (b.id)}
			<p class="conflict" role="note">{conflictNote(b)}</p>
		{/each}
		{#if adding}
			<div class="search">
				<input
					role="combobox"
					aria-label="Find a task {task.title} waits for"
					aria-expanded="true"
					aria-controls="{id}-list"
					aria-autocomplete="list"
					aria-activedescendant={choices[active] ? `${id}-opt-${choices[active].id}` : undefined}
					placeholder="Find a task it waits for"
					autocomplete="off"
					bind:value={query}
					oninput={() => (active = 0)}
					onkeydown={keydown}
					onblur={(e) => {
						if (!(e.relatedTarget as Element | null)?.closest?.(`#${id}-list`)) close();
					}}
					use:focusOnShow
				/>
				<ul class="choices" role="listbox" id="{id}-list" aria-label="Tasks">
					{#each choices as t, i (t.id)}
						<li
							role="option"
							id="{id}-opt-{t.id}"
							aria-selected={i === active}
							tabindex="-1"
							onpointerdown={(e) => e.preventDefault()}
							onclick={() => void pick(t)}
							onkeydown={() => {}}
						>
							<StatusIcon category={categoryOf(t)} size={14} />
							<span class="title">{t.title}</span>
							<span class="when">{when(t)}</span>
						</li>
					{:else}
						<li class="empty" role="presentation">No task by that name.</li>
					{/each}
				</ul>
			</div>
		{:else}
			<button type="button" class="add" bind:this={addButton} onclick={() => (adding = true)}
				><Plus size={14} />Add a task it waits for</button
			>
		{/if}
	</div>
</section>

<section class="deps blocking" aria-labelledby="{id}-blocking">
	<h4 id="{id}-blocking">Blocking</h4>
	<div class="what">
		{#if waiting.length}
			<ul>
				{#each waiting as w (w.id)}
					{@render row(w, () => void stop(task, w), `Stop ${w.title} waiting for this task`)}
				{/each}
			</ul>
		{:else}
			<p class="none">Nothing waits for it.</p>
		{/if}
	</div>
</section>

<style>
	/* The panel's field rows: the name, then what it holds. */
	.deps {
		display: grid;
		grid-template-columns: calc(96px * var(--ui-scale)) minmax(0, 1fr);
		gap: var(--space-3);
		align-items: start;
	}
	.blocking {
		padding-top: var(--space-2);
		border-top: var(--border-width) solid var(--border);
	}
	h4 {
		font-family: var(--font-ui);
		font-size: var(--text-sm);
		font-weight: 400;
		line-height: var(--leading);
		color: var(--text-secondary);
		overflow-wrap: anywhere;
	}
	.what {
		display: grid;
		gap: var(--space-1);
		min-width: 0;
	}
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
		min-width: 0;
		padding-left: var(--space-1);
		border-radius: var(--radius-sm);
	}
	li:hover {
		background: var(--surface-2);
	}
	.open {
		flex: 1;
		min-width: 0;
		padding: var(--space-1) 0;
		overflow: hidden;
		border: none;
		background: none;
		text-align: left;
		text-overflow: ellipsis;
		white-space: nowrap;
		cursor: pointer;
	}
	.open.done {
		color: var(--text-muted);
		text-decoration: line-through;
	}
	.when {
		flex: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		white-space: nowrap;
	}
	.icon {
		display: grid;
		flex: none;
		place-items: center;
		min-width: max(var(--tap-min), 24px);
		min-height: max(var(--tap-min), 24px);
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.icon:hover {
		color: var(--text-primary);
	}
	.none {
		margin: 0;
		color: var(--text-muted);
		font-size: var(--text-sm);
	}
	.conflict {
		margin: 0;
		padding: var(--space-2) var(--space-3);
		border: var(--border-width) solid var(--danger);
		border-radius: var(--radius-sm);
		color: var(--danger);
		font-size: var(--text-sm);
	}
	.add {
		display: inline-flex;
		justify-self: start;
		align-items: center;
		gap: var(--space-1);
		min-height: max(var(--tap-min), 28px);
		padding: 0 var(--space-1);
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--accent-text);
		cursor: pointer;
	}
	.search {
		display: grid;
		gap: var(--space-1);
	}
	input {
		width: 100%;
		padding: var(--space-1) var(--space-2);
		border: var(--border-width) solid var(--accent);
		border-radius: var(--radius-sm);
		background: var(--surface-0);
		box-shadow: inset 0 0 0 1px var(--accent);
	}
	.choices {
		max-height: 16rem;
		padding: var(--space-1);
		overflow-y: auto;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-0);
	}
	[role='option'] {
		min-height: max(var(--tap-min), 30px);
		padding: 0 var(--space-2);
		cursor: pointer;
	}
	[role='option'][aria-selected='true'] {
		background: var(--accent-soft);
	}
	.title {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.empty {
		padding: var(--space-1) var(--space-2);
		color: var(--text-muted);
	}
</style>
