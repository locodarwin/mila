<script lang="ts">
	import { resolve } from '$app/paths';
	import CalendarDays from '@lucide/svelte/icons/calendar-days';
	import Inbox from '@lucide/svelte/icons/inbox';
	import Sun from '@lucide/svelte/icons/sun';
	import { fade, fly, slide } from 'svelte/transition';
	import Button from '../Button.svelte';
	import Checkbox from '../Checkbox.svelte';
	import LabelChip from '../LabelChip.svelte';
	import PriorityIcon from '../PriorityIcon.svelte';
	import SidebarLink from '../SidebarLink.svelte';
	import StatusIcon from '../StatusIcon.svelte';
	import TabBar from '../TabBar.svelte';
	import TaskRow from '../TaskRow.svelte';
	import TextInput from '../TextInput.svelte';
	import Toast from '../Toast.svelte';
	import TaskCardBody from '../views/TaskCardBody.svelte';
	import { reducedMotion, tokens } from '../../motion';
	import { engine } from '../../sync/engine.svelte';
	import { appearance } from '../../theme/appearance.svelte';
	import type { Scheme } from '../../theme/color';
	import { applyInline } from '../../theme/css';
	import { applyAccent } from '../../theme/custom';
	import type { ThemeDoc } from '../../theme/document';
	import '../../styles/doctext.css';
	import { PALETTE, PRIORITIES, STATUS_CATEGORIES } from '../../theme/palette';
	import { borrowSample, PREVIEW_TASK, PREVIEW_WS } from '../../theme/previewSample';
	import { byName } from '../../theme/tokens';

	// A theme drawn with the app's own components, in a container that
	// carries the theme's base, a scheme, the page's density and font size,
	// and the theme's values inline, so tokens.css works out every token on
	// it and nothing outside it changes. It is a picture: nothing in it can
	// be reached or used (inert). It shows the whole app, or the parts that
	// one group of tokens draws. Each part names the tokens that draw it,
	// most particular first (data-tokens), so the editor can go from a
	// click on the picture to its setting.
	interface Props {
		theme: ThemeDoc;
		scheme: Scheme;
		/** The person's accent hue, which a generated accent follows. */
		accentHue?: number | null;
		/** Says which scheme it shows, above it. */
		caption?: string;
		/** A group of tokens (theme-tokens.json) to show the parts of; none, the whole app. */
		part?: string | null;
		/** Given, a click on the picture names the tokens that draw what is under the pointer. */
		onpick?: (tokens: string[]) => void;
	}

	let { theme, scheme, accentHue = null, caption, part = null, onpick }: Props = $props();

	// Which parts of the picture each group of tokens draws.
	const PARTS: Record<string, string[]> = {
		app: [
			'surfaces',
			'text',
			'borders',
			'accent',
			'sidebar',
			'lists',
			'focus',
			'type',
			'spacing',
			'motion',
			'status',
			'palette'
		],
		board: ['cards', 'status', 'palette', 'shadows', 'shape', 'spacing', 'motion'],
		notes: ['pages', 'type'],
		controls: ['buttons', 'inputs', 'focus', 'accent', 'shape', 'borders'],
		icons: ['status'],
		chips: ['palette'],
		charts: ['charts'],
		menus: ['menus', 'shadows', 'shape', 'surfaces'],
		toasts: ['menus']
	};
	const shows = (block: string) => !part || PARTS[block].includes(part);

	// Pointing: the smallest part under the pointer that names its tokens,
	// found from the parts' boxes, since the picture itself takes no pointer.
	let hovered = $state<{ x: number; y: number; w: number; h: number; label: string } | null>(null);
	function partAt(e: PointerEvent | MouseEvent): HTMLElement | null {
		if (!box) return null;
		let best: HTMLElement | null = null;
		let area = Infinity;
		for (const el of box.querySelectorAll<HTMLElement>('[data-tokens]')) {
			const r = el.getBoundingClientRect();
			if (e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom)
				continue;
			if (r.width * r.height < area) {
				area = r.width * r.height;
				best = el;
			}
		}
		return best;
	}
	let frameEl = $state<HTMLElement>();
	function point(e: PointerEvent) {
		const el = partAt(e);
		if (!el || !frameEl) return (hovered = null);
		const r = el.getBoundingClientRect();
		const f = frameEl.getBoundingClientRect();
		const first = el.dataset.tokens!.split(' ')[0];
		hovered = {
			x: r.left - f.left,
			y: r.top - f.top,
			w: r.width,
			h: r.height,
			label: byName(first)?.label ?? first
		};
	}
	function pick(e: MouseEvent) {
		const el = partAt(e);
		if (el) onpick?.(el.dataset.tokens!.split(' '));
	}

	let box = $state<HTMLElement>();
	const density = $derived(appearance.current.density);
	const fontSize = $derived(appearance.current.fontSize);

	// The values go on at most once a frame, however fast a slider moves.
	let frame = 0;
	$effect(() => {
		const el = box;
		const doc = theme;
		const s = scheme;
		const hue = 'manual' in doc.accent ? null : accentHue;
		void density;
		void fontSize;
		if (!el) return;
		cancelAnimationFrame(frame);
		frame = requestAnimationFrame(() => {
			applyInline(el, doc, s);
			applyAccent(el, doc, s, hue);
		});
		return () => cancelAnimationFrame(frame);
	});

	const giveBack = borrowSample();
	$effect(() => () => giveBack());
	const task = $derived(engine.replica(PREVIEW_WS)?.task(PREVIEW_TASK));

	const rows = [
		{ title: 'Water the plants', status: 'done', priority: 'none', labels: [], due: 'Yesterday' },
		{
			title: 'Renew the passport',
			status: 'todo',
			priority: 'urgent',
			labels: [{ name: 'Errand', color: 'red' }],
			due: '2 days ago'
		},
		{
			title: 'Plan the garden',
			status: 'in-progress',
			priority: 'medium',
			labels: [
				{ name: 'Home', color: 'green' },
				{ name: 'Weekend', color: 'purple' }
			],
			due: 'Sat'
		}
	] as const;

	// Play motion: a row collapses and a panel slides in, with this
	// theme's durations and curves, then both come back.
	let played = $state(false);
	const motion = () => tokens(box);
	function rowOut(node: Element) {
		const m = motion();
		return reducedMotion()
			? fade(node, { duration: m.durations.fade })
			: slide(node, { duration: m.durations.panel, easing: m.ease });
	}
	function panelIn(node: Element) {
		const m = motion();
		return reducedMotion()
			? fade(node, { duration: m.durations.fade })
			: fly(node, { x: 24, duration: m.durations.panel, easing: m.spring });
	}
	let timer: ReturnType<typeof setTimeout> | undefined;
	export function play() {
		clearTimeout(timer);
		played = true;
		const m = motion();
		timer = setTimeout(() => (played = false), m.durations.panel + 900);
	}
	$effect(() => () => clearTimeout(timer));
