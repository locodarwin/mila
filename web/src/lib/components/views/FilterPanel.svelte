<script lang="ts">
	import Check from '@lucide/svelte/icons/check';
	import Minus from '@lucide/svelte/icons/minus';
	import Plus from '@lucide/svelte/icons/plus';
	import { tick } from 'svelte';
	import Checkbox from '../Checkbox.svelte';
	import type { ViewEdit } from '../../sync/data';
	import FieldConditionRow from './FieldCondition.svelte';
	import { CATEGORY_LABELS, FIELD_OPS, statusChoices } from '../../filter';
	import { session } from '../../session.svelte';
	import { engine } from '../../sync/engine.svelte';
	import type { DueFilter, FieldCondition, Priority, ViewFilter } from '../../sync/entities';

	interface Props {
		ws: string;
		filter: ViewFilter;
		/** Offers a project choice only for views across the workspace. */
		acrossProjects: boolean;
		/** The view's project, whose fields conditions can be on; else every field. */
		projectId?: string | null;
		/**
		 * A change: the parts it sets (undefined takes one away), or how to
		 * work them out from the filter as it stands when it is saved, so
		 * quick changes to a list build on each other.
		 */
		onchange: (edit: ViewEdit<ViewFilter, Partial<ViewFilter>>) => void | Promise<void>;
		/** Escape inside the panel. */
		onclose?: () => void;
		/**
		 * Parts that mean nothing here: My Day's own list has no use for "In
		 * my My Day", nor Reports, which count finished tasks, for "Show
		 * finished tasks".
		 */
		hide?: ('myDay' | 'showDone')[];
	}

	let { ws, filter, acrossProjects, projectId = null, onchange, onclose, hide }: Props = $props();
	let panel = $state<HTMLElement>();
	const replica = $derived(engine.replica(ws));
	const fields = $derived(replica?.fields(projectId ?? undefined) ?? []);
	// Fields of a type from a newer app cannot start a condition here.
	const usable = $derived(fields.filter((f) => FIELD_OPS[f.type]));
	// Parts of the wrong type (from a broken app) read as unset here, though
	// the view shows nothing until they are set again; a condition that is
	// not an object reads as one on no field, which can be removed.
	const listOf = (f: ViewFilter) => (Array.isArray(f.fields) ? (f.fields as FieldCondition[]) : []);
	const chosen = (f: ViewFilter, key: ListKey): string[] => {
		const v: unknown = f[key];
		return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : [];
	};
	const asCondition = (c: unknown): FieldCondition =>
		typeof c === 'object' && c !== null && !Array.isArray(c)
			? (c as FieldCondition)
			: ({} as FieldCondition);
	const conditions = $derived(listOf(filter));
	/** Changes the conditions as they stand when the change is saved. */
	const setConditions = (edit: (all: FieldCondition[]) => FieldCondition[]) =>
		onchange((f) => {
			const next = edit(listOf(f));
			return { fields: next.length ? next : undefined };
		});
	function addCondition() {
		const f = usable[0];
		if (!f) return;
		const op = FIELD_OPS[f.type][0];
		// A number or a date starts at 0 (today), so the new condition says something.
		const value = f.type === 'number' || f.type === 'date' ? 0 : undefined;
		void setConditions((all) => [...all, { field: f.id, op, value }]);
	}
	/** Removes a condition; focus goes to the next one's Remove, else to Add. */
	async function removeCondition(i: number) {
		await setConditions((all) => all.filter((_, j) => j !== i));
		await tick();
		const next =
			panel?.querySelectorAll<HTMLElement>('[data-remove-condition]')[i] ??
			panel?.querySelector<HTMLElement>('[data-add-condition]') ??
			panel?.querySelector<HTMLElement>('input');
		next?.focus();
	}

	type ListKey =
		| 'projects'
		| 'statuses'
		| 'statusIds'
		| 'priorities'
		| 'labels'
		| 'assignees'
		| 'dependencies'
		| 'time';

	// Every time condition must hold, so having an estimate and having none
	// take each other's place rather than match nothing together.
	const OPPOSITE: Record<string, string> = { estimated: 'unestimated', unestimated: 'estimated' };

	function toggle(key: ListKey, value: string) {
		void onchange((f) => {
			const list = chosen(f, key).filter((v) => key !== 'time' || v !== OPPOSITE[value]);
			const next = list.includes(value) ? list.filter((v) => v !== value) : [...list, value];
			return { [key]: next.length ? next : undefined };
		});
	}

	// Statuses by name beside the categories, from the projects in view: the
	// view's project, else the projects the filter picks, else every one.
	const inView = $derived.by(() => {
		if (projectId) return [projectId];
		const picked = chosen(filter, 'projects').filter((p) => p !== 'inbox');
		return picked.length ? picked : (replica?.projects() ?? []).map((p) => p.id);
	});
	const ownStatuses = $derived(statusChoices(inView.flatMap((p) => replica?.statuses(p) ?? [])));
	/** Whether a status choice is on: all its statuses picked, some, or none. */
	const pickedOf = (ids: string[]): boolean | 'mixed' => {
		const on = chosen(filter, 'statusIds');
		const n = ids.filter((id) => on.includes(id)).length;
		return n === 0 ? false : n === ids.length ? true : 'mixed';
	};
	/** Picks every status of a name, or, when all are picked, none. */
	function toggleStatus(ids: string[]) {
		void onchange((f) => {
			const list = chosen(f, 'statusIds');
			const next = ids.every((id) => list.includes(id))
				? list.filter((id) => !ids.includes(id))
				: [...list, ...ids.filter((id) => !list.includes(id))];
			return { statusIds: next.length ? next : undefined };
		});
	}
	const priorities: { value: Priority; label: string }[] = [
		{ value: 'urgent', label: 'Urgent' },
		{ value: 'high', label: 'High' },
		{ value: 'medium', label: 'Medium' },
		{ value: 'low', label: 'Low' },
		{ value: 'none', label: 'No priority' }
	];
	const dues: { value: DueFilter | ''; label: string }[] = [
		{ value: '', label: 'Any time' },
		{ value: 'overdue', label: 'Overdue' },
		{ value: 'today', label: 'Due today or before' },
		{ value: 'week', label: 'Due within a week' },
		{ value: 'dated', label: 'Has a due date' },
		{ value: 'none', label: 'No due date' }
	];
	const groups = $derived([
		...(acrossProjects
			? [
					{
						key: 'projects' as const,
						label: 'Project',
						items: [
							{ value: 'inbox', label: 'Inbox' },
							...(replica?.projects() ?? []).map((p) => ({ value: p.id, label: p.name }))
						]
					}
				]
			: []),
		{ key: 'statuses' as const, label: 'Status', items: CATEGORY_LABELS },
		{ key: 'priorities' as const, label: 'Priority', items: priorities },
		{
			key: 'labels' as const,
			label: 'Label',
			items: (replica?.labels() ?? []).map((l) => ({ value: l.id, label: l.name }))
		},
		{
			key: 'assignees' as const,
			label: 'Assignee',
			items: [
				{ value: 'me', label: 'Me' },
				...session
					.members(ws)
					.filter((m) => m.userId !== session.user?.id)
					.map((m) => ({ value: m.userId, label: m.name }))
			]
		},
		{
			key: 'dependencies' as const,
			label: 'Dependencies',
			items: [
				{ value: 'blocked', label: 'Is blocked' },
				{ value: 'blocking', label: 'Is blocking' }
			]
		},
		{
			key: 'time' as const,
			label: 'Time',
			items: [
				{ value: 'estimated', label: 'Has an estimate' },
				{ value: 'unestimated', label: 'No estimate' },
				{ value: 'over', label: 'Over the estimate' },
				{ value: 'recent', label: 'Logged in the last 7 days' },
				{ value: 'mine', label: 'Logged by me' }
			]
		}
	]);
