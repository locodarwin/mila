<script lang="ts">
	// A saved view's sort, grouping (lists and tables), columns (boards) and
	// the fields rows, cards or table columns show. Each change is a patch of the parts it touches
	// (lib/viewDisplay.ts), with what to say about it.
	import Plus from '@lucide/svelte/icons/plus';
	import X from '@lucide/svelte/icons/x';
	import { tick } from 'svelte';
	import Checkbox from '../Checkbox.svelte';
	import type { ViewEdit } from '../../sync/data';
	import type { SortBy, SortRule, ViewDisplay } from '../../sync/entities';
	import {
		GROUP_LABELS,
		SORT_KEYS,
		acrossProjects,
		columnFields,
		columnsValid,
		defaultDir,
		dirLabel,
		groupValid,
		offeredFields,
		resolveDisplay,
		shownPatch,
		sortLabel,
		sortRuleValid,
		sortSaid,
		storedSort,
		groupSaid,
		type DisplayLookups,
		type DisplayPath,
		type FieldsKind
	} from '../../viewDisplay';

	type Patch = Partial<Record<DisplayPath, unknown>>;

	interface Props {
		type: FieldsKind | 'timeline';
		display: ViewDisplay;
		look: DisplayLookups;
		/**
		 * A change: the parts it sets (undefined takes one away), or how to
		 * work them out from the display as it stands when it is saved, so
		 * quick changes build on each other; and what to say once it is.
		 */
		onchange: (
			edit: ViewEdit<ViewDisplay, Patch>,
			say?: (display: ViewDisplay) => string
		) => void | Promise<void>;
		/** Escape inside the panel. */
		onclose?: () => void;
		/**
		 * The grouping a list of the app's own has when none is chosen (Upcoming's
		 * days, My Day's sections), named so; a list's Group by offers it first.
		 */
		ownGroup?: string;
	}

	let { type, display, look, onchange, onclose, ownGroup }: Props = $props();
	let panel = $state<HTMLElement>();

	const resolved = $derived(resolveDisplay(display, type, look));
	const SORTABLE = ['text', 'url', 'number', 'date', 'select', 'user', 'checkbox'];
	const sortFields = $derived(look.fields.filter((f) => SORTABLE.includes(f.type)));
	const groupFields = $derived(look.fields.filter((f) => f.type === 'select' || f.type === 'user'));
	const keys = $derived(SORT_KEYS.filter((k) => !(k.by === 'project' && look.projectId)));

	// Sort: the rules as stored, each shown even when this app cannot use
	// it ("Not available"), so it can be seen and removed; such a rule may
	// be anything, null too, so only a valid one is read. A change to them
	// is worked out from the rules as they stand when it is saved.
	const rules = $derived(storedSort(display));
	const keyOf = (r: unknown) => {
		const rule = r as SortRule;
		return rule?.by === 'field' ? `field:${rule.field}` : String(rule?.by);
	};
	function setSort(edit: (rules: unknown[]) => unknown[]) {
		return onchange(
			(d) => {
				const next = edit(storedSort(d));
				return { sort: next.length ? next : undefined };
			},
			(d) =>
				sortSaid(
					storedSort(d).filter((r): r is SortRule => sortRuleValid(r, look)),
					look.fields
				)
		);
	}
	function setKey(i: number, key: string) {
		if (key === 'manual') return setSort((all) => all.slice(0, i));
		const [by, field] = key.startsWith('field:')
			? (['field', key.slice(6)] as const)
			: ([key as SortBy, undefined] as const);
		const rule: SortRule = { by, ...(field ? { field } : {}), dir: defaultDir(by) };
		return setSort((all) => all.map((r, j) => (j === i ? rule : r)));
	}
	function setDir(i: number, dir: 'asc' | 'desc') {
		return setSort((all) => all.map((r, j) => (j === i ? { ...(r as SortRule), dir } : r)));
	}
	function addSort() {
		return setSort((all) => {
			const used = new Set(all.map(keyOf));
			const next = keys.find((k) => k.by !== 'manual' && !used.has(k.by));
			return next && all.length < 3 ? [...all, { by: next.by, dir: defaultDir(next.by) }] : all;
		});
	}
	/** Removes a rule; focus goes to the next rule's Remove, else to Add a sort. */
	async function removeSort(i: number) {
		await setSort((all) => all.filter((_, j) => j !== i));
		await tick();
		const next =
			panel?.querySelectorAll<HTMLElement>('[data-remove-sort]')[i] ??
			panel?.querySelector<HTMLElement>('[data-add-sort]');
		next?.focus();
	}
	const SAY_KEY = ['Sort by', 'Then by', 'And then by'];

	// Grouping and columns, as "by" or "field:<id>".
	const groupKey = $derived(
		resolved.group.by === 'field' ? `field:${resolved.group.field}` : resolved.group.by
	);
	const groupLost = $derived(display.group !== undefined && !groupValid(display.group, look));
	const groupChoices = $derived([
		...(Object.keys(GROUP_LABELS) as (keyof typeof GROUP_LABELS)[])
			.filter((by) => !(by === 'project' && look.projectId))
			.map((by) => ({ value: by as string, label: GROUP_LABELS[by] })),
		...groupFields.map((f) => ({ value: `field:${f.id}`, label: f.name }))
	]);
	function setGroup(value: string) {
		const group = value.startsWith('field:')
			? { by: 'field' as const, field: value.slice(6) }
			: { by: value as keyof typeof GROUP_LABELS };
		void onchange({ group }, () => groupSaid(group, look.fields));
	}

	const columnsKey = $derived(
		resolved.columns.by === 'field' ? `field:${resolved.columns.field}` : resolved.columns.by
	);
	const columnsLost = $derived(
		display.columns !== undefined && !columnsValid(display.columns, look)
	);
	function setColumns(value: string) {
		const columns = value.startsWith('field:')
			? { by: 'field' as const, field: value.slice(6) }
			: { by: value as 'status' | 'priority' | 'due' };
		const name =
			columns.by === 'field'
				? (look.fields.find((f) => f.id === columns.field)?.name ?? 'a field')
				: columns.by === 'due'
					? 'due date'
					: columns.by;
		void onchange({ columns }, () => `Columns by ${name}`);
	}

	// Shown fields: the view's show and hide, over the type's defaults,
	// worked out from them as they stand when the change is saved.
	const offered = $derived(offeredFields(look));
	function setShown(id: string, on: boolean) {
		if (type === 'timeline') return; // a timeline shows no fields
		const kind = type;
		const label = offered.find((f) => f.id === id)?.label ?? 'The field';
		void onchange(
			(d) => ({ [`fields.${kind}`]: shownPatch(d, kind, id, on, acrossProjects(look)) }),
			() => `${label} ${on ? 'shown' : 'hidden'}`
		);
	}

	// A table's own column order and widths, set by dragging its headings.
	// Read here rather than through table.ts, which loads only with a table.
	const tableOf = (d: ViewDisplay): Record<string, unknown> =>
		typeof d.table === 'object' && d.table !== null && !Array.isArray(d.table)
			? (d.table as Record<string, unknown>)
			: {};
	const laidOut = $derived.by(() => {
		const t = tableOf(display);
		return (
			(Array.isArray(t.order) && t.order.length > 0) ||
			(Array.isArray(t.widths) && t.widths.length > 0)
		);
	});
	function resetLayout() {
		void onchange(
			(d) => {
				const rest = { ...tableOf(d) };
				delete rest.order;
				delete rest.widths;
				return { table: Object.keys(rest).length ? rest : undefined };
			},
			() => 'Columns back in their first order and widths'
		);
	}
