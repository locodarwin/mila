<script lang="ts">
	import { untrack } from 'svelte';
	import { intents } from '../../intents.svelte';
	import ArrowUpLeft from '@lucide/svelte/icons/arrow-up-left';
	import Expand from '@lucide/svelte/icons/expand';
	import FileCode from '@lucide/svelte/icons/file-code';
	import Shrink from '@lucide/svelte/icons/shrink';
	import Maximize2 from '@lucide/svelte/icons/maximize-2';
	import Minimize2 from '@lucide/svelte/icons/minimize-2';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import LayoutTemplate from '@lucide/svelte/icons/layout-template';
	import X from '@lucide/svelte/icons/x';
	import Checkbox from '../Checkbox.svelte';
	import PriorityIcon from '../PriorityIcon.svelte';
	import {
		addReminder,
		localToday,
		removeReminder,
		moveTask,
		setRecurrence,
		setTaskCompleted,
		setTaskDeleted,
		setTaskFields,
		setTaskStatus,
		setTaskTitle
	} from '../../sync/data';
	import { session } from '../../session.svelte';
	import { engine } from '../../sync/engine.svelte';
	import type { Priority } from '../../sync/entities';
	import { toasts } from '../../toasts.svelte';
	import { ui } from '../../ui.svelte';
	import AssigneePicker from './AssigneePicker.svelte';
	import Attachments from './Attachments.svelte';
	import Checklist from './Checklist.svelte';
	import Comments from './Comments.svelte';
	import Dependencies from './Dependencies.svelte';
	import FieldInput from './FieldInput.svelte';
	import History from './History.svelte';
	import LabelPicker from './LabelPicker.svelte';
	import DocEditor from '../doc/DocEditor.svelte';
	import HelpLink from '../help/HelpLink.svelte';
	import LinkedFrom from '../doc/LinkedFrom.svelte';
	import PanelSection from './PanelSection.svelte';
	import ReminderList from './ReminderList.svelte';
	import RepeatPicker from './RepeatPicker.svelte';
	import Subtasks from './Subtasks.svelte';
	import TimeSection from './TimeSection.svelte';
	import SaveTemplate from '../templates/SaveTemplate.svelte';
	import { canOrganize, canUseTemplates } from '../../roles';
	import { formatDate } from '../../dates';
	import { formatDuration } from '../../duration';
	import { preferences } from '../../preferences.svelte';
	import { describe, tryParseRule } from '../../recurrence';
	import {
		countOf,
		dependenciesOf,
		fieldsOf,
		notesOf,
		progressOf,
		scheduleOf,
		timeOf
	} from '../../taskPanel';

	interface Props {
		ws: string;
		taskId: string;
		onclose: () => void;
		onopen: (taskId: string) => void;
		/** Filling the page rather than peeking in beside the list. */
		expanded?: boolean;
		/** Switches between the two; left out where there is no choice (phones). */
		onexpand?: () => void;
		/** The layout, for the style guide; otherwise the person's preference. */
		layoutAs?: 'boxes' | 'summary';
	}

	let { ws, taskId, onclose, onopen, expanded = false, onexpand, layoutAs }: Props = $props();

	const panel = $derived(layoutAs ?? preferences.current.taskPanel);

	const replica = $derived(engine.replica(ws));
	const task = $derived(replica?.task(taskId));
	const parent = $derived(task?.parentId ? replica?.task(task.parentId) : undefined);
	const project = $derived(task?.projectId ? replica?.project(task.projectId) : undefined);
	const statuses = $derived(task?.projectId ? (replica?.statuses(task.projectId) ?? []) : []);
	const checklist = $derived(replica?.checklist(taskId) ?? []);
	const doneItems = $derived(checklist.filter((i) => i.done).length);
	const subtasks = $derived(replica?.subtasks(taskId) ?? []);
	const doneSubtasks = $derived(subtasks.filter((t) => t.completedAt).length);
	const files = $derived(replica?.attachments(taskId) ?? []);
	const comments = $derived(replica?.comments(taskId) ?? []);
	const fields = $derived(task ? (replica?.fields(task.projectId) ?? []) : []);
	const values = $derived(replica?.fieldValues(taskId) ?? {});
	const filledFields = $derived(
		fields.filter((f) => values[f.id] !== undefined && values[f.id] !== null && values[f.id] !== '')
			.length
	);
	const linkedCount = $derived.by(() => {
		const from = replica?.linkedFrom(taskId);
		return from ? from.pages.length + from.tasks.length : 0;
	});
	const repeatWords = $derived.by(() => {
		const rule = tryParseRule(task?.recurrence);
		return rule && task ? describe(rule, task.recurrenceMode, task.dueDate ?? localToday()) : null;
	});
	const reminders = $derived(
		session.user ? (replica?.reminders(session.user.id, taskId) ?? []) : []
	);
	let title = $derived(task?.title ?? '');
	let showHistory = $state(false);
	let templating = $state(false);
	// Save as a template, asked from a right-click menu (menus.ts).
	$effect(() => {
		void intents.pending;
		const id = taskId;
		if (untrack(() => intents.take('task', id)) === 'template') templating = true;
	});
	// The notes as Markdown, and the notes filling the panel for long writing.
	let notesSource = $state(false);
	let notesWide = $state(false);
	$effect(() => {
		void taskId;
		notesWide = notesSource = false;
	});

	const priorities: { value: Priority; label: string }[] = [
		{ value: 'none', label: 'No priority' },
		{ value: 'low', label: 'Low' },
		{ value: 'medium', label: 'Medium' },
		{ value: 'high', label: 'High' },
		{ value: 'urgent', label: 'Urgent' }
	];

	function saveTitle() {
		// A title is one line: lines pasted in are joined.
		const t = title.replace(/\s*[\r\n]+\s*/g, ' ').trim();
		if (task && t && t !== task.title) void setTaskTitle(ws, task, t);
		else title = task?.title ?? '';
	}

	async function remove() {
		if (!task) return;
		const { id, title: gone } = task;
		await setTaskDeleted(ws, id, true);
		toasts.show(`Deleted "${gone}"`, { onundo: () => void setTaskDeleted(ws, id, false) });
		onclose();
	}
