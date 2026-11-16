<script lang="ts">
	import { resolve } from '$app/paths';
	import { tick } from 'svelte';
	import { scopeOf, type Scope } from '../keys/commands';
	import { keymap } from '../keys/keymap.svelte';
	import { preferences } from '../preferences.svelte';
	import { ui } from '../ui.svelte';
	import Announcer from './Announcer.svelte';
	import ShortcutTable from './keys/ShortcutTable.svelte';

	let dialog = $state<HTMLDialogElement>();
	let heading = $state<HTMLElement>();
	// Anywhere, and what had focus when it opened: a task in a list, a
	// card, a calendar entry or the open task. The rest is on the
	// shortcuts page.
	let here = $state<Scope>('list');

	$effect(() => {
		if (ui.overlay === 'help' && dialog && !dialog.open) {
			here = scopeOf(document.activeElement);
			dialog.showModal();
			// The heading, not a link a reflexive Enter would follow; the
			// arrow keys then scroll the sheet.
			void tick().then(() => heading?.focus());
		} else if (ui.overlay !== 'help' && dialog?.open) dialog.close();
	});

	const stillOpens = $derived(keymap.say('help'));
</script>

<!-- Escape frees the overlay at once (cancel). The close event comes later,
     at the next frame, and by then a key may have opened the dialog again:
     a close event on an open dialog is a stale one and changes nothing. -->
<dialog
	bind:this={dialog}
	class="help"
	aria-label="Keyboard shortcuts"
	onclose={(e) => !e.currentTarget.open && ui.overlay === 'help' && ui.close()}
	oncancel={() => ui.overlay === 'help' && ui.close()}
>
	<h2 tabindex="-1" bind:this={heading}>Keyboard shortcuts</h2>
	<p class="links">
		<a
			title="See every keyboard shortcut"
			href={resolve('/(app)/help/shortcuts')}
			onclick={() => ui.close()}>All shortcuts</a
		>
		<a title="Open the help area" href={resolve('/(app)/help')} onclick={() => ui.close()}>Help</a>
		<a
			title="Change your keyboard shortcuts in Settings"
			href={resolve('/(app)/settings/keyboard')}
			onclick={() => ui.close()}>Change shortcuts</a
		>
	</p>
	{#if !preferences.current.singleKeyShortcuts}
		<p class="off">
			Single-key shortcuts are off in Settings, Keyboard{keymap.characterKeys()
				? `: ${keymap.characterKeys()} do nothing`
				: ''}. Keys with {keymap.apple ? 'Cmd' : 'Ctrl'} or {keymap.apple ? 'Option' : 'Alt'}, and
			keys on a focused task, still work.{stillOpens ? ` ${stillOpens} still opens this list.` : ''}
		</p>
	{/if}
	<div class="table">
		<ShortcutTable
			compact
			scopes={here === 'table' ? ['anywhere', 'table', 'list'] : ['anywhere', here]}
		/>
	</div>
	<div class="foot">
		<form method="dialog"><button>Close</button></form>
	</div>
	<!-- The page's live regions are inert behind a modal dialog. -->
	{#if ui.overlay === 'help'}<Announcer inDialog />{/if}
</dialog>

<style>
	.help {
		width: min(520px, calc(100vw - 32px));
		max-height: 80vh;
		padding: var(--space-5);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	.help::backdrop {
		/* With the value as written, for browsers whose ::backdrop does not inherit custom properties. */
		background: var(--scrim, oklch(0 0 0 / 0.25));
	}
	h2 {
		font-size: var(--text-xl);
	}
	/* Focused only to start reading from; the links come next. */
	h2:focus-visible {
		outline: none;
		box-shadow: none;
		text-decoration: underline;
		text-decoration-color: var(--accent);
		text-decoration-thickness: 2px;
	}
	.links {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1) var(--space-4);
		margin-top: var(--space-2);
	}
	.links a {
		color: var(--accent-text);
		font-size: var(--text-sm);
		font-weight: 550;
	}
	.off {
		margin-top: var(--space-2);
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	.table {
		margin-top: var(--space-4);
	}
	.foot {
		display: flex;
		justify-content: flex-end;
		margin-top: var(--space-4);
	}
	button {
		padding: var(--space-1) var(--space-3);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		cursor: pointer;
	}
</style>
