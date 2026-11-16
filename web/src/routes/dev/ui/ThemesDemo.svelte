<script lang="ts">
	// Custom themes' parts, loaded apart: the picker, a token row in each
	// state the editor shows, a color field with its panel, the swatch
	// card, the preview of a theme held here only, never saved, the
	// import's dry run, loaded only when asked for, and the workspace
	// theme notices, whose buttons do nothing here.
	import Button from '#lib/components/Button.svelte';
	import ColorField from '#lib/components/themes/ColorField.svelte';
	import ThemeCard from '#lib/components/themes/ThemeCard.svelte';
	import ThemePicker from '#lib/components/themes/ThemePicker.svelte';
	import ThemeNotice from '#lib/components/themes/ThemeNotice.svelte';
	import ThemePreview from '#lib/components/themes/ThemePreview.svelte';
	import TokenRow from '#lib/components/themes/TokenRow.svelte';
	import { appearance } from '#lib/theme/appearance.svelte.js';
	import { PAIRS, type Failure } from '#lib/theme/contrast.js';
	import { VERSION, type ThemeDoc, type ThemeValue } from '#lib/theme/document.js';
	import { byName } from '#lib/theme/tokens.js';

	const doc: ThemeDoc = {
		mila: 'theme',
		version: VERSION,
		id: '00000000-0000-4000-8000-000000000001',
		name: 'Orchard',
		base: 'calm',
		target: 'AA',
		accent: { hue: 30 },
		tokens: {
			'radius-md': { px: 9 },
			'button-radius': { px: 18 },
			'font-heading': { font: 'nunito' }
		},
		light: {
			'sidebar-bg': { oklch: [0.93, 0.04, 140] },
			'card-border': { oklch: [0.82, 0.05, 140] }
		},
		dark: {
			'sidebar-bg': { oklch: [0.2, 0.04, 140] }
		}
	};

	const pair = (fg: string) => PAIRS.find((p) => p.fg === fg)!;
	const passing: Failure[] = [
		{ pair: pair('text-primary'), bg: 'surface-0', ratio: 13.2, need: 4.5 }
	];
	const failing: Failure[] = [
		{ pair: pair('text-muted'), bg: 'surface-0', ratio: 3.9, need: 4.5 },
		{ pair: pair('text-muted'), bg: 'surface-3', ratio: 3.4, need: 4.5 }
	];
	const color = (l: number, c: number, h: number) => ({
		lch: [l, c, h] as [number, number, number],
		alpha: 1
	});

	let rows = $state<Record<string, ThemeValue | null>>({
		'text-primary': null,
		'text-muted': { oklch: [0.68, 0.01, 75] },
		'card-radius': { ref: 'radius-lg' },
		'font-heading': { font: 'nunito' },
		'shadow-md': null
	});
	const set = (name: string) => (v: ThemeValue | null) => (rows[name] = v);
	let field = $state<ThemeValue | null>({ oklch: [0.62, 0.14, 30] });
	// The row open in place: the failing one, with Fix.
	let open = $state<string | null>('text-muted');
	let importing = $state(false);
	const notes = ['Left out "glow", a setting this Mila does not have.'];
	const scheme = $derived(appearance.scheme);
	const offered = {
		id: doc.id,
		name: doc.name,
		body: doc,
		sourceId: null,
		suggestedAt: '2026-11-01T09:00:00Z',
		publishedBy: null,
		publishedAt: null,
		sortKey: 'a0'
	};
</script>

<h3>ThemePicker</h3>
<ThemePicker preview />

<h3>ThemeCard</h3>
<div class="cards">
	<ThemeCard base="standard" {scheme} />
	<ThemeCard base="playful" {scheme} />
	<ThemeCard base={doc.base} {doc} {scheme} />
</div>

<h3>TokenRow</h3>
<p class="note">
	A row each: from its base with its contrast, changed and failing (open in place, with Fix), a ref,
	a font, and a shadow's layers. A row opens and closes its editor.
