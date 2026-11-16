<script lang="ts">
	// Your picture, in Settings: chosen from a file, dropped or pasted,
	// cropped in AvatarCrop (loaded only then) and sent as a PNG.
	import { tick, type Component } from 'svelte';
	import { api, call, messageOf } from '../api/client';
	import { session } from '../session.svelte';
	import { engine } from '../sync/engine.svelte';
	import { toasts } from '../toasts.svelte';
	import Button from './Button.svelte';
	import Avatar from './task/Avatar.svelte';

	const NEEDS_NETWORK = 'Changing your picture needs a connection.';

	interface Props {
		/** Sends the cropped PNG; by default, to the server as your picture. */
		save?: (png: Blob) => Promise<void>;
		/** Removes the picture; by default, yours on the server. */
		remove?: () => Promise<void>;
		/** The picture shown; by default, yours. */
		picture?: string | null;
		/** Whose initials show without one; by default, yours. */
		name?: string;
	}

	let { save = upload, remove = removeMine, picture, name }: Props = $props();
	const shown = $derived(picture === undefined ? (session.user?.avatar ?? null) : picture);

	let cropping = $state<Blob | null>(null);
	let Crop = $state<Component<{
		file: Blob;
		onsave: (png: Blob) => Promise<void>;
		onclose: () => void;
	}> | null>(null);
	let dragging = $state(false);
	let fileInput = $state<HTMLInputElement>();
	let busy = $state(false);
	let online = $state(typeof navigator === 'undefined' ? true : navigator.onLine);
	const offline = $derived(!online || engine.state === 'offline');

	async function upload(png: Blob) {
		const body = new FormData();
		body.append('file', png, 'picture.png');
		let res: Response;
		try {
			res = await fetch('/api/v1/me/avatar', { method: 'PUT', body });
		} catch (err) {
			throw err instanceof TypeError ? new Error(NEEDS_NETWORK) : err;
		}
		if (!res.ok)
			throw new Error(
				(await res.json().catch(() => ({}))).message ?? 'The picture could not be saved.'
			);
		session.setUser(await res.json());
		void session.refreshPeople();
		toasts.show('Picture saved');
	}

	async function removeMine() {
		session.setUser(await call(api.DELETE('/me/avatar')));
		void session.refreshPeople();
		toasts.show('Picture removed');
	}

	async function removePicture() {
		busy = true;
		try {
			await remove();
			// The button pressed is gone; Upload picture is next to it.
			await tick();
			fileInput?.focus();
		} catch (err) {
			toasts.error(err instanceof TypeError ? NEEDS_NETWORK : messageOf(err));
		} finally {
			busy = false;
		}
	}

	async function crop(file: File | null | undefined) {
		if (!file) return;
		if (offline) {
			toasts.error(NEEDS_NETWORK);
			return;
		}
		if (!file.type.startsWith('image/')) {
			toasts.error('Choose a picture: a JPEG, PNG or other image.');
			return;
		}
		try {
			Crop ??= (await import('./AvatarCrop.svelte')).default;
		} catch (err) {
			toasts.error(messageOf(err));
			return;
		}
		cropping = file;
	}

	function picked(e: Event & { currentTarget: HTMLInputElement }) {
		const file = e.currentTarget.files?.[0];
		e.currentTarget.value = '';
		void crop(file);
	}

	const hasFiles = (e: DragEvent) => !!e.dataTransfer?.types.includes('Files');

	// Files dragged here never open in the browser, offline included.
	function over(e: DragEvent) {
		if (!hasFiles(e)) return;
		e.preventDefault();
		dragging = !offline;
	}

	function drop(e: DragEvent) {
		if (!hasFiles(e)) return;
		e.preventDefault();
		dragging = false;
		const file = [...(e.dataTransfer?.files ?? [])].find((f) => f.type.startsWith('image/'));
		void crop(file);
	}

	// A picture pasted anywhere on the page, but not into a field.
	function paste(e: ClipboardEvent) {
		const t = e.target as HTMLElement | null;
		if (t?.closest('input, textarea, select, [contenteditable]:not([contenteditable="false"])'))
			return;
		const file = e.clipboardData?.files[0];
		if (!file?.type.startsWith('image/')) return;
		e.preventDefault();
		void crop(file);
	}
</script>

<svelte:window
	onpaste={paste}
	ononline={() => (online = true)}
	onoffline={() => (online = false)}
/>

<div
	class="picture"
	class:dragging
	role="group"
	aria-label="Your picture"
	ondragenter={over}
	ondragover={over}
	ondragleave={() => (dragging = false)}
	ondrop={drop}
>
	<Avatar
		name={name ?? session.user?.name ?? ''}
		src={shown}
		size={96}
		label={shown ? 'Your picture' : 'No picture; your initials show instead'}
	/>
	<div class="side">
		<div class="buttons">
			<label class="upload" class:disabled={offline}>
				<input
					bind:this={fileInput}
					class="visually-hidden"
					type="file"
					accept="image/*"
					disabled={offline}
					onchange={picked}
				/>
				Upload picture
			</label>
			{#if shown}
				<Button variant="ghost" disabled={offline || busy} onclick={() => void removePicture()}
					>Remove picture</Button
				>
			{/if}
		</div>
		<p class="hint">
			{offline ? NEEDS_NETWORK : 'Choose a picture, or drop or paste one here.'}
		</p>
	</div>
</div>

{#if Crop && cropping}
	<Crop file={cropping} onsave={save} onclose={() => (cropping = null)} />
{/if}

<style>
	.picture {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-4);
		padding: var(--space-2);
		border-radius: var(--radius-lg);
		outline: 2px dashed transparent;
		outline-offset: 2px;
	}
	.dragging {
		outline-color: var(--accent);
	}
	.side {
		display: grid;
		gap: var(--space-2);
	}
	.buttons {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	/* A label that looks like the secondary button, around a hidden file input. */
	.upload {
		position: relative;
		display: inline-flex;
		align-items: center;
		height: calc(32px * var(--control-scale));
		min-height: var(--tap-min);
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		color: var(--text-primary);
		font-size: var(--text-sm);
		font-weight: 550;
		cursor: pointer;
		white-space: nowrap;
	}
	.upload:hover:not(.disabled) {
		background: var(--surface-2);
	}
	.upload:has(:focus-visible) {
		outline: 2px solid transparent;
		outline-offset: 2px;
		box-shadow: var(--focus-ring);
	}
	@media (forced-colors: active) {
		.upload:has(:focus-visible) {
			outline-color: Highlight;
		}
	}
	.upload.disabled {
		opacity: 0.5;
		cursor: default;
	}
	.hint {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
</style>
