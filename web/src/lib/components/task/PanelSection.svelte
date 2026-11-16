<script lang="ts">
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import type { Snippet } from 'svelte';
	import { layout } from '../../layout.svelte';
	import { collapse } from '../../motion';
	import { uuid } from '../../uuid';
	import HelpLink from '../help/HelpLink.svelte';

	// One section of the task panel, drawn the way the person's taskPanel
	// preference says: always showing (in a thin box, or open on the panel), or
	// as a line that says what is inside and opens in place. What the section
	// holds is mounted only while it shows.
	interface Props {
		/** Names the section in what this device remembers as open. */
		id: string;
		title: string;
		/** A small figure after the title, in the boxes layout. */
		count?: string;
		panel: 'boxes' | 'summary';
		/** Forms get a box in the boxes layout; reading areas stay open. */
		boxed?: boolean;
		/** The help topic the question mark after the title opens. */
		help?: { topic: string; section?: string; label: string };
		/** What is inside, on the closed line. */
		summary?: string;
		/** Opens the section, for something that needs what it holds. */
		wanted?: boolean;
		/** Leaves the section unnamed, when what it holds already goes by its title. */
		unnamed?: boolean;
		/** Buttons at the end of the heading, while the section shows. */
		tools?: Snippet;
		children: Snippet;
	}

	let {
		id,
		title,
		count,
		panel,
		boxed = false,
		help,
		summary = '',
		wanted = false,
		unnamed = false,
		tools,
		children
	}: Props = $props();

	const key = uuid();
	const named = $derived(unnamed ? undefined : `${key}-title`);
	const open = $derived(panel === 'boxes' || layout.sectionOpen(id));

	$effect(() => {
		if (wanted && panel === 'summary') layout.setSection(id, true);
	});
</script>

{#snippet helpMark()}
	{#if help}<HelpLink topic={help.topic} section={help.section} label={help.label} />{/if}
{/snippet}

{#if panel === 'boxes'}
	<section class:box={boxed} class:plain={!boxed} aria-labelledby={named}>
		<div class="head">
			<h3 id="{key}-title">
				{title}{#if count}<span class="count">{count}</span>{/if}
			</h3>
			{@render helpMark()}
			{#if tools}<span class="tools">{@render tools()}</span>{/if}
		</div>
		{@render children()}
	</section>
{:else}
	<section aria-labelledby={named}>
		<div class="fold-head">
			<h3>
				<button
					id="{key}-title"
					type="button"
					aria-expanded={open}
					aria-controls="{key}-body"
					onclick={() => layout.setSection(id, !open)}
					><ChevronDown size={14} aria-hidden="true" class={open ? 'chev open' : 'chev'} /><span
						class="name">{title}</span
					></button
				>
			</h3>
			{@render helpMark()}
			{#if open}
				{#if tools}<span class="tools">{@render tools()}</span>{/if}
			{:else if summary}
				<span class="summary" title={summary}>{summary}</span>
			{/if}
		</div>
		<div id="{key}-body">
			{#if open}
				<div class="fold-body" transition:collapse>{@render children()}</div>
			{/if}
		</div>
	</section>
{/if}

<style>
	h3 {
		min-width: 0;
		font-family: var(--font-ui);
		font-size: var(--text-sm);
		font-weight: 650;
	}
	.box,
	.plain {
		display: grid;
		gap: var(--space-2);
		min-width: 0;
	}
	.box {
		padding: var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
	}
	.plain {
		padding-inline: var(--space-3);
	}
	.head {
		display: flex;
		align-items: center;
		gap: var(--space-1);
		min-width: 0;
	}
	.tools {
		display: inline-flex;
		flex: none;
		align-items: center;
		gap: var(--space-1);
		margin-left: auto;
	}
	.count {
		margin-left: var(--space-2);
		font-size: var(--text-xs);
		font-weight: 500;
		color: var(--text-secondary);
	}
	/* Isolated, so what sits above the line's button stays under the panel's
	   sticky header when the panel scrolls. */
	.fold-head {
		position: relative;
		isolation: isolate;
		display: flex;
		align-items: center;
		gap: var(--space-1);
		min-height: max(var(--tap-min), 40px);
		padding-inline: var(--space-3);
	}
	.fold-head:hover {
		background: var(--surface-1);
	}
	.fold-head :global(.help-link),
	.fold-head .tools {
		position: relative;
		z-index: 1;
		flex: none;
	}
	.fold-head button {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-width: 0;
		padding: 0;
		border: none;
		background: none;
		color: var(--text-primary);
		font: inherit;
		cursor: pointer;
	}
	/* The whole line opens the section; the question mark and tools sit above this. */
	.fold-head button::after {
		position: absolute;
		inset: 0;
		content: '';
	}
	.fold-head button:focus-visible {
		outline: none;
	}
	.fold-head button:focus-visible::after {
		outline: 2px solid var(--accent);
		outline-offset: -2px;
	}
	.fold-head :global(.chev) {
		flex: none;
		color: var(--text-secondary);
		transform: rotate(-90deg);
		transition: transform var(--dur-micro) var(--ease);
	}
	.fold-head :global(.chev.open) {
		transform: none;
	}
	.name {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.summary {
		flex: 1 1 0;
		min-width: 0;
		padding-left: var(--space-3);
		overflow: hidden;
		color: var(--text-secondary);
		font-size: var(--text-xs);
		text-align: right;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.fold-body {
		display: grid;
		gap: var(--space-2);
		min-width: 0;
		padding: var(--space-1) var(--space-3) var(--space-3);
	}
</style>
