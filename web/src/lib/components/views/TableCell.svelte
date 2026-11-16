<script lang="ts">
	// What a table cell shows for a task: one column's value, read-only.
	// Editing happens in the table's picker; the title cell is the table's own.
	import Check from '@lucide/svelte/icons/check';
	import { formatDate, formatTime } from '../../dates';
	import { formatDuration, remaining } from '../../duration';
	import { repeatLabel } from '../../recurrence';
	import { session } from '../../session.svelte';
	import { localToday } from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import type { FieldValue, Status, Task } from '../../sync/entities';
	import type { TableColumn } from '../../table';
	import type { PaletteColor } from '../../theme/palette';
	import { CATEGORIES } from '../../viewDisplay';
	import LabelChip from '../LabelChip.svelte';
	import PriorityIcon from '../PriorityIcon.svelte';
	import ProjectIcon from '../ProjectIcon.svelte';
	import StatusIcon from '../StatusIcon.svelte';
	import Avatar from '../task/Avatar.svelte';

	interface Props {
		ws: string;
		task: Task;
		col: TableColumn;
		status?: Status;
		/** The task's value for a custom field's column. */
		value?: FieldValue;
	}

	let { ws, task, col, status, value }: Props = $props();
	const replica = $derived(engine.replica(ws));
	const today = localToday();
	const PRIORITY_NAMES = {
		urgent: 'Urgent',
		high: 'High',
		medium: 'Medium',
		low: 'Low',
		none: 'No priority'
	};
	const category = $derived(status?.category ?? (task.completedAt ? 'done' : 'todo'));
	const overdue = (d: string | null) => d !== null && d < today && task.completedAt === null;
	const people = (ids: string[]) => ids.map((id) => ({ id, ...session.personOf(ws, id) }));
	const options = $derived(
		col.field && (col.field.type === 'select' || col.field.type === 'multi_select')
			? (Array.isArray(value) ? value : [value]).flatMap((id) => {
					const o = col.field!.options.find((x) => x.id === id);
					return o ? [o] : [];
				})
			: []
	);
	const host = (url: string) => {
		try {
			return new URL(url).hostname || url;
		} catch {
			return url;
		}
	};
	const counted = (done: number, all: number) => (all ? `${done}/${all}` : '');
</script>

