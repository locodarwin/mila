<script lang="ts">
	import { formatDate, formatTime, weekdayName, deviceWeekStart } from '../dates';
	import type { Preferences } from '../preferences.svelte';

	interface Props {
		value: Pick<Preferences, 'weekStart' | 'dateFormat' | 'clock'>;
		onchange: (change: Partial<Pick<Preferences, 'weekStart' | 'dateFormat' | 'clock'>>) => void;
	}

	let { value, onchange }: Props = $props();
	const id = $props.id();

	// Each choice shows what it looks like; "auto" shows what the device does.
	const sample = '2026-11-03';
	const auto = { weekStart: 'auto', dateFormat: 'auto', clock: 'auto' } as const;
	const dates = $derived(
		(['auto', 'month-day', 'day-month', 'iso'] as const).map((f) => ({
			value: f,
			label:
				(f === 'auto' ? 'As this device does: ' : '') +
				formatDate(sample, { year: f === 'iso' ? undefined : false }, { ...auto, dateFormat: f })
		}))
	);
	const clocks = $derived(
		(['auto', '12', '24'] as const).map((c) => ({
			value: c,
			label:
				(c === 'auto' ? 'As this device does: ' : `${c}-hour: `) +
				formatTime('14:30', { ...auto, clock: c })
		}))
	);
	const weeks = $derived([
		{ value: 'auto', label: `As this device does: ${weekdayName(deviceWeekStart(), 'long')}` },
		{ value: 'sunday', label: 'Sunday' },
		{ value: 'monday', label: 'Monday' },
		{ value: 'saturday', label: 'Saturday' }
	]);
</script>

<div class="dates">
	<div class="field">
		<label for={`${id}-week`}>Weeks start on</label>
		<select
			id={`${id}-week`}
			value={value.weekStart}
			onchange={(e) => onchange({ weekStart: e.currentTarget.value as Preferences['weekStart'] })}
		>
			{#each weeks as w (w.value)}<option value={w.value}>{w.label}</option>{/each}
		</select>
	</div>
	<div class="field">
		<label for={`${id}-dates`}>Dates</label>
		<select
			id={`${id}-dates`}
			value={value.dateFormat}
			onchange={(e) => onchange({ dateFormat: e.currentTarget.value as Preferences['dateFormat'] })}
		>
			{#each dates as d (d.value)}<option value={d.value}>{d.label}</option>{/each}
		</select>
	</div>
	<div class="field">
		<label for={`${id}-times`}>Times</label>
		<select
			id={`${id}-times`}
			value={value.clock}
			onchange={(e) => onchange({ clock: e.currentTarget.value as Preferences['clock'] })}
		>
			{#each clocks as c (c.value)}<option value={c.value}>{c.label}</option>{/each}
		</select>
	</div>
</div>

<style>
	.dates {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-4);
	}
	.field {
		display: grid;
		gap: var(--space-1);
		min-width: 0;
	}
	label {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	select {
		max-width: 100%;
		height: calc(32px * var(--control-scale));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		color: var(--text-primary);
		font-size: var(--text-sm);
		font-weight: 400;
	}
</style>
