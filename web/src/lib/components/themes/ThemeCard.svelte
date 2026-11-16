<script lang="ts">
	import type { Character, Scheme } from '../../theme/color';
	import type { ThemeDoc } from '../../theme/document';

	// A small swatch of a theme: its page, a raised surface, text in its
	// heading face, secondary text and its accent, in one scheme. A theme
	// preview container of its own, so it shows the theme whatever the
	// page wears. Decorative: what it shows is the theme's name, beside it.
	interface Props {
		base: Character;
		/** A custom theme to show; without one, the built-in `base`. */
		doc?: ThemeDoc | null;
		scheme: Scheme;
		/** The person's accent hue, which a generated accent follows. */
		accentHue?: number | null;
	}

	let { base, doc = null, scheme, accentHue = null }: Props = $props();

	let box = $state<HTMLElement>();

	// A custom theme's values go on inline; the serializer loads only when
	// one is shown, so the pages with this card stay small.
	$effect(() => {
		const el = box;
		const theme = doc;
		const s = scheme;
		const hue = theme && 'manual' in theme.accent ? null : accentHue;
		if (!el) return;
		if (!theme) {
			if (hue === null) el.style.removeProperty('--accent-hue');
			else el.style.setProperty('--accent-hue', String(hue));
			return;
		}
		let stale = false;
		void Promise.all([import('../../theme/css'), import('../../theme/custom')]).then(
			([{ applyInline }, { applyAccent }]) => {
				if (stale) return;
				applyInline(el, theme, s);
				applyAccent(el, theme, s, hue);
			}
		);
		return () => {
			stale = true;
		};
	});
</script>

<span
	class="swatch"
	bind:this={box}
	data-theme={doc?.base ?? base}
	data-scheme={scheme}
	data-density="comfortable"
	data-font-size="m"
	aria-hidden="true"
>
	<span class="raised"></span>
	<span class="text">Aa</span>
	<span class="line"></span>
	<span class="accent"></span>
</span>

<style>
	.swatch {
		position: relative;
		display: block;
		width: 100%;
		height: 56px;
		overflow: hidden;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-0);
	}
	.raised {
		position: absolute;
		top: 0;
		bottom: 0;
		left: 0;
		width: 28%;
		background: var(--sidebar-bg);
		border-right: var(--border-width) solid var(--sidebar-border);
	}
	.text {
		position: absolute;
		top: 6px;
		left: 36%;
		color: var(--text-primary);
		font-family: var(--font-heading);
		font-size: 18px;
		font-weight: var(--heading-weight);
		line-height: 1.2;
	}
	.line {
		position: absolute;
		bottom: 12px;
		left: 36%;
		width: 30%;
		height: 4px;
		border-radius: var(--radius-full);
		background: var(--text-secondary);
	}
	.accent {
		position: absolute;
		right: 8px;
		bottom: 8px;
		width: 22px;
		height: 14px;
		border-radius: var(--button-radius);
		background: var(--accent);
	}
</style>
