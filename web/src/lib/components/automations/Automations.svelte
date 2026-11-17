<script lang="ts">
	// Settings, Automations: a workspace's rules, the builder,
	// a test against the tasks there, and what they did lately.
	import { resolve } from '$app/paths';
	import { api, call, type Role, type Schemas } from '../../api/client';
	import {
		CHANGES,
		DUE_DAYS,
		firstOf,
		fits,
		KIND,
		KINDS,
		MAX_AUTOMATIONS,
		MAX_CHANGES,
		PRIORITIES,
		readRule,
		ready,
		sentenceOf,
		SOON,
		STARTS,
		WHO,
		changeWords,
		type Change,
		type ChangeKind,
		type Names,
		type Picks,
		type Rule,
		type StartKind
	} from '../../automations';
	import { formatDate } from '../../dates';
	import { filterCount, type FilterContext } from '../../filter';
	import { canAutomateWorkspace } from '../../roles';
	import { session } from '../../session.svelte';
	import {
		deleteAutomation,
		makeAutomation,
		saveAutomation,
		setAutomationOn
	} from '../../sync/automations';
	import { localToday } from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import type { Automation, FieldType, ViewFilter } from '../../sync/entities';
	import { toasts } from '../../toasts.svelte';
	import Button from '../Button.svelte';
	import FilterPanel from '../views/FilterPanel.svelte';

	interface Props {
		ws: string;
		role: Role;
		me: string;
		/** Nothing from the server: the style guide's sample. */
		sample?: boolean;
	}

	let { ws, role, me, sample = false }: Props = $props();

	const replica = $derived(engine.replica(ws));
	const rules = $derived(replica?.automations() ?? []);
	const projects = $derived(replica?.projects() ?? []);
	const wide = $derived(canAutomateWorkspace(role));
	const people = $derived(session.members(ws).map((m) => ({ id: m.userId, name: m.name })));

	// The one being edited, or a new one.
	let editing = $state<string | null>(null);
	let name = $state('New automation');
	let project = $state<string | null>(null);
	let rule = $state<Rule>({ when: { on: 'created' }, then: [{ do: 'priority', value: 'high' }] });
	let open = $state(false);
	let saving = $state(false);
	let showFilter = $state(false);

	const statuses = $derived(project ? (replica?.statuses(project) ?? []) : []);
	const labels = $derived(replica?.labels() ?? []);
	const fields = $derived(replica?.fields(project ?? undefined) ?? []);

	const names = $derived<Names>({
		status: (id) => replica?.status(id)?.name,
		label: (id) => labels.find((l) => l.id === id)?.name,
		field: (id) =>
			fields.find((f) => f.id === id)?.name ?? replica?.fields().find((f) => f.id === id)?.name,
		person: (id) => session.memberName(ws, id) ?? undefined,
		project: (id) => projects.find((p) => p.id === id)?.name
	});

	const canEdit = (a: Automation) => wide || a.projectId !== null;

	function start() {
		editing = null;
		name = 'New automation';
		project = wide ? null : (projects[0]?.id ?? null);
		rule = { when: { on: 'created' }, then: [{ do: 'priority', value: 'high' }] };
		showFilter = false;
		open = true;
	}

	function edit(a: Automation) {
		const read = readRule(a.rule);
		if (!read) {
			toasts.error('This rule came from a newer Mila. Reload to change it.');
			return;
		}
		editing = a.id;
		name = a.name;
		project = a.projectId;
		rule = structuredClone($state.snapshot(read) as Rule);
		showFilter = filterCount(rule.config?.filter ?? {}) > 0;
		open = true;
	}

	// What can be chosen for a part of a rule, as it stands.
	function choices(picks: Picks | undefined): { id: string; name: string }[] {
		switch (picks) {
			case 'status':
				return project
					? statuses.map((s) => ({ id: s.id, name: s.name }))
					: KINDS.map((k) => ({ id: KIND + k.id, name: k.name }));
			case 'priority':
				return PRIORITIES;
			case 'anyone':
				return [{ id: '', name: 'anyone' }, ...people];
			case 'who':
				return [...people, ...WHO];
			case 'label':
				return labels.map((l) => ({ id: l.id, name: l.name }));
			case 'field':
				return fields.map((f) => ({ id: f.id, name: f.name }));
			case 'project':
				return projects.filter((p) => p.id !== project).map((p) => ({ id: p.id, name: p.name }));
			case 'soon':
				return SOON;
			case 'dueDay':
				return DUE_DAYS;
		}
		return [];
	}

	const first = (picks: Picks | undefined) =>
		firstOf(picks, {
			status: choices('status')[0]?.id ?? '',
			label: labels[0]?.id ?? '',
			field: fields[0]?.id ?? '',
			person: people[0]?.id ?? '',
			project: choices('project')[0]?.id ?? ''
		});

	const startPicks = $derived(STARTS.find((s) => s.on === rule.when.on)?.picks);
	const picksOf = (c: Change) => CHANGES.find((k) => k.do === c.do)?.picks;

	function setStart(on: StartKind) {
		rule.when = { on, value: first(STARTS.find((s) => s.on === on)?.picks) };
	}

	function setChange(i: number, kind: ChangeKind) {
		rule.then[i] = { do: kind, value: first(CHANGES.find((k) => k.do === kind)?.picks) };
	}

	function addChange() {
		rule.then.push({ do: 'priority', value: 'high' });
	}

	// A rule's status belongs to where it works, so a change of place starts them over.
	function setProject(next: string | null) {
		project = next;
		if (rule.when.on === 'status') rule.when.value = first('status');
		rule.then = rule.then.map((c) => (c.do === 'status' ? { ...c, value: first('status') } : c));
	}

	const filter = $derived<ViewFilter>(rule.config?.filter ?? {});
	const onlyCount = $derived(filterCount(filter));

	type FilterEdit = Partial<ViewFilter> | ((f: ViewFilter) => Partial<ViewFilter>);
	function changeFilter(change: FilterEdit) {
		const now = $state.snapshot(filter) as ViewFilter;
		const parts = typeof change === 'function' ? change(now) : change;
		const next: Record<string, unknown> = { ...now, ...parts };
		for (const key of Object.keys(next)) if (next[key] === undefined) delete next[key];
		rule.config = { filter: next as ViewFilter };
	}

	const sentence = $derived(sentenceOf(rule, project, names, onlyCount));
	// Someone who can't make a rule across the workspace needs a project to make one in.
	const hasPlace = $derived(wide || project !== null);
	const canSave = $derived(name.trim().length > 0 && hasPlace && ready(rule));

	async function save(e: SubmitEvent) {
		e.preventDefault();
		if (!canSave || saving) return;
		saving = true;
		const fieldsToSave = { name: name.trim(), rule: $state.snapshot(rule) as Rule };
		try {
			if (editing) await saveAutomation(ws, editing, me, fieldsToSave);
			else {
				editing = await makeAutomation(
					ws,
					me,
					{ projectId: project, ...fieldsToSave },
					rules.at(-1)?.sortKey ?? null
				);
			}
			toasts.show('Saved. It acts from now on.');
			open = false;
			void loadRuns();
		} catch (err) {
			toasts.error(err instanceof Error ? err.message : 'That rule could not be saved.');
		} finally {
			saving = false;
		}
	}

	function remove(a: Automation) {
		const { undo } = engine.track(() => deleteAutomation(ws, a.id, true));
		if (editing === a.id) open = false;
		toasts.show(`Deleted ${a.name}.`, { onundo: () => void undo() });
	}

	// The test: what the rule would do to the tasks where it works.
	const ctx = $derived.by((): FilterContext => {
		const types = new Map<string, FieldType>((replica?.fields() ?? []).map((f) => [f.id, f.type]));
		return {
			me,
			today: localToday(),
			category: (t) =>
				(t.statusId && replica?.status(t.statusId)?.category) || (t.completedAt ? 'done' : 'todo'),
			labels: (id) => (replica?.taskLabels(id) ?? []).map((l) => l.id),
			assignees: (id) => replica?.assignees(id) ?? [],
			blocked: (id) => replica?.isBlocked(id) ?? false,
			blocking: (id) => replica?.isBlocking(id) ?? false,
			entries: (id) => replica?.timeEntries(id) ?? [],
			fieldType: (id) => types.get(id),
			value: (task, field) => replica?.fieldValues(task)[field]
		};
	});
	const TEST_TASKS = 6;
	const tested = $derived.by(() => {
		if (!open) return [];
		const tasks = project
			? (replica?.tasks(project) ?? [])
			: projects.flatMap((p) => replica?.tasks(p.id) ?? []);
		const open_ = tasks.filter((t) => !t.completedAt);
		const yes = open_.filter((t) => fits(rule, t, ctx));
		const no = open_.filter((t) => !yes.includes(t));
		return [...yes.slice(0, TEST_TASKS), ...no.slice(0, Math.max(0, TEST_TASKS - yes.length))].map(
			(t) => ({ id: t.id, title: t.title, fits: yes.includes(t) })
		);
	});
	const testWords = $derived(rule.then.map((c) => changeWords(c, names)).join(', '));

	// What rules did lately.
	type Run = Schemas['AutomationRun'];
	let runs = $state<Run[]>([]);
	let runsFailed = $state(false);
	async function loadRuns() {
		if (sample || !ws) return;
		try {
			const out = await call(
				api.GET('/workspaces/{workspaceId}/automation-runs', {
					params: { path: { workspaceId: ws }, query: { limit: 50 } }
				})
			);
			runs = out.items;
			runsFailed = false;
		} catch {
			runsFailed = true;
		}
	}
	$effect(() => {
		void ws;
		void loadRuns();
	});

	const lastRun = (id: string) => runs.find((r) => r.automationId === id);
	const ruleName = (id: string) => rules.find((a) => a.id === id)?.name ?? 'A rule no longer here';
	const OUTCOMES: Record<Run['outcome'], string> = {
		ran: 'Ran',
		partly: 'Partly',
		failed: "Couldn't run"
	};
	const when = (at: string) => formatDate(at.slice(0, 10));

	const placeName = (a: Automation) =>
		a.projectId ? (names.project(a.projectId) ?? 'A project no longer here') : 'Whole workspace';
	const readSentence = (a: Automation) => {
		const read = readRule(a.rule);
		return read
			? sentenceOf(read, a.projectId, names, filterCount(read.config?.filter ?? {}))
			: 'This rule came from a newer Mila.';
	};
