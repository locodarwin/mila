<script lang="ts">
	// What a table cell's picker offers for one task and column: a choice
	// to pick (status, priority, project), several to turn on and off
	// (assignees, labels), a date, or a custom field's own input. Every
	// choice is saved at once; the table closes the picker.
	import { session } from '../../session.svelte';
	import {
		moveTask,
		setAssignee,
		setFieldValue,
		setTaskCompleted,
		setTaskFields,
		setTaskLabel,
		setTaskStatus
	} from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import type { Task } from '../../sync/entities';
	import type { TableColumn } from '../../table';
	import type { PaletteColor } from '../../theme/palette';
	import { PRIORITY_ORDER } from '../../viewDisplay';
	import Checkbox from '../Checkbox.svelte';
	import LabelChip from '../LabelChip.svelte';
	import PriorityIcon from '../PriorityIcon.svelte';
	import ProjectIcon from '../ProjectIcon.svelte';
	import StatusIcon from '../StatusIcon.svelte';
	import Avatar from '../task/Avatar.svelte';
	import FieldInput from '../task/FieldInput.svelte';

	interface Props {
		ws: string;
		task: Task;
		col: TableColumn;
		/** After a single choice is made: the picker closes. */
		ondone: () => void;
	}

	let { ws, task, col, ondone }: Props = $props();
	const replica = $derived(engine.replica(ws));
	let root = $state<HTMLElement>();

	const statuses = $derived(task.projectId ? (replica?.statuses(task.projectId) ?? []) : []);
	const assigned = $derived(new Set(replica?.assignees(task.id) ?? []));
	const labelled = $derived(new Set((replica?.taskLabels(task.id) ?? []).map((l) => l.id)));
	const value = $derived(col.field ? replica?.fieldValues(task.id)[col.field.id] : undefined);

	async function pick(write: () => Promise<unknown>) {
		ondone();
		await write();
	}

	// Up and down walk the choices. The table puts focus on the chosen one.
	function walk(e: KeyboardEvent) {
		if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
		const opts = [...(root?.querySelectorAll<HTMLElement>('[data-opt]') ?? [])];
		const at = opts.indexOf(document.activeElement as HTMLElement);
		if (at < 0) return;
		e.preventDefault();
		opts[(at + (e.key === 'ArrowDown' ? 1 : opts.length - 1)) % opts.length]?.focus();
	}
	const dateOf = $derived(
		col.id === 'start' ? task.startDate : typeof value === 'string' ? value : null
	);
	function setDate(d: string | null) {
		if (col.id === 'start') return setTaskFields(ws, task.id, { start_date: d });
		if (col.field) return setFieldValue(ws, task.id, col.field.id, d);
		return Promise.resolve();
	}
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="picker" bind:this={root} onkeydown={walk}>
	<p class="heading">{col.label}</p>
	{#if col.id === 'status'}
		<div role="menu" aria-label={col.label}>
			{#if task.projectId}
				{#each statuses as s (s.id)}
					<button
						type="button"
						role="menuitemradio"
						data-opt
						aria-checked={task.statusId === s.id}
						onclick={() => pick(() => setTaskStatus(ws, task, s.id))}
						><StatusIcon category={s.category} />{s.name}</button
					>
				{/each}
			{:else}
				<!-- An inbox task has no statuses: it is to do or done. -->
				{#each [false, true] as done (done)}
					<button
						type="button"
						role="menuitemradio"
						data-opt
						aria-checked={(task.completedAt !== null) === done}
						onclick={() => pick(() => setTaskCompleted(ws, task, done))}
						><StatusIcon category={done ? 'done' : 'todo'} />{done ? 'Done' : 'To do'}</button
					>
				{/each}
			{/if}
		</div>
	{:else if col.id === 'priority'}
		<div role="menu" aria-label={col.label}>
			{#each PRIORITY_ORDER as p (p.value)}
				<button
					type="button"
					role="menuitemradio"
					data-opt
					aria-checked={task.priority === p.value}
					onclick={() => pick(() => setTaskFields(ws, task.id, { priority: p.value }))}
					><PriorityIcon priority={p.value} />{p.label}</button
				>
			{/each}
		</div>
	{:else if col.id === 'project'}
		{#if task.parentId}
			<p class="note">A subtask is in its parent's project. Move the parent instead.</p>
		{:else}
			<div role="menu" aria-label={col.label}>
				<button
					type="button"
					role="menuitemradio"
					data-opt
					aria-checked={task.projectId === null}
					onclick={() => pick(() => moveTask(ws, task, null))}>Inbox</button
				>
				{#each replica?.projects() ?? [] as p (p.id)}
					<button
						type="button"
						role="menuitemradio"
						data-opt
						aria-checked={task.projectId === p.id}
						onclick={() => pick(() => moveTask(ws, task, p.id))}
						><ProjectIcon icon={p.icon} color={p.color} size={14} />{p.name}</button
					>
				{/each}
			</div>
		{/if}
	{:else if col.id === 'assignees'}
		{@const can = new Set(session.reachable(ws, task.projectId).map((m) => m.userId))}
		<div role="group" aria-label={col.label} class="checks">
			{#each session
				.members(ws)
				.filter((m) => can.has(m.userId) || assigned.has(m.userId)) as m (m.userId)}
				<span class="check" data-opt-row>
					<Checkbox
						label={m.name}
						checked={assigned.has(m.userId)}
						onchange={(on) => void setAssignee(ws, task.id, m.userId, on)}
					/>
					<Avatar name={m.name} src={m.avatar} />
				</span>
			{/each}
		</div>
	{:else if col.id === 'labels'}
		<div role="group" aria-label={col.label} class="checks">
			{#each replica?.labels() ?? [] as l (l.id)}
				<span class="check">
					<Checkbox
						label={l.name}
						hideLabel
						checked={labelled.has(l.id)}
						onchange={(on) => void setTaskLabel(ws, task.id, l.id, on)}
					/>
					<LabelChip color={l.color as PaletteColor} name={l.name} />
				</span>
			{:else}
				<p class="note">No labels yet. Make them in Settings, Labels and fields.</p>
			{/each}
		</div>
	{:else if col.id === 'start' || col.field?.type === 'date'}
		<div class="date">
			<input
				type="date"
				aria-label={col.label}
				value={dateOf ?? ''}
				onchange={(e) => void setDate(e.currentTarget.value || null)}
				onkeydown={(e) => e.key === 'Enter' && ondone()}
			/>
			{#if dateOf}
				<button type="button" class="clear" onclick={() => pick(() => setDate(null))}>Clear</button>
			{/if}
		</div>
	{:else if col.field}
		<div class="field">
			<FieldInput {ws} taskId={task.id} field={col.field} {value} />
		</div>
	{/if}
</div>

<style>
	.picker {
		display: grid;
		gap: 2px;
		min-width: 200px;
		max-height: min(60vh, 420px);
		overflow-y: auto;
	}
	.heading {
		padding: var(--space-1) var(--space-2) 2px;
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-muted);
	}
	[role='menu'] {
		display: grid;
		gap: 1px;
	}
	[role='menu'] button {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: max(var(--tap-min), 30px);
		padding: 0 var(--space-2);
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-primary);
		font-size: var(--text-sm);
		text-align: left;
		cursor: pointer;
	}
	[role='menu'] button:hover,
	[role='menu'] button:focus-visible {
		background: var(--surface-2);
	}
	[role='menu'] button[aria-checked='true'] {
		font-weight: 600;
	}
	[role='menu'] button[aria-checked='true']::after {
		content: '';
		width: 6px;
		height: 6px;
		margin-left: auto;
		border-radius: 50%;
		background: var(--accent);
	}
	.checks {
		display: grid;
		gap: var(--space-1);
		padding: var(--space-1) var(--space-2);
	}
	.check {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
	.note {
		max-width: 240px;
		padding: var(--space-1) var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.date,
	.field {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-1) var(--space-2) var(--space-2);
	}
	input[type='date'] {
		min-height: max(var(--tap-min), 30px);
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
	}
	.clear {
		min-height: max(var(--tap-min), 30px);
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		font-size: var(--text-sm);
		cursor: pointer;
	}
</style>
