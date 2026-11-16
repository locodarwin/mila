<script lang="ts">
	import { menu, taskActions } from '../../contextMenu.svelte';
	import ChevronLeft from '@lucide/svelte/icons/chevron-left';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import { announce } from '../../announce.svelte';
	import { liftDragImage } from '../../drag';
	import { uuid } from '../../uuid';
	import { localToday, setTaskFields } from '../../sync/data';
	import type { Task } from '../../sync/entities';
	import { openTask } from '../../taskLink';
	import type { CommandId } from '../../keys/commands';
	import { keymap } from '../../keys/keymap.svelte';
	import PriorityIcon from '../PriorityIcon.svelte';
	import RepeatIcon from '../RepeatIcon.svelte';
	import { repeatLabel, tryParseRule, upcoming } from '../../recurrence';
	import {
		daysIntoWeek,
		formatDate,
		formatMonth,
		formatTime,
		weekDays,
		weekdayName
	} from '../../dates';

	let { ws, tasks }: { ws: string; tasks: Task[] } = $props();

	const today = localToday();
	let month = $state(today.slice(0, 7)); // YYYY-MM

	function ymd(y: number, m: number, d: number): string {
		const date = new Date(y, m, d);
		return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
	}

	function shift(days: number, from: string): string {
		const [y, m, d] = from.split('-').map(Number);
		return ymd(y, m - 1, d + days);
	}

	// Six weeks from the first day of the week the 1st falls in.
	const days = $derived.by(() => {
		const [y, m] = month.split('-').map(Number);
		const lead = daysIntoWeek(new Date(y, m - 1, 1));
		return Array.from({ length: 42 }, (_, i) => ymd(y, m - 1, 1 - lead + i));
	});
	const byDay = $derived.by(() => {
		const map: Record<string, Task[]> = {};
		for (const t of tasks) {
			if (!t.dueDate) continue;
			(map[t.dueDate] ??= []).push(t);
		}
		for (const list of Object.values(map)) {
			list.sort(
				(a, b) =>
					(a.dueTime ?? '99').localeCompare(b.dueTime ?? '99') || a.title.localeCompare(b.title)
			);
		}
		return map;
	});
	// Where repeating tasks fall next: faded entries that open the real
	// task. Only schedule-mode series are known ahead, and none before
	// today, since a late completion skips those dates.
	const ghosts = $derived.by(() => {
		const map: Record<string, Task[]> = {};
		const from = days[0] > today ? days[0] : today;
		for (const t of tasks) {
			const rule = tryParseRule(t.recurrence);
			if (!rule || !t.dueDate || t.completedAt || t.recurrenceMode !== 'schedule') continue;
			for (const day of upcoming(rule, t.dueDate, from, days[41])) (map[day] ??= []).push(t);
		}
		return map;
	});
	const repeats = (t: Task) => repeatLabel(t.recurrence, t.recurrenceMode, t.dueDate);
	// Named after what it shows, so the visible text is part of the name.
	const ghostLabel = (t: Task, day: string) =>
		`${t.dueTime ? formatTime(t.dueTime) + ' ' : ''}${t.title}, repeats on ${formatDate(day, {
			month: 'long',
			year: false
		})}`;
	const title = $derived(formatMonth(month));
	const weekdays = $derived(weekDays().map((d) => weekdayName(d)));

	function step(n: number) {
		const [y, m] = month.split('-').map(Number);
		month = ymd(y, m - 1 + n, 1).slice(0, 7);
	}

	let dragging = $state<string | null>(null);
	let over = $state<string | null>(null);

	function reschedule(task: Task, day: string): Promise<unknown> {
		return task.dueDate !== day ? setTaskFields(ws, task.id, { due_date: day }) : Promise.resolve();
	}

	// The commands that move an entry, and by how many days.
	const MOVES: Partial<Record<CommandId, number>> = {
		'calendar.earlier-day': -1,
		'calendar.later-day': 1,
		'calendar.earlier-week': -7,
		'calendar.later-week': 7
	};
	const MOVE_IDS = Object.keys(MOVES) as CommandId[];

	function chipKeys(e: KeyboardEvent, task: Task) {
		const cmd = keymap.match('calendar', e);
		const by = cmd && MOVES[cmd];
		if (!by) return;
		// Taken even when there is nothing to move, so a key kept on both
		// the calendar and anywhere never does both.
		e.preventDefault();
		if (!task.dueDate) return;
		const day = shift(by, task.dueDate);
		if (day.slice(0, 7) !== month) month = day.slice(0, 7);
		// The entry is drawn anew in its new day once the edit lands; focus
		// follows it there.
		void reschedule(task, day).then(() => {
			announce(
				`Moved ${task.title} to ${formatDate(day, { weekday: 'long', month: 'long', year: false })}`
			);
			requestAnimationFrame(() =>
				document.querySelector<HTMLElement>(`[data-chip="${CSS.escape(task.id)}"]`)?.focus()
			);
		});
	}

	const helpId = `calendar-help-${uuid()}`;

	// The agenda (phones): the month's days that have something on them.
	const agenda = $derived(
		days.filter((d) => d.startsWith(month) && (byDay[d]?.length || ghosts[d]?.length))
	);
