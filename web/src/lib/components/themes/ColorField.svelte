<script lang="ts">
	import { hexToOklch, oklchToLinearSrgb, toHex, type Lch } from '../../theme/color';
	import { colorCss } from '../../theme/css';
	import type { ThemeValue } from '../../theme/document';
	import { candidates } from '../../theme/edit';
	import type { Color } from '../../theme/resolve';
	import { type TokenMeta } from '../../theme/tokens';

	// A color token's controls, open in its row in the theme editor (TokenRow):
	// "Same as" another token, lightness, chroma and hue, opacity where the
	// token allows it, a hex field and the system color picker. Moving a
	// slider shows each step at once (live); letting go is one change.
	interface Props {
		meta: TokenMeta;
		/** The theme's own value in this scheme, or null when it follows its base. */
		value: ThemeValue | null;
		/** The color as drawn now. */
		resolved: Color | undefined;
		onchange: (value: ThemeValue | null, final: boolean) => void;
	}

	let { meta, value, resolved, onchange }: Props = $props();

	const id = $derived(`color-${meta.name}`);
	const ref = $derived(value && 'ref' in value ? value.ref : '');
	const refs = $derived(candidates(meta));
	const lch = $derived<Lch>(
		value && 'oklch' in value ? value.oklch : (resolved?.lch ?? [0.5, 0, 0])
	);
	const alpha = $derived(
		value && 'oklch' in value ? (value.alpha ?? 1) : meta.alpha ? (resolved?.alpha ?? 1) : 1
	);
	const hex = $derived(toHex(oklchToLinearSrgb(...lch)));
	const round = (n: number, places = 3) => Number(n.toFixed(places));

	function emit(next: Lch, a: number, final: boolean) {
		const color: Lch = [
			round(Math.min(1, Math.max(0, next[0])), 4),
			round(Math.min(0.4, Math.max(0, next[1])), 4),
			round(Math.min(360, Math.max(0, next[2])), 2)
		];
		const v: ThemeValue =
			meta.alpha && a < 1 ? { oklch: color, alpha: round(Math.max(0, a), 3) } : { oklch: color };
		onchange(v, final);
	}

	const part = (i: 0 | 1 | 2, n: number): Lch => lch.with(i, n) as Lch;

	let hexError = $state('');
	function fromHex(text: string, final: boolean) {
		const c = hexToOklch(text);
		if (!c) {
			hexError = 'A hex color is # and three or six of 0 to 9 and a to f.';
			return;
		}
		hexError = '';
		emit(c, alpha, final);
	}

	// Each slider's track shows where it leads: dark to light, gray to
	// vivid, round the hues, at the color's own other two.
	const css = (l: number, c: number, h: number) => colorCss([l, c, h], 1);
	const tracks = $derived([
		`linear-gradient(90deg, ${css(0, lch[1], lch[2])}, ${css(0.5, lch[1], lch[2])}, ${css(1, lch[1], lch[2])})`,
		`linear-gradient(90deg, ${css(lch[0], 0, lch[2])}, ${css(lch[0], 0.2, lch[2])}, ${css(lch[0], 0.4, lch[2])})`,
		`linear-gradient(90deg, ${[0, 60, 120, 180, 240, 300, 360].map((h) => css(lch[0], Math.max(lch[1], 0.08), h)).join(', ')})`
	]);

	const sliders = [
		{ i: 0, name: 'Lightness', min: 0, max: 1, step: 0.005, places: 3 },
		{ i: 1, name: 'Chroma', min: 0, max: 0.4, step: 0.002, places: 3 },
		{ i: 2, name: 'Hue', min: 0, max: 360, step: 1, places: 0 }
	] as const;
</script>

