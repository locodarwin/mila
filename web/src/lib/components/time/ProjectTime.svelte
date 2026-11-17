<script lang="ts">
	// A project's Time tab: this month's time on it
	// in all, on its tasks and on the project itself; the person's timer for
	// the project and Log time, which go to the project itself; and every
	// entry, both kinds, newest first. An entry on the project itself is
	// changed here by its person (or an admin); one on a task opens the task.
	// Loaded only when the tab is shown.
	import Pencil from '@lucide/svelte/icons/pencil';
	import Play from '@lucide/svelte/icons/play';
	import Plus from '@lucide/svelte/icons/plus';
	import Square from '@lucide/svelte/icons/square';
	import { tick } from 'svelte';
	import Button from '../Button.svelte';
	import Stopwatch from '../Stopwatch.svelte';
	import Avatar from '../task/Avatar.svelte';
	import { announce } from '../../announce.svelte';
	import { formatDate } from '../../dates';
	import { MAX_ENTRY, formatDuration, parseDuration, sayDuration } from '../../duration';
	import { Growing } from '../../growing.svelte';
	import { canEditOthersTime } from '../../roles';
	import { session } from '../../session.svelte';
	import { localToday } from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import type { TimeEntry } from '../../sync/entities';
	import {
		deleteTime,
		editTime,
		logProjectTime,
		startProjectTimer,
		stopTimer
	} from '../../sync/time';
	import { openTask } from '../../taskLink';
	import { toasts } from '../../toasts.svelte';
	import { uuid } from '../../uuid';

	let {
		ws,
		projectId,
		/** Opens Log time at once, focused: the palette's and the menu's Log time. */
		logNow = false
	}: { ws: string; projectId: string; logNow?: boolean } = $props();

	const id = `ptime-${uuid()}`;
	const replica = $derived(engine.replica(ws));
	const project = $derived(replica?.project(projectId));
	const me = $derived(session.user?.id ?? '');
	const role = $derived(session.workspaces.find((w) => w.id === ws)?.role ?? 'member');
	const open = $derived(!!project && !project.archivedAt);
	const today = localToday();
	const monthStart = `${today.slice(0, 7)}-01`;

	/** Both kinds of time on the project: on its live tasks, and on itself. */
	const entries = $derived.by((): TimeEntry[] => {
		const r = replica;
		if (!r) return [];
		return r
			.allTimeEntries()
			.filter((e) =>
				e.taskId === null ? e.projectId === projectId : r.task(e.taskId)?.projectId === projectId
			)
			.sort(
				(a, b) =>
					b.day.localeCompare(a.day) ||
					(b.startedAt ?? '').localeCompare(a.startedAt ?? '') ||
					a.id.localeCompare(b.id)
			);
	});
	const month = $derived.by(() => {
		let tasks = 0;
		let itself = 0;
		for (const e of entries) {
			if (e.day < monthStart || e.day > today) continue;
			if (e.taskId === null) itself += e.minutes;
			else tasks += e.minutes;
		}
		return { tasks, itself, all: tasks + itself };
	});

	const timer = $derived(me ? replica?.runningTimer(me) : undefined);
	const mineHere = $derived(timer?.projectId === projectId ? timer : undefined);

	// Log time, or an entry on the project itself being changed in its place.
	let logging = $state(false);
	let editing = $state<string | null>(null);
	let spent = $state('');
	let day = $state(today);
	let note = $state('');
	let error = $state('');
	let section = $state<HTMLElement>();

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

	$effect(() => {
		if (logNow && open && !logging) openLog();
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
			await editTime(ws, editing, { minutes, day, note }, 'project_time_entry');
			announce(`Changed to ${sayDuration(minutes)}`);
		} else {
			await logProjectTime(ws, projectId, { minutes, day, note });
			announce(`Logged ${sayDuration(minutes)} to ${project?.name ?? 'the project'}`);
		}
		void close();
	}

	function remove(entry: TimeEntry) {
		const { undo } = engine.track(() => deleteTime(ws, entry.id, 'project_time_entry'));
		toasts.show(`Deleted ${formatDuration(entry.minutes)} of time`, { onundo: () => void undo() });
		void close();
	}

	async function toggleTimer() {
		if (mineHere) {
			const minutes = await stopTimer(ws, mineHere);
			toasts.show(
				minutes
					? `Logged ${formatDuration(minutes)} to ${project?.name ?? 'the project'}`
					: 'Stopped before a minute, so nothing was logged'
			);
			return;
		}
		// The timer stopped: on a task, or on another project.
		const before = !timer
			? null
			: timer.taskId
				? replica?.task(timer.taskId)?.title
				: replica?.project(timer.projectId ?? '')?.name;
		await startProjectTimer(ws, projectId);
		announce(before ? `Stopped the timer on ${before} and started one here` : 'Started a timer');
	}

	const canEdit = (e: TimeEntry) =>
		open && e.taskId === null && (e.userId === me || canEditOthersTime(role));
	const growing = new Growing(() => entries.length);
	const shown = $derived(entries.slice(0, growing.limit));
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

