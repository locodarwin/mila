<script lang="ts">
	// The style guide's right-click menu (contextMenu.svelte.ts): a task's, as it opens
	// at the pointer, drawn in place here. The app opens it from a right-click, Shift
	// F10 or the Menu key, or as a sheet from a held finger.
	import ContextMenu from '#lib/components/ContextMenu.svelte';
	import type { MenuSpec } from '#lib/contextMenu.svelte.js';

	const spec: MenuSpec = {
		name: 'Order seeds',
		kind: 'task actions',
		items: [
			{ id: 'open', label: 'Open', command: 'task.open' },
			{ id: 'complete', label: 'Complete', command: 'task.complete' },
			{ id: 'schedule', label: 'Schedule', sub: () => [] },
			{ id: 'my-day', label: 'Add to My Day', command: 'task.my-day' },
			{ id: 'priority', label: 'Priority', sub: () => [] },
			{ id: 'labels', label: 'Labels', sub: () => [] },
			'separator',
			{ id: 'duplicate', label: 'Duplicate' },
			{ id: 'link', label: 'Copy link' },
			'separator',
			{ id: 'delete', label: 'Delete', command: 'task.delete', danger: true }
		]
	};
	const labels: MenuSpec = {
		name: 'Order seeds',
		kind: 'task actions',
		items: [
			{ id: 'errand', label: 'Errand', checked: true },
			{ id: 'outdoors', label: 'Outdoors', checked: false },
			{ id: 'indoors', label: 'Indoors', checked: true }
		]
	};
	const anchor = document.createElement('div');
</script>

<section>
	<h2>Right-click menus</h2>
	<p class="note">
		A task's menu, and a list with checks such as Labels. The row it acts on is lit while it is
		open.
	</p>
	<div class="row">
		<ContextMenu inline open={{ spec, x: 0, y: 0, sheet: false, anchor, back: null }} />
		<ContextMenu inline open={{ spec: labels, x: 0, y: 0, sheet: false, anchor, back: null }} />
		<p class="menu-lit lit">Order seeds</p>
	</div>
</section>

<style>
	.row {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-start;
		gap: var(--space-4);
	}
	.lit {
		margin: 0;
		padding: var(--space-2) var(--space-3);
		border-radius: var(--radius-md);
	}
</style>
