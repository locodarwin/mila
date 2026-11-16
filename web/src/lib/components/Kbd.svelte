<script lang="ts">
	import { format, spoken, type Binding } from '../keys/chord';
	import type { CommandId } from '../keys/commands';
	import { keymap } from '../keys/keymap.svelte';

	interface Props {
		/** Literal keycaps, for text about keys that are not commands. */
		keys?: string[];
		/** A binding, shown as keycaps ("Mod+k": Ctrl K, or ⌘ K on a Mac). */
		binding?: Binding;
		/** A command's first key that works now; nothing when none does. */
		command?: CommandId;
		/** Shows a binding as on a Mac, or not, whatever this device is (the style guide). */
		apple?: boolean;
	}

	let { keys, binding, command, apple = keymap.apple }: Props = $props();

	const shown = $derived(binding ?? (command ? keymap.active(command)[0] : undefined));
	const steps = $derived(shown ? format(shown, apple) : keys ? [keys] : []);
</script>

{#if steps.length}
	<!-- A binding's keycaps are read as words: "Control+K", "G then I". -->
	<span class="combo">
		{#each steps as step, s (s)}
			{#if s}<span class="then" aria-hidden="true"></span>{/if}
			{#each step as key, i (i)}<kbd aria-hidden={shown ? 'true' : undefined}>{key}</kbd>{/each}
		{/each}
		{#if shown}<span class="visually-hidden">{spoken(shown, apple)}</span>{/if}
	</span>
{/if}

<style>
	.combo {
		position: relative;
		display: inline-flex;
		align-items: center;
		gap: 3px;
	}
	.then {
		width: var(--space-1);
	}
	kbd {
		display: inline-grid;
		place-items: center;
		min-width: calc(20px * var(--ui-scale));
		height: calc(20px * var(--ui-scale));
		padding: 0 5px;
		border: var(--border-width) solid var(--border);
		border-bottom-width: 2px;
		border-radius: var(--radius-sm);
		background: var(--surface-1);
		color: var(--text-secondary);
		font-family: var(--font-ui);
		font-size: var(--text-xs);
		font-weight: 600;
		white-space: nowrap;
	}
</style>
