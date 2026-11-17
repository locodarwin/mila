<script lang="ts">
	import { menu, taskActions } from '../contextMenu.svelte';
	import CalendarClock from '@lucide/svelte/icons/calendar-clock';
	import CalendarPlus from '@lucide/svelte/icons/calendar-plus';
	import Check from '@lucide/svelte/icons/check';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import ListChecks from '@lucide/svelte/icons/list-checks';
	import MessageSquare from '@lucide/svelte/icons/message-square';
	import { session } from '../session.svelte';
	import { localToday, setTaskCompleted, setTaskDeleted } from '../sync/data';
	import { engine } from '../sync/engine.svelte';
	import { scheduler } from '../schedule.svelte';
	import { swipe, type Swipe } from '../swipe';
	import { sayCount, selection } from '../selection.svelte';
	import { announce } from '../announce.svelte';
	import { openTask } from '../taskLink';
	import type { PaletteColor } from '../theme/palette';
	import LabelChip from './LabelChip.svelte';
	import PriorityIcon from './PriorityIcon.svelte';
	import ReminderIcon from './ReminderIcon.svelte';
	import RepeatIcon from './RepeatIcon.svelte';
	import BlockedMark from './BlockedMark.svelte';
	import TimeBadge from './TimeBadge.svelte';
	import { repeatLabel } from '../recurrence';
	import { bellLabel } from '../reminders';
	import { formatDate, formatTime } from '../dates';
	import Avatar from './task/Avatar.svelte';
	import type { Status, Task } from '../sync/entities';
	import { toasts } from '../toasts.svelte';
	import Checkbox from './Checkbox.svelte';
	import StatusIcon from './StatusIcon.svelte';
	import FieldChip from './views/FieldChip.svelte';

	interface Props {
		ws: string;
		task: Task;
		status?: Status;
		/**
		 * The fields to show, built-in names and custom field IDs (a saved
		 * view's choice); without it, the usual ones.
		 */
		shown?: Set<string>;
	}

	let { ws, task, status, shown }: Props = $props();
	// Parts not shown are not worked out: the markup never reads them.
	const show = (part: string) => !shown || shown.has(part);

	/** "2 of 3 subtasks done", for screen readers beside a "2/3" badge. */
	const badge = (done: number, all: number, what: string) =>
		`${done} of ${all} ${what}${all === 1 ? '' : 's'} done`;

	const replica = $derived(engine.replica(ws));
	// Selecting several: once anything is selected,
	// each row has a circle at its start; on a phone that is picking, it
	// takes the checkbox's place and swipes are off.
	const selecting = $derived(selection.on(ws));
	const picked = $derived(selection.has(ws, task.id));
	const picking = $derived(selecting && selection.picking);
	const labels = $derived(replica?.taskLabels(task.id) ?? []);
	const subtasks = $derived(replica?.subtasks(task.id) ?? []);
	const checklist = $derived(replica?.checklist(task.id) ?? []);
	const comments = $derived(replica?.comments(task.id).length ?? 0);
	const assignees = $derived(replica?.assignees(task.id) ?? []);
	const blockedBy = $derived(task.completedAt ? [] : (replica?.openBlockers(task.id) ?? []));
	const today = localToday();
	const repeats = $derived(repeatLabel(task.recurrence, task.recurrenceMode, task.dueDate));
	const reminders = $derived(
		session.user ? (replica?.reminders(session.user.id, task.id).length ?? 0) : 0
	);
	const overdue = $derived(
		task.dueDate !== null && task.dueDate < today && task.completedAt === null
	);
	const project = $derived(
		task.projectId ? (replica?.project(task.projectId)?.name ?? '') : 'Inbox'
	);
	const custom = $derived(
		shown ? (replica?.fields(task.projectId) ?? []).filter((f) => shown.has(f.id)) : []
	);
	const values = $derived(custom.length ? (replica?.fieldValues(task.id) ?? {}) : {});
	const dueLabel = $derived(
		task.dueDate
			? formatDate(task.dueDate) + (task.dueTime ? ` ${formatTime(task.dueTime)}` : '')
			: ''
	);

	// Completing holds the row for a second so a misclick is easy to take
	// back; only then does the change happen and the row move away.
	const HOLD_MS = 1000;
	let holding = $state(false);
	let timer: ReturnType<typeof setTimeout> | undefined;
	// What takes back the completion the hold ended in, for the swipe's undo.
	let undoCompletion: (() => Promise<boolean>) | undefined;
	const done = $derived(task.completedAt !== null || holding);

	function toggle(checked: boolean) {
		clearTimeout(timer);
		timer = undefined;
		if (task.completedAt !== null) {
			void setTaskCompleted(ws, task, false);
			return;
		}
		holding = checked;
		if (checked)
			timer = setTimeout(() => {
				timer = undefined;
				undoCompletion = engine.track(() => setTaskCompleted(ws, task, true)).undo;
			}, HOLD_MS);
	}
	$effect(() => () => clearTimeout(timer));

	// Swiping right completes through the same hold as the checkbox, or
	// reopens a completed task. A swipe misfires more easily than a click,
	// so it always offers an undo: during the hold that is letting go of
	// it, after it the change itself (with a repeat's next task) undone,
	// whatever else was edited since.
	function swipeComplete() {
		const { title } = task;
		if (task.completedAt !== null) {
			const { undo } = engine.track(() => setTaskCompleted(ws, task, false));
			toasts.show(`Reopened "${title}"`, { onundo: () => void undo() });
			return;
		}
		if (holding) return;
		toggle(true);
		undoCompletion = undefined;
		toasts.show(`Completed "${title}"`, {
			onundo: () => {
				if (timer !== undefined) toggle(false);
				else void undoCompletion?.();
			}
		});
	}

	// Swiping left schedules: the same chooser as the schedule button.
	let scheduleButton = $state<HTMLButtonElement>();
	function schedule() {
		scheduler.open(ws, task.id, scheduleButton ?? null);
	}

	// How far the row has been swiped, and what letting go would do.
	let offset = $state(0);
	let pending = $state<Swipe>(null);

	function remove() {
		void setTaskDeleted(ws, task.id, true);
		toasts.show(`Deleted "${task.title}"`, {
			onundo: () => void setTaskDeleted(ws, task.id, false)
		});
	}
