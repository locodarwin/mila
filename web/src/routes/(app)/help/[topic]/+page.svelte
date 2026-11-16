<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import EmptyState from '#lib/components/EmptyState.svelte';
	import ChevronLeft from '@lucide/svelte/icons/chevron-left';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import { tick } from 'svelte';
	import { groupOf, neighbors, topic as topicOf } from '#lib/help/index.js';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const t = $derived(data.topic);
	const Body = $derived(data.body);
	const related = $derived((t?.related ?? []).flatMap((slug) => topicOf(slug) ?? []));
	const group = $derived(t && groupOf(t.slug));
	const beside = $derived(t ? neighbors(t.slug) : {});

	/**
	 * Brings a section's heading into view and gives it focus, so reading
	 * and Tab go on from there. A link from another page or the palette
	 * changes the URL without a hashchange, so this runs after each
	 * navigation too, once the topic is drawn.
	 */
	function toSection(hash: string | undefined) {
		const id = hash ? decodeURIComponent(hash.slice(1)) : '';
		const heading = id ? document.getElementById(id) : null;
		if (!(heading instanceof HTMLElement)) return;
		heading.scrollIntoView({ block: 'start' });
		heading.focus({ preventScroll: true });
	}

	afterNavigate(({ to }) => toSection(to?.url.hash));
	$effect(() => {
		const back = () => toSection(location.hash);
		addEventListener('hashchange', back);
		return () => removeEventListener('hashchange', back);
	});

	const sectionHref = (id: string) => `#${id}`;

	// The section being read, for the list beside the topic: the last whose heading has
	// reached the top. One just picked counts as read, though it may be too near the
	// end of the page to get there.
	const READ_LINE = 120;
	let article = $state<HTMLElement>();
	let reading = $state('');
	let pickedAt = 0;

	function pick(id: string) {
		pickedAt = performance.now();
		reading = id;
		requestAnimationFrame(() => toSection(`#${id}`));
	}

	$effect(() => {
		const sections = t?.sections ?? [];
		const root = article;
		reading = sections[0]?.id ?? '';
		if (!root) return;
		let frame = 0;
		const update = () => {
			frame = 0;
			if (performance.now() - pickedAt < 300) return;
			let now = sections[0]?.id ?? '';
			for (const s of sections) {
				const heading = root.querySelector(`#${CSS.escape(s.id)}`);
				if (heading && heading.getBoundingClientRect().top <= READ_LINE) now = s.id;
			}
			reading = now;
		};
		const onScroll = () => {
			if (!frame) frame = requestAnimationFrame(update);
		};
		// The page's scroller depends on the layout, and scrolling doesn't bubble.
		document.addEventListener('scroll', onScroll, { capture: true, passive: true });
		void tick().then(update);
		return () => {
			document.removeEventListener('scroll', onScroll, { capture: true });
			if (frame) cancelAnimationFrame(frame);
		};
	});
</script>

<svelte:head><title>{t?.title ?? 'Help'} · Mila</title></svelte:head>