</script>

<svelte:window
	onkeydown={(e) => {
		// Escape closes the innermost thing: an open picker, menu or dialog
		// first, a field that handled it itself, and only then the panel.
		if (e.key !== 'Escape' || e.defaultPrevented || ui.overlay) return;
		if (document.querySelector(':popover-open, dialog[open]')) return;
		onclose();
	}}
/>

{#snippet expand()}
	{#if onexpand}
		<button
			class="icon"
			type="button"
			aria-label={expanded ? 'Show beside the list' : 'Expand to the full page'}
			onclick={onexpand}
			>{#if expanded}<Minimize2 size={16} />{:else}<Maximize2 size={16} />{/if}</button
		>
	{/if}
{/snippet}

<!-- The app layout wraps this in the "Task details" landmark, with the
     edge that resizes it. -->
<div class="detail" class:expanded class:notes-wide={notesWide}>
	{#if task}
		<!-- On a phone the header is also the sheet's handle: pull it down to close. -->
		<header data-grab>
			<span class="grip" aria-hidden="true"></span>
			{#if parent}
				<button class="crumb" type="button" onclick={() => onopen(parent.id)}>
					<ArrowUpLeft size={14} /><span class="crumb-text">{parent.title}</span>
				</button>
			{:else}
				<label class="crumb">
					<span class="visually-hidden">Project</span>
					<select
						aria-label="Project"
						value={task.projectId ?? ''}
						onchange={(e) => void moveTask(ws, task, e.currentTarget.value || null)}
					>
						{#if !session.workspace || canOrganize(session.workspace.role)}
							<option value="">Inbox</option>
						{/if}
						{#each replica?.projects() ?? [] as p (p.id)}
							<option value={p.id}>{p.name}</option>
						{/each}
					</select>
				</label>
			{/if}
			<span class="grow"></span>
			{@render expand()}
			{#if session.workspace && canUseTemplates(session.workspace.role)}
				<button
					class="icon"
					type="button"
					aria-label="Save as a template"
					title="Save as a template"
					onclick={() => (templating = true)}><LayoutTemplate size={16} /></button
				>
			{/if}
			<button class="icon" type="button" aria-label="Delete task" onclick={remove}
				><Trash2 size={16} /></button
			>
			<button class="icon" type="button" aria-label="Close" onclick={onclose}
				><X size={18} /></button
			>
		</header>

		<!-- The title is a field; this gives the sections below a heading to sit under. -->
		<h2 class="visually-hidden">{task.title}</h2>
		<div class="title-row">
			<Checkbox
				label="Completed"
				hideLabel
				checked={task.completedAt !== null}
				onchange={(done) => void setTaskCompleted(ws, task, done)}
			/>
			<textarea
				class="title"
				aria-label="Task title"
				rows="1"
				bind:value={title}
				onblur={saveTitle}
				onkeydown={(e) => {
					if (e.key === 'Enter') {
						e.preventDefault();
						e.currentTarget.blur();
					}
				}}></textarea>
		</div>

		{#snippet noteTools()}
			<button
				class="tool"
				type="button"
				aria-pressed={notesSource}
				title="Show the notes as Markdown"
				onclick={() => (notesSource = !notesSource)}><FileCode size={14} />Markdown</button
			>
			<button
				class="icon small"
				type="button"
				aria-label={notesWide ? 'Show the notes with the rest' : 'Open the notes wide'}
				title={notesWide ? 'Show the notes with the rest' : 'Open the notes wide'}
				onclick={() => (notesWide = !notesWide)}
				>{#if notesWide}<Shrink size={14} />{:else}<Expand size={14} />{/if}</button
			>
		{/snippet}

		<!-- What the task is and whose: first in either layout. -->
		{#snippet main()}
			{#if project}
				<div class="prop">
					<dt>Status</dt>
					<dd>
						<select
							aria-label="Status"
							value={task.statusId ?? ''}
							onchange={(e) => void setTaskStatus(ws, task, e.currentTarget.value)}
						>
							{#each statuses as s (s.id)}
								<option value={s.id}>{s.name}</option>
							{/each}
						</select>
					</dd>
				</div>
			{/if}
			<div class="prop">
				<dt>Priority</dt>
				<dd class="with-icon">
					{#if task.priority !== 'none'}<PriorityIcon priority={task.priority} />{/if}
					<select
						aria-label="Priority"
						value={task.priority}
						onchange={(e) =>
							void setTaskFields(ws, task.id, { priority: e.currentTarget.value as Priority })}
					>
						{#each priorities as p (p.value)}
							<option value={p.value}>{p.label}</option>
						{/each}
					</select>
				</dd>
			</div>
			<div class="prop">
				<dt>Assignees</dt>
				<dd>
					<AssigneePicker {ws} taskId={task.id} assigned={replica?.assignees(task.id) ?? []} />
				</dd>
			</div>
			<div class="prop">
				<dt>Labels</dt>
				<dd>
					<LabelPicker
						{ws}
						taskId={task.id}
						all={replica?.labels() ?? []}
						on={replica?.taskLabels(task.id) ?? []}
					/>
				</dd>
			</div>
		{/snippet}

		{#snippet fieldsSection()}
			{#if fields.length}
				<PanelSection
					id="fields"
					title={project ? `Fields for ${project.name}` : 'Custom fields'}
					{panel}
					boxed
					summary={fieldsOf(filledFields, fields.length)}
				>
					<dl class="props">
						{#each fields as f (f.id)}
							<div class="prop">
								<dt>{f.name}</dt>
								<dd><FieldInput {ws} taskId={task.id} field={f} value={values[f.id]} /></dd>
							</div>
						{/each}
					</dl>
				</PanelSection>
			{/if}
		{/snippet}

		{#snippet sections()}
			<PanelSection
				id="schedule"
				title="Schedule"
				{panel}
				boxed
				summary={scheduleOf({
					due: task.dueDate && formatDate(task.dueDate),
					start: task.startDate && formatDate(task.startDate),
					repeat: repeatWords,
					reminders: reminders.length
				})}
			>
				<dl class="props">
					<div class="prop">
						<dt>Start</dt>
						<dd>
							<input
								type="date"
								aria-label="Start date"
								value={task.startDate ?? ''}
								onchange={(e) =>
									void setTaskFields(ws, task.id, { start_date: e.currentTarget.value || null })}
							/>
						</dd>
					</div>
					<div class="prop">
						<dt>Due</dt>
						<dd class="due">
							<input
								type="date"
								aria-label="Due date"
								value={task.dueDate ?? ''}
								onchange={(e) =>
									void setTaskFields(ws, task.id, {
										due_date: e.currentTarget.value || null,
										...(e.currentTarget.value ? {} : { due_time: null })
									})}
							/>
							{#if task.dueDate}
								<input
									type="time"
									aria-label="Due time"
									value={task.dueTime ?? ''}
									onchange={(e) =>
										void setTaskFields(ws, task.id, { due_time: e.currentTarget.value || null })}
								/>
							{/if}
						</dd>
					</div>
					<div class="prop parted">
						<dt>Repeat<HelpLink topic="recurrence" label="How repeats work" /></dt>
						<dd>
							{#key task.id}
								<RepeatPicker
									rule={task.recurrence}
									mode={task.recurrenceMode}
									anchor={task.dueDate ?? localToday()}
									onchange={(rule, mode) => void setRecurrence(ws, task, rule, mode)}
								/>
							{/key}
						</dd>
					</div>
					<div class="prop parted">
						<dt>
							Reminders<HelpLink
								topic="reminders-and-notifications"
								section="reminders"
								label="How reminders work"
							/>
						</dt>
						<dd>
							<ReminderList
								{task}
								{reminders}
								onadd={(when) => void addReminder(ws, task.id, when)}
								onremove={(id) => void removeReminder(ws, id)}
							/>
						</dd>
					</div>
				</dl>
			</PanelSection>

			{#if panel === 'boxes'}{@render fieldsSection()}{/if}

			<!-- Escape in the wide notes shrinks them back, before the text takes it, unless the
			     / menu or the formatting bar is open: those close first. -->
			<div
				class="notes"
				onkeydowncapture={(e) => {
					if (e.key !== 'Escape' || !notesWide) return;
					const at = e.currentTarget as HTMLElement;
					if (at.querySelector('[role="listbox"], [role="toolbar"]:focus-within, dialog[open]'))
						return;
					e.preventDefault();
					e.stopPropagation();
					notesWide = false;
				}}
			>
				<PanelSection
					id="notes"
					title="Notes"
					{panel}
					summary={notesOf(task.notes)}
					unnamed
					tools={noteTools}
				>
					{#key task.id}
						<DocEditor
							{ws}
							target={{ kind: 'task', id: task.id }}
							markdown={task.notes}
							label="Notes"
							placeholder="Add notes, or type / for a list, table or image"
							bind:source={notesSource}
						/>
					{/key}
				</PanelSection>
			</div>

			{#if panel === 'summary'}{@render fieldsSection()}{/if}

			{#if !task.parentId}
				<PanelSection
					id="subtasks"
					title="Subtasks"
					count={subtasks.length ? `${doneSubtasks}/${subtasks.length}` : undefined}
					{panel}
					summary={progressOf(doneSubtasks, subtasks.length)}
				>
					<Subtasks {ws} parent={task} {subtasks} />
				</PanelSection>
			{/if}

			<PanelSection
				id="checklist"
				title="Checklist"
				count={checklist.length ? `${doneItems}/${checklist.length}` : undefined}
				{panel}
				summary={progressOf(doneItems, checklist.length)}
			>
				<Checklist {ws} taskId={task.id} items={checklist} />
			</PanelSection>

			{#if replica}
				<PanelSection
					id="dependencies"
					title="Dependencies"
					{panel}
					boxed
					help={{
						topic: 'timeline-and-dependencies',
						section: 'dependencies',
						label: 'How dependencies work'
					}}
					summary={dependenciesOf(
						replica.openBlockers(task.id).length,
						replica.waiting(task.id).length
					)}
				>
					<Dependencies {ws} {replica} {task} />
				</PanelSection>

				<PanelSection
					id="time"
					title="Time"
					{panel}
					boxed
					help={{ topic: 'time-tracking', label: 'How time tracking works' }}
					summary={timeOf(
						replica.logged(task.id) ? formatDuration(replica.logged(task.id)) : null,
						task.estimateMinutes ? formatDuration(task.estimateMinutes) : null
					)}
					wanted={ui.logTime === task.id}
				>
					<TimeSection {ws} {replica} {task} />
				</PanelSection>
			{/if}

			{#if linkedCount}
				<PanelSection
					id="linked"
					title="Linked from"
					{panel}
					summary={countOf(linkedCount, 'link')}
				>
					<LinkedFrom {ws} id={task.id} bare />
				</PanelSection>
			{/if}

			<PanelSection id="files" title="Files" {panel} summary={countOf(files.length, 'file')}>
				<Attachments {ws} taskId={task.id} items={files} />
			</PanelSection>

			<PanelSection
				id="comments"
				title="Comments"
				count={comments.length ? String(comments.length) : undefined}
				{panel}
				summary={countOf(comments.length, 'comment')}
			>
				<Comments {ws} taskId={task.id} items={comments} />
			</PanelSection>
		{/snippet}

		{#if panel === 'summary'}
			<dl class="props glance">{@render main()}</dl>
			<div class="stack">
				{@render sections()}
				<PanelSection id="history" title="History" {panel} summary="Show what changed">
					<History {ws} taskId={task.id} />
				</PanelSection>
			</div>
		{:else}
			<PanelSection id="details" title="Details" {panel} boxed>
				<dl class="props">{@render main()}</dl>
			</PanelSection>
			{@render sections()}
			<section class="history">
				<button
					class="history-toggle"
					type="button"
					aria-expanded={showHistory}
					onclick={() => (showHistory = !showHistory)}
				>
					{showHistory ? 'Hide history' : 'Show history'}
				</button>
				{#if showHistory}<History {ws} taskId={task.id} />{/if}
			</section>
		{/if}
		<SaveTemplate
			bind:open={templating}
			{ws}
			kind="task"
			sourceId={task.id}
			sourceName={task.title}
		/>
	{:else}
		<header data-grab>
			<span class="grip" aria-hidden="true"></span>
			<span class="grow"></span>
			<button class="icon" type="button" aria-label="Close" onclick={onclose}
				><X size={18} /></button
			>
		</header>
		<p class="gone">This task was deleted or is not in this workspace.</p>
	{/if}
</div>

<style>
	.detail {
		position: relative;
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		height: 100%;
		padding: var(--space-3) var(--space-5) var(--space-7);
		overflow-y: auto;
		/* A field reached with Tab or Shift+Tab scrolls clear of the sticky header. */
		scroll-padding-top: calc(max(var(--tap-min), 32px) + 2 * var(--space-2) + var(--space-3));
		background: var(--panel-bg);
	}
	/* Full page: the same panel, in a column that reads well. */
	.detail.expanded > :not(header),
	.detail.expanded > :global(section) {
		width: 100%;
		max-width: 760px;
		margin-inline: auto;
	}
	.grip {
		display: none;
	}
	/* The notes, wide: everything else in the panel steps aside. */
	.detail.notes-wide > :not(header):not(.notes):not(.stack),
	.detail.notes-wide > :global(section) {
		display: none;
	}
	.detail.notes-wide .stack > :global(:not(.notes)) {
		display: none;
	}
	.detail.notes-wide .stack {
		display: flex;
		flex: 1;
		flex-direction: column;
	}
	.detail.notes-wide .notes {
		flex: 1;
	}
	.notes {
		display: grid;
		min-width: 0;
	}
	.tool {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-height: max(var(--tap-min), 26px);
		padding: 0 var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-xs);
		cursor: pointer;
	}
	.tool:hover {
		background: var(--surface-2);
	}
	.tool[aria-pressed='true'] {
		background: var(--accent-soft);
		color: var(--accent-text);
	}
	header {
		position: sticky;
		top: calc(-1 * var(--space-3));
		z-index: 1;
		display: flex;
		align-items: center;
		gap: var(--space-1);
		margin: calc(-1 * var(--space-3)) calc(-1 * var(--space-5)) 0;
		padding: var(--space-2) var(--space-3) var(--space-2) var(--space-5);
		background: var(--panel-bg);
		border-bottom: var(--border-width) solid var(--border);
	}
	.grow {
		flex: 1;
	}
	.crumb {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		/* A long project name or parent title shortens with an ellipsis,
		   clear of the buttons beside it. */
		min-width: 0;
		max-width: 70%;
		padding: var(--space-1) var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.crumb:hover {
		background: var(--surface-2);
	}
	.crumb :global(svg) {
		flex: none;
	}
	.crumb-text {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.crumb select {
		min-width: 0;
		max-width: 100%;
		text-overflow: ellipsis;
		border: none;
		background: none;
		color: inherit;
		font-size: var(--text-sm);
	}
	.icon {
		display: grid;
		place-items: center;
		width: 32px;
		height: 32px;
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.icon:hover {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	.icon.small {
		width: max(var(--tap-min), 26px);
		height: max(var(--tap-min), 26px);
	}
	.title-row {
		display: flex;
		gap: var(--space-3);
		align-items: flex-start;
	}
	.title-row :global(.checkbox) {
		margin-top: 6px;
	}
	.title {
		flex: 1;
		padding: 2px var(--space-1);
		border: var(--border-width) solid transparent;
		border-radius: var(--radius-sm);
		background: none;
		font-family: var(--font-heading);
		font-size: var(--text-xl);
		font-weight: var(--heading-weight);
		resize: none;
		field-sizing: content;
	}
	.title:hover {
		border-color: var(--border);
	}
	/* A border and a ring inside it, two pixels of accent in all. */
	.title:focus-visible {
		border-color: var(--accent);
		box-shadow: inset 0 0 0 1px var(--accent);
	}
	.props {
		display: grid;
		gap: var(--space-2);
		margin: 0;
	}
	/* A name lines up with the top of what it names, however tall that grows. */
	.prop {
		display: grid;
		/* The names grow with the font size, so their column does too. */
		grid-template-columns: calc(96px * var(--ui-scale)) minmax(0, 1fr);
		gap: var(--space-3);
		align-items: start;
	}
	.parted {
		padding-top: var(--space-2);
		border-top: var(--border-width) solid var(--border);
	}
	dt {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-1);
		/* A field's name breaks rather than running under its value: its
		   help button goes under it first, and only a word too long for the
		   column alone breaks. */
		min-width: 0;
		min-height: max(var(--tap-min), 30px);
		overflow-wrap: break-word;
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	dd {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		justify-content: center;
		margin: 0;
		min-width: 0;
		min-height: max(var(--tap-min), 30px);
	}
	/* The summary layout's four main choices, each name over its control. */
	.glance {
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: var(--space-3) var(--space-4);
	}
	.glance .prop {
		grid-template-columns: minmax(0, 1fr);
		gap: var(--space-1);
		align-content: start;
	}
	.glance dt {
		min-height: 0;
		font-size: var(--text-xs);
	}
	.glance dd {
		align-items: stretch;
	}
	.glance .with-icon {
		align-items: center;
	}
	.glance select {
		flex: 1;
		width: 100%;
		min-width: 0;
	}
	.stack {
		min-width: 0;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
	}
	.stack > :global(* + *) {
		border-top: var(--border-width) solid var(--border);
	}
	.with-icon,
	.due {
		flex-direction: row;
		justify-content: flex-start;
		gap: var(--space-2);
		align-items: center;
	}
	/* The time goes under the date when they do not fit side by side. */
	.due {
		flex-wrap: wrap;
	}
	dd select,
	dd input {
		/* A long status or option name shortens instead of widening the panel. */
		max-width: 100%;
		text-overflow: ellipsis;
		height: 30px;
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		font-size: var(--text-sm);
	}
	dd select:hover,
	dd input:hover {
		border-color: var(--border-strong);
	}
	.history {
		display: grid;
		gap: var(--space-2);
		padding-inline: var(--space-3);
	}
	.history-toggle {
		justify-self: start;
		padding: var(--space-1) var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.history-toggle:hover {
		background: var(--surface-2);
	}
	.gone {
		color: var(--text-secondary);
	}

	/* A phone: a full-screen sheet, clear of the notch and the home bar. */
	@media (max-width: 720px) {
		.detail {
			padding-bottom: calc(var(--space-7) + env(safe-area-inset-bottom));
			scroll-padding-top: calc(
				max(var(--tap-min), 32px) + var(--space-2) + 2 * var(--space-3) + env(safe-area-inset-top)
			);
		}
		header {
			padding-top: calc(var(--space-3) + env(safe-area-inset-top));
			touch-action: none;
		}
	}
	/* A finger's hit area around the box (Checkbox.svelte) without moving it
	   off the title's first line. */
	@media (pointer: coarse) {
		.title-row :global(label.checkbox.bare) {
			margin-top: calc(6px - (var(--touch-target) - 18px * var(--ui-scale)) / 2);
		}
		.grip {
			position: absolute;
			top: calc(var(--space-1) + env(safe-area-inset-top));
			left: 50%;
			display: block;
			width: 36px;
			height: 4px;
			border-radius: var(--radius-full);
			background: var(--border-strong);
			transform: translateX(-50%);
		}
	}
</style>
