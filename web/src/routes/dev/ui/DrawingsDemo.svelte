<script lang="ts">
	// The style guide's drawings: one made from the Architecture starter in
	// the sample workspace, as a page shows it, with its editor a click away
	// (Edit or Full screen); the same at half the width; and the dialog
	// that asks how a new one starts.
	import Button from '#lib/components/Button.svelte';
	import DrawingPicker from '#lib/components/drawing/DrawingPicker.svelte';
	import DrawingView from '#lib/components/drawing/DrawingView.svelte';
	import { DRAWING_STARTERS } from '#lib/drawing.js';
	import { makeDrawing } from '#lib/sync/drawings.svelte.js';
	import { onMount } from 'svelte';
	import { PLANTING, SAMPLE_ME, SAMPLE_WS } from './sample';

	let id = $state('');
	let picking = $state(false);
	onMount(() => {
		const starter = DRAWING_STARTERS.find((s) => s.id === 'architecture')!;
		void makeDrawing(SAMPLE_WS, PLANTING, SAMPLE_ME, starter).then((made) => (id = made));
	});
</script>

<section>
	<h2>Drawings</h2>
	<p class="note">
		A drawing in a wiki page, drawn from its shapes in the theme's colors. Edit opens the editor
		full screen; Full screen opens it to look at only.
	</p>
	{#if id}
		<DrawingView
			ws={SAMPLE_WS}
			pageId={PLANTING}
			{id}
			name="Architecture"
			src=""
			size={null}
			editable={true}
			onclosed={() => {}}
		/>
		<h3>At half the width</h3>
		<DrawingView
			ws={SAMPLE_WS}
			pageId={PLANTING}
			{id}
			name=""
			src=""
			size={50}
			editable={false}
			onclosed={() => {}}
		/>
	{/if}
	<div class="row">
		<Button onclick={() => (picking = true)}>Insert a drawing</Button>
	</div>
	{#if picking}
		<DrawingPicker bind:open={picking} onpick={() => {}} />
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
