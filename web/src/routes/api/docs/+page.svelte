<script lang="ts">
	import { resolve } from '$app/paths';
	import { blocks, groupByTag } from '#lib/apidocs.js';
	import Operation from '#lib/components/apidocs/Operation.svelte';
	import OperationList from '#lib/components/apidocs/OperationList.svelte';
	import { session } from '#lib/session.svelte.js';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const spec = $derived(data.spec);
	const groups = $derived(spec ? groupByTag(spec) : []);
</script>

<svelte:head><title>API reference · Mila</title></svelte:head>

<div class="docs">
	<header class="top">
		<a class="brand" href={resolve('/(app)')}>
			<img src="/favicon.svg" alt="" width="24" height="24" />
			<span>Mila</span>
		</a>
		<span class="who">
			{#if session.user}
				Signed in as {session.user.name}
			{:else}
				<a class="link" href="{resolve('/(auth)/login')}?next=%2Fapi%2Fdocs">Sign in</a> to try requests
			{/if}
		</span>
	</header>

	{#if spec}
		<OperationList {groups} />
	{/if}

	<main>
		<h1>Mila API</h1>
		{#if !spec}
			<p class="lead">
				The API reference comes from the server, which cannot be reached now. It needs a connection;
				try again once you are online.
			</p>
		{:else}
			<div class="intro">
				{#each blocks(spec.info.description ?? '') as b, i (i)}
					{#if b.kind === 'p'}
						<p>{b.text}</p>
					{:else}
						<ul>
							{#each b.items as item (item)}<li>{item}</li>{/each}
						</ul>
					{/if}
				{/each}
				<p>
					Every path below starts with <code>/api/v1</code>. Try it sends a request from this page
					as you, with your session, so what it changes is changed for real.
				</p>
			</div>
			{#each groups as g (g.tag)}
				<section aria-labelledby="tag-{g.tag}">
					<h2 id="tag-{g.tag}">{g.tag}</h2>
					{#if g.description}<p class="about">{g.description}</p>{/if}
					<div class="ops">
						{#each g.operations as op (op.id)}
							<Operation {spec} {op} />
						{/each}
					</div>
				</section>
			{/each}
		{/if}
	</main>
</div>

<style>
	.docs {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		min-height: 100dvh;
		background: var(--surface-1);
	}
	.top {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-2) var(--space-4);
		padding: var(--space-3) var(--space-4);
		border-bottom: var(--border-width) solid var(--border);
		background: var(--surface-0);
	}
	.brand {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		min-height: var(--tap-min);
		font-family: var(--font-heading);
		font-weight: var(--heading-weight);
	}
	.who {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.link {
		color: var(--accent-text);
		text-decoration: underline;
	}
	.docs > :global(nav) {
		padding: var(--space-2) var(--space-3);
		border-bottom: var(--border-width) solid var(--border);
		background: var(--surface-0);
	}
	main {
		display: grid;
		gap: var(--space-5);
		align-content: start;
		min-width: 0;
		padding: var(--space-5) var(--space-4) var(--space-7);
	}
	h1 {
		font-size: var(--text-2xl);
	}
	.intro {
		display: grid;
		gap: var(--space-3);
		max-width: 72ch;
	}
	.intro ul {
		margin: 0;
		padding-left: var(--space-5);
	}
	.lead,
	.about {
		color: var(--text-secondary);
	}
	section {
		display: grid;
		gap: var(--space-2);
	}
	h2 {
		font-size: var(--text-lg);
	}
	.ops {
		display: grid;
		gap: var(--space-2);
	}
	@media (min-width: 900px) {
		.docs {
			grid-template-columns: 260px minmax(0, 1fr);
			grid-template-rows: auto 1fr;
		}
		.top {
			grid-column: 1 / -1;
		}
		.docs > :global(nav) {
			position: sticky;
			top: 0;
			align-self: start;
			max-height: 100dvh;
			overflow-y: auto;
			border-bottom: 0;
			border-right: var(--border-width) solid var(--border);
			background: var(--surface-1);
		}
		main {
			padding: var(--space-6) var(--space-6) var(--space-7);
		}
	}
</style>
