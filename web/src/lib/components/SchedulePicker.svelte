<script lang="ts">
	import { tick } from 'svelte';
	import CalendarCheck from '@lucide/svelte/icons/calendar-check';
	import CalendarClock from '@lucide/svelte/icons/calendar-clock';
	import CalendarX from '@lucide/svelte/icons/calendar-x';
	import Check from '@lucide/svelte/icons/check';
	import Sun from '@lucide/svelte/icons/sun';
	import Sunrise from '@lucide/svelte/icons/sunrise';
	import { formatDate } from '../dates';
	import { placeNear } from '../popover';
	import { scheduleChoices, scheduler, type ScheduleChoice } from '../schedule.svelte';
	import { session } from '../session.svelte';
	import { localToday, setInMyDay, setTaskFields } from '../sync/data';
	import { engine } from '../sync/engine.svelte';
	import { toasts } from '../toasts.svelte';

	// The style guide names its sample user; the app, whoever is signed in.
	let { me: sampleMe }: { me?: string } = $props();
	const me = $derived(sampleMe ?? session.user?.id ?? '');

	let panel = $state<HTMLElement>();
	const target = $derived(scheduler.target);
	const task = $derived(target ? engine.replica(target.ws)?.task(target.taskId) : undefined);
	const today = $derived(target ? localToday() : '');
	const choices = $derived(today ? scheduleChoices(today) : []);
	const inMyDay = $derived(
		target && me ? (engine.replica(target.ws)?.inMyDay(me, target.taskId, today) ?? false) : false
	);
	const icons = { today: Sun, tomorrow: Sunrise, 'next-week': CalendarClock, none: CalendarX };

	const phoneQuery = matchMedia('(max-width: 720px)');

	// Opens when a task is picked, beside what it was opened from on a
	// wide screen; a phone shows it as a sheet from the bottom (CSS).
	$effect(() => {
		if (!panel) return;
		if (target && !panel.matches(':popover-open')) {
			panel.showPopover();
			place();
			panel.querySelector<HTMLElement>('button')?.focus();
		} else if (!target && panel.matches(':popover-open')) {
			panel.hidePopover();
		}
	});

	function place() {
		if (!panel) return;
		const anchor = target?.anchor;
		if (phoneQuery.matches || !anchor?.isConnected) {
			panel.style.removeProperty('top');
			panel.style.removeProperty('left');
			return;
		}
		const { top, left } = placeNear(
			anchor.getBoundingClientRect(),
			{ width: panel.offsetWidth, height: panel.offsetHeight },
			{ width: innerWidth, height: innerHeight }
		);
		panel.style.top = `${top}px`;
		panel.style.left = `${left}px`;
	}

	function closed(e: ToggleEvent) {
		if (e.newState !== 'closed') return;
		const anchor = target?.anchor;
		scheduler.close();
		// Back to the button it came from, if it is still there, unless a
		// click elsewhere closed it and took focus with it.
		const focus = document.activeElement;
		const lost = !focus || focus === document.body || panel?.contains(focus);
		if (lost && anchor?.isConnected) anchor.focus();
	}

	/**
	 * Makes the edit, says what it did with an undo for that edit alone,
	 * and closes. Focus goes back to the task's schedule button once the
	 * edit shows, wherever the row is then: a new date can move it to
	 * another section, which draws it again.
	 */
	function edit(message: string, change: () => Promise<unknown>) {
		if (!target) return;
		const { taskId, anchor } = target;
		const { done, undo } = engine.track(change);
		toasts.show(message, { onundo: () => void undo() });
		panel?.hidePopover();
		void done
			.catch(() => {})
			.then(tick)
			.then(() => refocus(taskId, anchor));
	}

	function refocus(taskId: string, anchor: HTMLElement | null) {
		// A row on its way out is inert, and so is its button.
		const usable = (el: Element | null): el is HTMLElement =>
			el instanceof HTMLElement && el.isConnected && !el.closest('[inert]');
		// Focus left in the closed chooser is lost too.
		const focus = document.activeElement;
		if (focus && focus !== document.body && usable(focus) && !panel?.contains(focus)) return;
		const button = usable(anchor)
			? anchor
			: [...document.querySelectorAll(`[data-schedule="${CSS.escape(taskId)}"]`)].find(usable);
		button?.focus();
	}

	function pick(choice: ScheduleChoice | { date: string }) {
		if (!target || !task) return;
		const { ws } = target;
		const id = task.id;
		const date = choice.date;
		edit(
			date
				? `Due ${'label' in choice ? choice.label.toLowerCase() : formatDate(date, { weekday: 'short' })}`
				: 'Due date cleared',
			() => setTaskFields(ws, id, date ? { due_date: date } : { due_date: null, due_time: null })
		);
	}

	// A form rather than on change: typing a date fires change at every
	// part of it.
	function pickDate(e: SubmitEvent) {
		e.preventDefault();
		const date = new FormData(e.currentTarget as HTMLFormElement).get('date');
		if (typeof date === 'string' && date) pick({ date });
	}

	function toggleMyDay() {
		if (!target || !task || !me) return;
		const { ws } = target;
		const [id, user, on] = [task.id, me, !inMyDay];
		edit(on ? 'Added to My Day' : 'Taken out of My Day', () => setInMyDay(ws, user, id, on));
	}

	/** The arrow keys move through the choices. */
	function arrows(e: KeyboardEvent) {
		if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
		// The date field's own arrows step its day, month or year.
		if (e.target instanceof HTMLInputElement && e.target.type === 'date') return;
		const items = [...(panel?.querySelectorAll<HTMLElement>('button, input') ?? [])];
		const i = items.indexOf(document.activeElement as HTMLElement);
		if (i < 0) return;
		e.preventDefault();
		const step = e.key === 'ArrowDown' ? 1 : items.length - 1;
		items[(i + step) % items.length]?.focus();
	}
