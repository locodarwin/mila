<script lang="ts">
	import Checkbox from '../Checkbox.svelte';
	import {
		describe,
		formatRule,
		MAX_INTERVAL,
		tryParseRule,
		weekdayOfDate,
		type Freq,
		type RepeatMode,
		type Rule,
		type Weekday
	} from '../../recurrence';
	import { formatDate, weekDays } from '../../dates';

	interface Props {
		/** The rule as stored, or null for a task that does not repeat. */
		rule: string | null;
		mode: RepeatMode;
		/** The date presets are built around: the task's due date, else today. */
		anchor: string;
		onchange: (rule: string | null, mode: RepeatMode) => void;
	}

	let { rule, mode, anchor, onchange }: Props = $props();

	const DAYS = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'];
	const ORDINALS = ['first', 'second', 'third', 'fourth', 'fifth'];
	const UNITS: { freq: Freq; one: string; many: string }[] = [
		{ freq: 'DAILY', one: 'day', many: 'days' },
		{ freq: 'WEEKLY', one: 'week', many: 'weeks' },
		{ freq: 'MONTHLY', one: 'month', many: 'months' },
		{ freq: 'YEARLY', one: 'year', many: 'years' }
	];

	const parsed = $derived(tryParseRule(rule));
	const weekday = $derived(weekdayOfDate(anchor));
	const day = $derived(Number(anchor.slice(8, 10)));
	const monthLength = $derived(
		new Date(Date.UTC(Number(anchor.slice(0, 4)), Number(anchor.slice(5, 7)), 0)).getUTCDate()
	);

	const base = { interval: 1, weekdays: [] as Weekday[] };
	const presets = $derived([
		{ id: 'daily', rule: formatRule({ ...base, freq: 'DAILY' }) },
		{ id: 'weekdays', rule: formatRule({ ...base, freq: 'WEEKLY', weekdays: [0, 1, 2, 3, 4] }) },
		{ id: 'weekly', rule: formatRule({ ...base, freq: 'WEEKLY', weekdays: [weekday] }) },
		{ id: 'monthly', rule: formatRule({ ...base, freq: 'MONTHLY', monthDay: day }) },
		{ id: 'yearly', rule: formatRule({ ...base, freq: 'YEARLY' }) }
	]);

	// Custom stays open once picked, even if what is set there matches a preset.
	let customOpen = $state(false);
	const matched = $derived(
		parsed ? presets.find((p) => mode === 'schedule' && p.rule === formatRule(parsed))?.id : 'none'
	);
	const choice = $derived(customOpen || !matched ? 'custom' : matched);

	function label(id: string): string {
		switch (id) {
			case 'daily':
				return 'Every day';
			case 'weekdays':
				return 'Every weekday';
			case 'weekly':
				return `Weekly on ${DAYS[weekday]}`;
			case 'monthly':
				return `Monthly on day ${day}`;
			default:
				return `Yearly on ${formatDate(anchor, { year: false })}`;
		}
	}

	function pick(id: string) {
		if (id === 'custom') {
			customOpen = true;
			if (!parsed)
				onchange(formatRule({ ...base, freq: 'WEEKLY', weekdays: [weekday] }), 'schedule');
			return;
		}
		customOpen = false;
		onchange(id === 'none' ? null : (presets.find((p) => p.id === id)?.rule ?? null), 'schedule');
	}

	/** Saves the rule with some parts changed. */
	function edit(change: Partial<Rule>, nextMode: RepeatMode = mode) {
		if (!parsed) return;
		const next: Rule = { ...parsed, ...change };
		if (nextMode === 'completion' || next.freq !== 'WEEKLY') next.weekdays = [];
		if (nextMode === 'completion' || next.freq !== 'MONTHLY') {
			delete next.ordinal;
			delete next.monthDay;
		}
		onchange(formatRule(next), nextMode);
	}

	function setFreq(freq: Freq) {
		if (freq === 'WEEKLY') edit({ freq, weekdays: [weekday] });
		else if (freq === 'MONTHLY') edit({ freq, monthDay: day, ordinal: undefined });
		else edit({ freq });
	}

	function setInterval(value: string) {
		const n = Math.min(MAX_INTERVAL, Math.max(1, Math.round(Number(value)) || 1));
		edit({ interval: n });
	}

	function toggleDay(d: Weekday) {
		if (!parsed) return;
		const on = parsed.weekdays.includes(d);
		if (on && parsed.weekdays.length === 1) return; // a weekly rule needs a day
		edit({ weekdays: on ? parsed.weekdays.filter((x) => x !== d) : [...parsed.weekdays, d] });
	}

	// Ways a monthly rule can fall, built around the anchor, plus the rule's
	// own if it is none of them.
	type MonthlyOn = { key: string; label: string; ordinal?: Rule['ordinal']; monthDay?: number };
	const monthlyOptions = $derived.by(() => {
		const n = Math.ceil(day / 7);
		const opts: MonthlyOn[] = [{ key: `d${day}`, label: `On day ${day}`, monthDay: day }];
		if (n <= 4)
			opts.push({
				key: `o${n}:${weekday}`,
				label: `On the ${ORDINALS[n - 1]} ${DAYS[weekday]}`,
				ordinal: { n, day: weekday }
			});
		if (day + 7 > monthLength)
			opts.push({
				key: `o-1:${weekday}`,
				label: `On the last ${DAYS[weekday]}`,
				ordinal: { n: -1, day: weekday }
			});
		opts.push({ key: 'd-1', label: 'On the last day', monthDay: -1 });
		const own = currentMonthly();
		if (own && !opts.some((o) => o.key === own.key)) opts.unshift(own);
		return opts;
	});

	function currentMonthly(): MonthlyOn | undefined {
		if (!parsed || parsed.freq !== 'MONTHLY') return undefined;
		if (parsed.ordinal) {
			const { n, day: d } = parsed.ordinal;
			const which = n === -1 ? 'last' : n < 0 ? `${ORDINALS[-n - 1]} to last` : ORDINALS[n - 1];
			return { key: `o${n}:${d}`, label: `On the ${which} ${DAYS[d]}`, ordinal: parsed.ordinal };
		}
		const md = parsed.monthDay ?? day;
		return {
			key: `d${md}`,
			label: md === -1 ? 'On the last day' : `On day ${md}`,
			monthDay: md
		};
	}

	function setMonthly(key: string) {
		const o = monthlyOptions.find((x) => x.key === key);
		if (o) edit({ ordinal: o.ordinal, monthDay: o.monthDay });
	}