<span class="cell" class:end={col.end}>
	{#if col.id === 'status'}
		<StatusIcon {category} />
		<span class="text">{status?.name ?? CATEGORIES.find((c) => c.value === category)?.label}</span>
	{:else if col.id === 'priority'}
		{#if task.priority !== 'none'}<PriorityIcon priority={task.priority} /><span class="text muted"
				>{PRIORITY_NAMES[task.priority]}</span
			>
		{:else}<span class="empty">No priority</span>{/if}
	{:else if col.id === 'assignees'}
		{@const on = people(replica?.assignees(task.id) ?? [])}
		{#if on.length}
			<span class="avatars" aria-hidden="true">
				{#each on as p (p.id)}<Avatar name={p.name} src={p.avatar} />{/each}
			</span>
			<span class="visually-hidden">{on.map((p) => p.name).join(', ')}</span>
		{:else}<span class="empty">Nobody</span>{/if}
	{:else if col.id === 'due'}
		{#if task.dueDate}
			<span class="text tabular" class:overdue={overdue(task.dueDate)}
				>{formatDate(task.dueDate)}{task.dueTime ? ` ${formatTime(task.dueTime)}` : ''}</span
			>{#if overdue(task.dueDate)}<span class="visually-hidden">, overdue</span>{/if}
		{:else}<span class="empty">No date</span>{/if}
	{:else if col.id === 'start'}
		{#if task.startDate}<span class="text tabular">{formatDate(task.startDate)}</span>
		{:else}<span class="empty">No date</span>{/if}
	{:else if col.id === 'project'}
		{@const p = task.projectId ? replica?.project(task.projectId) : undefined}
		{#if p}<ProjectIcon icon={p.icon} color={p.color} size={14} /><span class="text">{p.name}</span>
		{:else}<span class="text muted">Inbox</span>{/if}
	{:else if col.id === 'labels'}
		{@const labels = replica?.taskLabels(task.id) ?? []}
		{#each labels as l (l.id)}<LabelChip color={l.color as PaletteColor} name={l.name} />
		{:else}<span class="empty">None</span>{/each}
	{:else if col.id === 'subtasks'}
		{@const subs = replica?.subtasks(task.id) ?? []}
		<span class="tabular">{counted(subs.filter((t) => t.completedAt).length, subs.length)}</span>
	{:else if col.id === 'checklist'}
		{@const items = replica?.checklist(task.id) ?? []}
		<span class="tabular">{counted(items.filter((i) => i.done).length, items.length)}</span>
	{:else if col.id === 'comments'}
		{@const n = replica?.comments(task.id).length ?? 0}
		<span class="tabular">{n || ''}</span>
	{:else if col.id === 'reminders'}
		{@const n = session.user ? (replica?.reminders(session.user.id, task.id).length ?? 0) : 0}
		<span class="tabular">{n || ''}</span>
	{:else if col.id === 'estimate'}
		{#if task.estimateMinutes !== null}<span class="tabular"
				>{formatDuration(task.estimateMinutes)}</span
			>
		{:else}<span class="empty">No estimate</span>{/if}
	{:else if col.id === 'logged'}
		{@const n = replica?.logged(task.id) ?? 0}
		{#if n}<span class="tabular">{formatDuration(n)}</span>
		{:else}<span class="empty">None logged</span>{/if}
	{:else if col.id === 'remaining'}
		{@const left = remaining(task.estimateMinutes, replica?.logged(task.id) ?? 0)}
		{#if !left}<span class="empty">No estimate</span>
		{:else if left.over}<span class="tabular overdue">{formatDuration(left.over)} over</span>
		{:else}<span class="tabular">{formatDuration(left.left)}</span>{/if}
	{:else if col.id === 'repeat'}
		<span class="text muted"
			>{repeatLabel(task.recurrence, task.recurrenceMode, task.dueDate) ?? ''}</span
		>
	{:else if col.field}
		{@const type = col.field.type}
		{#if value === undefined || value === null || value === '' || (Array.isArray(value) && !value.length)}
			<span class="empty">{type === 'checkbox' ? 'Not checked' : 'Empty'}</span>
		{:else if type === 'select' || type === 'multi_select'}
			{#each options as o (o.id)}<span
					class="option"
					style:--chip-soft={`var(--palette-${o.color}-soft)`}>{o.name}</span
				>{:else}<span class="empty">A deleted choice</span>{/each}
		{:else if type === 'checkbox'}
			{#if value === true}<Check size={15} /><span class="visually-hidden">Checked</span>
			{:else}<span class="empty">Not checked</span>{/if}
		{:else if type === 'user' && typeof value === 'string'}
			{@const p = session.personOf(ws, value)}
			<Avatar name={p.name} src={p.avatar} /><span class="text">{p.name}</span>
		{:else if type === 'date' && typeof value === 'string'}
			<span class="text tabular">{formatDate(value)}</span>
		{:else if type === 'url' && typeof value === 'string'}
			<span class="text link">{host(value)}</span>
		{:else if type === 'number'}
			<span class="tabular">{typeof value === 'number' ? value.toLocaleString() : ''}</span>
		{:else}
			<span class="text">{String(value)}</span>
		{/if}
	{/if}
</span>

<style>
	.cell {
		position: relative;
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-width: 0;
		height: 100%;
		overflow: hidden;
		white-space: nowrap;
	}
	.end {
		justify-content: flex-end;
	}
	.text {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.muted {
		color: var(--text-secondary);
	}
	/* Empty cells say so to screen readers and stay quiet on screen. */
	.empty {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}
	.overdue {
		color: var(--danger);
		font-weight: 550;
	}
	.avatars {
		display: inline-flex;
	}
	.avatars > :global(* + *) {
		margin-left: -5px;
	}
	.option {
		flex: none;
		height: calc(20px * var(--ui-scale));
		padding: 0 6px;
		border-radius: var(--radius-full);
		background: var(--chip-soft);
		color: var(--text-primary);
		font-size: var(--text-xs);
		font-weight: 550;
		line-height: calc(20px * var(--ui-scale));
	}
	.link {
		color: var(--accent-text);
	}
</style>
