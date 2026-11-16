<script lang="ts">
	// A timeline of tasks with a display: what the filter leaves, sorted and
	// grouped as the display says, each task's subtasks under it, at the
	// zoom the display keeps. Loaded only when a timeline is shown.
	import { keepsMatches, matches, type FilterContext } from '../../filter';
	import { session } from '../../session.svelte';
	import { localToday, type ViewEdit } from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import type { FieldType, Status, Task, ViewDisplay, ViewFilter } from '../../sync/entities';
	import { zoomOf, type Zoom } from '../../timeline';
	import {
		groupTasks,
		resolveDisplay,
		sortTasks,
		type DisplayLookups,
		type DisplayPath,
		type SortContext
	} from '../../viewDisplay';
	import Timeline from './Timeline.svelte';

	interface Props {
		ws: string;
		/** The view's project, or null for one across projects. */
		projectId: string | null;
		filter: ViewFilter;
		display: ViewDisplay;
		onchange: (
			edit: ViewEdit<ViewDisplay, Partial<Record<DisplayPath, unknown>>>,
			say?: (display: ViewDisplay) => string
		) => void | Promise<void>;
		label: string;
		emptyTitle: string;
		emptyHint: string;
		/**
		 * The tasks to start from, for a list of the app's own (Inbox, My Day,
		 * Upcoming); else the view's project's, or every live one's.
		 */
		scope?: Task[];
	}

	let {
		ws,
		projectId,
		filter,
		display,
		onchange,
		label,
		emptyTitle,
		emptyHint,
		scope: given
	}: Props = $props();
	const replica = $derived(engine.replica(ws));
	const me = $derived(session.user?.id ?? '');

	const values = $derived(replica?.fieldValuesByField() ?? new Map());
	const allFields = $derived(replica?.fields() ?? []);
	const look = $derived<DisplayLookups>({
		projectId,
		fields: projectId ? (replica?.fields(projectId) ?? []) : allFields
	});
	const statusById = $derived.by(() => {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- made afresh by each derivation
		const out = new Map<string, Status>();
		for (const p of replica?.projects() ?? [])
			for (const s of replica?.statuses(p.id) ?? []) out.set(s.id, s);
		return out;
	});
	const ctx = $derived.by((): FilterContext => {
		const types = new Map<string, FieldType>(allFields.map((f) => [f.id, f.type]));
		return {
			me,
			today: localToday(),
			category: (t) =>
				(t.statusId && statusById.get(t.statusId)?.category) || (t.completedAt ? 'done' : 'todo'),
			labels: (id) => (replica?.taskLabels(id) ?? []).map((l) => l.id),
			assignees: (id) => replica?.assignees(id) ?? [],
			blocked: (id) => replica?.isBlocked(id) ?? false,
			blocking: (id) => replica?.isBlocking(id) ?? false,
			entries: (id) => replica?.timeEntries(id) ?? [],
			fieldType: (id) => types.get(id),
			value: (task, field) => values.get(field)?.get(task)
		};
	});
	const resolved = $derived(resolveDisplay(display, 'timeline', look));
	const sortCtx = $derived<SortContext>({
		category: ctx.category,
		status: (t) => (t.statusId ? statusById.get(t.statusId) : undefined),
		projects: replica?.projects() ?? [],
		times: (id) => replica?.taskTimes(id) ?? { created: '', updated: '' },
		fields: look.fields,
		values,
		memberName: (id) => session.memberName(ws, id),
		logged: (id) => replica?.logged(id) ?? 0
	});
	// The filter and context are read here, not only when a task is asked about, so a
	// change to either makes a new matcher, and keepsMatches a fresh memory of answers.
	const hit = $derived.by(() => {
		const f = filter;
		const c = ctx;
		return (t: Task) => matches(t, f, c);
	});
	const keeps = $derived(keepsMatches(hit, (id) => replica?.subtasks(id) ?? []));
	const tasks = $derived.by(() => {
		if (!replica) return [];
		// Across projects, tasks in archived or deleted projects stay out.
		const live = new Set(replica.projects().map((p) => p.id));
		const scope =
			given ??
			(projectId
				? replica.tasks(projectId)
				: replica.allTasks().filter((t) => !t.parentId && (!t.projectId || live.has(t.projectId))));
		return scope.filter(keeps);
	});
	const sorted = $derived(sortTasks(tasks, resolved.sort, sortCtx));
	const members = $derived(session.members(ws).map((m) => ({ userId: m.userId, name: m.name })));
	const sections = $derived(
		groupTasks(sorted, resolved.group, {
			me,
			today: localToday(),
			projectId,
			projects: replica?.projects() ?? [],
			statuses: projectId ? (replica?.statuses(projectId) ?? []) : [],
			category: ctx.category,
			labels: replica?.labels() ?? [],
			taskLabels: ctx.labels,
			assignees: ctx.assignees,
			fields: look.fields,
			values,
			members,
			nameOf: (id) => session.nameOf(ws, id)
		})
	);
	/**
	 * A task's subtasks here: those the filter keeps, and those leading to
	 * one, in the view's order. A match always shows, under the task it is part of.
	 */
	const kids = $derived((task: Task) =>
		sortTasks((replica?.subtasks(task.id) ?? []).filter(keeps), resolved.sort, sortCtx)
	);
	const zoom = $derived(zoomOf(display));
	function onzoom(z: Zoom) {
		void onchange(
			(d) => ({
				timeline: { ...((d.timeline as object | undefined) ?? {}), zoom: z }
			}),
			() => `${z[0].toUpperCase()}${z.slice(1)} zoom`
		);
	}

	// Phones show Week and Month, with titles on the bars.
	let phone = $state(typeof matchMedia === 'function' && matchMedia('(max-width: 720px)').matches);
	$effect(() => {
		if (typeof matchMedia !== 'function') return;
		const q = matchMedia('(max-width: 720px)');
		const on = () => (phone = q.matches);
		q.addEventListener('change', on);
		return () => q.removeEventListener('change', on);
	});
</script>

<Timeline
	{ws}
	{sections}
	{kids}
	grouped={resolved.group.by !== 'none'}
	{zoom}
	{onzoom}
	{phone}
	{label}
	{emptyTitle}
	{emptyHint}
/>
