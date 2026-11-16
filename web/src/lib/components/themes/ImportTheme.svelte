<script lang="ts">
	import { onMount, tick } from 'svelte';
	import Announcer from '../Announcer.svelte';
	import Button from '../Button.svelte';
	import { appearance } from '../../theme/appearance.svelte';
	import { floorsOf, problems } from '../../theme/custom';
	import type { ThemeDoc } from '../../theme/document';
	import ThemeCard from './ThemeCard.svelte';
	import { BASE_NAMES } from './names';

	// What an import would bring, before anything is kept (the dry run, as
	// the CSV import's plan does): the theme's name and base, how it
	// looks, what was left out of it and why, and its contrast. Only Add
	// theme keeps it.
	interface Props {
		theme: ThemeDoc;
		notes: string[];
		/** Whether the person is at the most themes Mila keeps. */
		full: boolean;
		onadd: (theme: ThemeDoc) => void;
		onclose: () => void;
	}

	let { theme, notes, full, onadd, onclose }: Props = $props();

	let dialog = $state<HTMLDialogElement>();
	const hue = $derived(appearance.current.accentHue);
	const floors = $derived(floorsOf(theme, { accentHue: hue }));
	const found = $derived.by(() => {
		try {
			return problems(theme, { accentHue: hue });
		} catch {
			return { light: [], dark: [] };
		}
	});
	const count = $derived(found.light.length + found.dark.length);

	onMount(() => {
		dialog?.showModal();
		void tick().then(() => dialog?.querySelector<HTMLElement>('[data-first]')?.focus());
	});

	function add() {
		onadd(theme);
		dialog?.close();
	}
</script>

<dialog bind:this={dialog} class="import" aria-labelledby="import-title" onclose={() => onclose()}>
	<h2 id="import-title">Import {theme.name}</h2>
	<dl>
		<dt>Name</dt>
		<dd>{theme.name}</dd>
		<dt>Based on</dt>
		<dd>{BASE_NAMES[theme.base]}</dd>
		<dt>Contrast goal</dt>
		<dd>{theme.target}</dd>
	</dl>
	<div class="cards">
		<figure>
			<ThemeCard base={theme.base} doc={theme} scheme="light" accentHue={hue} />
			<figcaption>Light</figcaption>
		</figure>
		<figure>
			<ThemeCard base={theme.base} doc={theme} scheme="dark" accentHue={hue} />
			<figcaption>Dark</figcaption>
		</figure>
	</div>
	<h3>What Mila found</h3>
	<ul class="notes">
		{#each notes as note (note)}<li>{note}</li>{/each}
		{#if !notes.length}<li>Everything in the file can be used.</li>{/if}
		{#if floors}
			<li class="warn">{floors} You can add it and fix it in the editor, but not use it yet.</li>
		{:else if count}
			<li>
				{count}
				{count === 1 ? 'contrast problem' : 'contrast problems'} below {theme.target}: {found.light
					.length} in light, {found.dark.length} in dark. The editor shows each one, with Fix.
			</li>
		{:else}
			<li>No contrast problems at {theme.target}.</li>
		{/if}
		{#if full}
			<li class="warn">You have the most themes Mila keeps (20). Delete one to add this.</li>
		{/if}
	</ul>
	<div class="actions">
		<Button variant="ghost" onclick={() => dialog?.close()}>Cancel</Button>
		<Button variant="primary" data-first disabled={full} onclick={add}>Add theme</Button>
	</div>
	<Announcer inDialog />
</dialog>

<style>
	.import {
		display: grid;
		gap: var(--space-3);
		width: min(calc(440px * var(--ui-scale)), calc(100vw - 32px));
		max-height: 90vh;
		padding: var(--space-5);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	.import:not([open]) {
		display: none;
	}
	.import::backdrop {
		/* With the value as written, for browsers whose ::backdrop does not inherit custom properties. */
		background: var(--scrim, oklch(0 0 0 / 0.25));
	}
	h2 {
		font-size: var(--text-xl);
		overflow-wrap: anywhere;
	}
	h3 {
		font-size: var(--text-md);
	}
	dl {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: var(--space-1) var(--space-3);
		margin: 0;
		font-size: var(--text-sm);
	}
	dt {
		color: var(--text-secondary);
	}
	dd {
		margin: 0;
		overflow-wrap: anywhere;
	}
	.cards {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--space-3);
	}
	figure {
		display: grid;
		gap: var(--space-1);
		margin: 0;
	}
	figcaption {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.notes {
		display: grid;
		gap: var(--space-1);
		margin: 0;
		padding-left: var(--space-5);
		font-size: var(--text-sm);
	}
	.warn {
		color: var(--danger);
	}
	.actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-2);
	}
	@media (max-width: 720px) {
		.import {
			width: 100%;
			max-width: none;
			height: 100%;
			max-height: none;
			margin: 0;
			padding: var(--space-4) var(--space-4) calc(var(--space-4) + env(safe-area-inset-bottom));
			border: none;
			border-radius: 0;
		}
	}
</style>
