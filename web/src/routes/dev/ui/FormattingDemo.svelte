<script lang="ts">
	// The style guide's formatting beyond Markdown: a
	// page that uses it, drawn by the editor from its Markdown; a view in a page,
	// from the sample workspace's shared board; and the two dialogs it brings.
	import Button from '#lib/components/Button.svelte';
	import DocEditor from '#lib/components/doc/DocEditor.svelte';
	import ReportEmbed from '#lib/components/doc/ReportEmbed.svelte';
	import ViewEmbed from '#lib/components/doc/ViewEmbed.svelte';
	import ViewPicker from '#lib/components/doc/ViewPicker.svelte';
	import PageType from '#lib/components/wiki/PageType.svelte';
	import { SAMPLE_WS } from './sample';

	const MARKDOWN = `<!-- mila:page font=source-serif -->

Everything goes in after the last frost, which is <mark data-color="yellow">usually the second week of May</mark>. <span data-size="large">Check</span> <span data-font="jetbrains-mono">Frost dates</span> first, and <mark data-color="pink">never</mark> <mark data-color="blue">water</mark> <mark data-color="green">at</mark> <mark data-color="gray">night</mark>.

<!-- mila:columns 2:1 -->

**Sun all day**

- Tomatoes
- Peppers

<!-- mila:column -->

**Part shade**

- Lettuce

<!-- mila:end -->

<details open>
<summary>Last year's mistakes</summary>

Planted the squash too close to the beans.

</details>

<details>
<summary>Soil mix, if you're filling a new bed</summary>

A third compost, a third topsoil, a third coarse sand.

</details>`;

	let picking = $state(false);
	let typing = $state(false);
	let font = $state<string | null>('source-serif');
	let size = $state<string | null>(null);
</script>

<section>
	<h2>Formatting beyond Markdown</h2>
	<p class="note">
		Highlights, fonts and sizes from the formatting bar; columns, foldable sections and a view from
		the / menu; and a page's own font from its menu. The page below is drawn from its Markdown.
	</p>
	<DocEditor
		ws={SAMPLE_WS}
		target={{ kind: 'page', id: 'sample-formatting' }}
		markdown={MARKDOWN}
		label="Formatted page"
		placeholder=""
	/>
	<h3>A view in a page</h3>
	<ViewEmbed ws={SAMPLE_WS} id="v-team" name="Garden board" />
	<ViewEmbed ws={SAMPLE_WS} id="v-gone" name="An old view" />
	<h3>A report in a page</h3>
	<ReportEmbed ws={SAMPLE_WS} id="v-report" chart="by-status" name="Garden report" />
	<div class="row">
		<Button onclick={() => (picking = true)}>Choose a view</Button>
		<Button onclick={() => (typing = true)}>Font and size</Button>
	</div>
	{#if picking}
		<ViewPicker ws={SAMPLE_WS} bind:open={picking} onpick={() => {}} />
	{/if}
	{#if typing}
		<PageType
			bind:open={typing}
			{font}
			{size}
			onchange={(f, s) => {
				font = f;
				size = s;
			}}
		/>
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