</script>

<div class="calendar">
	<header>
		<h2>{title}</h2>
		<button type="button" class="icon" aria-label="Previous month" onclick={() => step(-1)}
			><ChevronLeft size={18} /></button
		>
		<button type="button" class="go-today" onclick={() => (month = today.slice(0, 7))}>Today</button
		>
		<button type="button" class="icon" aria-label="Next month" onclick={() => step(1)}
			><ChevronRight size={18} /></button
		>
	</header>

	<p id={helpId} hidden>{keymap.describe(MOVE_IDS, 'move it to another day')}</p>
	<div class="grid" aria-label={title} role="group">
		<div class="weekdays" aria-hidden="true">
			{#each weekdays as w (w)}<span>{w}</span>{/each}
		</div>
		<div class="days">
			{#each days as day (day)}
				<div
					class="day"
					class:outside={!day.startsWith(month)}
					class:is-today={day === today}
					class:over={over === day}
					role="group"
					aria-current={day === today ? 'date' : undefined}
					aria-label={formatDate(day, { weekday: 'long', month: 'long', year: false })}
					ondragover={(e) => {
						if (dragging) {
							e.preventDefault();
							if (over !== day) over = day;
						}
					}}
					ondragleave={() => over === day && (over = null)}
					ondrop={(e) => {
						e.preventDefault();
						const t = tasks.find((x) => x.id === dragging);
						if (t) void reschedule(t, day);
						dragging = over = null;
					}}
				>
					<span class="num">{Number(day.slice(8))}</span>
					{#each byDay[day] ?? [] as task (task.id)}
						<button
							type="button"
							class="chip"
							class:done={task.completedAt}
							class:lifted={dragging === task.id}
							draggable="true"
							data-chip={task.id}
							use:menu={taskActions(ws, task.id)}
							title={task.title}
							aria-describedby={helpId}
							onclick={() => openTask(task.id)}
							onkeydown={(e) => chipKeys(e, task)}
							ondragstart={(e) => {
								dragging = task.id;
								e.dataTransfer?.setData('text/plain', task.title);
								liftDragImage(e, e.currentTarget as HTMLElement);
							}}
							ondragend={() => (dragging = over = null)}
						>
							{#if task.priority !== 'none'}<PriorityIcon priority={task.priority} size={12} />{/if}
							{#if task.dueTime}<span class="time tabular">{formatTime(task.dueTime)}</span>{/if}
							<span class="name">{task.title}</span>
							{#if repeats(task)}<RepeatIcon label={repeats(task) ?? ''} />{/if}
						</button>
					{/each}
					{#each ghosts[day] ?? [] as task (task.id)}
						<button
							type="button"
							class="chip ghost"
							aria-label={ghostLabel(task, day)}
							title={ghostLabel(task, day)}
							onclick={() => openTask(task.id)}
						>
							{#if task.dueTime}<span class="time tabular">{formatTime(task.dueTime)}</span>{/if}
							<span class="name">{task.title}</span>
						</button>
					{/each}
				</div>
			{/each}
		</div>
	</div>

	<ol class="agenda" aria-label={`Agenda for ${title}`}>
		{#each agenda as day (day)}
			<li>
				<h3 class:is-today={day === today} aria-current={day === today ? 'date' : undefined}>
					{formatDate(day, { weekday: 'long', year: false })}
				</h3>
				{#each byDay[day] ?? [] as task (task.id)}
					<button
						type="button"
						class="chip"
						class:done={task.completedAt}
						onclick={() => openTask(task.id)}
					>
						{#if task.dueTime}<span class="time tabular">{formatTime(task.dueTime)}</span>{/if}
						<span class="name">{task.title}</span>
						{#if repeats(task)}<RepeatIcon label={repeats(task) ?? ''} />{/if}
					</button>
				{/each}
				{#each ghosts[day] ?? [] as task (task.id)}
					<button
						type="button"
						class="chip ghost"
						aria-label={ghostLabel(task, day)}
						onclick={() => openTask(task.id)}
					>
						{#if task.dueTime}<span class="time tabular">{formatTime(task.dueTime)}</span>{/if}
						<span class="name">{task.title}</span>
					</button>
				{/each}
			</li>
		{:else}
			<li class="none">Nothing is due this month.</li>
		{/each}
	</ol>
</div>

<style>
	.calendar {
		display: flex;
		flex-direction: column;
		height: 100%;
		padding: var(--space-3) var(--space-4) var(--space-4);
	}
	header {
		display: flex;
		align-items: center;
		gap: var(--space-1);
		padding-bottom: var(--space-3);
	}
	h2 {
		flex: 1;
		font-size: var(--text-lg);
	}
	.icon,
	.go-today {
		display: grid;
		place-items: center;
		height: 30px;
		min-width: max(var(--tap-min), 30px);
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		cursor: pointer;
	}
	.icon:hover,
	.go-today:hover {
		background: var(--surface-2);
	}
	.grid {
		display: flex;
		flex: 1;
		flex-direction: column;
		min-height: 0;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		overflow: hidden;
	}
	.weekdays,
	.days {
		display: grid;
		grid-template-columns: repeat(7, minmax(0, 1fr));
	}
	.weekdays span {
		padding: var(--space-1) var(--space-2);
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
		border-bottom: var(--border-width) solid var(--border);
	}
	/* The weeks share the height there is. Without min-height: 0 the grid's
	   own height comes from its busiest day, and every week grows to that. */
	.days {
		flex: 1;
		min-height: 0;
		grid-auto-rows: minmax(84px, 1fr);
	}
	.day {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
		padding: var(--space-1);
		border-right: 1px solid var(--border);
		border-bottom: 1px solid var(--border);
		/* A day with more than its square holds scrolls inside it. */
		overflow: hidden auto;
		overscroll-behavior: contain;
		transition:
			background-color var(--dur-micro) var(--ease),
			box-shadow var(--dur-micro) var(--ease);
	}
	.day:nth-child(7n) {
		border-right: none;
	}
	.day > * {
		flex: none;
	}
	.day.outside {
		background: var(--surface-1);
	}
	.day.outside .num {
		color: var(--text-muted);
	}
	.day.over {
		background: var(--accent-soft);
		box-shadow: inset 0 0 0 2px var(--accent);
	}
	.chip.lifted {
		opacity: 0.45;
	}
	.num {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.day.is-today .num {
		display: inline-grid;
		place-items: center;
		width: 20px;
		height: 20px;
		border-radius: 50%;
		background: var(--calendar-today-bg);
		color: var(--accent-contrast);
		font-weight: 700;
	}
	/* 24px tall at least, so stacked entries stay apart enough to hit
	   (WCAG 2.5.8). */
	.chip {
		display: flex;
		align-items: center;
		gap: 4px;
		min-width: 0;
		min-height: max(var(--tap-min), 24px);
		padding: 2px 6px;
		border: none;
		border-radius: var(--radius-sm);
		background: var(--accent-soft);
		font-size: var(--text-xs);
		text-align: left;
		cursor: pointer;
	}
	.chip:hover {
		background: color-mix(in oklch, var(--accent) 22%, var(--surface-0));
	}
	/* A repeat still to come: an outline of a chip, in quieter text. */
	.chip.ghost {
		border: 1px dashed var(--border-strong);
		background: none;
		color: var(--text-secondary);
	}
	.chip.ghost:hover {
		background: var(--surface-2);
	}
	.chip.done .name {
		color: var(--text-muted);
		text-decoration: line-through;
	}
	.name {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.time {
		color: var(--text-secondary);
	}
	.agenda {
		display: none;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.agenda li {
		display: grid;
		gap: var(--space-1);
		padding: var(--space-3) 0;
		border-bottom: 1px solid var(--border);
	}
	.agenda h3 {
		font-family: var(--font-ui);
		font-size: var(--text-sm);
	}
	.agenda h3.is-today {
		color: var(--accent-text);
	}
	.agenda .chip {
		padding: var(--space-2);
		font-size: var(--text-md);
	}
	.none {
		color: var(--text-secondary);
	}
	@media (max-width: 720px) {
		.grid {
			display: none;
		}
		.agenda {
			display: block;
		}
	}
</style>