<div class="color" role="group" aria-label="{meta.label} color">
	{#if refs.length}
		<label class="row">
			<span>Same as</span>
			<select
				value={ref}
				onchange={(e) => {
					const name = e.currentTarget.value;
					if (name) onchange({ ref: name }, true);
					else emit(lch, alpha, true);
				}}
			>
				<option value="">Its own color</option>
				{#each refs as t (t.name)}<option value={t.name}>{t.label}</option>{/each}
			</select>
		</label>
	{/if}
	{#each sliders as s (s.i)}
		<div class="row">
			<label for="{id}-{s.i}">{s.name}</label>
			<input
				id="{id}-{s.i}"
				type="range"
				min={s.min}
				max={s.max}
				step={s.step}
				value={lch[s.i]}
				style:--track={tracks[s.i]}
				aria-valuetext="{s.name} {round(lch[s.i], s.places)}"
				oninput={(e) => emit(part(s.i, Number(e.currentTarget.value)), alpha, false)}
				onchange={(e) => emit(part(s.i, Number(e.currentTarget.value)), alpha, true)}
			/>
			<input
				type="number"
				class="number"
				aria-label="{s.name}, exactly"
				min={s.min}
				max={s.max}
				step={s.step}
				value={round(lch[s.i], s.places)}
				onchange={(e) => {
					const n = Number(e.currentTarget.value);
					if (Number.isFinite(n)) emit(part(s.i, n), alpha, true);
				}}
			/>
		</div>
	{/each}
	{#if meta.alpha}
		<div class="row">
			<label for="{id}-alpha">Opacity</label>
			<input
				id="{id}-alpha"
				type="range"
				min="0"
				max="1"
				step="0.01"
				value={alpha}
				aria-valuetext="Opacity {Math.round(alpha * 100)} percent"
				oninput={(e) => emit(lch, Number(e.currentTarget.value), false)}
				onchange={(e) => emit(lch, Number(e.currentTarget.value), true)}
			/>
			<span class="number readout">{Math.round(alpha * 100)}%</span>
		</div>
	{/if}
	<div class="row">
		<label for="{id}-hex">Hex</label>
		<input
			id="{id}-hex"
			class="hex"
			value={hex}
			spellcheck="false"
			autocomplete="off"
			aria-describedby={hexError ? `${id}-hex-error` : undefined}
			onchange={(e) => fromHex(e.currentTarget.value, true)}
			onkeydown={(e) => {
				if (e.key === 'Enter') {
					e.preventDefault();
					fromHex(e.currentTarget.value, true);
				}
			}}
		/>
		<input
			type="color"
			class="picker"
			aria-label="Pick {meta.label} from the system colors"
			value={hex}
			oninput={(e) => fromHex(e.currentTarget.value, false)}
			onchange={(e) => fromHex(e.currentTarget.value, true)}
		/>
	</div>
	{#if hexError}<p class="error" id="{id}-hex-error">{hexError}</p>{/if}
</div>

<style>
	.color {
		display: grid;
		gap: var(--space-1);
	}
	.row {
		display: grid;
		grid-template-columns: 72px minmax(0, 1fr) auto;
		gap: var(--space-2);
		align-items: center;
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	label.row {
		grid-template-columns: 72px minmax(0, 1fr);
	}
	/* A track drawn with the color's own range; the thumb shows where it is. */
	input[type='range'] {
		width: 100%;
		min-height: max(var(--tap-min), 20px);
		margin: 0;
		background: transparent;
		appearance: none;
		cursor: pointer;
	}
	input[type='range']::-webkit-slider-runnable-track {
		height: 12px;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-full);
		background: var(--track);
	}
	input[type='range']::-moz-range-track {
		height: 12px;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-full);
		background: var(--track);
	}
	input[type='range']::-webkit-slider-thumb {
		width: 18px;
		height: 18px;
		margin-top: -4px;
		border: 2px solid var(--text-primary);
		border-radius: 50%;
		background: var(--surface-0);
		appearance: none;
	}
	input[type='range']::-moz-range-thumb {
		width: 14px;
		height: 14px;
		border: 2px solid var(--text-primary);
		border-radius: 50%;
		background: var(--surface-0);
	}
	input[type='range']:focus-visible {
		outline: none;
	}
	input[type='range']:focus-visible::-webkit-slider-thumb {
		box-shadow: var(--focus-ring);
	}
	input[type='range']:focus-visible::-moz-range-thumb {
		box-shadow: var(--focus-ring);
	}
	.number,
	.hex,
	select {
		min-height: max(var(--tap-min), calc(28px * var(--control-scale)));
		margin: 0;
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
		font-size: var(--text-sm);
	}
	.number {
		width: 72px;
		font-family: var(--font-mono);
		font-size: var(--text-xs);
	}
	.hex {
		font-family: var(--font-mono);
		font-size: var(--text-xs);
	}
	.readout {
		display: inline-flex;
		align-items: center;
		justify-content: flex-end;
	}
	input.picker {
		width: max(var(--tap-min), 36px);
		height: max(var(--tap-min), 28px);
		margin: 0;
		padding: 0;
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-sm);
		background: none;
	}
	.error {
		color: var(--danger);
		font-size: var(--text-xs);
	}
</style>
