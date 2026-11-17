<script lang="ts">
	// A wiki page's history: a version for each
	// editing session, by who and when, what each changed with the words
	// marked, and Restore. History lives on the server, so it needs a
	// connection. Loaded only when opened.
	import { api, call, messageOf, type Schemas } from '../../api/client';
	import { formatClock, formatDate } from '../../dates';
	import { restorePageBody } from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import { toasts } from '../../toasts.svelte';
	import { wordDiff } from '../../wordDiff';
	import Announcer from '../Announcer.svelte';

	interface Props {
		ws: string;
		pageId: string;
		title: string;
		/** Whether the dialog is open; closing it sets this false. */
		open: boolean;
		/** Versions to show instead of the server's: the style guide's. */
		sample?: Schemas['PageVersion'][];
	}

	let { ws, pageId, title, open = $bindable(), sample }: Props = $props();
	let dialog = $state<HTMLDialogElement>();
	let versions = $state<Schemas['PageVersion'][] | null>(null);
	let error = $state('');
	let chosen = $state(0);

	$effect(() => {
		if (open && dialog && !dialog.open) dialog.showModal();
		else if (!open && dialog?.open) dialog.close();
	});

	$effect(() => {
		if (!open) return;
		void pageId;
		versions = null;
		error = '';
		chosen = 0;
		if (sample) {
			versions = sample;
			return;
		}
		call(
			api.GET('/workspaces/{workspaceId}/pages/{pageId}/versions', {
				params: { path: { workspaceId: ws, pageId } }
			})
		)
			.then((v) => (versions = v))
			.catch((err) => (error = messageOf(err)));
	});

	const when = (at: string) => {
		const d = new Date(at);
		return `${formatDate(d)}, ${formatClock(d)}`;
	};
	const who = (v: Schemas['PageVersion']) =>
		v.actorId ? v.actorName || 'Someone' : 'Before its history began';

	const shown = $derived(versions?.[chosen]);
	const changes = $derived(
		versions && shown ? wordDiff(versions[chosen + 1]?.body ?? '', shown.body) : []
	);
	const current = $derived(engine.replica(ws)?.page(pageId)?.body ?? '');

	async function restore() {
		const v = shown;
		if (!v) return;
		const before = current;
		const { undo } = engine.track(() => restorePageBody(ws, pageId, v.body));
		open = false;
		toasts.show(`Restored the version from ${when(v.at)}`, {
			onundo: async () => {
				if (!(await undo())) await restorePageBody(ws, pageId, before);
			}
		});
	}
</script>

<dialog
	bind:this={dialog}
	class="history"
	aria-labelledby="page-history-title"
	onclose={() => (open = false)}
>
	<h2 id="page-history-title">History of {title}</h2>
	{#if error}
		<p class="hint" role="alert">{error}</p>
	{:else if !versions}
		<p class="hint">Loading…</p>
	{:else if !versions.length}
		<p class="hint">Nothing has been written on this page yet.</p>
	{:else}
		<div class="split">
			<ul class="versions" aria-label="Versions">
				{#each versions as v, i (v.id)}
					<li>
						<button
							type="button"
							aria-current={i === chosen ? 'true' : undefined}
							onclick={() => (chosen = i)}
						>
							<span class="who">{who(v)}</span>
							<span class="when">{when(v.at)}</span>
						</button>
					</li>
				{/each}
			</ul>
			{#if shown}
				<section class="version" aria-label={`The version from ${when(shown.at)}`}>
					<p class="hint">
						{#if chosen === versions.length - 1}The first version.{:else}What changed since the
							version before: added words are underlined, removed ones struck through.{/if}
					</p>
					<pre class="diff">{#each changes as c, i (i)}{#if c.kind === 'added'}<ins
									><span class="visually-hidden">Added: </span>{c.text}</ins
								>{:else if c.kind === 'removed'}<del
									><span class="visually-hidden">Removed: </span>{c.text}</del
								>{:else}{c.text}{/if}{/each}</pre>
					{#if shown.body === current}
						<p class="hint">This is how the page reads now.</p>
					{:else}
						<button type="button" class="restore" onclick={restore}>Restore this version</button>
					{/if}
				</section>
			{/if}
		</div>
	{/if}
	<form method="dialog"><button>Close</button></form>
	{#if open}<Announcer inDialog />{/if}
</dialog>

<style>
	.history {
		width: min(860px, calc(100vw - 32px));
		max-height: 85vh;
		padding: var(--space-5);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	.history[open] {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	.history::backdrop {
		/* With the value as written, for browsers whose ::backdrop does not inherit custom properties. */
		background: var(--scrim, oklch(0 0 0 / 0.25));
	}
	h2 {
		font-size: var(--text-xl);
		overflow-wrap: anywhere;
	}
	.hint {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.split {
		display: grid;
		grid-template-columns: minmax(180px, 240px) minmax(0, 1fr);
		gap: var(--space-4);
		min-height: 0;
		overflow: hidden;
	}
	.versions {
		display: grid;
		align-content: start;
		gap: 2px;
		margin: 0;
		padding: 0;
		overflow-y: auto;
		list-style: none;
	}
	.versions button {
		display: grid;
		width: 100%;
		min-height: max(var(--tap-min), 40px);
		padding: var(--space-1) var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-primary);
		text-align: left;
		cursor: pointer;
	}
	.versions button:hover {
		background: var(--surface-2);
	}
	.versions button[aria-current='true'] {
		background: var(--accent-soft);
	}
	.who {
		font-size: var(--text-sm);
		font-weight: 600;
	}
	.when {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.version {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		min-height: 0;
	}
	.diff {
		flex: 1;
		min-height: 120px;
		margin: 0;
		padding: var(--space-3);
		overflow: auto;
		border-radius: var(--radius-md);
		background: var(--surface-1);
		font-family: var(--font-mono);
		font-size: var(--text-sm);
		line-height: 1.6;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	ins {
		background: var(--accent-soft);
		color: var(--text-primary);
		text-decoration: underline;
	}
	del {
		color: var(--text-secondary);
		text-decoration: line-through;
	}
	.restore {
		align-self: flex-start;
	}
	@media (max-width: 640px) {
		.split {
			grid-template-columns: 1fr;
		}
	}
</style>
