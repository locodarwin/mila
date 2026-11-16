<script lang="ts">
	// The style guide's wiki: the Garden project's, on the sample workspace
	// (sample.ts), with its tree of pages beside the page shown.
	import Button from '#lib/components/Button.svelte';
	import PageComments from '#lib/components/wiki/PageComments.svelte';
	import PageHistory from '#lib/components/wiki/PageHistory.svelte';
	import { CommentPins } from '#lib/docs/commentPins.svelte.js';
	import Wiki from '#lib/components/wiki/Wiki.svelte';
	import WikiContents from '#lib/components/wiki/WikiContents.svelte';
	import { GARDEN_WIKI, SAMPLE_ME, SAMPLE_PROJECT, SAMPLE_WS } from './sample';

	const samplePins = new CommentPins();
	samplePins.active = 'th-beans';
	samplePins.ready = true;
	let pageId = $state<string | null>(null);
	let historyOpen = $state(false);
	let sampleDoc = $state<HTMLElement>();
	const hour = 3600_000;
	const history = [
		{
			id: 'v3',
			actorId: SAMPLE_ME,
			actorName: 'Sam Rivera',
			at: new Date(Date.now() - hour).toISOString(),
			body: 'Beds: tomatoes along the south fence, beans where the squash was.'
		},
		{
			id: 'v2',
			actorId: 'u-pia',
			actorName: 'Pia',
			at: new Date(Date.now() - 26 * hour).toISOString(),
			body: 'Beds: tomatoes along the fence, squash again.'
		},
		{
			id: 'v1',
			actorId: '',
			actorName: '',
			at: new Date(Date.now() - 80 * hour).toISOString(),
			body: 'Beds: tomatoes.'
		}
	];
</script>

<section>
	<h2>Wiki</h2>
	<p class="note">
		A project's wiki, on its Wiki tab: the tree of pages beside the page, which folds away, and the
		page's path, people, Markdown view and menu above it.
	</p>
	<p class="note">
		Links typed with [[ show what they point at; a page ends with what links to it. Its history
		shows each version's changes.
	</p>
	<p class="note">
		In the app, selecting words offers Comment. A thread's words are highlighted, and its comments
		are in the Comments tab beside the page, or over its edge when there is no room. The panel is
		below; a page with no live document yet, as here, draws no highlights.
	</p>
	<div>
		<Button size="sm" onclick={() => (historyOpen = true)}>Open page history</Button>
	</div>
	<PageHistory
		ws={SAMPLE_WS}
		pageId={GARDEN_WIKI}
		title="Garden"
		bind:open={historyOpen}
		sample={history}
	/>
	<div class="wiki-demo">
		<Wiki
			ws={SAMPLE_WS}
			projectId={SAMPLE_PROJECT}
			{pageId}
			heading="h2"
			onopen={(id) => {
				pageId = id;
			}}
		/>
	</div>
	<p class="note">
		The page's corner has a grip for its width. With 200 px left over beside the page, a panel there
		switches between the wiki's pages and the page's contents, as below.
	</p>
	<div class="contents-demo">
		<div class="doc-body" bind:this={sampleDoc}>
			<h3>This year</h3>
			<h4>South fence</h4>
			<h3>Suppliers</h3>
		</div>
		<WikiContents root={sampleDoc} scroller={sampleDoc} barHeight={0} source={false} />
	</div>
	<div class="comments-demo">
		<PageComments
			ws={SAMPLE_WS}
			pageId={GARDEN_WIKI}
			projectId={SAMPLE_PROJECT}
			pins={samplePins}
		/>
	</div>
</section>

<style>
	.wiki-demo {
		height: 420px;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		overflow: hidden;
	}
	.comments-demo {
		max-width: 340px;
		padding: var(--space-2);
		background: var(--surface-1);
	}
	.contents-demo {
		display: grid;
		grid-template-columns: 1fr 240px;
		gap: var(--space-4);
	}
	/* As the style guide's own sections. */
	section {
		display: grid;
		gap: var(--space-4);
		padding: var(--space-6) 0;
		border-bottom: var(--border-width) solid var(--border);
	}
	h2 {
		font-size: var(--text-xl);
	}
	.note {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
</style>
