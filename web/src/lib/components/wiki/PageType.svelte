<script lang="ts">
	// A wiki page's own font and size, for everyone who reads it (docs/PLAN.md
	// section 3), from the page's menu. Without them each reader's theme decides.
	// Each choice is an edit to the page, made as it's chosen.
	import { FONT_NAMES, FONT_STACKS, PAGE_FONTS, PAGE_SIZES } from '../../docs/formatting';
	import Announcer from '../Announcer.svelte';

	interface Props {
		/** Whether the dialog is open; closing it sets this false. */
		open: boolean;
		/** The page's font and size now. */
		font: string | null;
		size: string | null;
		onchange: (font: string | null, size: string | null) => void;
	}

	let { open = $bindable(), font, size, onchange }: Props = $props();
	let dialog = $state<HTMLDialogElement>();

	$effect(() => {
		if (open && dialog && !dialog.open) dialog.showModal();
		else if (!open && dialog?.open) dialog.close();
	});

	const SIZE_NAMES: Record<string, string> = { small: 'Small', large: 'Large' };
</script>

<dialog
	bind:this={dialog}
	class="type"
	aria-labelledby="page-type-title"
	onclose={() => (open = false)}
>
	<h2 id="page-type-title">Font and size</h2>
	<p class="hint">For everyone who reads this page. Without them, each person's theme decides.</p>
	<label class="field" for="page-font">Font</label>
	<select
		id="page-font"
		value={font ?? ''}
		onchange={(e) => onchange(e.currentTarget.value || null, size)}
	>
		<option value="">The theme's</option>
		{#each PAGE_FONTS as f (f)}
			<option value={f} style:font-family={FONT_STACKS[f]}>{FONT_NAMES[f]}</option>
		{/each}
	</select>
	<fieldset>
		<legend>Size</legend>
		{#each ['', ...PAGE_SIZES] as s (s)}
			<label class="choice">
				<input
					type="radio"
					name="page-size"
					value={s}
					checked={(size ?? '') === s}
					onchange={() => onchange(font, s || null)}
				/>
				{s ? SIZE_NAMES[s] : 'Normal'}
			</label>
		{/each}
	</fieldset>
	<div class="actions">
		<form method="dialog"><button>Done</button></form>
	</div>
	{#if open}<Announcer inDialog />{/if}
</dialog>

<style>
	.type {
		width: min(380px, calc(100vw - 32px));
		padding: var(--space-5);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	.type::backdrop {
		/* With the value as written, for browsers whose ::backdrop does not inherit custom properties. */
		background: var(--scrim, oklch(0 0 0 / 0.25));
	}
	h2 {
		font-size: var(--text-xl);
	}
	.hint {
		margin: var(--space-2) 0 var(--space-4);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.field,
	legend {
		display: block;
		margin-bottom: var(--space-1);
		font-size: var(--text-sm);
		font-weight: 600;
	}
	select {
		width: 100%;
		min-height: max(var(--tap-min), 34px);
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--radius-md);
		background: var(--input-bg);
		color: var(--text-primary);
		font-size: var(--text-sm);
	}
	fieldset {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1) var(--space-4);
		margin: var(--space-4) 0 0;
		padding: 0;
		border: none;
	}
	legend {
		width: 100%;
	}
	.choice {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-height: max(var(--tap-min), 30px);
		font-size: var(--text-sm);
	}
	.actions {
		display: flex;
		justify-content: flex-end;
		margin-top: var(--space-4);
	}
</style>
