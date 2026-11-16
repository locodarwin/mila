<script lang="ts">
	// A task's Time (docs/PLAN.md section 3): its own estimate, the time
	// logged on it and what is left, its subtasks' added in, the person's
	// timer, Log time, and every entry, which its person (or an admin)
	// changes in place.
	import Pencil from '@lucide/svelte/icons/pencil';
	import Play from '@lucide/svelte/icons/play';
	import Plus from '@lucide/svelte/icons/plus';
	import Square from '@lucide/svelte/icons/square';
	import Button from '../Button.svelte';
	import Stopwatch from '../Stopwatch.svelte';
	import { tick } from 'svelte';
	import Avatar from './Avatar.svelte';
	import { announce } from '../../announce.svelte';
	import { formatDate } from '../../dates';
	import {
		MAX_ENTRY,
		MAX_ESTIMATE,
		formatDuration,
		parseDuration,
		remaining,
		sayDuration
	} from '../../duration';
	import { canEditOthersTime } from '../../roles';
	import { session } from '../../session.svelte';
	import { localToday } from '../../sync/data';
	import {
		deleteTime,
		editTime,
		logTime,
		setEstimate,
		startTimer,
		stopTimer
	} from '../../sync/time';
	import { engine } from '../../sync/engine.svelte';
	import type { Task, TimeEntry } from '../../sync/entities';
	import type { Replica } from '../../sync/replica.svelte';
	import { toasts } from '../../toasts.svelte';
	import { ui } from '../../ui.svelte';
	import { uuid } from '../../uuid';

	let { ws, replica, task }: { ws: string; replica: Replica; task: Task } = $props();
	const id = `time-${uuid()}`;
	const me = $derived(session.user?.id ?? '');
	const role = $derived(session.workspaces.find((w) => w.id === ws)?.role ?? 'member');
	const today = localToday();

	const entries = $derived(replica.timeEntries(task.id));
	const logged = $derived(replica.logged(task.id));
	const left = $derived(remaining(task.estimateMinutes, logged));
	const hasSubtasks = $derived(replica.subtasks(task.id).length > 0);
	const all = $derived(hasSubtasks ? replica.withSubtasks(task.id) : null);
	const timer = $derived(me ? replica.runningTimer(me) : undefined);
	const mineHere = $derived(timer?.taskId === task.id ? timer : undefined);
	const share = $derived(
		task.estimateMinutes ? Math.min(100, (logged / task.estimateMinutes) * 100) : 0
	);

	// The estimate as typed, kept until it is read.
	let estimate = $derived(
		task.estimateMinutes === null ? '' : formatDuration(task.estimateMinutes)
	);
	let estimateError = $state('');

	function saveEstimate() {
		const text = estimate.trim();
		const minutes = text ? parseDuration(text) : null;
		if (text && minutes === null) {
			estimateError = 'Write the estimate as 1h 30m, 90m, 1.5h or 1:30.';
			return;
		}
		if (minutes !== null && minutes > MAX_ESTIMATE) {
			estimateError = 'An estimate is at most 999 hours.';
			return;
		}
		estimateError = '';
		if (minutes !== task.estimateMinutes) void setEstimate(ws, task.id, minutes);
		estimate = minutes === null ? '' : formatDuration(minutes);
	}

	// Log time, or an entry being changed in its place.
	let logging = $state(false);
	let editing = $state<string | null>(null);
	let spent = $state('');
	let day = $state(today);
	let note = $state('');
	let error = $state('');
	let showAll = $state(false);
	const SHOWN = 10;

	let section = $state<HTMLElement>();

	/** Focus on the form's first field once it is drawn. */
	async function focusForm() {
		await tick();
		const input = section?.querySelector<HTMLInputElement>('.entry-form input');
		input?.scrollIntoView({ block: 'nearest' });
		input?.focus();
	}

	function openLog() {
		editing = null;
		spent = '';
		day = localToday();
		note = '';
		error = '';
		logging = !logging;
		if (logging) void focusForm();
	}

	// Log time on the task, asked for from the palette: the form opens and takes focus.
	$effect(() => {
		if (ui.logTime !== task.id) return;
		ui.logTime = null;
		if (!logging) openLog();
		else void focusForm();
	});

	function openEdit(e: TimeEntry) {
		logging = false;
		editing = e.id;
		spent = formatDuration(e.minutes);
		day = e.day;
		note = e.note;
		error = '';
		void focusForm();
	}

	/** Closes the form, focus going back to the entry's pencil, or Log time. */
	async function close() {
		const was = editing;
		logging = false;
		editing = null;
		error = '';
		await tick();
		(
			(was && section?.querySelector<HTMLElement>(`[data-entry="${was}"]`)) ||
			section?.querySelector<HTMLElement>('[data-log-time]')
		)?.focus();
	}

	/** The minutes typed, or null with the error said. */
	function read(): number | null {
		const minutes = parseDuration(spent);
		if (minutes === null) error = 'Write the time as 1h 30m, 90m, 1.5h or 1:30.';
		else if (minutes < 1) error = 'Log at least a minute.';
		else if (minutes > MAX_ENTRY) error = 'One entry is at most 24 hours. Log the rest as another.';
		else if (!day || day > localToday()) error = 'Time can be logged for today or before.';
		else return minutes;
		return null;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		const minutes = read();
		if (minutes === null) return;
		if (editing) {
			await editTime(ws, editing, { minutes, day, note });
			announce(`Changed to ${sayDuration(minutes)}`);
		} else {
			await logTime(ws, task.id, { minutes, day, note });
			announce(`Logged ${sayDuration(minutes)}`);
		}
		void close();
	}

	function remove(entry: TimeEntry) {
		const { undo } = engine.track(() => deleteTime(ws, entry.id));
		toasts.show(`Deleted ${formatDuration(entry.minutes)} of time`, { onundo: () => void undo() });
		void close();
	}

	async function toggleTimer() {
		if (mineHere) {
			const minutes = await stopTimer(ws, mineHere);
			toasts.show(
				minutes
					? `Logged ${formatDuration(minutes)} on ${task.title}`
					: 'Stopped before a minute, so nothing was logged'
			);
			return;
		}
		// The timer stopped: on another task, or on a project itself.
		const before = !timer
			? null
			: timer.taskId
				? replica.task(timer.taskId)?.title
				: replica.project(timer.projectId ?? '')?.name;
		await startTimer(ws, task.id);
		announce(before ? `Stopped the timer on ${before} and started one here` : 'Started a timer');
	}

	const canEdit = (e: TimeEntry) => e.userId === me || canEditOthersTime(role);
	const shown = $derived(showAll ? entries : entries.slice(0, SHOWN));
	const days = $derived.by(() => {
		const out: { day: string; items: TimeEntry[] }[] = [];
		for (const e of shown) {
			const last = out.at(-1);
			if (last?.day === e.day) last.items.push(e);
			else out.push({ day: e.day, items: [e] });
		}
		return out;
	});
	const dayName = (d: string) => (d === today ? 'Today' : formatDate(d, { weekday: 'short' }));
