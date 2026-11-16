<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements';

	interface Props extends HTMLInputAttributes {
		label: string;
		value?: string;
	}

	let { label, value = $bindable(''), ...rest }: Props = $props();
</script>

<label class="field">
	<span class="label">{label}</span>
	<input bind:value {...rest} />
</label>

<style>
	.field {
		display: grid;
		gap: var(--space-1);
	}
	.label {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	input {
		height: calc(34px * var(--control-scale));
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
		transition: border-color var(--dur-micro) var(--ease);
	}
	input::placeholder {
		color: var(--input-placeholder);
	}
	input:hover {
		border-color: var(--text-secondary);
	}
	input:focus-visible {
		border-color: var(--accent);
	}
</style>