</script>

<div class="repeat">
	<select aria-label="Repeat" value={choice} onchange={(e) => pick(e.currentTarget.value)}>
		<option value="none">Does not repeat</option>
		{#each presets as p (p.id)}
			<option value={p.id}>{label(p.id)}</option>
		{/each}
		<option value="custom">Custom</option>
	</select>

	{#if parsed && choice === 'custom'}
		<div class="custom" role="group" aria-label="Custom repeat">
			<div class="line">
				<span>Every</span>
				<input
					class="interval"
					type="number"
					min="1"
					max={MAX_INTERVAL}
					aria-label="Repeat every"
					value={parsed.interval}
					onchange={(e) => setInterval(e.currentTarget.value)}
				/>
				<select
					aria-label="Repeat unit"
					value={parsed.freq}
					onchange={(e) => setFreq(e.currentTarget.value as Freq)}
				>
					{#each UNITS as u (u.freq)}
						<option value={u.freq}>{parsed.interval === 1 ? u.one : u.many}</option>
					{/each}
				</select>
			</div>
			{#if mode === 'schedule' && parsed.freq === 'WEEKLY'}
				<div class="days" role="group" aria-label="Repeat on these days">
					<!-- In the order of the user's week; rules count from Monday as 0. -->
					{#each weekDays().map((js) => ((js + 6) % 7) as Weekday) as d (d)}
						{@const name = DAYS[d]}
						{@const on = parsed.weekdays.length ? parsed.weekdays.includes(d) : d === weekday}
						<button
							type="button"
							class="day"
							aria-label={name}
							aria-pressed={on}
							onclick={() =>
								parsed.weekdays.length
									? toggleDay(d)
									: edit({ weekdays: d === weekday ? [d] : [weekday, d] })}
							>{name.slice(0, 2)}</button
						>
					{/each}
				</div>
			{:else if mode === 'schedule' && parsed.freq === 'MONTHLY'}
				<select
					aria-label="Repeat on"
					value={currentMonthly()?.key}
					onchange={(e) => setMonthly(e.currentTarget.value)}
				>
					{#each monthlyOptions as o (o.key)}
						<option value={o.key}>{o.label}</option>
					{/each}
				</select>
			{/if}
			<label class="line">
				<span>Until</span>
				<input
					type="date"
					aria-label="Repeat until"
					value={parsed.until ?? ''}
					onchange={(e) => edit({ until: e.currentTarget.value || undefined })}
				/>
			</label>
		</div>
	{/if}

	{#if parsed}
		<Checkbox
			label="Repeat after completion"
			checked={mode === 'completion'}
			onchange={(on) => edit({}, on ? 'completion' : 'schedule')}
		/>
		<p class="description">{describe(parsed, mode, anchor)}</p>
	{/if}
</div>

<style>
	.repeat {
		display: grid;
		gap: var(--space-2);
		/* As wide as the space it is given, so a long choice fits inside it. */
		grid-template-columns: minmax(0, 1fr);
		justify-items: start;
		width: 100%;
	}
	.custom {
		display: grid;
		gap: var(--space-2);
		padding: var(--space-3);
		border-radius: var(--radius-md);
		background: var(--surface-1);
	}
	.line {
		display: flex;
		gap: var(--space-2);
		align-items: center;
		font-size: var(--text-sm);
	}
	select,
	input {
		max-width: 100%;
		height: 30px;
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		font-size: var(--text-sm);
	}
	select:hover,
	input:hover {
		border-color: var(--border-strong);
	}
	.interval {
		width: 4.5em;
	}
	/* One row of seven, which shrink to fit a phone rather than wrap. */
	.days {
		display: grid;
		grid-template-columns: repeat(7, minmax(0, 32px));
		gap: var(--space-1);
		width: 100%;
	}
	.day {
		width: 100%;
		min-width: 0;
		height: 32px;
		padding: 0;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-full);
		background: var(--surface-0);
		font-size: var(--text-xs);
		font-weight: 600;
		cursor: pointer;
	}
	.day:hover {
		border-color: var(--border-strong);
	}
	.day[aria-pressed='true'] {
		border-color: var(--accent);
		background: var(--accent);
		color: var(--accent-contrast);
	}
	.description {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
</style>
