<script lang="ts">
	import { menu } from '../../contextMenu.svelte';
	import Paperclip from '@lucide/svelte/icons/paperclip';
	import X from '@lucide/svelte/icons/x';
	import { hasFile } from '../../fileCache';
	import { deleteAttachment } from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import type { Attachment } from '../../sync/entities';
	import { toasts } from '../../toasts.svelte';
	import { fileUrl, uploadFile } from '../../uploads';

	let { ws, taskId, items }: { ws: string; taskId: string; items: Attachment[] } = $props();
	let busy = $state(false);

	// Offline, a file opens only if this device has opened it before.
	// The browser knows at once; the engine, when a request or its socket fails.
	let online = $state(navigator.onLine);
	const away = $derived(!online || engine.state === 'offline');
	let held = $state.raw<Set<string> | null>(null);
	$effect(() => {
		held = null;
		if (!away) return;
		const ids = items.map((a) => a.id);
		let stale = false;
		void Promise.all(ids.map((id) => hasFile(fileUrl(ws, id)))).then((found) => {
			if (!stale) held = new Set(ids.filter((_, i) => found[i]));
		});
		return () => {
			stale = true;
		};
	});

	function size(n: number) {
		if (n < 1024) return `${n} B`;
		if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`;
		return `${(n / 1024 / 1024).toFixed(1)} MB`;
	}

	async function upload(e: Event & { currentTarget: HTMLInputElement }) {
		const file = e.currentTarget.files?.[0];
		e.currentTarget.value = '';
		if (!file) return;
		busy = true;
		try {
			await uploadFile(ws, { kind: 'task', id: taskId }, file);
		} catch (err) {
			toasts.error(err instanceof Error ? err.message : 'The upload failed.');
		} finally {
			busy = false;
		}
	}
</script>

<svelte:window ononline={() => (online = true)} onoffline={() => (online = false)} />

{#if items.length}
	<ul aria-label="Attachments">
		{#each items as a (a.id)}
			<li use:menu={() => import('../../menus').then((m) => m.fileMenu(ws, a))}>
				<Paperclip size={14} />
				{#if held && !held.has(a.id)}
					<span class="away">{a.filename}</span>
					<span class="size">Needs a connection</span>
				{:else}
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- a file download, not a page -->
					<a href={fileUrl(ws, a.id)} target="_blank" rel="noopener">{a.filename}</a>
					<span class="size">{size(a.size)}</span>
				{/if}
				<button
					type="button"
					aria-label={`Remove ${a.filename}`}
					onclick={() => void deleteAttachment(ws, a.id)}
				>
					<X size={14} />
				</button>
			</li>
		{/each}
	</ul>
{/if}
<label class="upload" class:busy>
	<input type="file" class="visually-hidden" onchange={upload} disabled={busy} />
	<Paperclip size={14} />{busy ? 'Uploading…' : 'Add a file'}
</label>

<style>
	ul {
		display: grid;
		gap: 2px;
		margin: 0 0 var(--space-2);
		padding: 0;
		list-style: none;
	}
	.away {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	li {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		color: var(--text-secondary);
	}
	a {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		color: var(--text-primary);
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	a:hover {
		text-decoration: underline;
	}
	.size {
		font-size: var(--text-xs);
	}
	button {
		display: grid;
		place-items: center;
		width: 24px;
		height: 24px;
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-muted);
		cursor: pointer;
	}
	button:hover {
		background: var(--surface-2);
		color: var(--danger);
	}
	.upload {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-1) var(--space-2);
		border-radius: var(--radius-md);
		color: var(--text-secondary);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.upload:hover {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	.upload:has(:focus-visible) {
		box-shadow: var(--focus-ring);
	}
	.busy {
		opacity: 0.6;
	}
</style>
