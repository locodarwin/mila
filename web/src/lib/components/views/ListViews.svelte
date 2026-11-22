<script lang="ts">
	// Inbox, My Day and Upcoming as lists of the app's own: each keeps what it
	// holds, and on this device takes a view (list, board, calendar, table or
	// timeline), a filter that narrows it and a display, as a saved view does;
	// Save as view makes one of what is set, and Reset puts the list back as it
	// comes. The page gives its heading, its tasks, its own sections and its own
	// list.
	import type { Snippet } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import BookmarkPlus from '@lucide/svelte/icons/bookmark-plus';
	import CalendarDays from '@lucide/svelte/icons/calendar-days';
	import ChartGantt from '@lucide/svelte/icons/chart-gantt';
	import List from '@lucide/svelte/icons/list';
	import Kanban from '@lucide/svelte/icons/kanban';
	import ListFilter from '@lucide/svelte/icons/list-filter';
	import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
	import SlidersHorizontal from '@lucide/svelte/icons/sliders-horizontal';
	import Table2 from '@lucide/svelte/icons/table-2';
	import { announce } from '../../announce.svelte';
	import { filterCount } from '../../filter';
	import {
		changed,
		freshSetup,
		listViews,
		type FixedList,
		type ListType
	} from '../../listViews.svelte';
	import { canEditViews } from '../../roles';
	import { session } from '../../session.svelte';
	import { createView, type ViewEdit } from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import type { Task, ViewDisplay, ViewFilter } from '../../sync/entities';
	import { toasts } from '../../toasts.svelte';
	import type { DisplayLookups, DisplayPath } from '../../viewDisplay';

	interface Section {
		id: string;
		title: string;
		tasks: Task[];
		tone?: 'danger';
		action?: Snippet;
	}

	interface Props {
		ws: string;
		list: FixedList;
		/** The list's name: the page's heading, and the start of a saved view's name. */
		title: string;
		/** Beside the heading (My Day's date). */
		extra?: Snippet;
		/** What the list holds, before any filter: what a board, calendar, table or timeline shows. */
		scope: Task[];
		/** The list's own sections of the tasks a filter keeps. */
		sectionsOf: (keep: (task: Task) => boolean) => Section[];
		/** The list as it comes, while nothing is set: its own list, with what it does. */
		own: Snippet;
		/** Above the list while a filter or display is set: the Inbox's add box, kept. */
		add?: Snippet;
		label: string;
		emptyTitle: string;
		emptyHint: string;
	}

	let { ws, list, title, extra, scope, sectionsOf, own, add, label, emptyTitle, emptyHint }: Props =
		$props();

	const replica = $derived(engine.replica(ws));
	const me = $derived(session.user?.id ?? '');
	const setup = $derived(listViews.get(ws, list));
	const canSave = $derived(!!session.workspace && canEditViews(session.workspace.role));

	// What a saved view of this list holds, so it means the same list.
	const scopeFilter = (f: ViewFilter): ViewFilter =>
		list === 'inbox'
			? { ...f, projects: ['inbox'] }
			: list === 'myday'
				? { ...f, myDay: true }
				: { ...f, due: f.due ?? 'dated' };

	const look = $derived<DisplayLookups>({
		projectId: null,
		fields: replica?.fields() ?? [],
		inbox: list === 'inbox',
		columns: list === 'upcoming' ? { by: 'due' } : undefined
	});

	const types: { id: ListType; label: string; icon: typeof List }[] = [
		{ id: 'list', label: 'List', icon: List },
		{ id: 'board', label: 'Board', icon: Kanban },
		{ id: 'calendar', label: 'Calendar', icon: CalendarDays },
		{ id: 'table', label: 'Table', icon: Table2 },
		{ id: 'timeline', label: 'Timeline', icon: ChartGantt }
	];

	const plain = $derived(
		setup.type === 'list' &&
			filterCount(setup.filter) === 0 &&
			Object.keys(setup.display).length === 0
	);
	const isChanged = $derived(changed(setup));
	const displayed = $derived(Object.keys(setup.display).length > 0);

	let filtering = $state(false);
	let displaying = $state(false);
	let filterButton = $state<HTMLButtonElement>();
	let displayButton = $state<HTMLButtonElement>();

	function setType(type: ListType) {
		listViews.setType(ws, list, type);
		announce(`${types.find((t) => t.id === type)?.label} view`);
	}
	function setFilter(edit: ViewEdit<ViewFilter, Partial<ViewFilter>>) {
		listViews.editFilter(ws, list, edit);
	}
	function setDisplay(
		edit: ViewEdit<ViewDisplay, Partial<Record<DisplayPath, unknown>>>,
		say?: (display: ViewDisplay) => string
	) {
		const display = listViews.editDisplay(ws, list, edit);
		if (say) announce(say(display));
	}
	function closePanel(which: 'filter' | 'display') {
		if (which === 'filter') {
			filtering = false;
			filterButton?.focus();
		} else {
			displaying = false;
			displayButton?.focus();
		}
	}
	function reset() {
		const before = setup;
		filtering = displaying = false;
		listViews.set(ws, list, freshSetup());
		toasts.show(`${title} is back to how it comes`, {
			onundo: () => listViews.set(ws, list, before)
		});
	}
	async function saveAsView() {
		const type = setup.type;
		const name = `${title} ${types.find((t) => t.id === type)?.label.toLowerCase()}`;
		const id = await createView(ws, {
			name,
			type,
			projectId: null,
			ownerId: me,
			filter: scopeFilter(setup.filter),
			// A board keeps the list's own columns, which a saved view would not default to.
			display:
				type === 'board' && !setup.display.columns && look.columns
					? { ...setup.display, columns: look.columns }
					: setup.display
		});
		toasts.show(`Saved "${name}" as a view. Only you see it until you share it.`);
		await goto(resolve('/(app)/views/[id]', { id }));
	}