</script>

<svelte:window onresize={() => target && place()} />

<div
	class="schedule"
	popover="auto"
	role="dialog"
	tabindex="-1"
	aria-labelledby="schedule-title"
	bind:this={panel}
	ontoggle={closed}
	onkeydown={arrows}
>
	<span class="grip" aria-hidden="true"></span>
	<h2 id="schedule-title">
		Schedule <span class="name">{task?.title ?? ''}</span>
	</h2>
	<ul aria-label="When">
		{#each choices as c (c.id)}
			{@const Icon = icons[c.id]}
			{@const current = (task?.dueDate ?? null) === c.date}
			<li>
				<button type="button" aria-current={current || undefined} onclick={() => pick(c)}>
					<Icon size={16} aria-hidden="true" />
					<span class="label">{c.label}</span>
					{#if c.hint}<span class="hint">{c.hint}</span>{/if}
					{#if current}<Check size={14} aria-hidden="true" />{/if}
				</button>
			</li>
		{/each}
	</ul>
	<form class="pick" onsubmit={pickDate}>
		<label for="schedule-date">Pick a date</label>
		<input id="schedule-date" name="date" type="date" value={task?.dueDate ?? ''} required />
		<button title="Use this date" type="submit" class="set">Set</button>
	</form>
	{#if me}
		<button type="button" class="my-day" aria-pressed={inMyDay} onclick={toggleMyDay}>
			<CalendarCheck size={16} aria-hidden="true" />
			{inMyDay ? 'Take out of My Day' : 'Add to My Day'}
		</button>
	{/if}
</div>

<style>
	.schedule {
		position: fixed;
		inset: auto;
		width: min(280px, calc(100vw - 2 * var(--space-2)));
		margin: 0;
		padding: var(--space-2);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	.schedule::backdrop {
		background: transparent;
	}
	.grip {
		display: none;
	}
	h2 {
		display: flex;
		gap: var(--space-1);
		min-width: 0;
		padding: var(--space-1) var(--space-2) var(--space-2);
		font-family: var(--font-ui);
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.name {
		overflow: hidden;
		color: var(--text-primary);
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	ul {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	button {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		width: 100%;
		min-height: max(var(--tap-min), calc(32px * var(--control-scale)));
		padding: 0 var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		text-align: left;
		cursor: pointer;
	}
	button:hover {
		background: var(--surface-2);
	}
	.label {
		flex: 1;
	}
	.hint {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.pick {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		margin: var(--space-1) 0;
		padding: var(--space-1) var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.pick label {
		white-space: nowrap;
	}
	.pick .set {
		width: auto;
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--border-strong);
		justify-content: center;
	}
	.pick input {
		flex: 1;
		min-width: 0;
		height: calc(30px * var(--control-scale));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		color: var(--text-primary);
	}
	.my-day {
		border-top: var(--border-width) solid var(--border);
		border-radius: 0 0 var(--radius-md) var(--radius-md);
	}

	/* A phone: a sheet from the bottom, over a dimmed page. */
	@media (max-width: 720px) {
		.schedule {
			right: 0;
			bottom: 0;
			left: 0;
			width: 100%;
			max-width: none;
			padding: var(--space-2) var(--space-3) calc(var(--space-3) + env(safe-area-inset-bottom));
			border-width: var(--border-width) 0 0;
			border-radius: var(--radius-lg) var(--radius-lg) 0 0;
		}
		.schedule:popover-open {
			animation: sheet-in var(--dur-panel) var(--ease);
		}
		.schedule::backdrop {
			background: var(--scrim-sheet, oklch(0 0 0 / 0.3));
		}
		.grip {
			display: block;
			width: 36px;
			height: 4px;
			margin: 0 auto var(--space-2);
			border-radius: var(--radius-full);
			background: var(--border-strong);
		}
		button {
			min-height: var(--touch-target);
		}
		.pick input {
			height: var(--touch-target);
		}
	}
	@keyframes sheet-in {
		from {
			opacity: 0;
			transform: translateY(calc(100% * var(--motion)));
		}
	}
</style>
