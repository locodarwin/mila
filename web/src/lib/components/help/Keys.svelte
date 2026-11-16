<script lang="ts">
	import type { CommandId } from '../../keys/commands';
	import { keymap } from '../../keys/keymap.svelte';
	import Kbd from '../Kbd.svelte';

	interface Props {
		/** A command, or several shown together ("Alt+← and Alt+→"). */
		id: CommandId | CommandId[];
		/**
		 * Prose: the words before and after the keys, shown only while each
		 * command has a key that works now, so a sentence never offers a key
		 * that is off or missing. Otherwise is shown in their place.
		 */
		lead?: string;
		trail?: string;
		otherwise?: string;
	}

	// A command's keys as the person has them, for help text. Without prose
	// it says "off" (single-key shortcuts are off) or "no key" in a table.
	let { id, lead, trail, otherwise }: Props = $props();

	const ids = $derived(Array.isArray(id) ? id : [id]);
	const active = $derived(ids.map((c) => keymap.active(c)));
	const prose = $derived(lead !== undefined || trail !== undefined || otherwise !== undefined);
	const ready = $derived(active.every((keys) => keys.length > 0));
	const any = $derived(ids.every((c) => keymap.keys(c).length > 0));
</script>

<!-- One line, so no white space slips into the sentence around it. -->
<!-- prettier-ignore -->
{#snippet keys()}<span class="keys">{#each active as list, c (ids[c])}{#if c}<span class="or">and</span>{/if}{#each list as b, i (b)}{#if i}<span class="or">or</span>{/if}<Kbd binding={b} />{/each}{/each}</span>{/snippet}
<!-- prettier-ignore -->
{#if prose}{#if ready}{lead ?? ''}{@render keys()}{trail ?? ''}{:else}{otherwise ?? ''}{/if}{:else if ready}{@render keys()}{:else}<span class="none">{any ? 'off' : 'no key'}</span>{/if}

<style>
	.keys {
		display: inline-flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-1);
		vertical-align: middle;
	}
	.or,
	.none {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.none {
		padding: 0 var(--space-1);
		border: var(--border-width) dashed var(--border);
		border-radius: var(--radius-sm);
	}
</style>
