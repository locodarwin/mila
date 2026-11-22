<script lang="ts">
	// A drawing in a wiki page, drawn from the page's rows in the theme's
	// colors, at a share of the page's width, with its name under it. Its
	// buttons open the editor full screen: to change it, or only to look.
	// Where the rows are not on this device, the picture saved with the page
	// shows instead. Loaded with the editor (nodeviews.svelte.ts).
	import Maximize2 from '@lucide/svelte/icons/maximize-2';
	import Pencil from '@lucide/svelte/icons/pencil';
	import { boundsOf, drawingSummary, inOrder, shapeSvg, type Shape } from '../../drawing';
	import { livePaint } from '../../drawingPaint';
	import { engine } from '../../sync/engine.svelte';
	import { shapesOf } from '../../sync/drawings.svelte';
	import type { Component } from 'svelte';
	import { uuid } from '../../uuid';

	interface Props {
		ws: string;
		pageId: string;
		id: string;
		name: string;
		src: string;
		size: number | null;
		/** Whether the page can be changed here. */
		editable: boolean;
		/** Called when the editor closes, with whether anything changed. */
		onclosed: (changed: boolean) => void;
		/** Opens at once, as for a drawing just made. */
		openAtStart?: boolean;
	}

	let {
		ws,
		pageId,
		id,
		name,
		src,
		size,
		editable,
		onclosed,
		openAtStart = false
	}: Props = $props();

	let host = $state<HTMLElement>();
	let font = $state('sans-serif');
	$effect(() => {
		if (host) font = getComputedStyle(host).fontFamily || 'sans-serif';
	});
	const replica = $derived(engine.replica(ws));
	const known = $derived(!!replica?.get('page_drawing', id));
	const shapes = $derived.by((): Shape[] => {
		return replica ? shapesOf(ws, pageId, id) : [];
	});
	const paint = $derived(livePaint(ws, font));
	const byId = $derived(new Map(shapes.map((s) => [s.id, s])));
	const bounds = $derived(boundsOf(shapes));
	const markup = $derived(
		inOrder(shapes)
			.map((s) => shapeSvg(s, byId, paint))
			.join('')
	);
	const label = $derived(name.trim() || 'Drawing');
	const summary = $derived(drawingSummary(shapes));
	const descId = `drawing-desc-${uuid()}`;

	function svgMarkup(el: SVGGElement, html: string) {
		el.innerHTML = html;
		return { update: (h: string) => (el.innerHTML = h) };
	}

	let Editor = $state<Component<Record<string, unknown>> | null>(null);
	let open = $state(false);
	let readonly = $state(false);
	async function openEditor(asReader: boolean) {
		readonly = asReader;
		Editor ??= (await import('./DrawingEditor.svelte')).default as never;
		open = true;
	}
	let started = false;
	$effect(() => {
		if (openAtStart && !started && editable) {
			started = true;
			void openEditor(false);
		}
	});
</script>

<figure class="drawing" bind:this={host} style:width={size && size < 100 ? `${size}%` : '100%'}>
	<div class="bar" role="toolbar" aria-label="{label} tools">
		{#if editable}
			<button type="button" class="tool" onclick={() => openEditor(false)} title="Edit the drawing">
				<Pencil size={15} aria-hidden="true" /><span>Edit</span>
			</button>
		{/if}
		<button
			type="button"
			class="tool icon"
			onclick={() => openEditor(true)}
			aria-label="Full screen"
			title="Show the drawing full screen"
		>
			<Maximize2 size={15} aria-hidden="true" />
		</button>
	</div>
	{#if shapes.length}
		<svg
			class="art"
			viewBox="{bounds.x} {bounds.y} {bounds.w} {bounds.h}"
			style:max-width="{Math.ceil(bounds.w)}px"
			role="img"
			aria-label={label}
			aria-describedby={descId}
			ondblclick={() => editable && openEditor(false)}
		>
			<g use:svgMarkup={markup}></g>
		</svg>
		<p class="visually-hidden" id={descId}>{summary}</p>
	{:else if !known && src}
		<img class="art" {src} alt={label} />
	{:else}
		<div class="empty">
			{known || !replica ? 'Empty drawing' : 'This drawing is not on this device yet.'}
		</div>
	{/if}
	{#if name.trim()}<figcaption>{name}</figcaption>{/if}
</figure>

{#if open && Editor}
	<Editor
		{ws}
		{pageId}
		{id}
		{readonly}
		onclose={(changed: boolean) => {
			open = false;
			onclosed(changed);
		}}
	/>
{/if}

<style>
	.drawing {
		position: relative;
		display: grid;
		justify-items: center;
		gap: var(--space-1);
		box-sizing: border-box;
		max-width: 100%;
		margin: 0.2em auto;
		padding: var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-0);
	}
	.art {
		display: block;
		width: 100%;
		height: auto;
	}
	.empty {
		display: grid;
		place-items: center;
		width: 100%;
		min-height: 96px;
		color: var(--text-muted);
		font-family: var(--font-ui);
		font-size: var(--text-sm);
	}
	figcaption {
		font-size: var(--text-sm);
		font-style: italic;
		color: var(--text-secondary);
	}
	.bar {
		position: absolute;
		top: var(--space-1);
		right: var(--space-1);
		z-index: 1;
		display: flex;
		gap: 2px;
		padding: 2px;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--menu-bg);
		box-shadow: var(--shadow-sm);
		opacity: 0;
		transition: opacity var(--dur-fade) var(--ease);
	}
	.drawing:hover .bar,
	.drawing:focus-within .bar {
		opacity: 1;
	}
	@media (pointer: coarse) {
		.bar {
			position: static;
			justify-self: end;
			opacity: 1;
		}
	}
	.tool {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-height: max(var(--tap-min), 28px);
		padding: 0 var(--space-2);
		border: 0;
		border-radius: var(--radius-sm);
		background: transparent;
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-family: var(--font-ui);
		cursor: pointer;
	}
	.tool.icon {
		min-width: max(var(--tap-min), 28px);
		justify-content: center;
		padding: 0;
	}
	.tool:hover {
		background: var(--button-ghost-hover);
		color: var(--text-primary);
	}
</style>
