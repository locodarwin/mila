<script lang="ts">
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import ListChecks from '@lucide/svelte/icons/list-checks';
	import MessageSquare from '@lucide/svelte/icons/message-square';
	import { session } from '../../session.svelte';
	import { localToday } from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import type { Task } from '../../sync/entities';
	import type { PaletteColor } from '../../theme/palette';
	import LabelChip from '../LabelChip.svelte';
	import PriorityIcon from '../PriorityIcon.svelte';
	import ReminderIcon from '../ReminderIcon.svelte';
	import RepeatIcon from '../RepeatIcon.svelte';
	import BlockedMark from '../BlockedMark.svelte';
	import TimeBadge from '../TimeBadge.svelte';
	import { repeatLabel } from '../../recurrence';
	import { bellLabel } from '../../reminders';
	import { formatDate } from '../../dates';
	import Avatar from '../task/Avatar.svelte';
	import StatusIcon from '../StatusIcon.svelte';
	import FieldChip from './FieldChip.svelte';

	interface Props {
		ws: string;
		task: Task;
		/**
		 * The fields to show, built-in names and custom field IDs (a saved
		 * view's choice); without it, the usual ones.
		 */
		shown?: Set<string>;
	}

	let { ws, task, shown }: Props = $props();
	// Parts not shown are not worked out: the markup never reads them.
	const show = (part: string) => !shown || shown.has(part);
	const badge = (done: number, all: number, what: string) =>
		`${done} of ${all} ${what}${all === 1 ? '' : 's'} done`;
	const replica = $derived(engine.replica(ws));
	const labels = $derived(replica?.taskLabels(task.id) ?? []);
	const checklist = $derived(replica?.checklist(task.id) ?? []);
	const subtasks = $derived(replica?.subtasks(task.id) ?? []);
	const comments = $derived(replica?.comments(task.id).length ?? 0);
	const assignees = $derived(replica?.assignees(task.id) ?? []);
	const blockedBy = $derived(task.completedAt ? [] : (replica?.openBlockers(task.id) ?? []));
	const today = localToday();
	const repeats = $derived(repeatLabel(task.recurrence, task.recurrenceMode, task.dueDate));
	const reminders = $derived(
		session.user ? (replica?.reminders(session.user.id, task.id).length ?? 0) : 0
	);
	const overdue = $derived(task.dueDate !== null && task.dueDate < today && !task.completedAt);
	const status = $derived(shown?.has('status') ? replica?.status(task.statusId) : undefined);
	const project = $derived(
		shown?.has('project')
			? task.projectId
				? (replica?.project(task.projectId)?.name ?? '')
				: 'Inbox'
			: ''
	);
	const custom = $derived(
		shown ? (replica?.fields(task.projectId) ?? []).filter((f) => shown.has(f.id)) : []
	);
	const values = $derived(custom.length ? (replica?.fieldValues(task.id) ?? {}) : {});
	const hasMeta = $derived(
		!!status ||
			!!project ||
			(show('priority') && task.priority !== 'none') ||
			(show('due') && task.dueDate !== null) ||
			(shown?.has('start') && task.startDate !== null) ||
			(show('repeat') && repeats !== null) ||
			(show('reminders') && reminders > 0) ||
			(show('labels') && labels.length > 0) ||
			(show('checklist') && checklist.length > 0) ||
			(show('subtasks') && subtasks.length > 0) ||
			(show('comments') && comments > 0) ||
			(show('assignees') && assignees.length > 0) ||
			(show('estimate') && task.estimateMinutes !== null) ||
			(show('logged') && (replica?.logged(task.id) ?? 0) > 0) ||
			(!!shown?.has('remaining') && task.estimateMinutes !== null) ||
			(!!session.user && replica?.runningTimer(session.user.id)?.taskId === task.id) ||
			custom.some((f) => values[f.id] !== undefined)
	);
</script>

<span class="title" class:done={task.completedAt}>{task.title}</span>
{#if hasMeta}<span class="meta">
		{#if status}<StatusIcon category={status.category} /><span class="visually-hidden"
				>, {status.name}</span
			>{/if}
		{#if project}<span class="badge"><span class="visually-hidden">, project </span>{project}</span
			>{/if}
		{#if show('priority') && task.priority !== 'none'}<PriorityIcon priority={task.priority} />{/if}
		{#if show('labels')}{#each labels as l (l.id)}<LabelChip
					color={l.color as PaletteColor}
					name={l.name}
				/>{/each}{/if}
		<!-- Numbers and dates say what they are, for screen readers; a card's
		     name is all of this (Board.svelte). -->
		{#if show('subtasks') && subtasks.length}<span class="badge"
				><span aria-hidden="true"
					>{subtasks.filter((t) => t.completedAt).length}/{subtasks.length}</span
				><span class="visually-hidden"
					>, {badge(subtasks.filter((t) => t.completedAt).length, subtasks.length, 'subtask')}</span
				></span
			>{/if}
		{#if show('checklist') && checklist.length}<span class="badge"
				><ListChecks size={12} /><span aria-hidden="true"
					>{checklist.filter((i) => i.done).length}/{checklist.length}</span
				><span class="visually-hidden"
					>, {badge(
						checklist.filter((i) => i.done).length,
						checklist.length,
						'checklist item'
					)}</span
				></span
			>{/if}
		{#if show('comments') && comments}<span class="badge"
				><MessageSquare size={12} /><span aria-hidden="true">{comments}</span><span
					class="visually-hidden">, {comments} {comments === 1 ? 'comment' : 'comments'}</span
				></span
			>{/if}
		{#if blockedBy.length}<BlockedMark names={blockedBy} />{/if}
		{#if show('repeat') && repeats}<RepeatIcon label={repeats} />{/if}
		{#if show('reminders') && reminders}<ReminderIcon label={bellLabel(reminders)} />{/if}
		<TimeBadge {ws} {task} {show} left={shown?.has('remaining')} lead=", " />
		{#if shown?.has('start') && task.startDate}<span class="badge"
				><span class="visually-hidden">, </span>Starts {formatDate(task.startDate)}</span
			>{/if}
		{#if show('due') && task.dueDate}
			<span class="due" class:overdue>
				{#if overdue}<CircleAlert size={12} />{/if}<span class="visually-hidden"
					>, {overdue ? 'overdue, due' : 'due'}</span
				>
				{formatDate(task.dueDate)}
			</span>
		{/if}
		{#if show('assignees')}{#each assignees as u (u)}{@const who = session.personOf(ws, u)}<Avatar
					name={who.name}
					src={who.avatar}
					label={`assigned to ${who.name}`}
					size={20}
				/>{/each}{/if}
		{#each custom as f (f.id)}<FieldChip {ws} field={f} value={values[f.id]} />{/each}
	</span>{/if}

<style>
	.title {
		font-weight: 550;
		overflow-wrap: anywhere;
	}
	.title.done {
		color: var(--text-muted);
		text-decoration: line-through;
	}
	.meta {
		position: relative;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
	.badge,
	.due {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		font-size: var(--text-xs);
		color: var(--row-meta-text);
	}
	.due.overdue {
		color: var(--danger);
		font-weight: 600;
	}
	:global([data-density='dense']) .meta {
		display: none;
	}
	:global([data-density='dense']) :global(.card:hover) .meta,
	:global([data-density='dense']) :global(.card:focus-visible) .meta {
		display: flex;
	}
</style>
