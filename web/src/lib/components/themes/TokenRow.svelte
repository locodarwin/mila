<script lang="ts">
	import CircleCheck from '@lucide/svelte/icons/circle-check';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import Button from '../Button.svelte';
	import Checkbox from '../Checkbox.svelte';
	import type { Background, Failure } from '../../theme/contrast';
	import { colorCss } from '../../theme/css';
	import { EASES, FONTS, type ShadowLayer, type ThemeValue } from '../../theme/document';
	import { candidates, EASE_NAMES, easeOf, FONT_NAMES, fontOf, layersOf } from '../../theme/edit';
	import type { Color, Resolved } from '../../theme/resolve';
	import { oklchToLinearSrgb, toHex } from '../../theme/color';
	import { byName, type TokenMeta } from '../../theme/tokens';
	import ColorField from './ColorField.svelte';

	// One token in the theme editor: a row with its swatch or kind, name,
	// value and contrast, which opens in place to say what it is and where it
	// is used, the editor for its kind, whether the theme changed it, and its
	// contrast against what it is read on, with Fix.
	interface Props {
		meta: TokenMeta;
		/** The theme's own value in the scheme being edited, or null when it follows its base. */
		value: ThemeValue | null;
		resolved: Resolved | undefined;
		/** The base's name: "From Calm". */
		baseName: string;
		/** Why it cannot be changed here, if it cannot (the generated accent). */
		locked?: string;
		/** Its contrast as text or a mark on its backgrounds, passing or not. */
		measures: Failure[];
		/** Pairs failing on it as a background. */
		under: Failure[];
		target: 'AA' | 'AAA';
		/** Whether its editor is open, in the row. */
		expanded: boolean;
		/** Its part of the theme, said beside it where rows of several parts mix (a search). */
		part?: string;
		ontoggle: () => void;
		onchange: (value: ThemeValue | null, final: boolean) => void;
		onfix: () => void;
	}

	let {
		meta,
		value,
		resolved,
		baseName,
		locked,
		measures,
		under,
		target,
		expanded,
		part,
		ontoggle,
		onchange,
		onfix
	}: Props = $props();

	const name = $derived(meta.name);
	const changed = $derived(value !== null);
	const refs = $derived(candidates(meta));
	const ref = $derived(value && 'ref' in value ? value.ref : '');
	const failing = $derived(measures.filter((m) => m.ratio < m.need));
	const lowest = $derived(
		measures.length ? measures.reduce((a, b) => (b.ratio < a.ratio ? b : a)) : null
	);
	const labelOf = (bg: Background) =>
		typeof bg === 'string' ? (byName(bg)?.label ?? bg) : bg.label;
	const ratio = (n: number) => `${n.toFixed(1)}:1`;

	// Numbers: px, ms, plain numbers and hues.
	const numeric = $derived(['px', 'ms', 'number', 'hue'].includes(meta.type));
	const range = $derived(
		meta.type === 'hue'
			? { min: 0, max: 360, step: 1 }
			: { min: meta.min ?? 0, max: meta.max ?? 100, step: meta.step ?? 1 }
	);
	const unit = $derived(meta.type === 'px' ? 'px' : meta.type === 'ms' ? 'ms' : '');
	const current = $derived.by(() => {
		if (value) {
			if ('px' in value) return value.px;
			if ('ms' in value) return value.ms;
			if ('n' in value) return value.n;
			if ('hue' in value) return value.hue;
		}
		return typeof resolved === 'number' ? resolved : range.min;
	});
	function number(n: number, final: boolean) {
		if (!Number.isFinite(n)) return;
		const v = Math.min(range.max, Math.max(range.min, n));
		const key = meta.type === 'number' ? 'n' : meta.type;
		onchange({ [key]: Number(v.toFixed(4)) } as ThemeValue, final);
	}

	// Fonts and curves are chosen from a list; a ref is one of the choices.
	const font = $derived(
		value && 'font' in value ? value.font : typeof resolved === 'string' ? fontOf(resolved) : null
	);
	const ease = $derived(
		value && !('ref' in value) ? value : typeof resolved === 'string' ? easeOf(resolved) : null
	);
	const easeChoice = $derived(
		ref
			? `ref:${ref}`
			: ease && 'ease' in ease
				? ease.ease
				: ease && 'bezier' in ease
					? 'custom'
					: ''
	);
	const bezier = $derived<[number, number, number, number]>(
		ease && 'bezier' in ease ? ease.bezier : [0.2, 0.8, 0.2, 1]
	);

	// Shadows: up to three layers, each down, blur and spread, in the shadow color.
	const layers = $derived<ShadowLayer[]>(
		value && 'layers' in value
			? value.layers
			: typeof resolved === 'string'
				? layersOf(resolved)
				: []
	);
	function setLayer(i: number, key: keyof ShadowLayer, n: number) {
		if (!Number.isFinite(n)) return;
		const bounds = { y: [-32, 64], blur: [0, 128], spread: [-32, 32] }[key];
		const next = layers.map((l, j) =>
			j === i ? { ...l, [key]: Math.min(bounds[1], Math.max(bounds[0], n)) } : l
		);
		onchange({ layers: next }, true);
	}

	const color = $derived(
		resolved && typeof resolved === 'object' && 'lch' in resolved ? (resolved as Color) : undefined
	);
	// The color as the theme has it now, its own value or as drawn.
	const shown = $derived(
		color
			? value && 'oklch' in value
				? { lch: value.oklch, alpha: value.alpha ?? 1 }
				: color
			: null
	);
	const hex = $derived(shown ? toHex(oklchToLinearSrgb(...shown.lch)) : '');
	const round = (n: number, places = 3) => Number(n.toFixed(places));
	const refLabel = $derived(ref ? (byName(ref)?.label ?? ref) : '');

	// The row's value in a few characters, and in words for its name.
	const summary = $derived.by((): { text: string; said: string } => {
		if (locked) return { text: 'From the accent hue', said: 'worked out from the accent hue' };
		if (ref) return { text: `= ${refLabel}`, said: `same as ${refLabel}` };
		if (meta.type === 'color' && shown)
			return {
				text: hex,
				said: `lightness ${round(shown.lch[0], 2)}, chroma ${round(shown.lch[1], 3)}, hue ${Math.round(shown.lch[2])}${meta.alpha && shown.alpha < 1 ? `, opacity ${Math.round(shown.alpha * 100)} percent` : ''}`
			};
		if (numeric && meta.name === 'celebrate')
			return current === 1 ? { text: 'On', said: 'on' } : { text: 'Off', said: 'off' };
		if (numeric) {
			const t = `${round(current, 3)}${unit ? ` ${unit}` : ''}`;
			return { text: t, said: t };
		}
		if (meta.type === 'font') {
			const f = font ? (FONT_NAMES[font] ?? '') : '';
			return { text: f, said: f };
		}
		if (meta.type === 'ease') {
			const e = easeChoice === 'custom' ? 'A curve of its own' : (EASE_NAMES[easeChoice] ?? '');
			return { text: e, said: e.toLowerCase() };
		}
		if (meta.type === 'shadow') {
			const n = layers.length;
			const t = n ? `${n} ${n === 1 ? 'layer' : 'layers'}` : 'None';
			return { text: t, said: t.toLowerCase() };
		}
		return { text: '', said: '' };
	});
	const problems = $derived([...failing, ...under]);
	const glyph = $derived(
		meta.type === 'font'
			? 'Aa'
			: meta.type === 'ms' || meta.type === 'ease'
				? '~'
				: meta.type === 'shadow'
					? '◫'
					: '#'
	);

	let summaryButton = $state<HTMLButtonElement>();
	/** Closes the row's editor, and puts focus back on the row. */
	function done() {
		ontoggle();
		summaryButton?.focus();
	}