</script>

<div class="frame" data-screen>
	<header class="page">
		<h1>{title}</h1>
		{#if extra}{@render extra()}{/if}
		<div class="types" role="group" aria-label="Show as">
			{#each types as t (t.id)}
				<button
					type="button"
					class="tool"
					aria-pressed={setup.type === t.id}
					title={`Show ${title} as a ${t.label.toLowerCase()}`}
					onclick={() => setType(t.id)}
				>
					<t.icon size={15} /><span class="word">{t.label}</span>
				</button>
			{/each}
		</div>
		<button
			title={`Choose which of ${title}'s tasks show`}
			class="tool"
			type="button"
			aria-expanded={filtering}
			bind:this={filterButton}
			onclick={() => (filtering = !filtering)}
		>
			<ListFilter size={15} /><span class="word">Filter</span>
			{#if filterCount(setup.filter)}<span class="badge tabular">{filterCount(setup.filter)}</span
				>{/if}
		</button>
		{#if setup.type !== 'calendar'}
			<button
				title="Choose what shows, and how it is sorted and grouped"
				class="tool"
				type="button"
				aria-expanded={displaying}
				aria-describedby={displayed ? `${list}-display-changed` : undefined}
				bind:this={displayButton}
				onclick={() => (displaying = !displaying)}
			>
				<SlidersHorizontal size={15} /><span class="word">Display</span>
				{#if displayed}<span class="dot" aria-hidden="true"></span>{/if}
			</button>
			<span id={`${list}-display-changed`} hidden>Changed from the defaults</span>
		{/if}
		<span class="spacer"></span>
		{#if isChanged}
			<button
				class="tool"
				type="button"
				title={`Put ${title} back as it comes: a list, no filter, the usual display`}
				onclick={reset}
			>
				<RotateCcw size={15} /><span class="word">Reset</span>
			</button>
		{/if}
		{#if canSave}
			<button
				class="tool"
				type="button"
				title="Save this as a view of its own"
				onclick={() => void saveAsView()}
			>
				<BookmarkPlus size={15} /><span class="word">Save as view</span>
			</button>
		{/if}
	</header>
	{#if filtering}
		<!-- Loaded when first opened, as on a saved view's page. -->
		{#await import('./FilterPanel.svelte') then { default: FilterPanel }}
			<FilterPanel
				{ws}
				filter={setup.filter}
				acrossProjects={list !== 'inbox'}
				projectId={null}
				hide={list === 'myday' ? ['myDay'] : undefined}
				onchange={setFilter}
				onclose={() => closePanel('filter')}
			/>
		{/await}
	{/if}
	{#if displaying && setup.type !== 'calendar'}
		{#await import('./DisplayPanel.svelte') then { default: DisplayPanel }}
			<DisplayPanel
				type={setup.type}
				display={setup.display}
				{look}
				ownGroup={`As ${title} has it`}
				onchange={setDisplay}
				onclose={() => closePanel('display')}
			/>
		{/await}
	{/if}
	<div class="body" class:rows={plain || setup.type === 'list'}>
		{#if plain}
			{@render own()}
		{:else}
			{#if add && setup.type === 'list'}{@render add()}{/if}
			{#await import('./ListBody.svelte') then { default: ListBody }}
				<ListBody
					{ws}
					{list}
					{setup}
					{scope}
					{sectionsOf}
					unfiltered={filterCount(setup.filter) === 0}
					onchange={setDisplay}
					{label}
					{emptyTitle}
					{emptyHint}
				/>
			{/await}
		{/if}
	</div>
</div>

<style>
	/* One screen tall, less what the layout puts over it. */
	.frame {
		display: flex;
		flex-direction: column;
		height: calc(100% - var(--above, 0px));
	}
	/* The header and the panels keep their height; the tasks below take what
	   is left and scroll. */
	.frame > :global(:not(.body)) {
		flex: none;
	}
	.page {
		flex-wrap: wrap;
		height: auto;
		min-height: 52px;
		padding-block: var(--space-2);
		gap: var(--space-2);
	}
	/* The tasks scroll here, below the header, as on a saved view's page. */
	.body {
		flex: 1;
		min-height: 0;
		overflow: auto;
	}
	/* Rows scroll clear of the phone's FAB. */
	.body.rows {
		padding-bottom: var(--fab-room, 0px);
	}
	.types {
		display: inline-flex;
		padding: 2px;
		border-radius: var(--radius-md);
		background: var(--surface-2);
	}
	.spacer {
		flex: 1;
	}
	.tool {
		position: relative;
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		padding: var(--space-1) var(--space-2);
		border: none;
		border-radius: calc(var(--radius-md) - 2px);
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.types .tool[aria-pressed='true'] {
		background: var(--surface-0);
		color: var(--text-primary);
		box-shadow: var(--shadow-sm);
	}
	.tool:hover,
	.tool[aria-expanded='true'] {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	.badge {
		min-width: 18px;
		padding: 0 4px;
		border-radius: var(--radius-full);
		background: var(--accent);
		color: var(--accent-contrast);
		font-size: var(--text-xs);
		text-align: center;
	}
	.dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--accent);
	}
	/* On a phone the buttons keep their icons and lose their words. */
	@media (max-width: 720px) {
		.word {
			position: absolute;
			width: 1px;
			height: 1px;
			overflow: hidden;
			clip-path: inset(50%);
			white-space: nowrap;
		}
	}
</style>
