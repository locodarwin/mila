<script lang="ts">
	import Checkbox from './Checkbox.svelte';
	import LabelChip from './LabelChip.svelte';
	import PriorityIcon from './PriorityIcon.svelte';
	import StatusIcon from './StatusIcon.svelte';
	import type { PaletteColor, Priority, StatusCategory } from '../theme/palette';

	interface Props {
		title: string;
		status: StatusCategory;
		priority: Priority;
		labels?: { name: string; color: PaletteColor }[];
		due?: string;
		/** Called once the completed row has held long enough to undo. */
		oncomplete?: () => void;
	}

	let { title, status, priority, labels = [], due, oncomplete }: Props = $props();

	// Completing holds the row in place for a moment so a misclick is easy to
	// take back, then hands it to the parent to animate away.
	const HOLD_MS = 1000;
	let done = $state(false);
	let timer: ReturnType<typeof setTimeout> | undefined;

	function toggle(checked: boolean) {
		done = checked;
		clearTimeout(timer);
		if (checked) timer = setTimeout(() => oncomplete?.(), HOLD_MS);
	}

	$effect(() => () => clearTimeout(timer));
</script>

<div class="row" class:done>
	<Checkbox label={title} hideLabel checked={done} onchange={toggle} />
	<PriorityIcon {priority} />
	<StatusIcon category={done ? 'done' : status} />
	<span class="title">{title}</span>
	<span class="meta">
		{#each labels as label (label.name)}
			<LabelChip {...label} />
		{/each}
		{#if due}<span class="due tabular">{due}</span>{/if}
	</span>
</div>

<style>
	.row {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		min-height: var(--row-height);
		padding: 0 var(--pad-x);
		border-bottom: 1px solid var(--row-divider);
		background: var(--surface-0);
		transition: background-color var(--dur-micro) var(--ease);
	}
	.row:hover {
		background: var(--row-hover-bg);
	}
	.title {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		background: linear-gradient(currentColor, currentColor) no-repeat 0 55% / 0 1px;
		transition:
			background-size var(--dur-panel) var(--ease),
			color var(--dur-panel) var(--ease);
	}
	.done .title {
		background-size: 100% 1px;
		color: var(--text-muted);
	}
	.meta {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		flex: none;
	}
	.due {
		font-size: var(--text-xs);
		color: var(--row-meta-text);
	}
	:global([data-density='dense']) .meta :global(.chip) {
		display: none;
	}
</style>
