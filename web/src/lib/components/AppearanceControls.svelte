<script lang="ts">
	import Button from './Button.svelte';
	import ThemePicker from './themes/ThemePicker.svelte';
	import {
		appearance,
		DENSITIES,
		FONT_SIZES,
		SCHEMES,
		type Appearance
	} from '../theme/appearance.svelte';

	interface Props {
		/** Only show the look on this page, leaving the user's preferences alone (the style guide). */
		preview?: boolean;
	}

	let { preview = false }: Props = $props();

	function choose(patch: Partial<Appearance>) {
		if (preview) appearance.preview(patch);
		else appearance.set(patch);
	}

	$effect(() => {
		if (preview) return () => appearance.preview(null);
	});

	// A custom theme with its accent set by hand ignores the hue.
	const handSet = $derived(!!appearance.custom && 'manual' in appearance.custom.accent);

	const labelFor: Record<string, string> = {
		system: 'System',
		light: 'Light',
		dark: 'Dark',
		comfortable: 'Comfortable',
		dense: 'Dense',
		s: 'S',
		m: 'M',
		l: 'L',
		xl: 'XL',
		xxl: 'XXL'
	};
</script>

{#snippet segmented(
	name: string,
	options: readonly string[],
	value: string,
	pick: (v: string) => void
)}
	<fieldset class="segmented">
		<legend>{name}</legend>
		<div>
			{#each options as option (option)}
				<label class:active={value === option}>
					<input
						type="radio"
						{name}
						value={option}
						checked={value === option}
						onchange={() => pick(option)}
						class="visually-hidden"
					/>
					{labelFor[option]}
				</label>
			{/each}
		</div>
	</fieldset>
{/snippet}

<div class="controls">
	<div class="themes"><ThemePicker {preview} /></div>
	{@render segmented('Scheme', SCHEMES, appearance.current.scheme, (v) =>
		choose({ scheme: v as (typeof SCHEMES)[number] })
	)}
	{@render segmented('Density', DENSITIES, appearance.current.density, (v) =>
		choose({ density: v as (typeof DENSITIES)[number] })
	)}
	{@render segmented('Font size', FONT_SIZES, appearance.current.fontSize, (v) =>
		choose({ fontSize: v as (typeof FONT_SIZES)[number] })
	)}
	<fieldset class="accent">
		<legend>Accent</legend>
		<div>
			<input
				type="range"
				min="0"
				max="359"
				aria-label="Accent hue"
				aria-describedby={handSet ? 'accent-hand-set' : undefined}
				value={appearance.current.accentHue ?? 262}
				oninput={(e) => choose({ accentHue: Number(e.currentTarget.value) })}
			/>
			<Button
				size="sm"
				variant="ghost"
				disabled={appearance.current.accentHue === null}
				onclick={() => choose({ accentHue: null })}>Theme default</Button
			>
		</div>
		{#if handSet}
			<p class="note" id="accent-hand-set">
				This theme sets its accent by hand, so the hue does not change it here.
			</p>
		{/if}
	</fieldset>
</div>

<style>
	.controls {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-4);
		align-items: flex-end;
	}
	fieldset {
		margin: 0;
		padding: 0;
		border: none;
	}
	legend {
		margin-bottom: var(--space-1);
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.segmented div {
		display: inline-flex;
		padding: 2px;
		border-radius: var(--radius-md);
		background: var(--surface-2);
	}
	.segmented label {
		padding: var(--space-1) var(--space-3);
		border-radius: calc(var(--radius-md) - 2px);
		font-size: var(--text-sm);
		color: var(--text-secondary);
		cursor: pointer;
		transition: background-color var(--dur-micro) var(--ease);
	}
	.segmented label.active {
		background: var(--surface-0);
		color: var(--text-primary);
		box-shadow: var(--shadow-sm);
		font-weight: 550;
	}
	.segmented label:has(:focus-visible) {
		box-shadow: var(--focus-ring);
	}
	.accent div {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
	input[type='range'] {
		width: 160px;
		accent-color: var(--accent);
	}
	.themes {
		flex-basis: 100%;
		min-width: 0;
	}
	.note {
		max-width: 260px;
		margin-top: var(--space-1);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
</style>