</script>

<div
	class="swipe"
	use:menu={taskActions(ws, task.id)}
	class:moving={offset !== 0}
	class:toward-right={offset > 0}
	class:toward-left={offset < 0}
	class:armed={pending !== null}
	use:swipe={{
		off: picking,
		onright: swipeComplete,
		onleft: schedule,
		onmove: (dx, outcome) => {
			offset = dx;
			pending = outcome;
		}
	}}
>
	<div class="reveal" aria-hidden="true">
		<span class="complete">
			{#if task.completedAt !== null}<RotateCcw size={18} />Reopen{:else}<Check
					size={18}
				/>Complete{/if}
		</span>
		<span class="later">Schedule<CalendarClock size={18} /></span>
	</div>
	<div
		class="row"
		class:done
		class:selecting
		class:picked
		style:transform={offset ? `translateX(${offset}px)` : undefined}
	>
		{#if selecting}
			<button
				type="button"
				class="pick"
				role="checkbox"
				aria-checked={picked}
				aria-label={`Select ${task.title}`}
				title={picked ? 'Take out of the selection' : 'Add to the selection'}
				onclick={(e) => {
					// Plain, or with the keys the list hears too: here it is always one.
					e.stopPropagation();
					selection.toggle(ws, task.id);
					announce(sayCount());
				}}
				><span class="dot" aria-hidden="true"
					>{#if picked}<Check size={11} strokeWidth={3} />{/if}</span
				></button
			>
		{/if}
		{#if !picking}<Checkbox label={task.title} hideLabel checked={done} onchange={toggle} />{/if}
		{#if status && show('status')}<StatusIcon category={done ? 'done' : status.category} />{/if}
		<button class="title" type="button" onclick={() => openTask(task.id)}
			><span class="text">{task.title}</span></button
		>
		<span class="meta">
			{#if blockedBy.length}<BlockedMark names={blockedBy} />{/if}
			{#if shown?.has('project') && project}<span class="badge"
					><span class="visually-hidden">Project: </span>{project}</span
				>{/if}
			{#if show('priority') && task.priority !== 'none'}<PriorityIcon
					priority={task.priority}
				/>{/if}
			{#if show('labels')}{#each labels as l (l.id)}<span
						class="chipmenu"
						use:menu={() => import('../menus').then((m) => m.labelMenu(ws, task.id, l))}
						><LabelChip color={l.color as PaletteColor} name={l.name} /></span
					>{/each}{/if}
			<!-- The badges say in words, for screen readers, what their numbers mean. -->
			{#if show('subtasks') && subtasks.length}<span class="badge tabular" title="Subtasks done"
					><span aria-hidden="true"
						>{subtasks.filter((t) => t.completedAt).length}/{subtasks.length}</span
					><span class="visually-hidden"
						>{badge(subtasks.filter((t) => t.completedAt).length, subtasks.length, 'subtask')}</span
					></span
				>{/if}
			{#if show('checklist') && checklist.length}<span
					class="badge tabular"
					title="Checklist items done"
					><ListChecks size={12} /><span aria-hidden="true"
						>{checklist.filter((i) => i.done).length}/{checklist.length}</span
					><span class="visually-hidden"
						>{badge(
							checklist.filter((i) => i.done).length,
							checklist.length,
							'checklist item'
						)}</span
					></span
				>{/if}
			{#if show('comments') && comments}<span class="badge tabular" title="Comments"
					><MessageSquare size={12} /><span aria-hidden="true">{comments}</span><span
						class="visually-hidden">{comments} {comments === 1 ? 'comment' : 'comments'}</span
					></span
				>{/if}
			{#if show('repeat') && repeats}<RepeatIcon label={repeats} />{/if}
			{#if show('reminders') && reminders}<ReminderIcon label={bellLabel(reminders)} />{/if}
			<TimeBadge {ws} {task} {show} left={shown?.has('remaining')} />
			{#if show('assignees')}{#each assignees as userId (userId)}{@const p = session.personOf(
						ws,
						userId
					)}<Avatar
						name={p.name}
						src={p.avatar}
						label={`Assigned to ${p.name}`}
						size={20}
					/>{/each}{/if}
			{#each custom as f (f.id)}<FieldChip {ws} field={f} value={values[f.id]} />{/each}
			{#if shown?.has('start') && task.startDate}<span class="badge tabular"
					>Starts {formatDate(task.startDate)}</span
				>{/if}
		</span>
		<!-- One button whether or not there is a date, so focus stays on it
		     when the schedule chooser sets or clears one. -->
		<button
			class={task.dueDate && show('due') ? 'due tabular' : 'icon'}
			class:overdue
			type="button"
			aria-label={task.dueDate
				? `${overdue ? 'Overdue, due' : 'Due'} ${dueLabel}, schedule ${task.title}`
				: `Schedule ${task.title}`}
			data-schedule={task.id}
			bind:this={scheduleButton}
			onclick={schedule}
		>
			<!-- Overdue is marked by a shape as well as the color. -->
			{#if task.dueDate && show('due')}{#if overdue}<CircleAlert
						size={12}
					/>{/if}{dueLabel}{:else}<CalendarPlus size={15} />{/if}
		</button>
		<button class="icon delete" type="button" aria-label={`Delete ${task.title}`} onclick={remove}>
			<Trash2 size={15} />
		</button>
	</div>
</div>

<style>
	/* The title always keeps room; on a narrow screen the details wrap to a
	   second line instead of squeezing it. */
	/* What a swipe shows under the row as it moves aside. */
	.swipe {
		position: relative;
		overflow: hidden;
	}
	.reveal {
		position: absolute;
		inset: 0;
		display: none;
		align-items: center;
		justify-content: space-between;
		padding: 0 var(--space-4);
		color: var(--text-primary);
		font-size: var(--text-sm);
		font-weight: 600;
	}
	.reveal span {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
	}
	.moving .reveal {
		display: flex;
	}
	.toward-right .reveal {
		background: color-mix(in oklch, var(--status-done) 22%, var(--surface-0));
	}
	.toward-right.armed .reveal {
		background: color-mix(in oklch, var(--status-done) 45%, var(--surface-0));
	}
	.toward-left .reveal {
		justify-content: flex-end;
		background: var(--accent-soft);
	}
	.toward-left.armed .reveal {
		background: color-mix(in oklch, var(--accent) 35%, var(--surface-0));
	}
	.toward-right .later,
	.toward-left .complete {
		display: none;
	}
	.row {
		position: relative;
		transition:
			background-color var(--dur-micro) var(--ease),
			transform var(--dur-panel) var(--ease);
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-1) var(--space-3);
		min-height: var(--row-height);
		padding: 4px var(--pad-x);
		border-bottom: 1px solid var(--row-divider);
		background: var(--surface-0);
	}
	.moving .row {
		transition: none;
	}
	.row:hover,
	.row:focus-within {
		background: var(--row-hover-bg);
	}
	.row.picked,
	.row.picked:hover {
		background: var(--row-selected-bg);
	}
	/* The circle that adds a task to the selection, or takes it out. */
	.pick {
		display: grid;
		place-items: center;
		width: max(var(--tap-min), calc(20px * var(--ui-scale)));
		height: calc(20px * var(--ui-scale));
		margin: 0 calc(-4px * var(--ui-scale)) 0 calc(-6px * var(--ui-scale));
		padding: 0;
		border: none;
		background: none;
		cursor: pointer;
	}
	.pick .dot {
		display: grid;
		place-items: center;
		width: calc(16px * var(--ui-scale));
		height: calc(16px * var(--ui-scale));
		border: 1.5px solid var(--border-control);
		border-radius: 50%;
		color: var(--checkbox-mark);
	}
	.picked .pick .dot {
		border-color: var(--checkbox-on-bg);
		background: var(--checkbox-on-bg);
	}
	.pick:focus-visible {
		outline: none;
	}
	.pick:focus-visible .dot {
		box-shadow: var(--focus-ring);
	}
	.title {
		flex: 1 1 10rem;
		min-width: 8rem;
		padding: 0;
		border: none;
		background: none;
		text-align: left;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		cursor: text;
		transition: color var(--dur-panel) var(--ease);
	}
	/* The strike draws across the words only. */
	.text {
		background: linear-gradient(currentColor, currentColor) no-repeat 0 55% / 0 1px;
		transition: background-size var(--dur-panel) var(--ease);
	}
	.done .title {
		color: var(--text-muted);
	}
	.done .text {
		background-size: 100% 1px;
	}
	.meta {
		position: relative;
		display: flex;
		flex: 0 1 auto;
		flex-wrap: wrap;
		align-items: center;
		justify-content: flex-end;
		gap: var(--space-2);
		margin-left: auto;
	}
	.meta:empty {
		display: none;
	}
	.badge {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		font-size: var(--text-xs);
		color: var(--row-meta-text);
	}
	.due {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		padding: 2px var(--space-1);
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		font-size: var(--text-xs);
		color: var(--row-meta-text);
		cursor: pointer;
	}
	.due:hover {
		background: var(--surface-3);
		color: var(--text-primary);
	}
	.due.overdue {
		color: var(--danger);
		font-weight: 600;
	}
	:global([data-density='dense']) .meta :global(.chip) {
		display: none;
	}
	.icon {
		display: grid;
		place-items: center;
		width: calc(28px * var(--ui-scale));
		height: calc(28px * var(--ui-scale));
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-muted);
		cursor: pointer;
		opacity: 0;
	}
	.row:hover .icon,
	.icon:focus-visible {
		opacity: 1;
	}
	.icon:hover {
		background: var(--surface-3);
		color: var(--text-primary);
	}
	.delete:hover {
		color: var(--danger);
	}
	@media (hover: none) {
		.icon {
			opacity: 1;
		}
	}
</style>
