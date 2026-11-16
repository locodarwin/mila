<script lang="ts">
	import type { TokenKind } from '../../quickadd';
	import { TOKEN_LABELS } from '../../quickaddTokens';

	// A quick add token as quick add shows it, for help text: "Project #work".
	let { kind, text }: { kind: TokenKind; text: string } = $props();
</script>

<span class="token"><span class="kind">{TOKEN_LABELS[kind]}</span> <code>{text}</code></span>

<style>
	.token {
		display: inline-flex;
		align-items: baseline;
		gap: var(--space-1);
		padding: 0 var(--space-2);
		border-radius: var(--radius-full);
		background: var(--accent-soft);
		font-size: var(--text-xs);
		font-weight: 600;
		white-space: nowrap;
	}
	.kind {
		color: var(--text-secondary);
		font-weight: 500;
	}
	code {
		font-family: var(--font-mono);
		font-size: inherit;
	}
</style>