{#snippet contents()}
	<ul>
		{#each t?.sections ?? [] as s (s.id)}
			<li>
				<!-- eslint-disable svelte/no-navigation-without-resolve -- a section of this page -->
				<a
					href={sectionHref(s.id)}
					aria-current={reading === s.id ? 'location' : undefined}
					onclick={() => pick(s.id)}>{s.title}</a
				>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			</li>
		{/each}
	</ul>
{/snippet}

{#if t && Body}
	<header class="page">
		<nav class="crumbs" aria-label="Breadcrumb">
			<a href={resolve('/(app)/help')}>Help</a><span class="sep" aria-hidden="true">/</span>
			<span class="here" aria-current="page">{t.title}</span>
		</nav>
	</header>
	<div class="room">
		<div class="topic">
			<div class="reading">
				<header class="title">
					{#if group}<p class="kicker">{group.title}</p>{/if}
					<h1>{t.title}</h1>
					<p class="lede">{t.summary}</p>
				</header>
				<details class="toc narrow">
					<summary>On this page</summary>
					<nav aria-label="On this page">{@render contents()}</nav>
				</details>
				<article class="prose" bind:this={article}>
					<Body />
				</article>
				{#if related.length}
					<nav class="related" aria-label="Related topics">
						<h2>Keep reading</h2>
						<ul>
							{#each related as r (r.slug)}
								<li>
									<a href={resolve('/(app)/help/[topic]', { topic: r.slug })}>
										<span class="link">{r.title}</span>
										<span class="hint">{r.summary}</span>
									</a>
								</li>
							{/each}
						</ul>
					</nav>
				{/if}
				<nav class="pager" aria-label="Previous and next topics">
					{#if beside.previous}
						<a href={resolve('/(app)/help/[topic]', { topic: beside.previous.slug })} rel="prev">
							<ChevronLeft size={16} />
							<span
								><span class="hint">Previous</span><span class="link">{beside.previous.title}</span
								></span
							>
						</a>
					{:else}
						<a href={resolve('/(app)/help')}>
							<ChevronLeft size={16} />
							<span><span class="hint">Previous</span><span class="link">Help</span></span>
						</a>
					{/if}
					{#if beside.next}
						<a
							class="next"
							href={resolve('/(app)/help/[topic]', { topic: beside.next.slug })}
							rel="next"
						>
							<span
								><span class="hint">Next</span><span class="link">{beside.next.title}</span></span
							>
							<ChevronRight size={16} />
						</a>
					{/if}
				</nav>
			</div>
			<nav class="toc wide" aria-label="On this page">
				<h2 class="toc-title">On this page</h2>
				{@render contents()}
			</nav>
		</div>
	</div>
{:else}
	<header class="page"><h1>Help</h1></header>
	<EmptyState
		title="This help topic isn't here"
		hint="It may have moved. Help lists every topic there is."
	/>
	<p class="back"><a class="link" href={resolve('/(app)/help')}>Go to Help</a></p>
{/if}

<style>
	.here {
		color: var(--text-primary);
		font-size: var(--text-sm);
		font-weight: 600;
	}
	/* The list beside the topic needs the room the page has, sidebar or no sidebar. */
	.room {
		container-type: inline-size;
	}
	.topic {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 190px;
		align-items: start;
		gap: var(--space-6);
		padding: var(--space-5);
	}
	.reading {
		display: grid;
		gap: var(--space-4);
		min-width: 0;
	}
	.title {
		display: grid;
		gap: var(--space-2);
	}
	.kicker {
		color: var(--accent-text);
		font-size: var(--text-sm);
		font-weight: 600;
	}
	h1 {
		font-size: var(--text-2xl);
	}
	.lede {
		color: var(--text-secondary);
		font-size: var(--text-lg);
		line-height: 1.4;
	}
	.toc ul {
		display: grid;
		gap: var(--space-1);
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.toc a {
		display: block;
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	.toc a:hover {
		color: var(--text-primary);
		text-decoration: underline;
	}
	.toc-title {
		margin-bottom: var(--space-2);
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.toc.wide {
		position: sticky;
		top: var(--space-5);
	}
	.toc.wide ul {
		border-left: var(--border-width) solid var(--border);
	}
	.toc.wide a {
		margin-left: calc(var(--border-width) * -1);
		padding: 2px 0 2px var(--space-3);
		border-left: 2px solid transparent;
	}
	.toc.wide a[aria-current] {
		border-left-color: var(--accent);
		color: var(--accent-text);
		font-weight: 600;
	}
	.toc.narrow {
		display: none;
	}
	.toc.narrow summary {
		font-size: var(--text-sm);
		font-weight: 600;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.toc.narrow nav {
		padding: var(--space-2) var(--space-4) 0;
	}
	.related,
	.pager {
		padding-top: var(--space-4);
		border-top: var(--border-width) solid var(--border);
	}
	.related {
		display: grid;
		gap: var(--space-2);
	}
	.related h2 {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.related ul {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
		gap: var(--space-3);
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.related a,
	.pager a {
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
	}
	.related a {
		display: grid;
		align-content: start;
		gap: 2px;
		height: 100%;
		padding: var(--space-3);
	}
	.related a:hover,
	.pager a:hover {
		background: var(--surface-2);
	}
	.pager {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--space-3);
	}
	.pager a {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-width: 0;
		min-height: var(--tap-min);
		padding: var(--space-2) var(--space-3);
		color: var(--text-secondary);
	}
	.pager a > span {
		display: grid;
		min-width: 0;
	}
	.pager .hint {
		font-size: var(--text-xs);
	}
	.pager .next {
		grid-column: 2;
		justify-content: flex-end;
		text-align: right;
	}
	.pager :global(svg) {
		flex: none;
	}
	.back {
		text-align: center;
	}
	@container (max-width: 860px) {
		.topic {
			grid-template-columns: minmax(0, 1fr);
			gap: var(--space-4);
		}
		.toc.wide {
			display: none;
		}
		.toc.narrow {
			display: block;
		}
	}
	@media (max-width: 720px) {
		.topic {
			padding: var(--space-4);
		}
	}
</style>