<div class="project-time" bind:this={section}>
	<section aria-labelledby="{id}-month">
		<h2 id="{id}-month">This month</h2>
		<div class="tiles">
			<div class="tile">
				<span class="v tabular">{formatDuration(month.all)}</span>
				<span class="k">In all</span>
			</div>
			<div class="tile">
				<span class="v tabular">{formatDuration(month.tasks)}</span>
				<span class="k">On its tasks</span>
			</div>
			<div class="tile">
				<span class="v tabular">{formatDuration(month.itself)}</span>
				<span class="k">On the project itself</span>
			</div>
		</div>
	</section>

	{#if open}
		<div class="actions">
			<Button
				size="sm"
				variant={mineHere ? 'primary' : undefined}
				title={mineHere ? undefined : 'Time the project itself, not one of its tasks'}
				onclick={() => void toggleTimer()}
			>
				{#if mineHere}<Square size={13} />Stop the timer
					<Stopwatch since={mineHere.startedAt} />{:else}<Play size={13} />Start a timer{/if}
			</Button>
			<Button
				title="Add time spent on the project itself, not on one of its tasks"
				size="sm"
				aria-expanded={logging}
				data-log-time
				onclick={openLog}><Plus size={13} />Log time</Button
			>
		</div>
	{/if}

	{#snippet form(label: string)}
		<!-- Escape from any of its fields closes it. -->
		<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
		<form
			class="entry-form"
			aria-label={label}
			onsubmit={submit}
			novalidate
			onkeydown={(e) => {
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
				<input
					bind:value={note}
					maxlength="500"
					autocomplete="off"
					placeholder="A meeting, planning, travel"
				/>
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

	{#if logging}{@render form(`Log time to ${project?.name ?? 'the project'}`)}{/if}

	<section aria-labelledby="{id}-all">
		<h2 id="{id}-all">Time logged</h2>
		{#if entries.length}
			<ul class="entries" aria-labelledby="{id}-all">
				{#each days as d (d.day)}
					<li class="day">{dayName(d.day)}</li>
					{#each d.items as e (e.id)}
						{@const who = session.personOf(ws, e.userId)}
						{@const task = e.taskId ? replica?.task(e.taskId) : undefined}
						<li class="entry">
							{#if editing === e.id}
								{@render form(`Change ${who.name}'s time`)}
							{:else}
								<Avatar name={who.name} src={who.avatar} size={22} />
								<span class="who">{who.name}</span>
								<span class="on">
									{#if task}
										<button
											type="button"
											class="task"
											title="Open the task"
											onclick={() => void openTask(task.id)}>{task.title || 'Untitled task'}</button
										>
									{:else}
										<span class="itself">The project itself</span>
									{/if}
									{#if e.note}<span class="note-text">{e.note}</span>{/if}
								</span>
								{#if e.running}
									<span class="dur running"><Stopwatch since={e.startedAt} /> running</span>
								{:else}
									<span class="dur tabular">{formatDuration(e.minutes)}</span>
								{/if}
								{#if canEdit(e) && !e.running}
									<Button
										size="sm"
										variant="ghost"
										data-entry={e.id}
										aria-label={`Change ${who.name}'s ${formatDuration(e.minutes)} on ${dayName(d.day)}`}
										title="Change this time"
										onclick={() => openEdit(e)}><Pencil size={13} /></Button
									>
								{/if}
							{/if}
						</li>
					{/each}
				{/each}
			</ul>
			<div use:growing.more aria-hidden="true"></div>
		{:else}
			<p class="empty">
				No time logged yet, on the project or its tasks.{#if open}
					Log time here for work on the project itself, such as a meeting or planning.{/if}
			</p>
		{/if}
	</section>
</div>

<style>
	.project-time {
		display: grid;
		gap: var(--space-5);
		max-width: 760px;
		padding: var(--space-4) var(--space-5) var(--space-6);
	}
	section {
		display: grid;
		gap: var(--space-3);
	}
	h2 {
		font-size: var(--text-md);
	}
	.tiles {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
		gap: var(--space-2);
	}
	.tile {
		display: grid;
		gap: 2px;
		padding: var(--space-2) var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
	}
	.tile .v {
		font-size: var(--text-xl);
		font-weight: 650;
	}
	.tile .k {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.tabular {
		font-variant-numeric: tabular-nums;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.entry-form {
		display: grid;
		grid-template-columns: minmax(0, 8rem) minmax(0, 10rem) minmax(0, 1fr);
		gap: var(--space-2);
		padding: var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-1);
	}
	.entry-form label {
		display: grid;
		gap: 2px;
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.entry-form input {
		min-width: 0;
		height: calc(30px * var(--control-scale));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		color: var(--text-primary);
		font-size: var(--text-sm);
	}
	.wide {
		grid-column: 1 / -1;
	}
	.foot {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: flex-end;
		gap: var(--space-2);
	}
	.hint {
		margin-right: auto;
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.error {
		color: var(--danger);
		font-size: var(--text-sm);
	}
	.entries {
		display: grid;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.day {
		padding: var(--space-3) 0 var(--space-1);
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.entry {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		min-height: var(--row-height);
		border-top: var(--border-width) solid var(--border);
	}
	.who {
		flex: none;
		min-width: 6rem;
		font-size: var(--text-sm);
	}
	.on {
		display: flex;
		flex: 1;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 0 var(--space-2);
		min-width: 10rem;
		font-size: var(--text-sm);
	}
	.task {
		padding: 0;
		border: none;
		background: none;
		color: var(--accent-text);
		font: inherit;
		text-align: left;
		cursor: pointer;
	}
	.task:hover {
		text-decoration: underline;
	}
	.itself {
		color: var(--text-primary);
		font-weight: 550;
	}
	.note-text {
		color: var(--text-secondary);
	}
	.dur {
		font-size: var(--text-sm);
	}
	.running {
		color: var(--accent-text);
	}
	.empty {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	@media (max-width: 720px) {
		.project-time {
			padding-inline: var(--space-4);
		}
		.entry-form {
			grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
		}
		.entry-form .note {
			grid-column: 1 / -1;
		}
	}
</style>
