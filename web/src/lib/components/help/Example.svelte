<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { TokenKind } from '../../quickadd';
	import Token from './Token.svelte';

	// A line to type, what quick add makes of it and what comes out, for a help topic.
	interface Props {
		typed: string;
		tokens: { kind: TokenKind; text: string }[];
		result: string;
		children?: Snippet;
	}

	let { typed, tokens, result, children }: Props = $props();
</script>

<div class="example">
	<p class="label">Try typing this</p>
	<p class="line"><code>{typed}</code></p>
	<p class="chips">
		{#each tokens as t (t.text)}<Token kind={t.kind} text={t.text} />{/each}
	</p>
	<p class="result">{result}</p>
	{@render children?.()}
</div>

<style>
	.example {
		display: grid;
		gap: var(--space-2);
		justify-items: start;
		min-width: 0;
		padding: var(--space-4);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface-1);
	}
	.label {
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-weight: 600;
	}
	.line {
		width: 100%;
		padding: var(--space-2) var(--space-3);
		overflow-wrap: anywhere;
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
	}
	/* Above the code in a topic's text, which comes with a background of its own. */
	.example .line code {
		padding: 0;
		background: none;
		font-size: var(--text-sm);
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.result {
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
</style>
