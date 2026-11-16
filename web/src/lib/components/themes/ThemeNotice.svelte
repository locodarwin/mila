<script lang="ts">
	import { announce } from '../../announce.svelte';
	import { appearance } from '../../theme/appearance.svelte';
	import { floorsOf } from '../../theme/custom';
	import type { ThemeNotice } from '../../theme/follow.svelte';
	import { answer, saveCopy, switchToBase, use } from '../../theme/workspaceActions';
	import { workspaceDoc } from '../../theme/workspaceThemes.svelte';
	import { toasts } from '../../toasts.svelte';
	import Button from '../Button.svelte';
	import ThemeCard from './ThemeCard.svelte';
	import { BASE_NAMES } from './names';

	// The theme notice, above the page: a workspace suggesting a theme,
	// asked once (Use it, or No thanks), or the theme in use no longer
	// offered by its workspace, which stays until the person saves a copy
	// or goes back to its base. Loaded only while there is one.
	// With preview (the style guide) the buttons do nothing.
	let { notice, preview = false }: { notice: ThemeNotice; preview?: boolean } = $props();

	const hue = $derived(appearance.current.accentHue);
	const scheme = $derived(appearance.scheme);
	const doc = $derived(notice.kind === 'gone' ? notice.kept.doc : workspaceDoc(notice.theme));
	// A suggestion this person could not read is not offered to them.
	const readable = $derived(
		notice.kind === 'gone' || (!!doc && !floorsOf(doc, { accentHue: hue }))
	);

	// Each reads the notice first: using a theme ends the notice at once.
	function useIt() {
		if (preview) return;
		const n = notice;
		if (n.kind !== 'suggest') return;
		const why = use(n.ws, n.theme);
		if (why) return toasts.error(why);
		answer(n.theme.id);
		announce(`${n.theme.name} is in use. Change it any time in Appearance.`);
	}

	function noThanks() {
		if (preview) return;
		if (notice.kind !== 'suggest') return;
		answer(notice.theme.id);
		announce('Kept your theme');
	}

	function keep() {
		if (preview) return;
		const n = notice;
		if (n.kind !== 'gone') return;
		const name = n.kept.doc.name;
		const why = saveCopy(n.kept);
		if (why) toasts.error(why);
		else toasts.show(`Saved ${name} to your themes`);
	}

	function toBase() {
		if (preview) return;
		const n = notice;
		if (n.kind !== 'gone') return;
		const base = BASE_NAMES[n.kept.doc.base];
		switchToBase(n.kept);
		announce(`${base} is in use`);
	}
</script>

{#if doc && readable}
	<section
		class="notice"
		aria-label={notice.kind === 'suggest' ? 'Theme suggestion' : 'Theme no longer offered'}
	>
		<span class="card"><ThemeCard base={doc.base} {doc} {scheme} accentHue={hue} /></span>
		{#if notice.kind === 'suggest'}
			<p>
				<strong>{notice.wsName}</strong> suggests the <strong>{notice.theme.name}</strong> theme. Try
				it; you can change it any time in Appearance.
			</p>
			<span class="actions">
				<Button title="Keep the theme you have" size="sm" variant="ghost" onclick={noThanks}
					>No thanks</Button
				>
				<Button title="Switch to this theme" size="sm" variant="primary" onclick={useIt}
					>Use it</Button
				>
			</span>
		{:else}
			<p>
				<strong>{notice.kept.wsName}</strong> no longer offers <strong>{doc.name}</strong>. You
				still see it until you choose another.
				<span class="sub"
					>{BASE_NAMES[doc.base]} is the theme {doc.name} was built on. Saving a copy makes it one of
					yours, unchanged.</span
				>
			</p>
			<span class="actions">
				<Button title="Keep this theme as one of your own" size="sm" onclick={keep}
					>Save a copy to my themes</Button
				>
				<Button size="sm" variant="primary" onclick={toBase}
					>Switch to {BASE_NAMES[doc.base]}</Button
				>
			</span>
		{/if}
	</section>
{/if}

<style>
	.notice {
		position: relative;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-3);
		margin: var(--space-3) var(--space-4) 0;
		padding: var(--space-3) var(--space-4);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface-1);
		color: var(--text-primary);
		font-size: var(--text-sm);
	}
	.card {
		flex: none;
		width: 64px;
	}
	p {
		flex: 1;
		min-width: 200px;
		margin: 0;
	}
	.sub {
		display: block;
		margin-top: var(--space-1);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
</style>
