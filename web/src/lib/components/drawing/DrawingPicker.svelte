<script lang="ts">
	// Choosing how a new drawing starts: blank, or with a few shapes to
	// change. Loaded with the editor's tools (DocTools).
	import { DRAWING_STARTERS, type DrawingStarter } from '../../drawing';
	import Announcer from '../Announcer.svelte';

	interface Props {
		/** Whether the dialog is open; closing it sets this false. */
		open: boolean;
		/** Makes the drawing chosen. */
		onpick: (starter: DrawingStarter) => void;
	}

	let { open = $bindable(), onpick }: Props = $props();
	let dialog = $state<HTMLDialogElement>();

	$effect(() => {
		if (open && dialog && !dialog.open) dialog.showModal();
		else if (!open && dialog?.open) dialog.close();
	});

	function pick(starter: DrawingStarter) {
		open = false;
		onpick(starter);
	}
</script>

<dialog
	bind:this={dialog}
	class="picker"
	aria-labelledby="drawing-picker-title"
	onclose={() => (open = false)}
>
	<h2 id="drawing-picker-title">Insert a drawing</h2>
	<p class="hint">It opens full screen to draw in. Everyone who can edit the page can change it.</p>
	<ul aria-label="How it starts">
		{#each DRAWING_STARTERS as s (s.id)}
			<li>
				<button type="button" onclick={() => pick(s)}>
					<span class="name">{s.label}</span>
					<span class="detail">{s.detail}</span>
				</button>
			</li>
		{/each}
	</ul>
	<div class="actions">
		<form method="dialog"><button>Cancel</button></form>
	</div>
	{#if open}<Announcer inDialog />{/if}
</dialog>

<style>
	.picker {
		width: min(480px, calc(100vw - 32px));
		max-height: 80vh;
		padding: var(--space-5);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	.picker::backdrop {
		/* With the value as written, for browsers whose ::backdrop does not inherit custom properties. */
		background: var(--scrim, oklch(0 0 0 / 0.25));
	}
	h2 {
		font-size: var(--text-xl);
	}
	.hint {
		margin-top: var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	ul {
		display: grid;
		gap: var(--space-1);
		margin: var(--space-4) 0 0;
		padding: 0;
		list-style: none;
	}
	li button {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: var(--space-3);
		width: 100%;
		min-height: max(var(--tap-min), 36px);
		padding: var(--space-1) var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-1);
		color: var(--text-primary);
		font-size: var(--text-sm);
		text-align: left;
		cursor: pointer;
	}
	li button:hover {
		border-color: var(--border-strong);
	}
	.name {
		font-weight: 600;
	}
	.detail {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.actions {
		display: flex;
		justify-content: flex-end;
		margin-top: var(--space-4);
	}
</style>
