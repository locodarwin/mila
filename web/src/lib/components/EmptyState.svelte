<script lang="ts">
	import type { CommandId } from '../keys/commands';
	import Kbd from './Kbd.svelte';

	let { title, hint, command }: { title: string; hint: string; command?: CommandId } = $props();
</script>

<div class="empty">
	<h2>{title}</h2>
	<p>
		{hint}
		{#if command}<Kbd {command} />{/if}
	</p>
</div>

<style>
	.empty {
		display: grid;
		place-content: center;
		gap: var(--space-2);
		min-height: 50vh;
		text-align: center;
	}
	h2 {
		font-size: var(--text-xl);
	}
	p {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-2);
		color: var(--text-secondary);
	}
</style>