</p>
<div class="rows">
	<TokenRow
		meta={byName('text-primary')!}
		value={rows['text-primary']}
		resolved={color(0.3, 0.02, 60)}
		baseName="Calm"
		measures={passing}
		under={[]}
		target="AA"
		expanded={open === 'text-primary'}
		ontoggle={() => (open = open === 'text-primary' ? null : 'text-primary')}
		onchange={set('text-primary')}
		onfix={() => {}}
	/>
	<TokenRow
		meta={byName('text-muted')!}
		value={rows['text-muted']}
		resolved={color(0.68, 0.01, 75)}
		baseName="Calm"
		measures={failing}
		under={[]}
		target="AA"
		expanded={open === 'text-muted'}
		ontoggle={() => (open = open === 'text-muted' ? null : 'text-muted')}
		onchange={set('text-muted')}
		onfix={() => {}}
	/>
	<TokenRow
		meta={byName('accent')!}
		value={null}
		resolved={color(0.53, 0.12, 165)}
		baseName="Calm"
		locked="Worked out from the accent hue. Set the accent by hand to choose it yourself."
		measures={[]}
		under={[]}
		target="AA"
		expanded={open === 'accent'}
		ontoggle={() => (open = open === 'accent' ? null : 'accent')}
		onchange={() => {}}
		onfix={() => {}}
	/>
	<TokenRow
		meta={byName('card-radius')!}
		value={rows['card-radius']}
		resolved={8}
		baseName="Calm"
		measures={[]}
		under={[]}
		target="AA"
		expanded={open === 'card-radius'}
		ontoggle={() => (open = open === 'card-radius' ? null : 'card-radius')}
		onchange={set('card-radius')}
		onfix={() => {}}
	/>
	<TokenRow
		meta={byName('font-heading')!}
		value={rows['font-heading']}
		resolved="'Nunito Variable', system-ui, sans-serif"
		baseName="Calm"
		measures={[]}
		under={[]}
		target="AA"
		expanded={open === 'font-heading'}
		ontoggle={() => (open = open === 'font-heading' ? null : 'font-heading')}
		onchange={set('font-heading')}
		onfix={() => {}}
	/>
	<TokenRow
		meta={byName('shadow-md')!}
		value={rows['shadow-md']}
		resolved="0 2px 6px currentcolor, 0 1px 2px currentcolor"
		baseName="Calm"
		measures={[]}
		under={[]}
		target="AA"
		expanded={open === 'shadow-md'}
		ontoggle={() => (open = open === 'shadow-md' ? null : 'shadow-md')}
		onchange={set('shadow-md')}
		onfix={() => {}}
	/>
</div>

<h3>ColorField</h3>
<p class="note">
	A color's controls, as they open in its row: Same as, lightness, chroma and hue on tracks of the
	color's own range, opacity where it may be see-through, a hex field and the system's picker.
</p>
<div class="rows">
	<ColorField
		meta={byName('accent-soft')!}
		value={field}
		resolved={color(0.62, 0.14, 30)}
		onchange={(v) => (field = v)}
	/>
</div>

<h3>ImportTheme</h3>
<p class="note">The dry run of an import, as a dialog: nothing is kept here.</p>
<div class="start"><Button onclick={() => (importing = true)}>Show an import</Button></div>
{#if importing}
	{#await import('#lib/components/themes/ImportTheme.svelte') then { default: ImportTheme }}
		<ImportTheme
			theme={doc}
			{notes}
			full={false}
			onadd={() => {}}
			onclose={() => (importing = false)}
		/>
	{/await}
{/if}

<h3>ThemeNotice</h3>
<p class="note">
	A workspace suggesting a theme, and a theme in use that its workspace stopped offering.
</p>
<div class="notices">
	<ThemeNotice
		preview
		notice={{ kind: 'suggest', ws: 'sample', wsName: 'Acme Studio', theme: offered }}
	/>
	<ThemeNotice
		preview
		notice={{
			kind: 'gone',
			kept: { ws: 'sample', wsName: 'Acme Studio', doc, gone: true, user: null }
		}}
	/>
</div>

<h3>ThemePreview</h3>
<div class="previews">
	<ThemePreview theme={doc} scheme="light" caption="Light" />
	<ThemePreview theme={doc} scheme="dark" caption="Dark" />
</div>
<ThemePreview theme={doc} {scheme} part="pages" caption="One part, as the editor shows it: Pages" />

<style>
	h3 {
		font-size: var(--text-md);
	}
	.note {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.notices {
		display: grid;
		gap: var(--space-2);
	}
	.cards {
		display: grid;
		grid-template-columns: repeat(3, 120px);
		gap: var(--space-3);
	}
	.rows {
		max-width: 560px;
	}
	.start {
		display: flex;
	}
	.previews {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
		gap: var(--space-4);
	}
</style>
