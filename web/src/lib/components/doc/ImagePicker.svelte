<script lang="ts">
	// Choosing an image for a task's notes or a wiki page: one of the task's
	// or page's own image files, or a new one uploaded to it. Loaded with
	// the editor's tools (DocTools).
	import ImagePlus from '@lucide/svelte/icons/image-plus';
	import { engine } from '../../sync/engine.svelte';
	import type { DocTarget } from '../../sync/docs';
	import { toasts } from '../../toasts.svelte';
	import { fileUrl, IMAGE_TYPES, uploadFile } from '../../uploads';
	import Announcer from '../Announcer.svelte';

	interface Props {
		ws: string;
		target: DocTarget;
		/** Whether the dialog is open; closing it sets this false. */
		open: boolean;
		/** Puts the image chosen in the document: its address and its alt text. */
		onpick: (src: string, alt: string) => void;
	}

	let { ws, target, open = $bindable(), onpick }: Props = $props();
	let dialog = $state<HTMLDialogElement>();
	let busy = $state(false);
	const images = $derived.by(() => {
		const r = engine.replica(ws);
		const files =
			target.kind === 'task' ? r?.attachments(target.id) : r?.pageAttachments(target.id);
		return (files ?? []).filter((f) => IMAGE_TYPES.includes(f.mime));
	});
	const where = $derived(target.kind === 'task' ? 'task' : 'page');

	$effect(() => {
		if (open && dialog && !dialog.open) dialog.showModal();
		else if (!open && dialog?.open) dialog.close();
	});

	/** An image's alt text to start with: its file name without the extension. */
	const altOf = (filename: string) => filename.replace(/\.[a-z0-9]+$/i, '');

	function pick(id: string, filename: string) {
		open = false;
		onpick(fileUrl(ws, id), altOf(filename));
	}

	async function upload(e: Event & { currentTarget: HTMLInputElement }) {
		const file = e.currentTarget.files?.[0];
		e.currentTarget.value = '';
		if (!file) return;
		if (!IMAGE_TYPES.includes(file.type)) {
			toasts.error('Images can be PNG, JPEG, GIF, WebP or SVG.');
			return;
		}
		busy = true;
		try {
			pick(await uploadFile(ws, target, file), file.name);
		} catch (err) {
			toasts.error(err instanceof Error ? err.message : 'The upload failed.');
		} finally {
			busy = false;
		}
	}
</script>

<dialog
	bind:this={dialog}
	class="picker"
	aria-labelledby="image-picker-title"
	onclose={() => (open = false)}
>
	<h2 id="image-picker-title">Insert an image</h2>
	{#if images.length}
		<ul class="grid" aria-label={`Images on this ${where}`}>
			{#each images as img (img.id)}
				<li>
					<button type="button" onclick={() => pick(img.id, img.filename)}>
						<img src={fileUrl(ws, img.id)} alt="" loading="lazy" />
						<span>{img.filename}</span>
					</button>
				</li>
			{/each}
		</ul>
	{:else}
		<p class="hint">This {where} has no images yet. Upload one to put it here.</p>
	{/if}
	<div class="actions">
		<label class="upload" class:busy>
			<input
				type="file"
				class="visually-hidden"
				accept={IMAGE_TYPES.join(',')}
				onchange={upload}
				disabled={busy}
			/>
			<ImagePlus size={15} />{busy ? 'Uploading…' : 'Upload an image'}
		</label>
		<form method="dialog"><button>Cancel</button></form>
	</div>
	{#if open}<Announcer inDialog />{/if}
</dialog>

<style>
	.picker {
		width: min(520px, calc(100vw - 32px));
		max-height: 80vh;
		padding: var(--space-5);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	.picker::backdrop {
		/* With the value as written, for browsers whose ::backdrop does not inherit custom properties. */
		background: var(--scrim, oklch(0 0 0 / 0.25));
	}
	h2 {
		font-size: var(--text-xl);
	}
	.hint {
		margin-top: var(--space-3);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
		gap: var(--space-2);
		margin: var(--space-4) 0 0;
		padding: 0;
		list-style: none;
	}
	.grid button {
		display: grid;
		gap: var(--space-1);
		width: 100%;
		padding: var(--space-1);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-1);
		color: var(--text-primary);
		font-size: var(--text-xs);
		text-align: left;
		cursor: pointer;
	}
	.grid button:hover {
		border-color: var(--border-strong);
	}
	.grid img {
		width: 100%;
		aspect-ratio: 4 / 3;
		object-fit: cover;
		border-radius: var(--radius-sm);
		background: var(--surface-2);
	}
	.grid span {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.actions {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-3);
		margin-top: var(--space-4);
	}
	.upload {
		position: relative;
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-height: max(var(--tap-min), 32px);
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-md);
		cursor: pointer;
	}
	.upload:has(:focus-visible) {
		box-shadow: var(--focus-ring);
	}
	.upload.busy {
		color: var(--text-secondary);
	}
</style>
