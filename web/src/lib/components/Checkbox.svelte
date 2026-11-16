<script lang="ts">
	import { uuid } from '../uuid';

	interface Props {
		checked?: boolean;
		label: string;
		/** Hide the label visually; it stays available to screen readers. */
		hideLabel?: boolean;
		disabled?: boolean;
		/** Says why it is disabled: shown on hover, and read with the box as its description. */
		title?: string;
		onchange?: (checked: boolean) => void;
	}

	let {
		checked = $bindable(false),
		label,
		hideLabel = false,
		disabled = false,
		title,
		onchange
	}: Props = $props();

	const descriptionId = `checkbox-${uuid()}`;
</script>

<label class="checkbox" class:bare={hideLabel} class:disabled {title}>
	<input
		type="checkbox"
		bind:checked
		{disabled}
		aria-describedby={title ? descriptionId : undefined}
		onchange={() => onchange?.(checked)}
		class="visually-hidden"
	/>
	<span class="box" aria-hidden="true">
		<svg viewBox="0 0 16 16"><path d="M3.5 8.5l3 3 6-7" /></svg>
	</span>
	<span class:visually-hidden={hideLabel}>{label}</span>
</label>
<!-- Outside the label, which would add it to the box's name. -->
{#if title}<span id={descriptionId} hidden>{title}</span>{/if}

<style>
	.checkbox {
		/* Anchors the hidden input, which would otherwise be placed against
		   the page and stretch it from deep inside a scrolled panel. */
		position: relative;
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		cursor: pointer;
		font-size: var(--text-md);
	}
	.box {
		display: grid;
		place-items: center;
		width: calc(18px * var(--ui-scale));
		height: calc(18px * var(--ui-scale));
		border: 1.5px solid var(--border-control);
		border-radius: var(--checkbox-radius);
		background: var(--input-bg);
		transition:
			background-color var(--dur-micro) var(--ease),
			border-color var(--dur-micro) var(--ease),
			transform var(--dur-panel) var(--ease-spring);
	}
	svg {
		width: calc(14px * var(--ui-scale));
		height: calc(14px * var(--ui-scale));
		fill: none;
		stroke: var(--checkbox-mark);
		stroke-width: 2.2;
		stroke-linecap: round;
		stroke-linejoin: round;
		stroke-dasharray: 16;
		stroke-dashoffset: 16;
		transition: stroke-dashoffset var(--dur-panel) var(--ease);
	}
	input:checked + .box {
		background: var(--checkbox-on-bg);
		border-color: var(--checkbox-on-bg);
		transform: scale(calc(1 + 0.08 * var(--motion)));
	}
	input:checked + .box svg {
		stroke-dashoffset: 0;
	}
	input:focus-visible + .box {
		box-shadow: var(--focus-ring);
	}
	.checkbox:hover .box {
		border-color: var(--accent);
	}
	.checkbox.disabled {
		cursor: not-allowed;
	}
	/* Touch: a box without a visible label keeps its size and place, but
	   a finger anywhere in 44px around it hits it; the negative margins
	   give the room back to the row. */
	@media (pointer: coarse) {
		.checkbox.bare {
			justify-content: center;
			min-width: var(--touch-target);
			min-height: var(--touch-target);
			margin: calc((var(--touch-target) - 18px * var(--ui-scale)) / -2);
		}
	}
	.checkbox.disabled .box {
		opacity: 0.55;
	}
	.checkbox.disabled:hover input:not(:checked) + .box {
		border-color: var(--border-control);
	}
</style>
