<script lang="ts">
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
</script>

<main class="auth">
	<div class="card">
		<div class="brand">
			<img src="/favicon.svg" alt="" width="32" height="32" />
			<span>Mila</span>
		</div>
		{@render children()}
	</div>
</main>

<style>
	.auth {
		display: grid;
		place-items: center;
		min-height: 100dvh;
		padding: var(--space-5) var(--space-4);
		background: var(--surface-1);
	}
	.card {
		display: grid;
		gap: var(--space-5);
		width: min(400px, 100%);
		padding: var(--space-6);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface-0);
		box-shadow: var(--shadow-md);
	}
	.brand {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-family: var(--font-heading);
		font-size: var(--text-lg);
		font-weight: var(--heading-weight);
	}
	.card :global(h1) {
		font-size: var(--text-xl);
	}
	/* Focused when it replaces a form, to be read; it is not a control. */
	.card :global(h1:focus-visible) {
		box-shadow: none;
	}
	.card :global(form),
	.card :global(.stack) {
		display: grid;
		gap: var(--space-3);
	}
	.card :global(.lead) {
		color: var(--text-secondary);
	}
	.card :global(.error) {
		color: var(--danger);
		font-size: var(--text-sm);
	}
	.card :global(.divider) {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		font-size: var(--text-xs);
		color: var(--text-muted);
	}
	.card :global(.divider)::before,
	.card :global(.divider)::after {
		content: '';
		flex: 1;
		border-top: 1px solid var(--border);
	}
	.card :global(.button) {
		justify-content: center;
	}
</style>
