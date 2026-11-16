<script lang="ts">
	// The person's running timer (docs/PLAN.md section 3): at the foot of
	// the sidebar, or a bar above the tabs on a phone, on every page. It
	// notes that they are about while it runs, and after 10 hours asks
	// whether to stop it when they were last active, at a time they
	// choose, or keep it going. Loaded only while a timer runs.
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import Square from '@lucide/svelte/icons/square';
	import Announcer from './Announcer.svelte';
	import Button from './Button.svelte';
	import Stopwatch from './Stopwatch.svelte';
	import { formatClock, formatDate } from '../dates';
	import { formatDuration, minutesBetween } from '../duration';
	import { session } from '../session.svelte';
	import { stopTimer, touchTimer } from '../sync/time';
	import { engine } from '../sync/engine.svelte';
	import type { TimeEntry } from '../sync/entities';
	import { openTask } from '../taskLink';
	import { toasts } from '../toasts.svelte';
	import { uuid } from '../uuid';

	let {
		variant = 'sidebar',
		collapsed = false
	}: { variant?: 'sidebar' | 'phone'; collapsed?: boolean } = $props();

	const LONG = 10 * 3600_000;
	const TOUCH_EVERY = 5 * 60_000;
	const id = `timer-${uuid()}`;

	/** The timer and its workspace: the one in view first, then any other. */
	const found = $derived.by((): { ws: string; entry: TimeEntry } | null => {
		const me = session.user?.id;
		if (!me) return null;
		const order = [...engine.replicas.keys()].sort((a, b) =>
			a === session.workspace?.id ? -1 : b === session.workspace?.id ? 1 : 0
		);
		for (const ws of order) {
			const entry = engine.replica(ws)?.runningTimer(me);
			if (entry) return { ws, entry };
		}
		return null;
	});
	const task = $derived(
		found?.entry.taskId ? engine.replica(found.ws)?.task(found.entry.taskId) : undefined
	);
	// A timer on a project itself is named for the project.
	const project = $derived(
		found?.entry.projectId ? engine.replica(found.ws)?.project(found.entry.projectId) : undefined
	);
	const title = $derived(
		task?.title ?? project?.name ?? (found?.entry.taskId ? 'A task' : 'A project')
	);

	/** Opens what the timer is on: its task, or its project's Time tab. */
	function open() {
		if (!found) return;
		if (found.entry.taskId) return void openTask(found.entry.taskId);
		const to = resolve('/(app)/projects/[id]', { id: found.entry.projectId ?? '' });
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved, with a query
		void goto(`${to}?tab=time&ws=${found.ws}`);
	}

	async function stop(at?: Date) {
		if (!found) return;
		const minutes = await stopTimer(found.ws, found.entry, at);
		toasts.show(
			minutes
				? `Logged ${formatDuration(minutes)} on ${title}`
				: 'Stopped before a minute, so nothing was logged'
		);
	}

	// Being about: a key or a pointer, noted at most every five minutes.
	let lastTouch = 0;
	$effect(() => {
		const current = found;
		if (!current) return;
		const note = () => {
			const now = Date.now();
			if (now - lastTouch < TOUCH_EVERY) return;
			lastTouch = now;
			void touchTimer(current.ws, current.entry);
		};
		window.addEventListener('pointerdown', note, { passive: true });
		window.addEventListener('keydown', note);
		return () => {
			window.removeEventListener('pointerdown', note);
			window.removeEventListener('keydown', note);
		};
	});

	// After 10 hours: asked once on this device, again later if put off.
	let now = $state(Date.now());
	$effect(() => {
		const t = setInterval(() => (now = Date.now()), 60_000);
		return () => clearInterval(t);
	});
	const LATER_KEY = 'mila.timer.later';
	const later = (): Record<string, number> => {
		try {
			return JSON.parse(localStorage.getItem(LATER_KEY) ?? '{}') ?? {};
		} catch {
			return {};
		}
	};
	let putOff = $state(later());
	function putOffFor(ms: number) {
		if (!found) return;
		putOff = { [found.entry.id]: Date.now() + ms };
		try {
			localStorage.setItem(LATER_KEY, JSON.stringify(putOff));
		} catch {
			// Private browsing: it asks again next time.
		}
	}
	const started = $derived(
		found?.entry.startedAt ? new Date(found.entry.startedAt).getTime() : now
	);
	const long = $derived(!!found && now - started >= LONG && (putOff[found.entry.id] ?? 0) <= now);
	const lastActive = $derived.by(() => {
		const at = found?.entry.activeAt ? new Date(found.entry.activeAt) : null;
		return at && at.getTime() > started ? at : null;
	});

	let dialog = $state<HTMLDialogElement>();
	let choice = $state<'active' | 'at' | 'keep'>('active');
	let at = $state('');
	let error = $state('');
	$effect(() => {
		if (long && dialog && !dialog.open) {
			choice = lastActive ? 'active' : 'at';
			at = '';
			error = '';
			dialog.showModal();
		} else if (!long && dialog?.open) dialog.close();
	});

	const when = (d: Date) =>
		`${formatClock(d)}${d.toDateString() === new Date().toDateString() ? '' : `, ${formatDate(d, { weekday: 'short' })}`}`;

	/** The latest moment at a time of day after the start and not after now. */
	function moment(clock: string): Date | null {
		const m = /^(\d{1,2}):(\d{2})/.exec(clock);
		if (!m) return null;
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- worked out once, never shown
		const d = new Date();
		d.setHours(Number(m[1]), Number(m[2]), 0, 0);
		if (d.getTime() > Date.now()) d.setDate(d.getDate() - 1);
		return d.getTime() > started ? d : null;
	}

	async function decide(e: SubmitEvent) {
		e.preventDefault();
		if (choice === 'keep') {
			putOffFor(LONG);
			dialog?.close();
			return;
		}
		const end = choice === 'active' ? lastActive : moment(at);
		if (!end) {
			error = 'Choose a time after the timer started.';
			return;
		}
		dialog?.close();
		await stop(end);
	}