</script>

<!-- Escape, from the row or its editor, closes the editor, and nothing under it. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="token"
	class:open={expanded}
	id="token-{name}"
	onkeydown={(e) => {
		if (e.key === 'Escape' && expanded) {
			e.preventDefault();
			done();
		}
	}}
>
	<button
		bind:this={summaryButton}
		type="button"
		class="summary"
		aria-expanded={expanded}
		aria-controls="detail-{name}"
		aria-label="{meta.label}: {summary.said}"
		aria-describedby="state-{name}"
		onclick={ontoggle}
	>
		{#if meta.type === 'color' && shown}
			<span class="swatch" style:background={colorCss(shown.lch, shown.alpha)}></span>
		{:else}
			<span class="glyph" aria-hidden="true">{glyph}</span>
		{/if}
		<span class="label" id="label-{name}"
			>{meta.label}{#if part}<span class="part">{part}</span>{/if}</span
		>
		<span class="current">{summary.text}</span>
		<span class="marks" id="state-{name}">
			{#if changed}<span class="dot" title="Changed"
					><span class="visually-hidden">Changed</span></span
				>{/if}
			{#if problems.length}
				<span class="badge warn"
					><TriangleAlert size={12} aria-hidden="true" />{ratio(
						Math.min(...problems.map((f) => f.ratio))
					)}<span class="visually-hidden"
						>, {problems.length}
						{problems.length === 1 ? 'contrast problem' : 'contrast problems'}</span
					></span
				>
			{:else if lowest}
				<span class="badge ok"
					>{ratio(lowest.ratio)}<span class="visually-hidden"> on {labelOf(lowest.bg)}</span></span
				>
			{/if}
		</span>
	</button>
	{#if expanded}
		<div class="detail" id="detail-{name}" role="group" aria-labelledby="label-{name}">
			<p class="used">
				{meta.used}
				<code>--{name}</code>
				{#if !meta.perScheme}<span class="tag">Both schemes</span>{/if}
			</p>
			<div class="head">
				<span class="state" class:changed>{changed ? 'Changed' : `From ${baseName}`}</span>
				{#if changed}
					<Button
						size="sm"
						variant="ghost"
						aria-label="Reset {meta.label}"
						onclick={() => onchange(null, true)}>Reset</Button
					>
				{/if}
			</div>
			<div class="value">
				{#if locked}
					{#if color}<span class="swatch" style:background={colorCss(color.lch, color.alpha)}
						></span>{/if}
					<p class="locked">{locked}</p>
				{:else if meta.type === 'color'}
					<ColorField {meta} {value} resolved={color} {onchange} />
				{:else if numeric && meta.name === 'celebrate'}
					<Checkbox
						label="Confetti when a project is finished"
						checked={current === 1}
						onchange={(on) => number(on ? 1 : 0, true)}
					/>
				{:else if numeric}
					{#if refs.length}
						<label class="inline">
							<span>Same as</span>
							<select
								value={ref}
								onchange={(e) =>
									e.currentTarget.value
										? onchange({ ref: e.currentTarget.value }, true)
										: number(current, true)}
							>
								<option value="">Its own value</option>
								{#each refs as t (t.name)}<option value={t.name}>{t.label}</option>{/each}
							</select>
						</label>
					{/if}
					<input
						type="range"
						aria-labelledby="label-{name}"
						min={range.min}
						max={range.max}
						step={range.step}
						value={current}
						aria-valuetext="{meta.label} {current}{unit ? ` ${unit}` : ''}"
						oninput={(e) => number(Number(e.currentTarget.value), false)}
						onchange={(e) => number(Number(e.currentTarget.value), true)}
					/>
					<input
						type="number"
						class="number"
						aria-label="{meta.label}, exactly{unit ? ` in ${unit}` : ''}"
						min={range.min}
						max={range.max}
						step={range.step}
						value={current}
						onchange={(e) => number(Number(e.currentTarget.value), true)}
					/>
					{#if unit}<span class="unit" aria-hidden="true">{unit}</span>{/if}
				{:else if meta.type === 'font'}
					<fieldset class="choices">
						<legend class="visually-hidden">{meta.label}</legend>
						{#each refs as t (t.name)}
							<label class="choice">
								<input
									type="radio"
									name="font-{name}"
									checked={ref === t.name}
									onchange={() => onchange({ ref: t.name }, true)}
								/>
								Same as {t.label}
							</label>
						{/each}
						{#each Object.entries(FONTS) as [id, stack] (id)}
							<label class="choice" style:font-family={stack}>
								<input
									type="radio"
									name="font-{name}"
									checked={!ref && font === id}
									onchange={() => onchange({ font: id as keyof typeof FONTS }, true)}
								/>
								{FONT_NAMES[id]}
							</label>
						{/each}
					</fieldset>
				{:else if meta.type === 'ease'}
					<fieldset class="choices">
						<legend class="visually-hidden">{meta.label}</legend>
						{#each refs as t (t.name)}
							<label class="choice">
								<input
									type="radio"
									name="ease-{name}"
									checked={easeChoice === `ref:${t.name}`}
									onchange={() => onchange({ ref: t.name }, true)}
								/>
								Same as {t.label}
							</label>
						{/each}
						{#each Object.keys(EASES) as id (id)}
							<label class="choice">
								<input
									type="radio"
									name="ease-{name}"
									checked={easeChoice === id}
									onchange={() => onchange({ ease: id as keyof typeof EASES }, true)}
								/>
								{EASE_NAMES[id]}
							</label>
						{/each}
						<label class="choice">
							<input
								type="radio"
								name="ease-{name}"
								checked={easeChoice === 'custom'}
								onchange={() => onchange({ bezier }, true)}
							/>
							A curve of your own
						</label>
					</fieldset>
					{#if easeChoice === 'custom'}
						<div class="bezier">
							{#each ['x1', 'y1', 'x2', 'y2'] as axis, i (axis)}
								<label class="inline">
									<span>{axis}</span>
									<input
										type="number"
										class="number"
										aria-label="{meta.label}, {axis}"
										min={i % 2 ? -1 : 0}
										max={i % 2 ? 2 : 1}
										step="0.05"
										value={bezier[i]}
										onchange={(e) => {
											const n = Number(e.currentTarget.value);
											const lo = i % 2 ? -1 : 0;
											const hi = i % 2 ? 2 : 1;
											if (Number.isFinite(n))
												onchange(
													{
														bezier: bezier.with(i, Math.min(hi, Math.max(lo, n))) as [
															number,
															number,
															number,
															number
														]
													},
													true
												);
										}}
									/>
								</label>
							{/each}
						</div>
					{/if}
				{:else if meta.type === 'shadow'}
					{#if refs.length}
						<label class="inline">
							<span>Same as</span>
							<select
								value={ref}
								onchange={(e) =>
									e.currentTarget.value
										? onchange({ ref: e.currentTarget.value }, true)
										: onchange({ layers }, true)}
							>
								<option value="">Its own layers</option>
								{#each refs as t (t.name)}<option value={t.name}>{t.label}</option>{/each}
							</select>
						</label>
					{/if}
					<div class="layers">
						{#each layers as layer, i (i)}
							<div class="layer" role="group" aria-label="{meta.label}, layer {i + 1}">
								{#each [['y', 'Down'], ['blur', 'Blur'], ['spread', 'Spread']] as const as [key, word] (key)}
									<label class="inline">
										<span>{word}</span>
										<input
											type="number"
											class="number"
											step="1"
											value={layer[key]}
											onchange={(e) => setLayer(i, key, Number(e.currentTarget.value))}
										/>
									</label>
								{/each}
								<Button
									size="sm"
									variant="ghost"
									aria-label="Remove layer {i + 1} of {meta.label}"
									onclick={() => onchange({ layers: layers.filter((_, j) => j !== i) }, true)}
									>Remove</Button
								>
							</div>
						{/each}
						{#if !layers.length}<p class="locked">No shadow.</p>{/if}
						{#if layers.length < 3}
							<div>
								<Button
									size="sm"
									aria-label="Add a layer to {meta.label}"
									onclick={() =>
										onchange({ layers: [...layers, { y: 2, blur: 6, spread: 0 }] }, true)}
									>Add a layer</Button
								>
							</div>
						{/if}
					</div>
				{/if}
			</div>

			{#if failing.length || under.length || lowest}
				<ul class="badges">
					{#each failing as f (labelOf(f.bg))}
						<li class="warn">
							<TriangleAlert size={14} aria-hidden="true" />
							{ratio(f.ratio)} on {labelOf(f.bg)}; {target} needs {f.need}
						</li>
					{/each}
					{#each under as f (`${f.pair.fg} ${labelOf(f.bg)}`)}
						<li class="warn">
							<TriangleAlert size={14} aria-hidden="true" />
							{byName(f.pair.fg)?.label ?? f.pair.fg} on this: {ratio(f.ratio)}; {target} needs {f.need}
						</li>
					{/each}
					{#if !failing.length && !under.length && lowest}
						<li>
							<CircleCheck size={14} aria-hidden="true" />
							{ratio(lowest.ratio)} on {labelOf(lowest.bg)}
						</li>
					{/if}
				</ul>
				{#if (failing.length || under.length) && meta.type === 'color' && !locked}
					<div>
						<Button size="sm" aria-label="Fix {meta.label}" onclick={onfix}>Fix</Button>
					</div>
				{/if}
			{/if}
			<div class="foot"><Button size="sm" onclick={done}>Done</Button></div>
		</div>
	{/if}
</div>

<style>
	.token {
		border-radius: var(--radius-md);
		scroll-margin: var(--space-3);
	}
	.token.open {
		margin: var(--space-1) 0;
		border: var(--border-width) solid var(--accent);
		background: var(--surface-1);
	}
	/* The row: swatch or kind, name, value, and its marks, in one line. */
	.summary {
		display: grid;
		grid-template-columns: 22px minmax(0, 1fr) auto auto;
		align-items: center;
		gap: var(--space-2);
		width: 100%;
		min-height: max(var(--tap-min), calc(36px * var(--control-scale)));
		padding: 0 var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-primary);
		font: inherit;
		font-size: var(--text-sm);
		text-align: left;
		cursor: pointer;
	}
	.summary:hover {
		background: var(--surface-2);
	}
	.open .summary:hover {
		background: none;
	}
	.open .label {
		font-weight: 600;
	}
	.glyph {
		display: grid;
		place-items: center;
		width: 22px;
		height: 22px;
		border-radius: var(--radius-sm);
		background: var(--surface-2);
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-weight: 600;
	}
	.label {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.part {
		margin-left: var(--space-2);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.current {
		max-width: 16ch;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-family: var(--font-mono);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.marks {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
	}
	.dot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--accent);
	}
	.badge {
		display: inline-flex;
		align-items: center;
		gap: 2px;
		padding: 1px 6px;
		border-radius: var(--radius-full);
		font-size: var(--text-xs);
		font-variant-numeric: tabular-nums;
	}
	.badge.ok {
		background: var(--surface-2);
		color: var(--text-secondary);
	}
	.badge.warn {
		background: var(--surface-2);
		color: var(--danger);
		font-weight: 600;
	}
	.detail {
		display: grid;
		gap: var(--space-2);
		padding: 0 var(--space-3) var(--space-3);
	}
	.foot {
		display: flex;
		justify-content: flex-end;
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}

	code {
		font-family: var(--font-mono);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.tag {
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-full);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.state {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.state.changed {
		color: var(--accent-text);
		font-weight: 600;
	}
	.used {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.value {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
	.swatch {
		width: 22px;
		height: 22px;
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-sm);
	}
	.locked {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	input[type='range'] {
		flex: 1;
		min-width: 120px;
		max-width: 280px;
		min-height: max(var(--tap-min), 20px);
		accent-color: var(--accent);
	}
	.number,
	select {
		min-height: max(var(--tap-min), calc(28px * var(--control-scale)));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
	}
	.number {
		width: 80px;
	}
	.unit {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.inline {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		font-size: var(--text-sm);
	}
	.choices {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
		gap: var(--space-1) var(--space-3);
		width: 100%;
		margin: 0;
		padding: 0;
		border: none;
	}
	.choice {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: max(var(--tap-min), 28px);
		cursor: pointer;
	}
	input[type='radio'] {
		accent-color: var(--accent);
	}
	.bezier,
	.layers {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
		width: 100%;
	}
	.layers {
		flex-direction: column;
	}
	.layer {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
	.badges {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1) var(--space-3);
		margin: 0;
		padding: 0;
		list-style: none;
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.badges li {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
	}
	.badges .warn {
		color: var(--danger);
		font-weight: 600;
	}
</style>
