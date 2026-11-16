<script lang="ts">
	// Log time to a project itself from the Time page (docs/PLAN.md section 3):
	// the same entry as the project's Time tab makes, with the project chosen
	// here. A toast says where it went, with Undo.
	import { onMount } from 'svelte';
	import Button from '../Button.svelte';
	import { announce } from '../../announce.svelte';
	import { MAX_ENTRY, formatDuration, parseDuration, sayDuration } from '../../duration';
	import { localToday } from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import type { Project } from '../../sync/entities';
	import { logProjectTime } from '../../sync/time';
	import { toasts } from '../../toasts.svelte';
	import { uuid } from '../../uuid';

	let {
		ws,
		projects,
		/** The project picked first: the one the report is showing, if any. */
		start = '',
		/** Takes focus as it opens; the style guide's stays put. */
		focus = true,
		onclose
	}: {
		ws: string;
		projects: Project[];
		start?: string;
		focus?: boolean;
		onclose: () => void;
	} = $props();

	const id = `logtime-${uuid()}`;
	const today = localToday();
	// svelte-ignore state_referenced_locally
	let project = $state(projects.some((p) => p.id === start) ? start : '');
	let spent = $state('');
	let day = $state(today);
	let note = $state('');
	let error = $state('');
	let form = $state<HTMLFormElement>();

	// Focus goes to the first thing to fill in, once, as the form opens.
	onMount(() => {
		if (focus) form?.querySelector<HTMLElement>(project ? '[data-spent]' : 'select')?.focus();
	});

	/** The minutes typed, or null with the error said. */
	function read(): number | null {
		const minutes = parseDuration(spent);
		if (!project) error = 'Choose the project the time was spent on.';
		else if (minutes === null) error = 'Write the time as 1h 30m, 90m, 1.5h or 1:30.';
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
		const name = projects.find((p) => p.id === project)?.name ?? 'the project';
		const { done, undo } = engine.track(() => logProjectTime(ws, project, { minutes, day, note }));
		await done;
		announce(`Logged ${sayDuration(minutes)} to ${name}`);
		toasts.show(`Logged ${formatDuration(minutes)} to ${name}`, { onundo: () => void undo() });
		onclose();
	}
</script>

<!-- Escape from any of its fields closes it. -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<form
	class="log-time"
	aria-label="Log time to a project"
	bind:this={form}
	onsubmit={submit}
	novalidate
	onkeydown={(e) => {
		if (e.key !== 'Escape') return;
		e.preventDefault();
		onclose();
	}}
>
	<label class="project">
		<span>Project</span>
		<select bind:value={project} aria-invalid={error && !project ? 'true' : undefined}>
			<option value="" disabled>Choose a project</option>
			{#each projects as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
		</select>
	</label>
	<label>
		<span>Time spent</span>
		<input
			class="tabular"
			data-spent
			bind:value={spent}
			placeholder="1h 30m"
			autocomplete="off"
			aria-describedby="{id}-hint{error ? ` ${id}-err` : ''}"
			aria-invalid={error && project ? 'true' : undefined}
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
	{#if error}<p class="error wide" id="{id}-err" role="alert">{error}</p>{/if}
	<div class="foot wide">
		<span class="hint" id="{id}-hint"
			>Time on the project itself, not one of its tasks. Like 1h 30m, 90m, 1.5h or 1:30</span
		>
		<Button size="sm" onclick={onclose}>Cancel</Button>
		<Button size="sm" type="submit" variant="primary">Log it</Button>
	</div>
</form>

<style>
	.log-time {
		display: grid;
		grid-template-columns: minmax(0, 14rem) minmax(0, 8rem) minmax(0, 10rem) minmax(0, 1fr);
		gap: var(--space-2);
		padding: var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-1);
	}
	label {
		display: grid;
		gap: 2px;
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	input,
	select {
		min-width: 0;
		height: calc(30px * var(--control-scale));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		color: var(--text-primary);
		font-size: var(--text-sm);
	}
	.tabular {
		font-variant-numeric: tabular-nums;
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
	@media (max-width: 720px) {
		.log-time {
			grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
		}
		.project,
		.note {
			grid-column: 1 / -1;
		}
	}
</style>