</script>

<figure class="preview-frame" bind:this={frameEl}>
	{#if caption}<figcaption>{caption}</figcaption>{/if}
	<div
		class="theme-preview"
		bind:this={box}
		data-theme={theme.base}
		data-scheme={scheme}
		data-density={density}
		data-font-size={fontSize}
		inert
		aria-hidden="true"
	>
		{#if shows('app')}
			<div class="app" data-tokens="border surface-0">
				<div class="sidebar" data-tokens="sidebar-bg sidebar-border">
					<div data-tokens="sidebar-item-active sidebar-text-active">
						<SidebarLink href={resolve('/(app)')} label="Inbox" icon={Inbox} current />
					</div>
					<div data-tokens="sidebar-text sidebar-item-hover">
						<SidebarLink href={resolve('/(app)/today')} label="My Day" icon={Sun} />
					</div>
					<div data-tokens="sidebar-text sidebar-item-hover">
						<SidebarLink href={resolve('/(app)/upcoming')} label="Upcoming" icon={CalendarDays} />
					</div>
				</div>
				<div class="main" data-tokens="surface-0 text-primary">
					<p class="heading" data-tokens="text-primary font-heading heading-weight type-lg">
						This week
					</p>
					<p data-tokens="text-primary font-ui type-base leading">
						Plain text, <span class="secondary" data-tokens="text-secondary">secondary text</span>,
						<span class="muted" data-tokens="text-muted">muted text</span>,
						<span class="link" data-tokens="accent-text">a link</span>,
						<span class="danger" data-tokens="danger">a warning</span> and
						<span class="selected" data-tokens="selection-bg">selected text</span>.
					</p>
					<p class="group-header" data-tokens="group-header-text type-xs">Today</p>
					<div class="list" data-tokens="row-divider row-meta-text row-base">
						{#each rows as row, i (row.title)}
							{#if !(played && i === 0)}
								<div out:rowOut>
									<TaskRow {...row} labels={[...row.labels]} />
								</div>
							{/if}
						{/each}
						<div class="state hovered" data-tokens="row-hover-bg">A hovered row</div>
						<div class="state focused" data-tokens="row-selected-bg">The focused row</div>
					</div>
				</div>
				{#if played}
					<div class="panel" in:panelIn out:fade={{ duration: 0 }}>A task panel</div>
				{/if}
			</div>
		{/if}

		{#if shows('board')}
			<div class="board">
				<div class="column" data-tokens="board-column-bg radius-lg">
					<p class="group-header" data-tokens="group-header-text">Doing</p>
					{#if task}
						<div class="card" data-tokens="card-bg card-border card-radius card-shadow">
							<TaskCardBody ws={PREVIEW_WS} {task} />
						</div>
						<div class="card lifted" data-tokens="card-shadow-lifted card-bg">
							<TaskCardBody ws={PREVIEW_WS} {task} />
						</div>
					{/if}
				</div>
				<div class="today">
					<span data-tokens="calendar-today-bg accent-contrast">3</span> Today in the calendar
				</div>
			</div>
		{/if}

		{#if shows('notes')}
			<!-- Task notes, drawn with the editor's own styles (styles/doctext.css),
				     so the Pages colors show here as they will there. -->
			<div class="doc-text notes" data-tokens="panel-bg font-page type-page">
				<h1 data-tokens="page-heading-1 font-page-heading">Garden plan</h1>
				<!-- A picture of a link: the preview is inert, so it goes nowhere. -->
				<!-- svelte-ignore a11y_missing_attribute -->
				<p data-tokens="text-primary font-page">
					Notes with <a data-tokens="page-link">a link</a>,
					<code data-tokens="page-code-text page-code-bg">some code</code> and a list:
				</p>
				<ul>
					<li data-tokens="page-list-marker">Order the seeds</li>
					<li data-tokens="page-list-marker">Fix the fence</li>
				</ul>
				<h2 data-tokens="page-heading-2 font-page-heading">The beds</h2>
				<blockquote data-tokens="page-quote-text page-quote-edge">Water before nine.</blockquote>
				<h3 data-tokens="page-heading-3 font-page-heading">What it costs</h3>
				<table>
					<tbody>
						<tr>
							<th data-tokens="page-table-head-bg page-table-head-text page-table-border">Item</th>
							<th data-tokens="page-table-head-bg page-table-head-text page-table-border">Cost</th>
						</tr>
						<tr
							><td data-tokens="page-table-border">Seeds</td><td data-tokens="page-table-border"
								>12</td
							></tr
						>
						<tr data-tokens="page-table-stripe"><td>Soil</td><td>30</td></tr>
						<tr
							><td data-tokens="page-table-border">Posts</td><td data-tokens="page-table-border"
								>45</td
							></tr
						>
					</tbody>
				</table>
				<hr data-tokens="page-rule" />
			</div>
		{/if}

		{#if shows('controls')}
			<div class="wrap">
				<span data-tokens="button-primary-bg button-primary-text button-primary-hover button-radius"
					><Button variant="primary">Primary</Button></span
				>
				<span data-tokens="button-secondary-bg button-secondary-border button-secondary-text"
					><Button>Secondary</Button></span
				>
				<span data-tokens="button-ghost-hover button-secondary-text"
					><Button variant="ghost">Quiet</Button></span
				>
				<span data-tokens="button-danger"><Button variant="danger">Delete</Button></span>
			</div>
			<div class="wrap">
				<span data-tokens="input-bg input-border input-text input-placeholder input-radius"
					><TextInput label="Preview field" placeholder="A placeholder" /></span
				>
				<span data-tokens="checkbox-on-bg checkbox-mark checkbox-radius"
					><Checkbox label="Preview checked box" checked /></span
				>
				<span data-tokens="border-control checkbox-radius"
					><Checkbox label="Preview empty box" /></span
				>
				<span class="show-ring" data-tokens="focus-ring-color focus-ring-width focus-ring-gap"
					>Focused</span
				>
			</div>
		{/if}
		{#if shows('icons')}
			<div class="wrap">
				{#each STATUS_CATEGORIES as s (s)}<span data-tokens="status-{s}"
						><StatusIcon category={s} /></span
					>{/each}
				{#each PRIORITIES as p (p)}<span data-tokens="priority-{p}"
						><PriorityIcon priority={p} /></span
					>{/each}
			</div>
		{/if}
		{#if shows('chips')}
			<div class="wrap">
				{#each PALETTE as color (color)}<span data-tokens="palette-{color} palette-{color}-soft"
						><LabelChip {color} name={color} /></span
					>{/each}
			</div>
		{/if}
		{#if shows('charts')}
			<div class="chart" data-tokens="surface-0">
				{#each [1, 2, 3, 4, 5, 6] as n (n)}
					<span
						class="bar"
						data-tokens="chart-series-{n}"
						style:height="{[60, 85, 45, 70, 35, 55][n - 1]}%"
						style:background="var(--chart-series-{n})"
					></span>
				{/each}
			</div>
		{/if}
		{#if shows('menus')}
			<div class="wrap">
				<div class="menu" data-tokens="menu-bg menu-border menu-radius shadow-md">
					<div class="heading">A menu</div>
					<div class="option">An option</div>
					<div class="option">Another</div>
				</div>
				<div class="scrim" data-tokens="scrim">
					<div class="dialog" data-tokens="dialog-bg shadow-lg">A dialog</div>
				</div>
			</div>
		{/if}
		{#if shows('toasts')}
			<div class="wrap">
				<span data-tokens="toast-bg toast-text"
					><Toast message="Task completed" onundo={() => {}} /></span
				>
				<span data-tokens="toast-bg danger"><Toast message="That did not work" tone="error" /></span
				>
			</div>
			<div data-tokens="tabbar-bg tabbar-active"><TabBar inline /></div>
		{/if}
	</div>
	{#if onpick}
		<!-- The pointer's way to a setting; the editor's list reaches every one
		     from the keyboard, so this is for the mouse alone. -->
		<div
			class="pick"
			aria-hidden="true"
			onpointermove={point}
			onpointerleave={() => (hovered = null)}
			onclick={pick}
		></div>
		{#if hovered}
			<span
				class="hover"
				aria-hidden="true"
				style:left="{hovered.x}px"
				style:top="{hovered.y}px"
				style:width="{hovered.w}px"
				style:height="{hovered.h}px"><span class="hover-label">{hovered.label}</span></span
			>
		{/if}
	{/if}
</figure>

<style>
	.preview-frame {
		position: relative;
		display: grid;
		gap: var(--space-1);
		min-width: 0;
		margin: 0;
	}
	/* Over the picture, taking the pointer it does not. */
	.pick {
		position: absolute;
		inset: 0;
		cursor: crosshair;
	}
	.hover {
		position: absolute;
		border: 2px solid var(--focus-ring-color);
		border-radius: var(--radius-sm);
		pointer-events: none;
	}
	.hover-label {
		position: absolute;
		bottom: 100%;
		left: -2px;
		margin-bottom: 2px;
		padding: 1px var(--space-2);
		border-radius: var(--radius-sm);
		background: var(--text-primary);
		color: var(--surface-0);
		font-size: var(--text-xs);
		white-space: nowrap;
	}
	.chart {
		display: flex;
		align-items: flex-end;
		gap: var(--space-2);
		height: 90px;
		padding: var(--space-2);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
	}
	.bar {
		flex: 1;
		border-radius: var(--radius-sm) var(--radius-sm) 0 0;
	}
	figcaption {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.theme-preview {
		display: grid;
		gap: var(--space-3);
		min-width: 0;
		padding: var(--space-3);
		overflow: hidden;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface-0);
		color: var(--text-primary);
		font-family: var(--font-ui);
		font-size: var(--text-md);
		line-height: var(--leading);
	}
	.app {
		position: relative;
		display: grid;
		grid-template-columns: minmax(0, 30%) minmax(0, 1fr);
		overflow: hidden;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
	}
	.sidebar {
		display: grid;
		align-content: start;
		gap: 2px;
		min-width: 0;
		overflow: hidden;
		padding: var(--space-2);
		background: var(--sidebar-bg);
		border-right: var(--border-width) solid var(--sidebar-border);
	}
	.main {
		display: grid;
		align-content: start;
		gap: var(--space-2);
		min-width: 0;
		padding: var(--space-2);
	}
	.heading {
		font-family: var(--font-heading);
		font-size: var(--text-lg);
		font-weight: var(--heading-weight);
	}
	.secondary {
		color: var(--text-secondary);
	}
	.muted {
		color: var(--text-muted);
	}
	.link {
		color: var(--accent-text);
		text-decoration: underline;
	}
	.danger {
		color: var(--danger);
	}
	.selected {
		background: var(--selection-bg);
	}
	.group-header {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--group-header-text);
	}
	.list {
		min-width: 0;
		overflow: hidden;
	}
	.state {
		padding: var(--space-1) var(--pad-x);
		font-size: var(--text-sm);
	}
	.hovered {
		background: var(--row-hover-bg);
	}
	.focused {
		background: var(--row-selected-bg);
	}
	.panel {
		position: absolute;
		top: 0;
		right: 0;
		bottom: 0;
		width: 45%;
		padding: var(--space-3);
		border-left: var(--border-width) solid var(--border);
		background: var(--panel-bg);
		box-shadow: var(--shadow-md);
	}
	.board {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-3);
		align-items: flex-start;
	}
	.column {
		display: grid;
		flex: 1;
		gap: var(--space-2);
		min-width: 180px;
		padding: var(--space-2);
		border-radius: var(--radius-lg);
		background: var(--board-column-bg);
	}
	/* As the board draws a card (Board.svelte). */
	.card {
		display: grid;
		gap: var(--space-2);
		padding: calc(var(--space-3) * var(--control-scale));
		border: var(--border-width) solid var(--card-border);
		border-radius: var(--card-radius);
		background: var(--card-bg);
		box-shadow: var(--card-shadow);
	}
	.card.lifted {
		box-shadow: var(--card-shadow-lifted);
		transform: rotate(calc(-1.5deg * var(--motion)));
	}
	.today {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-sm);
	}
	.today span {
		display: grid;
		place-items: center;
		width: 24px;
		height: 24px;
		border-radius: var(--radius-full);
		background: var(--calendar-today-bg);
		color: var(--accent-contrast);
		font-weight: 600;
	}
	.notes {
		display: grid;
		gap: var(--space-2);
		padding: var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--panel-bg);
		font-size: var(--text-md);
		line-height: 1.55;
	}
	.notes :is(ul, blockquote, hr) {
		margin: 0;
	}
	.wrap {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
		align-items: center;
	}
	.show-ring {
		padding: var(--space-1) var(--space-2);
		border-radius: var(--radius-md);
		box-shadow: var(--focus-ring);
		font-size: var(--text-sm);
	}
	.menu {
		min-width: 140px;
	}
	.scrim {
		display: grid;
		place-items: center;
		padding: var(--space-3);
		border-radius: var(--radius-md);
		background: linear-gradient(var(--scrim), var(--scrim)), var(--surface-2);
	}
	.dialog {
		padding: var(--space-2) var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		box-shadow: var(--shadow-lg);
		font-size: var(--text-sm);
	}
</style>