</script>

<div class="time" bind:this={section}>
	<div class="figs">
		<label class="fig">
			<span class="k">Estimate</span>
			<input
				class="tabular"
				placeholder="None"
				bind:value={estimate}
				aria-invalid={estimateError ? 'true' : undefined}
				aria-describedby={estimateError ? `${id}-err` : undefined}
				onchange={saveEstimate}
				onkeydown={(e) => {
					if (e.key === 'Enter') saveEstimate();
				}}
			/>
		</label>
		<div class="fig">
			<span class="k">Logged</span>
			<span class="v tabular">{formatDuration(logged)}</span>
		</div>
		<div class="fig">
			<span class="k">{left?.over ? 'Over' : 'Remaining'}</span>
			<span class="v tabular" class:over={left?.over}
				>{left ? formatDuration(left.over || left.left) : 'None'}</span
			>
		</div>
	</div>
	{#if estimateError}<p class="error" id="{id}-err" role="alert">{estimateError}</p>{/if}
	{#if task.estimateMinutes}
		<div class="meter" aria-hidden="true">
			<span class:over={left?.over} style:width={`${share}%`}></span>
		</div>
	{/if}
	{#if all && (all.estimated || all.logged)}
		<p class="subs">
			With subtasks: <strong class="tabular">{formatDuration(all.logged)}</strong> logged
			{#if all.estimated}of <strong class="tabular">{formatDuration(all.estimate)}</strong> estimated{/if}.
		</p>
	{/if}
	<div class="actions">
		<Button size="sm" variant={mineHere ? 'primary' : undefined} onclick={() => void toggleTimer()}>
			{#if mineHere}<Square size={13} />Stop the timer
				<Stopwatch since={mineHere.startedAt} />{:else}<Play size={13} />Start a timer{/if}
		</Button>
		<Button
			title="Add time you have already spent on this task"
			size="sm"
			aria-expanded={logging}
			data-log-time
			onclick={openLog}><Plus size={13} />Log time</Button
		>
	</div>

	{#snippet form(label: string)}
		<!-- Escape from any of its fields closes it. -->
		<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
		<form
			class="entry-form"
			aria-label={label}
			onsubmit={submit}
			novalidate
			onkeydown={(e) => {
				// Escape closes the form, not the task.
				if (e.key !== 'Escape') return;
				e.preventDefault();
				void close();
			}}
		>
			<label>
				<span>Time spent</span>
				<input
					class="tabular"
					bind:value={spent}
					placeholder="1h 30m"
					autocomplete="off"
					aria-describedby="{id}-hint{error ? ` ${id}-form-err` : ''}"
					aria-invalid={error ? 'true' : undefined}
				/>
			</label>
			<label>
				<span>Day</span>
				<input type="date" bind:value={day} max={today} />
			</label>
			<label class="note">
				<span>Note</span>
				<input bind:value={note} maxlength="500" autocomplete="off" />
			</label>
			{#if error}<p class="error wide" id="{id}-form-err" role="alert">{error}</p>{/if}
			<div class="foot wide">
				<span class="hint" id="{id}-hint">Like 1h 30m, 90m, 1.5h or 1:30</span>
				{#if editing}
					{@const entry = entries.find((x) => x.id === editing)}
					{#if entry}<Button size="sm" variant="danger" onclick={() => remove(entry)}>Delete</Button
						>{/if}
				{/if}
				<Button size="sm" onclick={close}>Cancel</Button>
				<Button size="sm" type="submit" variant="primary">{editing ? 'Save' : 'Log it'}</Button>
			</div>
		</form>
	{/snippet}

	{#if logging}{@render form('Log time')}{/if}

	{#if entries.length}
		<ul class="entries" aria-label="Time logged">
			{#each days as d (d.day)}
				<li class="day">{dayName(d.day)}</li>
				{#each d.items as e (e.id)}
					{@const who = session.personOf(ws, e.userId)}
					<li class="entry">
						{#if editing === e.id}
							{@render form(`Change ${who.name}'s time`)}
						{:else}
							<Avatar name={who.name} src={who.avatar} size={22} />
							<span class="who"
								>{who.name}{#if e.note}<span class="note-text">&nbsp;· {e.note}</span
									>{/if}{#if e.startedAt && !e.running}<span class="note-text">&nbsp;· timer</span
									>{/if}</span
							>
							{#if e.running}
								<span class="dur running"><Stopwatch since={e.startedAt} /> running</span>
							{:else}
								<span class="dur tabular">{formatDuration(e.minutes)}</span>
							{/if}
							{#if canEdit(e) && !e.running}
								<button
									class="edit"
									type="button"
									aria-label={`Change ${formatDuration(e.minutes)} logged by ${who.name}`}
									title="Change or delete"
									data-entry={e.id}
									onclick={() => openEdit(e)}><Pencil size={13} /></button
								>
							{:else}<span></span>{/if}
						{/if}
					</li>
				{/each}
			{/each}
		</ul>
		{#if entries.length > SHOWN}
			<button class="more" type="button" onclick={() => (showAll = !showAll)}
				>{showAll ? 'Show fewer' : `Show all ${entries.length}`}</button
			>
		{/if}
	{/if}
</div>

<style>
	.time {
		display: grid;
		gap: var(--space-2);
	}
	.figs {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: var(--space-2);
	}
	.fig {
		display: grid;
		gap: 2px;
		padding: var(--space-2);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
	}
	.k {
		color: var(--text-muted);
		font-size: var(--text-xs);
		font-weight: 550;
	}
	.v,
	.fig input {
		font-size: var(--text-md);
		font-weight: 650;
	}
	.fig input {
		width: 100%;
		min-width: 0;
		min-height: max(var(--tap-min), 26px);
		padding: 0 var(--space-1);
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-sm);
		background: var(--surface-0);
	}
	.over {
		color: var(--danger);
	}
	.meter {
		display: flex;
		height: 6px;
		overflow: hidden;
		border-radius: var(--radius-full);
		background: var(--surface-2);
	}
	.meter span {
		background: var(--accent);
	}
	.meter span.over {
		background: var(--danger);
	}
	.subs {
		margin: 0;
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	.subs strong {
		color: var(--text-primary);
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.entry-form {
		display: grid;
		grid-template-columns: minmax(80px, 1fr) minmax(120px, 1.2fr) minmax(0, 2fr);
		gap: var(--space-2);
		width: 100%;
		padding: var(--space-2);
		border-radius: var(--radius-md);
		background: var(--surface-1);
	}
	.entry-form label {
		display: grid;
		gap: 2px;
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-weight: 550;
	}
	.entry-form input {
		width: 100%;
		min-width: 0;
		min-height: max(var(--tap-min), 28px);
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-sm);
		background: var(--surface-0);
		color: var(--text-primary);
		font-size: var(--text-sm);
		font-weight: 400;
	}
	.wide {
		grid-column: 1 / -1;
	}
	.foot {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
	.hint {
		margin-right: auto;
		color: var(--text-muted);
		font-size: var(--text-xs);
	}
	.error {
		margin: 0;
		color: var(--danger);
		font-size: var(--text-xs);
	}
	@media (max-width: 480px) {
		.entry-form {
			grid-template-columns: 1fr 1fr;
		}
		.entry-form .note {
			grid-column: 1 / -1;
		}
	}
	.entries {
		display: grid;
		gap: 2px;
		margin: 0;
		padding: 0;
		list-style: none;
		font-size: var(--text-sm);
	}
	.day {
		padding: var(--space-1) 0 0;
		color: var(--text-muted);
		font-size: var(--text-xs);
		font-weight: 650;
	}
	.entry {
		display: grid;
		grid-template-columns: 22px minmax(0, 1fr) auto auto;
		align-items: center;
		gap: var(--space-2);
		min-height: 30px;
	}
	.entry:has(form) {
		display: block;
	}
	.who {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.note-text {
		color: var(--text-secondary);
	}
	.dur {
		font-weight: 600;
	}
	.running {
		color: var(--accent-text);
	}
	.edit,
	.more {
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-muted);
		cursor: pointer;
	}
	.edit {
		display: grid;
		place-items: center;
		width: max(var(--tap-min), 26px);
		height: max(var(--tap-min), 26px);
	}
	.edit:hover,
	.more:hover {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	.more {
		justify-self: start;
		padding: var(--space-1) var(--space-2);
		font-size: var(--text-xs);
	}
</style>
