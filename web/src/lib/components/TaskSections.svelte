<script lang="ts">
	import type { Snippet } from 'svelte';
	import { Growing, upTo } from '../growing.svelte';
	import { collapse } from '../motion';
	import { engine } from '../sync/engine.svelte';
	import type { Task } from '../sync/entities';
	import { taskKeys } from '../taskKeys';
	import EmptyState from './EmptyState.svelte';
	import TaskItem from './TaskItem.svelte';

	interface Section {
		id: string;
		title: string;
		tasks: Task[];
		tone?: 'danger';
		action?: Snippet;
	}

	interface Props {
		ws: string;
		sections: Section[];
		label: string;
		emptyTitle: string;
		emptyHint: string;
		/** The fields rows show (a saved view's); without it, the usual ones. */
		shown?: Set<string>;
		/**
		 * What the move commands say instead of moving, when the list has
		 * no order of its own to change (a sorted view, or one mixing projects).
		 */
		fixed?: string;
	}

	let { ws, sections, label, emptyTitle, emptyHint, shown: fields, fixed }: Props = $props();
	const replica = $derived(engine.replica(ws));
	const visible = $derived(sections.filter((s) => s.tasks.length));
	// A long list is drawn a screenful at a time, as it is scrolled.
	const growing = new Growing(() => visible.reduce((n, s) => n + s.tasks.length, 0));
	const drawn = $derived(upTo(visible, growing.limit));
</script>

{#if visible.length === 0}
	<EmptyState title={emptyTitle} hint={emptyHint} />
{:else}
	<div
		aria-label={label}
		role="list"
		use:taskKeys={{
			ws,
			groups: () => visible.map((s) => ({ id: s.id, tasks: s.tasks })),
			fixed
		}}
	>
		{#each drawn as { group: section, shown } (section.id)}
			<!-- A heading in the list, so heading keys reach each section; its
			     action belongs to it. -->
			<div class="heading" class:danger={section.tone === 'danger'} role="listitem">
				<h2>{section.title}</h2>
				<span class="count tabular"
					>{section.tasks.length}<span class="visually-hidden">
						{section.tasks.length === 1 ? 'task' : 'tasks'}</span
					></span
				>
				{#if section.action}<span class="action">{@render section.action()}</span>{/if}
			</div>
			{#each shown as task (task.id)}
				<!-- A row is focusable so the list can be driven from the keyboard;
				     taskKeys keeps one row in the tab order (a roving tabindex).
				     A task can be in more than one section, so its row is known
				     by both. -->
				<div
					role="listitem"
					data-task-id={task.id}
					data-row-key={`${section.id}/${task.id}`}
					data-group={section.id}
					class="focusable"
					transition:collapse
				>
					<TaskItem {ws} {task} status={replica?.status(task.statusId)} shown={fields} />
				</div>
			{/each}
		{/each}
	</div>
	<div use:growing.more aria-hidden="true"></div>
{/if}

<style>
	.heading {
		position: relative;
		display: flex;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-4) var(--pad-x) var(--space-1);
		font-size: var(--text-sm);
		font-weight: 650;
	}
	.heading h2 {
		font-family: var(--font-ui);
		font-size: inherit;
		font-weight: inherit;
	}
	.heading.danger {
		color: var(--danger);
	}
	.count {
		font-size: var(--text-xs);
		font-weight: 500;
		color: var(--text-secondary);
	}
	.action {
		margin-left: auto;
	}
	.focusable:focus-visible {
		position: relative;
		z-index: 1;
		outline: none;
		box-shadow: inset 3px 0 0 var(--accent);
	}
	.focusable:focus-visible :global(.row) {
		background: var(--row-selected-bg);
	}
</style>