</script>

<!-- Escape from any control in it closes the panel. -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
	class="panel"
	role="group"
	aria-label="Display"
	bind:this={panel}
	onkeydown={(e) => {
		if (e.key === 'Escape' && onclose) {
			e.preventDefault();
			onclose();
		}
	}}
>
	<div class="part" role="group" aria-label="Sort">
		<span class="name">Sort</span>
		<div class="rows">
			{#each rules as rule, i (i)}
				{@const valid = sortRuleValid(rule, look)}
				{@const r = rule as SortRule}
				{@const field = valid ? look.fields.find((f) => f.id === r.field) : undefined}
				<div class="rule">
					<select
						aria-label={SAY_KEY[i]}
						value={valid ? keyOf(rule) : ''}
						onchange={(e) => void setKey(i, e.currentTarget.value)}
					>
						{#if !valid}<option value="" disabled>Not available</option>{/if}
						{#each keys as k (k.by)}<option value={k.by}>{k.label}</option>{/each}
						{#each sortFields as f (f.id)}<option value={`field:${f.id}`}>{f.name}</option>{/each}
					</select>
					{#if valid}
						<select
							aria-label={`Direction of ${sortLabel(r, look.fields)}`}
							value={r.dir}
							onchange={(e) => void setDir(i, e.currentTarget.value as 'asc' | 'desc')}
						>
							<option value="asc">{dirLabel({ ...r, dir: 'asc' }, field)}</option>
							<option value="desc">{dirLabel({ ...r, dir: 'desc' }, field)}</option>
						</select>
					{/if}
					<button
						class="icon"
						data-remove-sort
						type="button"
						aria-label={`Remove sort by ${valid ? sortLabel(r, look.fields) : 'something not available'}`}
						title="Remove sort"
						onclick={() => removeSort(i)}><X size={14} /></button
					>
				</div>
			{:else}
				<span class="note">Manual order</span>
			{/each}
			{#if rules.length < 3}
				<button class="add" type="button" data-add-sort onclick={addSort}
					><Plus size={14} />Add a sort</button
				>
			{/if}
		</div>
	</div>
	{#if type === 'list' || type === 'table' || type === 'timeline'}
		<label class="part">
			<span class="name">Group by</span>
			<select
				value={groupLost ? '' : ownGroup && type === 'list' && !display.group ? 'own' : groupKey}
				onchange={(e) =>
					e.currentTarget.value === 'own'
						? void onchange({ group: undefined }, () => ownGroup ?? '')
						: setGroup(e.currentTarget.value)}
			>
				{#if groupLost}<option value="" disabled>Not available</option>{/if}
				{#if ownGroup && type === 'list'}<option value="own">{ownGroup}</option>{/if}
				{#each groupChoices as g (g.value)}<option value={g.value}>{g.label}</option>{/each}
			</select>
		</label>
	{:else if type === 'board'}
		<label class="part">
			<span class="name">Columns</span>
			<select
				value={columnsLost ? '' : columnsKey}
				onchange={(e) => setColumns(e.currentTarget.value)}
			>
				{#if columnsLost}<option value="" disabled>Not available</option>{/if}
				{#if look.projectId}<option value="status">Status</option>{/if}
				<option value="priority">Priority</option>
				<option value="due">Due date</option>
				{#each columnFields(look) as f (f.id)}<option value={`field:${f.id}`}>{f.name}</option
					>{/each}
			</select>
		</label>
	{/if}
	{#if type !== 'timeline'}
		<div class="part" role="group" aria-label="Show">
			<span class="name">Show</span>
			<div class="shown">
				{#each offered as f (f.id)}
					<Checkbox
						label={f.label}
						checked={resolved.shown.includes(f.id)}
						onchange={(on) => setShown(f.id, on)}
					/>
				{/each}
			</div>
		</div>
	{/if}
	{#if type === 'table'}
		<div class="part" role="group" aria-label="Columns">
			<span class="name">Columns</span>
			<div class="rows">
				{#if laidOut}
					<button class="add" type="button" onclick={resetLayout}>Reset order and widths</button>
				{:else}
					<span class="note">Drag a heading to move a column, or its edge to resize it.</span>
				{/if}
			</div>
		</div>
	{/if}
</div>

<style>
	.panel {
		position: relative;
		display: grid;
		gap: var(--space-3);
		padding: var(--space-3) var(--space-4);
		border-bottom: var(--border-width) solid var(--border);
		background: var(--surface-1);
	}
	.part {
		display: grid;
		grid-template-columns: 80px 1fr;
		gap: var(--space-3);
		align-items: start;
	}
	.name,
	.note {
		padding-top: 5px;
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.rows {
		display: grid;
		gap: var(--space-2);
		justify-items: start;
	}
	.rule {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
	select {
		justify-self: start;
		max-width: 360px;
		min-height: max(var(--tap-min), 30px);
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
		font-size: var(--text-sm);
	}
	.icon,
	.add {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-1);
		min-width: max(var(--tap-min), 28px);
		min-height: max(var(--tap-min), 28px);
		padding: 0 var(--space-1);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.add {
		padding: 0 var(--space-2);
	}
	.icon:hover,
	.add:hover {
		background: var(--surface-3);
		color: var(--text-primary);
	}
	.shown {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2) var(--space-4);
		padding-top: 4px;
	}
	@media (max-width: 720px) {
		.part {
			grid-template-columns: 1fr;
			gap: var(--space-1);
		}
	}
</style>
