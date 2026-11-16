<script lang="ts">
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
</script>

<!-- Help's pages share one column, as wide as the panel, and the look of their text. -->
<div class="help">{@render children()}</div>

<style>
	.help :global(.crumbs a) {
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	.help :global(.crumbs a:hover) {
		color: var(--text-primary);
	}
	.help :global(.crumbs .sep) {
		margin-left: var(--space-2);
		color: var(--text-muted);
	}
	.help :global(.column) {
		display: grid;
		gap: var(--space-4);
		padding: var(--space-5);
	}
	.help :global(.prose) {
		display: grid;
		gap: var(--space-3);
		line-height: 1.6;
	}
	.help :global(.prose h2) {
		margin-top: var(--space-4);
		font-size: var(--text-lg);
		scroll-margin-top: var(--space-4);
	}
	/* Focused when a link lands on it; the underline marks the place. */
	.help :global(.prose h2:focus-visible) {
		outline: none;
		box-shadow: none;
		text-decoration: underline;
		text-decoration-color: var(--accent);
		text-decoration-thickness: 2px;
	}
	/* Two pictures to compare, one under the other once there's no room. */
	.help :global(.prose .pictures) {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
		gap: var(--space-3);
	}
	.help :global(.prose ul) {
		display: grid;
		gap: var(--space-2);
		margin: 0;
		padding-left: var(--space-5);
	}
	.help :global(.prose code) {
		padding: 0 var(--space-1);
		border-radius: var(--radius-sm);
		background: var(--surface-2);
		font-family: var(--font-mono);
		font-size: 0.9em;
	}
	.help :global(.prose a),
	.help :global(.link) {
		color: var(--accent-text);
		font-weight: 550;
	}
	/* Links inside text are underlined, not told apart by color alone. */
	.help :global(.prose a) {
		text-decoration: underline;
	}
	.help :global(.hint) {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	@media (max-width: 720px) {
		.help :global(.column) {
			padding: var(--space-4);
		}
	}
</style>
