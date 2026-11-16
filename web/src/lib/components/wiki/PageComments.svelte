<script lang="ts">
	// The comments on a wiki page (docs/PLAN.md section 3), beside it or over its
	// edge, or in a sheet on a phone: the open threads in the order they come in
	// the page, those whose words are gone after them, and the resolved below.
	// Loaded when the Comments tab or a thread first shows.
	import { tick } from 'svelte';
	import { DRAFT, type CommentPins } from '../../docs/commentPins.svelte';
	import { referencesToStored, toStored, type Person, type Reference } from '../../mentions';
	import { session } from '../../session.svelte';
	import { engine } from '../../sync/engine.svelte';
	import type { PageComment } from '../../sync/entities';
	import { startThread } from '../../sync/pageComments';
	import Button from '../Button.svelte';
	import Kbd from '../Kbd.svelte';
	import { referenceSource } from '../task/commentSources';
	import MentionInput from '../task/MentionInput.svelte';
	import PageThread from './PageThread.svelte';

	interface Props {
		ws: string;
		pageId: string;
		projectId: string | null;
		pins: CommentPins;
	}

	let { ws, pageId, projectId, pins }: Props = $props();

	const replica = $derived(engine.replica(ws));
	const me = $derived(session.user?.id ?? '');
	const people = $derived(session.mentionable(ws, projectId));
	const references = $derived(referenceSource(ws));

	const byThread = $derived.by(() => {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- made afresh by each derivation
		const out = new Map<string, PageComment[]>();
		for (const c of replica?.pageComments(pageId) ?? []) {
			const list = out.get(c.threadId);
			if (list) list.push(c);
			else out.set(c.threadId, [c]);
		}
		return out;
	});
	// A thread whose comments have all been deleted is no longer shown.
	const threads = $derived((replica?.pageThreads(pageId) ?? []).filter((t) => byThread.has(t.id)));
	const open = $derived.by(() => {
		const place = new Map(pins.order.map((id, i) => [id, i]));
		const at = (id: string) => place.get(id) ?? Infinity;
		return threads.filter((t) => !t.resolvedAt).sort((a, b) => at(a.id) - at(b.id));
	});
	const resolved = $derived(threads.filter((t) => t.resolvedAt));

	async function pick(id: string) {
		pins.active = id;
		pins.reveal?.(id);
	}

	// A thread picked elsewhere (its highlight, a notification) comes into view here.
	let list = $state<HTMLElement>();
	$effect(() => {
		const id = pins.active;
		if (!id) return;
		void tick().then(() => {
			list
				?.querySelector(`[data-thread-card="${CSS.escape(id)}"]`)
				?.scrollIntoView({ block: 'nearest' });
		});
	});

	// The first comment of a thread being begun on the words picked.
	let body = $state('');
	let picked = $state<Person[]>([]);
	let refs = $state<Reference[]>([]);
	function clear() {
		body = '';
		picked = [];
		refs = [];
		pins.draft = null;
		if (pins.active === DRAFT) pins.active = null;
	}
	async function send(e?: Event) {
		e?.preventDefault();
		const pin = pins.draft;
		if (!pin || !body.trim() || !me) return;
		const text = toStored(referencesToStored(body, refs), picked, people);
		clear();
		pins.active = await startThread(ws, pageId, me, pin, text);
	}
</script>

<div class="comments" bind:this={list}>
	{#if pins.draft}
		<form class="new" aria-label="New comment" onsubmit={send}>
			<p class="quote">{pins.draft.quote}</p>
			<MentionInput
				label="Your comment on the selected words"
				placeholder="Write a comment. @ mentions someone, / refers to a task or wiki page"
				bind:value={body}
				bind:picked
				bind:refs
				{people}
				{references}
				autofocus
				onkeydown={(e) => {
					if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') void send(e);
					else if (e.key === 'Escape' && !e.defaultPrevented) {
						e.preventDefault();
						clear();
					}
				}}
			/>
			<div class="actions">
				<Button
					title="Post your comment"
					type="submit"
					size="sm"
					variant="primary"
					disabled={!body.trim()}>Comment</Button
				>
				<Button title="Drop this comment" size="sm" variant="ghost" onclick={clear}>Cancel</Button>
			</div>
		</form>
	{/if}
	{#if !open.length && !pins.draft}
		<p class="empty">
			Nothing to discuss yet. Select some words in the page and choose Comment, or press <Kbd
				command="doc.comment"
			/>.
		</p>
	{/if}
	{#each open as t (t.id)}
		<PageThread
			{ws}
			thread={t}
			comments={byThread.get(t.id) ?? []}
			{people}
			active={pins.active === t.id}
			gone={pins.ready && pins.gone.has(t.id)}
			onpick={pick}
		/>
	{/each}
	{#if resolved.length}
		<h3>Resolved</h3>
		{#each resolved as t (t.id)}
			<PageThread
				{ws}
				thread={t}
				comments={byThread.get(t.id) ?? []}
				{people}
				active={pins.active === t.id}
				gone={false}
				onpick={pick}
			/>
		{/each}
	{/if}
</div>

<style>
	.comments {
		display: grid;
		gap: var(--space-2);
		min-width: 0;
	}
	.new {
		display: grid;
		gap: var(--space-2);
		min-width: 0;
		padding: var(--space-3);
		border: var(--border-width) solid var(--accent);
		border-radius: var(--radius-md);
		background: var(--surface-0);
	}
	.quote {
		padding-left: var(--space-2);
		border-left: 2px solid var(--border-strong);
		color: var(--text-secondary);
		font-size: var(--text-xs);
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		display: -webkit-box;
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		overflow: hidden;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.empty {
		padding: var(--space-2);
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	h3 {
		margin-top: var(--space-2);
		padding: 0 var(--space-1);
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-weight: 600;
	}
</style>