</script>

<!-- Escape from any control in it closes the panel. -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
	class="panel"
	role="group"
	aria-label="Filter"
	bind:this={panel}
	onkeydown={(e) => {
		if (e.key === 'Escape' && onclose) {
			e.preventDefault();
			onclose();
		}
	}}
>
	<label class="search">
		<span>Words</span>
		<input
			type="search"
			aria-label="Words in the title or notes"
			placeholder="In the title or notes"
			value={typeof filter.text === 'string' ? filter.text : ''}
			onchange={(e) => onchange({ text: e.currentTarget.value.trim() || undefined })}
		/>
	</label>
	{#each groups as g (g.key)}
		{#if g.items.length}
			<div class="group" role="group" aria-label={g.label}>
				<span class="name">{g.label}</span>
				<div class="chips">
					{#each g.items as item (item.value)}
						{@const on = chosen(filter, g.key).includes(item.value)}
						<!-- A check as well as the color shows which are on. -->
						<button type="button" aria-pressed={on} onclick={() => toggle(g.key, item.value)}
							>{#if on}<Check size={12} />{/if}{item.label}</button
						>
					{/each}
					{#if g.key === 'statuses'}
						{#each ownStatuses as s (s.ids[0])}
							{@const on = pickedOf(s.ids)}
							<!-- Some of a name's statuses picked: a project added it since. -->
							<button
								type="button"
								aria-pressed={on}
								title={on === 'mixed'
									? `${s.name}: picked in some projects only; click to pick it in all`
									: undefined}
								onclick={() => toggleStatus(s.ids)}
								>{#if on === 'mixed'}<Minus size={12} />{:else if on}<Check
										size={12}
									/>{/if}{s.name}</button
							>
						{/each}
					{/if}
				</div>
			</div>
		{/if}
	{/each}
	<label class="search">
		<span>Due</span>
		<select
			aria-label="Due"
			value={typeof filter.due === 'string' ? filter.due : ''}
			onchange={(e) =>
				onchange({ due: (e.currentTarget.value || undefined) as DueFilter | undefined })}
		>
			{#each dues as d (d.value)}<option value={d.value}>{d.label}</option>{/each}
		</select>
	</label>
	{#if !hide?.includes('myDay')}
		<Checkbox
			label="In my My Day"
			checked={filter.myDay === true}
			onchange={(on) => onchange({ myDay: on || undefined })}
		/>
	{/if}
	{#if !hide?.includes('showDone')}
		<Checkbox
			label="Show finished tasks"
			checked={filter.showDone === true}
			onchange={(on) => onchange({ showDone: on || undefined })}
		/>
	{/if}
	{#if fields.length || conditions.length}
		<div class="group fields" role="group" aria-label="Fields">
			<span class="name">Fields</span>
			<div class="conditions">
				{#each conditions as c, i (i)}
					<FieldConditionRow
						{ws}
						condition={asCondition(c)}
						{fields}
						n={i + 1}
						onchange={(edit) =>
							void setConditions((all) => all.map((x, j) => (j === i ? edit(asCondition(x)) : x)))}
						onremove={() => removeCondition(i)}
					/>
				{/each}
				{#if usable.length}
					<button class="add" type="button" data-add-condition onclick={addCondition}
						><Plus size={14} />Add a field condition</button
					>
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
	.search,
	.group {
		display: grid;
		grid-template-columns: 80px 1fr;
		gap: var(--space-3);
		align-items: center;
	}
	.search span,
	.name {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	input,
	select {
		max-width: 360px;
		height: 30px;
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		font-size: var(--text-sm);
	}
	.fields {
		align-items: start;
	}
	.conditions {
		display: grid;
		gap: var(--space-2);
		justify-items: start;
	}
	.add {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-height: max(var(--tap-min), 28px);
		padding: 0 var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.add:hover {
		background: var(--surface-3);
		color: var(--text-primary);
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1);
	}
	.chips button {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		padding: 2px var(--space-2);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-full);
		background: var(--surface-0);
		color: var(--text-secondary);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.chips button:hover {
		border-color: var(--border-strong);
	}
	.chips button[aria-pressed='true'],
	.chips button[aria-pressed='mixed'] {
		border-color: var(--accent);
		background: var(--accent-soft);
		color: var(--text-primary);
	}
	@media (max-width: 720px) {
		.search,
		.group {
			grid-template-columns: 1fr;
			gap: var(--space-1);
		}
	}
</style>
