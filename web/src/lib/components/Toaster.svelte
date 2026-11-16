<script lang="ts">
	import { rise } from '../motion';
	import { toasts } from '../toasts.svelte';
	import Toast from './Toast.svelte';

	const errors = $derived(toasts.items.filter((t) => t.tone === 'error'));
	const infos = $derived(toasts.items.filter((t) => t.tone !== 'error'));

	let toaster = $state<HTMLElement>();

	// Paused while the pointer or focus is on a toast (toasts.svelte.ts).
	let hovered = false;
	let focused = false;
	function update() {
		if (hovered || focused) toasts.pause();
		else toasts.resume();
	}

	// Where focus was before it came to the toasts, to go back to after Undo.
	let cameFrom: HTMLElement | null = null;

	// A focused toast that goes away (or turns inert as it leaves) loses
	// focus without a focusout in every browser, so look again whenever the
	// toasts change rather than staying paused for good.
	$effect(() => {
		void toasts.items;
		const at = document.activeElement;
		const still = !!at && !!toaster?.contains(at) && !at.closest('[inert]');
		if (focused && !still) {
			focused = false;
			update();
		}
	});

	// Undo, an action such as Reload, or closing a sticky toast: do it,
	// take the toast away, and put the keyboard back where it was.
	function act(t: (typeof toasts.items)[number], run?: () => void) {
		const had = !!toaster?.contains(document.activeElement);
		run?.();
		toasts.dismiss(t.id);
		if (!had) return;
		focused = false;
		update();
		// Back to where the keyboard was, not to the top of the page.
		const back =
			cameFrom?.isConnected && !cameFrom.closest('[inert]')
				? cameFrom
				: document.getElementById('main');
		back?.focus();
	}
</script>

{#snippet item(t: (typeof toasts.items)[number])}
	<div class="item" transition:rise>
		<Toast
			message={t.message}
			tone={t.tone}
			onundo={t.onundo ? () => act(t, t.onundo) : undefined}
			action={t.action ? { label: t.action.label, run: () => act(t, t.action?.run) } : undefined}
			ondismiss={t.sticky && t.closable ? () => act(t) : undefined}
		/>
	</div>
{/snippet}

<!-- Two live regions, always in the page: errors interrupt (role=alert),
     everything else waits its turn (role=status). The toasts themselves
     carry no role, so nothing is read twice. -->
<div
	class="toaster"
	role="presentation"
	bind:this={toaster}
	onpointerenter={() => ((hovered = true), update())}
	onpointerleave={() => ((hovered = false), update())}
	onfocusin={(e) => {
		const from = e.relatedTarget as HTMLElement | null;
		if (!e.currentTarget.contains(from)) cameFrom = from;
		focused = true;
		update();
	}}
	onfocusout={(e) => {
		focused = e.currentTarget.contains(e.relatedTarget as Node);
		update();
	}}
>
	<div class="stack" role="alert">
		{#each errors as t (t.id)}{@render item(t)}{/each}
	</div>
	<div class="stack" role="status">
		{#each infos as t (t.id)}{@render item(t)}{/each}
	</div>
</div>

<style>
	.toaster {
		position: fixed;
		bottom: calc(var(--space-5) + var(--selection-bar, 0px) + env(safe-area-inset-bottom));
		left: 50%;
		z-index: 100;
		display: grid;
		gap: var(--space-2);
		justify-items: center;
		transform: translateX(-50%);
		pointer-events: none;
		/* Stays on top while the page crossfades to another view. */
	}
	.stack {
		display: grid;
		gap: var(--space-2);
		justify-items: center;
	}
	.item {
		pointer-events: auto;
	}
	@media (max-width: 720px) {
		.toaster {
			bottom: calc(
				max(var(--tabbar-height), var(--selection-bar, 0px)) + var(--space-4) +
					env(safe-area-inset-bottom)
			);
		}
	}
</style>
