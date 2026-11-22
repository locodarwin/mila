<script lang="ts">
	// The style guide's diagrams: every kind a new one starts as, drawn by the
	// editor from its Markdown, and the states of a single diagram (a mistake
	// in its source, with its source showing; empty).
	import Button from '#lib/components/Button.svelte';
	import Diagram from '#lib/components/doc/Diagram.svelte';
	import DiagramPicker from '#lib/components/doc/DiagramPicker.svelte';
	import DocEditor from '#lib/components/doc/DocEditor.svelte';
	import { STARTERS } from '#lib/docs/diagrams.js';
	import { SAMPLE_WS } from './sample';

	const MARKDOWN = STARTERS.map((s) => `**${s.label}**\n\n\`\`\`mermaid\n${s.source}\n\`\`\``).join(
		'\n\n'
	);
	const BROKEN = 'flowchart LR\n  A[Request] --> B{Approved?\n  B --> C';
	let picking = $state(false);
</script>

<section>
	<h2>Diagrams</h2>
	<p class="note">
		A code block in the language mermaid draws as a diagram, in pages and task notes. Each kind the
		Diagram block starts as is below, in the theme's colors.
	</p>
	<DocEditor
		ws={SAMPLE_WS}
		target={{ kind: 'page', id: 'sample-diagrams' }}
		markdown={MARKDOWN}
		label="Diagrams"
		placeholder=""
	/>
	<h3>A mistake in the source</h3>
	<Diagram source={BROKEN} editing={true} editable={true} onedit={() => {}} />
	<h3>An empty diagram</h3>
	<Diagram source="" editing={false} editable={true} onedit={() => {}} />
	<div class="row">
		<Button onclick={() => (picking = true)}>Insert a diagram</Button>
	</div>
	{#if picking}
		<DiagramPicker bind:open={picking} onpick={() => {}} />
	{/if}
</section>

<style>
	section {
		display: grid;
		gap: var(--space-3);
	}
	.row {
		display: flex;
		gap: var(--space-2);
	}
</style>
