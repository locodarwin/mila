<script lang="ts">
	// The style guide's wiki toolbar: the real one on a sample page, as wide
	// as it gets and folded to fit a narrow bar.
	import DocEditor from '#lib/components/doc/DocEditor.svelte';
	import type { ToolbarParts } from '#lib/docs/toolbar.js';
	import { SAMPLE_WS } from './sample';

	let editor = $state<{ toolbar(): ToolbarParts | null }>();
	const parts = $derived(editor?.toolbar() ?? null);
</script>

<section>
	<h2>Wiki toolbar</h2>
	<p class="note">
		The toolbar in a wiki page's bar. It folds its rarer buttons into Insert and More formatting
		when the bar is short of room; the second one below has 300 px.
	</p>
	{#if parts}
		<div class="bar">
			<parts.Toolbar {...parts} room={2000} />
		</div>
		<div class="bar narrow">
			<parts.Toolbar {...parts} room={300} />
		</div>
	{/if}
	<DocEditor
		ws={SAMPLE_WS}
		target={{ kind: 'page', id: 'sample-toolbar' }}
		markdown=""
		label="Toolbar page"
		placeholder="Type here, then try the toolbar."
		page
		bind:this={editor}
	/>
</section>

<style>
	.bar {
		display: flex;
		align-items: center;
		min-height: calc(46px * var(--control-scale));
		margin-bottom: var(--space-2);
		padding: var(--space-1) var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-0);
	}
	.narrow {
		width: 340px;
	}
</style>
