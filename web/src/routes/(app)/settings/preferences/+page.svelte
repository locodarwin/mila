<script lang="ts">
	import Checkbox from '#lib/components/Checkbox.svelte';
	import DatePreferences from '#lib/components/DatePreferences.svelte';
	import Keys from '#lib/components/help/Keys.svelte';
	import SidebarEditor from '#lib/components/SidebarEditor.svelte';
	import { preferences, type Preferences } from '#lib/preferences.svelte.js';

	const startPages: { value: Preferences['startPage']; label: string }[] = [
		{ value: 'inbox', label: 'Inbox' },
		{ value: 'today', label: 'My Day' },
		{ value: 'upcoming', label: 'Upcoming' },
		{ value: 'projects', label: 'Projects' }
	];
	const taskPanels: { value: Preferences['taskPanel']; label: string; says: string }[] = [
		{
			value: 'boxes',
			label: 'Labeled boxes',
			says: 'Everything shows, grouped under clear names.'
		},
		{
			value: 'summary',
			label: 'Summary lines',
			says: 'The main choices on top, then one line per section that opens in place.'
		}
	];
</script>

<svelte:head><title>Preferences · Mila</title></svelte:head>

<p class="hint">These follow you to every device you sign in on.</p>

<section class="half">
	<h2>Dates and times</h2>
	<DatePreferences value={preferences.current} onchange={(change) => preferences.set(change)} />
</section>

<section class="half">
	<h2>Lists</h2>
	<div class="start">
		<label for="start-page">Open Mila on</label>
		<select
			id="start-page"
			value={preferences.current.startPage}
			onchange={(e) =>
				preferences.set({ startPage: e.currentTarget.value as Preferences['startPage'] })}
		>
			{#each startPages as p (p.value)}<option value={p.value}>{p.label}</option>{/each}
		</select>
	</div>
	<Checkbox
		label="Show completed tasks when a list opens"
		checked={preferences.current.showCompleted}
		onchange={(on) => preferences.set({ showCompleted: on })}
	/>
</section>

<section class="half">
	<h2>Task panel</h2>
	<div class="panel-choices" role="radiogroup" aria-label="Task panel layout">
		{#each taskPanels as p (p.value)}
			<label class="panel-choice">
				<input
					type="radio"
					name="task-panel"
					value={p.value}
					checked={preferences.current.taskPanel === p.value}
					onchange={() => preferences.set({ taskPanel: p.value })}
				/>
				<span><strong>{p.label}</strong><span class="says">{p.says}</span></span>
			</label>
		{/each}
	</div>
</section>

<section class="half">
	<h2>Sidebar</h2>
	<p class="hint">
		Hide what you do not use, and put the rest in your order: the arrow buttons move an item<Keys
			id="sidebar.move"
			lead=", as do "
		/>.
	</p>
	<SidebarEditor
		order={preferences.current.sidebarOrder}
		hidden={preferences.current.sidebarHidden}
		onchange={(change) =>
			preferences.set({
				...(change.order ? { sidebarOrder: change.order } : {}),
				...(change.hidden ? { sidebarHidden: change.hidden } : {})
			})}
	/>
</section>

<style>
	.start {
		display: grid;
		gap: var(--space-1);
		justify-self: start;
	}
	label {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.panel-choices {
		display: grid;
		gap: var(--space-2);
	}
	.panel-choice {
		display: flex;
		align-items: flex-start;
		gap: var(--space-2);
		min-height: var(--tap-min);
		font-size: var(--text-sm);
		font-weight: 400;
		cursor: pointer;
	}
	.panel-choice input {
		flex: none;
		width: max(var(--tap-min), 13px);
		height: max(var(--tap-min), 13px);
		margin: 3px 0 0;
		accent-color: var(--accent);
	}
	.panel-choice strong {
		display: block;
		color: var(--text-primary);
		font-weight: 550;
	}
	.says {
		color: var(--text-secondary);
	}
	select {
		height: calc(32px * var(--control-scale));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		color: var(--text-primary);
		font-size: var(--text-sm);
		font-weight: 400;
	}
</style>
