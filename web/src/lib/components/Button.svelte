<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';

	interface Props extends HTMLButtonAttributes {
		variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
		size?: 'sm' | 'md';
		children: Snippet;
	}

	let { variant = 'secondary', size = 'md', type = 'button', children, ...rest }: Props = $props();
</script>

<button class="button {variant} {size}" {type} {...rest}>
	{@render children()}
</button>

<style>
	.button {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		height: calc(32px * var(--control-scale));
		padding: 0 var(--space-3);
		border: var(--border-width) solid transparent;
		border-radius: var(--button-radius);
		font-size: var(--text-sm);
		font-weight: 550;
		cursor: pointer;
		white-space: nowrap;
		transition:
			background-color var(--dur-micro) var(--ease),
			border-color var(--dur-micro) var(--ease),
			transform var(--dur-micro) var(--ease-spring);
	}
	.button:active:not(:disabled) {
		transform: scale(calc(1 - 0.03 * var(--motion)));
	}
	.button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.sm {
		height: calc(26px * var(--control-scale));
		padding: 0 var(--space-2);
		font-size: var(--text-xs);
	}
	.primary {
		background: var(--button-primary-bg);
		color: var(--button-primary-text);
	}
	.primary:hover:not(:disabled) {
		background: var(--button-primary-hover);
	}
	.secondary {
		background: var(--button-secondary-bg);
		border-color: var(--button-secondary-border);
		color: var(--button-secondary-text);
	}
	.secondary:hover:not(:disabled) {
		background: var(--button-ghost-hover);
	}
	.ghost {
		background: transparent;
		color: var(--text-secondary);
	}
	.ghost:hover:not(:disabled) {
		background: var(--button-ghost-hover);
		color: var(--text-primary);
	}
	.danger {
		background: transparent;
		border-color: var(--button-danger);
		color: var(--button-danger);
	}
	.danger:hover:not(:disabled) {
		background: color-mix(in oklch, var(--button-danger) 10%, var(--surface-0));
	}
</style>