</script>

<section class="rules">
	<div class="head">
		<div>
			<h2>Automations</h2>
			<p class="hint">
				When something happens to a task, and it fits, Mila makes the changes you chose. A rule runs
				as whoever last saved it or turned it on.
				<a class="link" href={resolve('/(app)/help/[topic]', { topic: 'automations' })}
					>How automations work</a
				>
			</p>
		</div>
		<Button title="Make a new automation" disabled={rules.length >= MAX_AUTOMATIONS} onclick={start}
			>New automation</Button
		>
	</div>

	{#if rules.length}
		<ul class="list" aria-label="Automations">
			{#each rules as a (a.id)}
				{@const last = lastRun(a.id)}
				<li class:off={!a.enabled} class:picked={open && editing === a.id}>
					<label class="switch">
						<input
							type="checkbox"
							checked={a.enabled}
							disabled={!canEdit(a)}
							aria-label="{a.name} is on"
							title={a.enabled ? 'Turn this automation off' : 'Turn this automation on'}
							onchange={(e) => void setAutomationOn(ws, a.id, me, a.rule, e.currentTarget.checked)}
						/>
						<span aria-hidden="true"></span>
					</label>
					<div class="text">
						<strong>{a.name}</strong>
						<span class="says">{readSentence(a)}</span>
						<span class="meta">
							{placeName(a)}. Runs as {a.madeBy === me
								? 'you'
								: (session.memberName(ws, a.madeBy) ?? 'someone no longer here')}.
							{#if last}Last ran {when(last.createdAt)}.{/if}
						</span>
						{#if a.pausedWhy}<span class="paused" role="note">Stopped: {a.pausedWhy}</span>{/if}
					</div>
					{#if canEdit(a)}
						<div class="actions">
							<Button
								title="Change {a.name}"
								aria-label="Edit {a.name}"
								size="sm"
								variant="ghost"
								onclick={() => edit(a)}
							>
								Edit
							</Button>
							<Button
								title="Delete {a.name}"
								aria-label="Delete {a.name}"
								size="sm"
								variant="ghost"
								onclick={() => remove(a)}
							>
								Delete
							</Button>
						</div>
					{/if}
				</li>
			{/each}
		</ul>
	{:else if !open}
		<p class="hint">No automations yet.</p>
	{/if}
</section>

{#if open}
	<section class="builder" aria-labelledby="automation-builder">
		<h2 id="automation-builder">{editing ? 'Change this automation' : 'New automation'}</h2>
		<form onsubmit={save}>
			<label class="field">
				<span>Name</span>
				<input bind:value={name} maxlength="80" required />
			</label>
			<p class="sentence" role="status">{sentence}</p>
			{#if !hasPlace}
				<p class="hint">
					Make a project first. A rule for the whole workspace is for owners and admins.
				</p>
			{/if}

			<div class="part">
				<span class="word" id="rule-in">In</span>
				<select
					aria-labelledby="rule-in"
					value={project ?? ''}
					disabled={editing !== null}
					title={editing ? "Where a rule works can't change" : undefined}
					onchange={(e) => setProject(e.currentTarget.value || null)}
				>
					{#if wide || editing}
						<option value="">The whole workspace</option>
					{/if}
					{#each projects as p (p.id)}
						<option value={p.id}>{p.name}</option>
					{/each}
				</select>
			</div>

			<div class="part">
				<span class="word" id="rule-when">When</span>
				<div class="row">
					<select
						aria-labelledby="rule-when"
						value={rule.when.on}
						onchange={(e) => setStart(e.currentTarget.value as StartKind)}
					>
						{#each STARTS as s (s.on)}
							<option value={s.on}>{s.label}</option>
						{/each}
					</select>
					{#if startPicks}
						<select aria-label="Which" bind:value={rule.when.value}>
							{#each choices(startPicks) as c (c.id)}
								<option value={c.id}>{c.name}</option>
							{/each}
						</select>
					{/if}
				</div>
			</div>

			<div class="part">
				<span class="word">Only if</span>
				<div class="stack">
					<Button
						title="Choose which tasks this works on"
						size="sm"
						variant="ghost"
						aria-expanded={showFilter}
						onclick={() => (showFilter = !showFilter)}
					>
						{onlyCount
							? `${onlyCount} ${onlyCount === 1 ? 'condition' : 'conditions'}`
							: 'Any task'}
					</Button>
					{#if showFilter}
						<FilterPanel
							{ws}
							{filter}
							acrossProjects={project === null}
							projectId={project}
							onchange={changeFilter}
							onclose={() => (showFilter = false)}
						/>
					{/if}
				</div>
			</div>

			<div class="part">
				<span class="word">Then</span>
				<div class="stack">
					{#each rule.then as change, i (i)}
						{@const picks = picksOf(change)}
						<div class="row">
							<select
								aria-label="Change {i + 1}"
								value={change.do}
								onchange={(e) => setChange(i, e.currentTarget.value as ChangeKind)}
							>
								{#each CHANGES as k (k.do)}
									<option value={k.do}>{k.label}</option>
								{/each}
							</select>
							{#if picks === 'text'}
								<input
									aria-label="What it says"
									bind:value={rule.then[i].value}
									maxlength={change.do === 'comment' ? 20000 : 500}
									required
								/>
							{:else if picks}
								<select aria-label="Change {i + 1}, which" bind:value={rule.then[i].value}>
									{#each choices(picks) as c (c.id)}
										<option value={c.id}>{c.name}</option>
									{/each}
								</select>
							{/if}
							<Button
								title="Remove this change"
								size="sm"
								variant="ghost"
								aria-label="Remove change {i + 1}"
								onclick={() => rule.then.splice(i, 1)}>Remove</Button
							>
						</div>
					{/each}
					{#if rule.then.length < MAX_CHANGES}
						<div>
							<Button title="Add another change" size="sm" variant="ghost" onclick={addChange}>
								Add a change
							</Button>
						</div>
					{/if}
				</div>
			</div>

			<div class="row">
				<Button type="submit" variant="primary" disabled={!canSave || saving}>Save</Button>
				<Button variant="ghost" title="Leave without saving" onclick={() => (open = false)}>
					Cancel
				</Button>
			</div>
		</form>

		<div class="test">
			<h3>Test it on your tasks</h3>
			<p class="hint">
				What it would do if the When happened to these now. This changes nothing. A few open tasks,
				those that fit first.
			</p>
			{#if tested.length}
				<ul aria-label="What this rule would do">
					{#each tested as t (t.id)}
						<li>
							<span class="title">{t.title}</span>
							{#if t.fits}
								<span class="yes">It would {testWords || 'do nothing yet'}.</span>
							{:else}
								<span class="hint">Doesn't fit, so nothing.</span>
							{/if}
						</li>
					{/each}
				</ul>
			{:else}
				<p class="hint">No open tasks here to try it on.</p>
			{/if}
		</div>
	</section>
{/if}

<section class="runs" aria-labelledby="automation-runs">
	<h2 id="automation-runs">Recent runs</h2>
	<p class="hint">The last 30 days. A run that couldn't finish tells whoever saved the rule.</p>
	{#if runsFailed}
		<p class="hint">This needs a connection.</p>
	{:else if runs.length}
		<div class="scroll">
			<table>
				<thead>
					<tr>
						<th scope="col">When</th>
						<th scope="col">Automation</th>
						<th scope="col">Task</th>
						<th scope="col">What happened</th>
					</tr>
				</thead>
				<tbody>
					{#each runs as r (r.id)}
						<tr>
							<td>{when(r.createdAt)}</td>
							<td>{ruleName(r.automationId)}</td>
							<td>{r.taskTitle}</td>
							<td><span class={r.outcome}>{OUTCOMES[r.outcome]}</span>: {r.detail}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{:else}
		<p class="hint">Nothing has run yet.</p>
	{/if}
</section>

<style>
	section {
		display: grid;
		gap: var(--space-3);
		min-width: 0;
	}
	.link {
		color: var(--accent-text);
		font-weight: 550;
	}
	.link:hover {
		text-decoration: underline;
	}
	section + section {
		margin-top: var(--space-5);
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-start;
		justify-content: space-between;
		gap: var(--space-3);
	}
	.head > div {
		flex: 1 1 20rem;
		display: grid;
		gap: var(--space-1);
	}
	.list {
		display: grid;
		margin: 0;
		padding: 0;
		overflow: hidden;
		list-style: none;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
	}
	.list li {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: start;
		gap: var(--space-3);
		padding: var(--space-3);
	}
	.list li + li {
		border-top: var(--border-width) solid var(--border);
	}
	.list li.picked {
		background: var(--accent-soft);
	}
	.text {
		display: grid;
		gap: 2px;
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.says {
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	.off strong,
	.off .says {
		color: var(--text-muted);
	}
	.meta {
		color: var(--text-secondary);
		font-size: var(--text-xs);
	}
	.paused {
		color: var(--danger);
		font-size: var(--text-sm);
		font-weight: 550;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1);
	}
	.switch {
		position: relative;
		display: block;
		width: max(var(--tap-min), 36px);
		height: max(var(--tap-min), 20px);
	}
	.switch input {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		margin: 0;
		opacity: 0;
		cursor: pointer;
	}
	.switch span {
		position: absolute;
		top: calc(50% - 10px);
		left: calc(50% - 18px);
		width: 36px;
		height: 20px;
		border-radius: var(--radius-full);
		background: var(--border-control);
		pointer-events: none;
	}
	.switch span::after {
		content: '';
		position: absolute;
		top: 2px;
		left: 2px;
		width: 16px;
		height: 16px;
		border-radius: 50%;
		background: var(--surface-0);
	}
	.switch input:checked + span {
		background: var(--accent);
	}
	.switch input:checked + span::after {
		left: 18px;
	}
	.switch input:focus-visible + span {
		outline: 2px solid var(--focus-ring, var(--accent));
		outline-offset: 2px;
	}
	.switch input:disabled {
		cursor: default;
	}
	.builder {
		padding: var(--space-4);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface-1);
	}
	form {
		display: grid;
		gap: var(--space-3);
		min-width: 0;
	}
	.field {
		display: grid;
		gap: var(--space-1);
		max-width: 28rem;
		font-size: var(--text-sm);
		font-weight: 550;
	}
	.sentence {
		padding: var(--space-2) var(--space-3);
		border-left: 3px solid var(--accent);
		border-radius: 0 var(--radius-md) var(--radius-md) 0;
		background: var(--surface-0);
		overflow-wrap: anywhere;
	}
	.part {
		display: grid;
		grid-template-columns: 4.5rem minmax(0, 1fr);
		gap: var(--space-2) var(--space-3);
		align-items: start;
	}
	.word {
		padding-top: var(--space-1);
		color: var(--accent-text);
		font-size: var(--text-sm);
		font-weight: 700;
	}
	.row {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		min-width: 0;
	}
	.stack {
		display: grid;
		justify-items: start;
		gap: var(--space-2);
		min-width: 0;
	}
	.stack .row {
		width: 100%;
	}
	input:not([type='checkbox']),
	select {
		min-width: 0;
		max-width: 100%;
		height: max(var(--tap-min), calc(32px * var(--control-scale)));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
	}
	.row input {
		flex: 1 1 12rem;
	}
	.test {
		display: grid;
		gap: var(--space-2);
		padding-top: var(--space-3);
		border-top: var(--border-width) solid var(--border);
	}
	h3 {
		font-size: var(--text-sm);
	}
	.test ul {
		display: grid;
		gap: var(--space-2);
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.test li {
		display: grid;
		gap: 2px;
		overflow-wrap: anywhere;
	}
	.title {
		font-size: var(--text-sm);
		font-weight: 550;
	}
	.yes {
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	.scroll {
		overflow-x: auto;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--text-sm);
	}
	th {
		padding: var(--space-1) var(--space-3) var(--space-1) 0;
		border-bottom: var(--border-width) solid var(--border);
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-weight: 600;
		text-align: left;
	}
	td {
		padding: var(--space-2) var(--space-3) var(--space-2) 0;
		border-bottom: var(--border-width) solid var(--border);
		vertical-align: top;
	}
	.partly,
	.failed {
		color: var(--danger);
		font-weight: 600;
	}
	.ran {
		font-weight: 600;
	}
	@media (max-width: 720px) {
		.list li {
			grid-template-columns: auto minmax(0, 1fr);
		}
		.actions {
			grid-column: 2;
		}
		.part {
			grid-template-columns: minmax(0, 1fr);
		}
	}
</style>