</script>

{#if found}
	<div class="timer {variant}" class:collapsed role="group" aria-label="Your timer">
		<span class="dot" aria-hidden="true"></span>
		{#if !collapsed}
			<button
				class="task"
				type="button"
				title={found.entry.taskId ? 'Open the task' : 'Open the project'}
				onclick={open}>{title}</button
			>
		{/if}
		<span class="clock"><Stopwatch since={found.entry.startedAt} /></span>
		<button
			class="stop"
			type="button"
			aria-label={`Stop the timer on ${title}`}
			title="Stop the timer"
			onclick={() => void stop()}
			><Square size={12} />{#if !collapsed}<span>Stop</span>{/if}</button
		>
	</div>
{/if}

<dialog
	bind:this={dialog}
	class="long"
	aria-labelledby="{id}-h"
	onclose={() => long && putOffFor(3600_000)}
>
	{#if found}
		<form onsubmit={decide}>
			<h2 id="{id}-h">Your timer has run {Math.floor((now - started) / 3600_000)} hours</h2>
			<p>
				It is on <strong>{title}</strong>, started {when(new Date(started))}.{#if lastActive}
					The last thing you did in Mila was at {when(lastActive)}.{/if}
			</p>
			<fieldset>
				<legend class="visually-hidden">What to keep</legend>
				{#if lastActive}
					<label class="choice"
						><input type="radio" name="{id}-choice" value="active" bind:group={choice} />Stop it at {when(
							lastActive
						)}
						<span class="muted"
							>({formatDuration(minutesBetween(new Date(started), lastActive))})</span
						></label
					>
				{/if}
				<label class="choice"
					><input type="radio" name="{id}-choice" value="at" bind:group={choice} />Stop it at
					<input
						type="time"
						aria-label="Stop time"
						bind:value={at}
						onfocus={() => (choice = 'at')}
					/></label
				>
				<label class="choice"
					><input type="radio" name="{id}-choice" value="keep" bind:group={choice} />Keep it running</label
				>
			</fieldset>
			{#if error}<p class="error" role="alert">{error}</p>{/if}
			<p class="muted">One entry holds a day at most; anything longer is logged as a day.</p>
			<div class="foot">
				<Button title="Keep the timer running for now" onclick={() => dialog?.close()}>Later</Button
				>
				<Button title="Do what you chose above" type="submit" variant="primary">Done</Button>
			</div>
		</form>
		{#if long}<Announcer inDialog />{/if}
	{/if}
</dialog>

<style>
	.timer {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		grid-template-areas: 'dot task stop' 'dot clock stop';
		align-items: center;
		gap: 0 var(--space-2);
		padding: var(--space-2) var(--space-2) var(--space-2) var(--space-3);
		border: var(--border-width) solid var(--accent);
		border-radius: var(--radius-md);
		background: var(--accent-soft);
		color: var(--text-primary);
	}
	.sidebar {
		margin: var(--space-2) 0 0;
	}
	.collapsed {
		grid-template-columns: auto;
		grid-template-areas: 'dot' 'clock' 'stop';
		justify-items: center;
		padding: var(--space-2) var(--space-1);
		font-size: var(--text-xs);
	}
	.phone {
		position: fixed;
		left: var(--space-3);
		right: calc(56px + 2 * var(--space-4));
		bottom: calc(var(--tabbar-height) + var(--space-4) + env(safe-area-inset-bottom));
		z-index: 19;
		grid-template-areas: 'dot task clock stop';
		grid-template-columns: auto minmax(0, 1fr) auto auto;
		min-height: 56px;
		background: var(--accent);
		color: var(--accent-contrast);
		box-shadow: var(--shadow-lg);
	}
	.dot {
		grid-area: dot;
		width: 8px;
		height: 8px;
		border-radius: var(--radius-full);
		background: var(--accent);
	}
	.phone .dot {
		background: var(--accent-contrast);
	}
	.task {
		grid-area: task;
		min-width: 0;
		overflow: hidden;
		padding: 0;
		border: none;
		background: none;
		color: inherit;
		font-size: var(--text-sm);
		font-weight: 550;
		text-align: left;
		text-overflow: ellipsis;
		white-space: nowrap;
		cursor: pointer;
	}
	.task:hover {
		text-decoration: underline;
	}
	.clock {
		grid-area: clock;
		font-size: var(--text-sm);
		font-weight: 650;
	}
	.stop {
		grid-area: stop;
		display: inline-flex;
		align-items: center;
		gap: 4px;
		min-height: max(var(--tap-min), 28px);
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-sm);
		background: var(--surface-0);
		color: var(--text-primary);
		font-size: var(--text-xs);
		cursor: pointer;
	}
	.long {
		width: min(440px, calc(100vw - 32px));
		padding: var(--space-5);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	.long::backdrop {
		/* With the value as written, for browsers whose ::backdrop does not inherit custom properties. */
		background: var(--scrim, oklch(0 0 0 / 0.25));
	}
	.long form {
		display: grid;
		gap: var(--space-3);
	}
	.long h2 {
		font-size: var(--text-xl);
	}
	.long p {
		margin: 0;
		font-size: var(--text-sm);
	}
	fieldset {
		display: grid;
		gap: var(--space-2);
		margin: 0;
		padding: 0;
		border: none;
	}
	.choice {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		min-height: max(var(--tap-min), 32px);
		font-size: var(--text-sm);
	}
	.choice input[type='time'] {
		min-height: max(var(--tap-min), 28px);
		padding: 0 var(--space-1);
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-sm);
		background: var(--surface-0);
	}
	.muted {
		color: var(--text-secondary);
	}
	.error {
		color: var(--danger);
	}
	.foot {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-2);
	}
</style>
