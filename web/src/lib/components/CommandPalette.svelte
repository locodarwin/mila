<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import ArrowLeft from '@lucide/svelte/icons/arrow-left';
	import ArrowRightLeft from '@lucide/svelte/icons/arrow-right-left';
	import CalendarDays from '@lucide/svelte/icons/calendar-days';
	import CalendarX from '@lucide/svelte/icons/calendar-x';
	import ChartColumn from '@lucide/svelte/icons/chart-column';
	import Clock from '@lucide/svelte/icons/clock';
	import Play from '@lucide/svelte/icons/play';
	import Square from '@lucide/svelte/icons/square';
	import CircleCheck from '@lucide/svelte/icons/circle-check';
	import CircleQuestionMark from '@lucide/svelte/icons/circle-question-mark';
	import Flag from '@lucide/svelte/icons/flag';
	import BookOpen from '@lucide/svelte/icons/book-open';
	import FolderKanban from '@lucide/svelte/icons/folder-kanban';
	import Inbox from '@lucide/svelte/icons/inbox';
	import Keyboard from '@lucide/svelte/icons/keyboard';
	import Layers from '@lucide/svelte/icons/layers';
	import LayoutTemplate from '@lucide/svelte/icons/layout-template';
	import ListTodo from '@lucide/svelte/icons/list-todo';
	import Moon from '@lucide/svelte/icons/moon';
	import Palette from '@lucide/svelte/icons/palette';
	import Plus from '@lucide/svelte/icons/plus';
	import Rows3 from '@lucide/svelte/icons/rows-3';
	import Settings from '@lucide/svelte/icons/settings';
	import Sun from '@lucide/svelte/icons/sun';
	import SunMoon from '@lucide/svelte/icons/sun-moon';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import Type from '@lucide/svelte/icons/type';
	import X from '@lucide/svelte/icons/x';
	import type { Component } from 'svelte';
	import { canOrganize, canUseTemplates } from '../roles';
	import { wikiMain } from '../sync/pages';
	import { session } from '../session.svelte';
	import { localToday, moveTask, setInMyDay, setTaskCompleted, setTaskFields } from '../sync/data';
	import { formatDuration } from '../duration';
	import type { Priority, Task } from '../sync/entities';
	import { engine } from '../sync/engine.svelte';
	import { openTask } from '../taskLink';
	import { appearance } from '../theme/appearance.svelte';
	import { presets } from '../theme/presets.svelte';
	import { themes } from '../theme/themes.svelte';
	import { workspaceThemes } from '../theme/workspaceThemes.svelte';
	import { preferences } from '../preferences.svelte';
	import { uuid } from '../uuid';
	import { toasts } from '../toasts.svelte';
	import { ui } from '../ui.svelte';
	import { announce } from '../announce.svelte';
	import Announcer from './Announcer.svelte';
	import { ACTIONS } from '../keys/actions';
	import { command, type CommandId } from '../keys/commands';
	import { keymap } from '../keys/keymap.svelte';
	import { helpHref } from '../help/links';
	import { score } from '../search';
	import { pageHref, pageTitle } from '../wiki';

	interface Item {
		id: string;
		title: string;
		hint?: string;
		/** The registry command it runs, whose key is shown while one works. */
		command?: CommandId;
		icon: Component;
		run: () => unknown;
		/** Keeps the palette open, for a command that asks a second question. */
		stay?: boolean;
	}

	let dialog = $state<HTMLDialogElement>();
	let query = $state('');
	let active = $state(0);
	// The style guide gives it a sample workspace; the app, the one in view.
	let { ws: sample }: { ws?: string } = $props();
	const ws = $derived(sample ?? session.workspace?.id ?? '');
	const me = $derived(session.user?.id ?? '');
	const replica = $derived(engine.replica(ws));

	// The task commands act on: the one focused when the palette opened (a
	// row, card or calendar entry), else the one open in the panel.
	let targetId = $state<string | null>(null);
	const target = $derived(targetId ? replica?.task(targetId) : undefined);
	// "move" asks which project the task goes to.
	let mode = $state<'root' | 'move'>('root');

	function focusedTask(): string | null {
		const el = (document.activeElement as HTMLElement | null)?.closest<HTMLElement>(
			'[data-task-id], [data-card], [data-chip]'
		);
		return el?.dataset.taskId ?? el?.dataset.card ?? el?.dataset.chip ?? null;
	}

	/** An item for a registry command: its name, its key, and what it does. */
	const registry = (id: CommandId, icon: Component): Item => ({
		id,
		title: command(id).name,
		command: id,
		icon,
		run: () => ACTIONS[id]?.()
	});

	const commands: Item[] = [
		registry('quick-add', Plus),
		registry('go.inbox', Inbox),
		registry('go.today', Sun),
		registry('go.upcoming', CalendarDays),
		registry('go.projects', FolderKanban),
		registry('go.wiki', BookOpen),
		registry('go.time', Clock),
		registry('go.reports', ChartColumn),
		registry('go.trash', Trash2),
		registry('go.settings', Settings),
		registry('go.preferences', Settings),
		registry('go.keyboard', Keyboard),
		registry('scheme.dark', Moon),
		registry('scheme.light', Sun),
		registry('scheme.system', SunMoon),
		...(
			[
				['standard', 'Standard'],
				['calm', 'Calm'],
				['playful', 'Playful'],
				['contrast', 'High contrast']
			] as const
		).map(([character, name]) => ({
			id: `theme-${character}`,
			title: `Theme: ${name}`,
			icon: Palette,
			run: () => appearance.set({ character })
		})),
		...(
			[
				['comfortable', 'Comfortable'],
				['dense', 'Dense']
			] as const
		).map(([density, name]) => ({
			id: `density-${density}`,
			title: `Density: ${name}`,
			icon: Rows3,
			run: () => appearance.set({ density })
		})),
		...(
			[
				['s', 'Small'],
				['m', 'Medium'],
				['l', 'Large'],
				['xl', 'Extra large'],
				['xxl', 'Extra extra large']
			] as const
		).map(([fontSize, name]) => ({
			id: `font-${fontSize}`,
			title: `Font size: ${name}`,
			icon: Type,
			run: () => appearance.set({ fontSize })
		})),
		registry('help', Keyboard),
		registry('go.help', CircleQuestionMark)
	];

	// Those made from Omarchy's, the workspace's themes, the person's own,
	// and making a theme of the one in use. A theme Mila cannot show
	// readably is refused (themes.use, presets.use, and the workspace's use).
	const themeCommands = $derived<Item[]>([
		...presets.list.map((p) => ({
			id: `theme:${p.doc.id}`,
			title: `Theme: ${p.doc.name}`,
			hint: 'Omarchy',
			icon: Palette,
			run: async () => {
				const why = await presets.use(p.doc.id);
				if (why) toasts.error(why);
			}
		})),
		...(workspaceThemes.offered?.list ?? []).map((t) => ({
			id: `theme:${t.id}`,
			title: `Theme: ${t.name}`,
			hint: workspaceThemes.offered?.wsName,
			icon: Palette,
			run: async () => {
				const offered = workspaceThemes.offered;
				if (!offered) return;
				const why = (await import('../theme/workspaceActions')).use(offered.ws, t);
				if (why) toasts.error(why);
			}
		})),
		...themes.list.map((t) => ({
			id: `theme:${t.id}`,
			title: `Theme: ${t.name}`,
			hint: workspaceThemes.offered ? 'Yours' : undefined,
			icon: Palette,
			run: async () => {
				const why = await themes.use(t.id);
				if (why) toasts.error(why);
			}
		})),
		{
			id: 'theme-customize',
			title: 'Customize this theme',
			icon: Palette,
			run: () => customize()
		}
	]);

	async function customize() {
		const custom = appearance.custom;
		if (custom && themes.get(custom.id))
			return goto(resolve('/(app)/settings/themes/[id]', { id: custom.id }));
		const { blank, uniqueName, LIMITS } = await import('../theme/document');
		if (themes.list.length >= LIMITS.themes)
			return toasts.error(`You have ${LIMITS.themes} themes, the most Mila keeps.`);
		const names: Record<string, string> = {
			standard: 'Standard',
			calm: 'Calm',
			playful: 'Playful',
			contrast: 'High contrast'
		};
		const base = appearance.current.character;
		const taken = themes.list.map((t) => t.name);
		// A workspace's theme is customized as a copy of one's own, which
		// comes into use.
		const doc = custom
			? { ...$state.snapshot(custom), id: uuid(), name: uniqueName(custom.name, taken) }
			: blank(base, uniqueName(`My ${names[base]}`, taken));
		themes.put(doc);
		if (custom) preferences.set({ customTheme: doc.id, theme: doc.base });
		return goto(resolve('/(app)/settings/themes/[id]', { id: doc.id }));
	}

	// Help's index and search load on the first query, so they cost no
	// page that never searches.
	let help = $state<typeof import('../help/search')>();
	$effect(() => {
		if (query.trim() && !help) void import('../help/search').then((m) => (help = m));
	});

	const PRIORITIES: [Priority, string][] = [
		['urgent', 'Urgent'],
		['high', 'High'],
		['medium', 'Medium'],
		['low', 'Low'],
		['none', 'None']
	];

	function dayAfter(day: string): string {
		const [y, m, n] = day.split('-').map(Number);
		const d = new Date(y, m - 1, n + 1);
		return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
	}

	/** Runs an edit to the task and says what it did, with undo. */
	function edit(done: string, change: () => Promise<unknown>) {
		return () => {
			const { done: edited, undo } = engine.track(change);
			return edited
				.then(() => toasts.show(done, { onundo: () => void undo() }))
				.catch((err) => toasts.error(err instanceof Error ? err.message : 'That did not work.'));
		};
	}

	// The person's running timer, which can be stopped from anywhere.
	const timer = $derived(me ? replica?.runningTimer(me) : undefined);
	function stopIt(title: string) {
		const running = timer;
		if (!running) return;
		return import('../sync/time')
			.then((m) => m.stopTimer(ws, running))
			.then((minutes) =>
				toasts.show(
					minutes
						? `Logged ${formatDuration(minutes)} on ${title}`
						: 'Stopped before a minute, so nothing was logged'
				)
			);
	}
	const timerCommands = $derived.by((): Item[] => {
		if (!timer || !ws || (timer.taskId !== null && timer.taskId === target?.id)) return [];
		// A timer on a project itself is named for the project.
		const title = timer.taskId
			? (replica?.task(timer.taskId)?.title ?? 'a task')
			: (replica?.project(timer.projectId ?? '')?.name ?? 'a project');
		return [
			{
				id: 'timer-stop',
				title: `Stop the timer on ${title}`,
				icon: Square,
				run: () => stopIt(title)
			}
		];
	});

	// On a project's page: time logged to the project itself, typed in or timed.
	const projectTimeCommands = $derived.by((): Item[] => {
		const id = page.route.id?.startsWith('/(app)/projects/[id]') ? page.params.id : null;
		const p = id ? replica?.project(id) : undefined;
		if (!p || !ws || p.archivedAt) return [];
		const time = `${resolve('/(app)/projects/[id]', { id: p.id })}?tab=time`;
		const items: Item[] = [
			{
				id: 'project-log-time',
				title: `Log time to ${p.name}`,
				hint: 'Project',
				icon: Clock,
				// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved, with a query
				run: () => goto(`${time}#log`)
			}
		];
		if (timer?.projectId !== p.id)
			items.push({
				id: 'project-timer',
				title: `Start a timer for ${p.name}`,
				hint: 'Project',
				icon: Play,
				run: () =>
					import('../sync/time')
						.then((m) => m.startProjectTimer(ws, p.id))
						.then(() => toasts.show(`Timing ${p.name}`))
			});
		return items;
	});

	// What can be done to the target task, first in the list when there is one.
	const taskCommands = $derived.by((): Item[] => {
		const t = target;
		if (!t || !ws) return [];
		const hint = t.title;
		const today = localToday();
		const inMyDay = me ? replica?.myDay(me, today).some((e) => e.task.id === t.id) : false;
		const timing = timer?.taskId === t.id;
		const items: Item[] = [
			{
				id: 'task-timer',
				title: timing ? 'Stop the timer' : 'Start a timer on the task',
				hint,
				icon: timing ? Square : Play,
				run: () =>
					timing
						? stopIt(t.title)
						: import('../sync/time')
								.then((m) => m.startTimer(ws, t.id))
								.then(() => toasts.show('Timer started'))
			},
			{
				id: 'task-log-time',
				title: 'Log time on the task',
				hint,
				icon: Clock,
				run: async () => {
					await openTask(t.id);
					ui.logTime = t.id;
				}
			},
			{
				id: 'task-complete',
				title: t.completedAt ? 'Reopen the task' : 'Complete the task',
				hint,
				icon: CircleCheck,
				run: edit(t.completedAt ? 'Reopened' : 'Completed', () =>
					setTaskCompleted(ws, t, !t.completedAt)
				)
			},
			{
				id: 'task-move',
				title: 'Move the task to a project',
				hint,
				icon: ArrowRightLeft,
				stay: true,
				run: () => {
					mode = 'move';
					query = '';
				}
			},
			{
				id: 'task-today',
				title: 'Due today',
				hint,
				icon: CalendarDays,
				run: edit('Due today', () => setTaskFields(ws, t.id, { due_date: today }))
			},
			{
				id: 'task-tomorrow',
				title: 'Due tomorrow',
				hint,
				icon: CalendarDays,
				run: edit('Due tomorrow', () => setTaskFields(ws, t.id, { due_date: dayAfter(today) }))
			},
			...(t.dueDate
				? [
						{
							id: 'task-no-date',
							title: 'Clear the due date',
							hint,
							icon: CalendarX,
							run: edit('Due date cleared', () =>
								setTaskFields(ws, t.id, { due_date: null, due_time: null })
							)
						}
					]
				: []),
			...(me
				? [
						{
							id: 'task-my-day',
							title: inMyDay ? 'Take the task out of My Day' : 'Add the task to My Day',
							hint,
							icon: Sun,
							run: edit(inMyDay ? 'Taken out of My Day' : 'Added to My Day', () =>
								setInMyDay(ws, me, t.id, !inMyDay)
							)
						}
					]
				: []),
			...PRIORITIES.filter(([p]) => p !== t.priority).map(([p, name]) => ({
				id: `task-priority-${p}`,
				title: `Priority: ${name}`,
				hint,
				icon: Flag,
				run: edit(`Priority: ${name}`, () => setTaskFields(ws, t.id, { priority: p }))
			}))
		];
		return items;
	});

	/** The places the target task can move to. */
	function destinations(task: Task, q: string): (Item & { s: number })[] {
		const places = [
			...(!session.workspace || canOrganize(session.workspace.role)
				? [{ id: null as string | null, name: 'Inbox', icon: Inbox }]
				: []),
			...(replica?.projects() ?? []).map((p) => ({ id: p.id, name: p.name, icon: FolderKanban }))
		];
		const out: (Item & { s: number })[] = [];
		for (const p of places) {
			if (p.id === task.projectId) continue;
			const s = score(p.name, q);
			if (s)
				out.push({
					id: 'to:' + (p.id ?? 'inbox'),
					title: p.name,
					hint: p.id ? 'Project' : undefined,
					icon: p.icon,
					s,
					run: edit(`Moved to ${p.name}`, () => moveTask(ws, task, p.id))
				});
		}
		return out.sort((a, b) => b.s - a.s);
	}

	const results = $derived.by(() => {
		const q = query.trim().toLowerCase();
		if (mode === 'move' && target) return destinations(target, q).slice(0, 30);
		const out: (Item & { s: number })[] = [];
		for (const c of taskCommands) {
			const s = score(c.title, q);
			if (s) out.push({ ...c, s: s + 6 });
		}
		for (const c of [...timerCommands, ...projectTimeCommands, ...commands, ...themeCommands]) {
			if (c.id === 'go.inbox' && session.workspace?.role === 'guest') continue;
			const s = score(c.title, q);
			if (s) out.push({ ...c, s: s + 5 });
		}
		// Up to five help topics and sections, after any command that
		// matches (Upcoming goes there before it explains it) and, on a tie,
		// ahead of projects, views and tasks. Commands are here already.
		const below = Math.min(100, ...out.map((c) => c.s - 1));
		for (const h of (help?.searchHelp(q) ?? []).filter((h) => !h.command).slice(0, 5)) {
			const href = helpHref(h);
			out.push({
				id: 'h:' + href,
				title: h.title,
				hint: h.context === 'Help' ? 'Help' : `Help, ${h.context}`,
				icon: CircleQuestionMark,
				s: Math.min(h.s, below),
				// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved in helpHref, with a fragment
				run: () => goto(href)
			});
		}
		for (const p of replica?.projects() ?? []) {
			const s = score(p.name, q);
			if (s)
				out.push({
					id: 'p:' + p.id,
					title: p.name,
					hint: 'Project',
					icon: FolderKanban,
					s,
					run: () => goto(resolve('/(app)/projects/[id]', { id: p.id }))
				});
		}
		for (const v of replica?.views(session.user?.id ?? '') ?? []) {
			const s = score(v.name, q);
			if (s)
				out.push({
					id: 'v:' + v.id,
					title: v.name,
					hint: 'View',
					icon: Layers,
					s,
					run: () => goto(resolve('/(app)/views/[id]', { id: v.id }))
				});
		}
		// Templates, which guests never see: one entry each, and their page.
		if (session.workspace && canUseTemplates(session.workspace.role)) {
			const here = page.route.id?.startsWith('/(app)/projects/[id]')
				? (page.params.id ?? null)
				: null;
			for (const t of replica?.templates() ?? []) {
				const title = `New ${t.kind} from a template: ${t.name}`;
				const s = score(title, q);
				if (s)
					out.push({
						id: 'tpl:' + t.id,
						title,
						hint: 'Template',
						icon: LayoutTemplate,
						s,
						run: () =>
							t.kind === 'page'
								? // In the wiki of the project on screen, or the Workspace Wiki.
									import('../templateActions')
										.then((m) =>
											m.newPageForTemplate(ws, t.id, {
												projectId: here,
												parentId: page.url.searchParams.get('page') ?? wikiMain(ws, here)
											})
										)
										// eslint-disable-next-line svelte/no-navigation-without-resolve -- a page's address, made by pageHref
										.then((address) => goto(address))
								: t.kind === 'project'
									? // eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved, with a query
										goto(`${resolve('/(app)/projects')}?template=${t.id}`)
									: import('../templateActions').then((m) => m.newTaskFromTemplate(ws, t.id, here))
					});
			}
			const s = score('Manage templates', q);
			if (s)
				out.push({
					id: 'tpl:manage',
					title: 'Manage templates',
					hint: 'Settings',
					icon: LayoutTemplate,
					s,

					run: () => goto(resolve('/(app)/settings/w/[ws]/templates', { ws }))
				});
		}
		if (q && ws) {
			// Wiki pages by title, and by what they say.
			const isGuest = session.workspace?.role === 'guest';
			for (const p of replica?.allPages() ?? []) {
				if (isGuest && !p.projectId) continue;
				const project = p.projectId ? replica?.project(p.projectId) : null;
				if (p.projectId && (!project || project.archivedAt)) continue; // an archived or deleted project's
				const title = pageTitle(ws, p, project);
				const s = Math.max(score(title, q), score(p.body, q) / 2);
				if (s)
					out.push({
						id: 'w:' + p.id,
						title,
						hint: project ? `${project.name} wiki` : 'Workspace Wiki',
						icon: BookOpen,
						s,
						// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved in pageHref, with a query
						run: () => goto(pageHref(p))
					});
			}
		}
		if (q) {
			for (const t of replica?.activeTasks() ?? []) {
				const s = Math.max(score(t.title, q), score(t.notes, q) / 2);
				if (s)
					out.push({
						id: 't:' + t.id,
						title: t.title,
						hint: t.completedAt ? 'Done' : (replica?.project(t.projectId ?? '')?.name ?? 'Inbox'),
						icon: ListTodo,
						s: s - (t.completedAt ? 5 : 0),
						run: () => openTask(t.id)
					});
			}
		}
		// A stable sort keeps commands in their order when scores tie.
		return out.sort((a, b) => b.s - a.s).slice(0, 30);
	});

	$effect(() => {
		void query;
		void mode;
		active = 0;
	});

	// The count of results is said as the query changes; the list itself
	// is not read while typing. Only a new query or count is said: the
	// results are worked out again whenever the workspace syncs, and
	// saying it again each time would hold the words back for good in a
	// busy workspace.
	let counted = '';
	$effect(() => {
		const n = results.length;
		if (ui.overlay !== 'palette' || !query) {
			counted = '';
			return;
		}
		if (`${query}\n${n}` === counted) return;
		counted = `${query}\n${n}`;
		announce(n ? `${n} ${n === 1 ? 'result' : 'results'}` : 'Nothing matches', 500);
	});

	// The chosen option stays in view as the arrows move past the edge.
	$effect(() => {
		const id = results[active]?.id;
		if (!id || !dialog?.open) return;
		dialog.querySelector(`#palette-${CSS.escape(id)}`)?.scrollIntoView({ block: 'nearest' });
	});

	$effect(() => {
		if (ui.overlay === 'palette' && dialog && !dialog.open) {
			query = '';
			mode = 'root';
			targetId = focusedTask() ?? page.url.searchParams.get('task');
			// The person's themes and Omarchy's, for their commands, if not fetched yet this visit.
			void themes.ensure();
			void presets.ensure().catch(() => {});
			dialog.showModal();
			// autofocus is not enough when a key opened it: the key's target keeps focus.
			dialog.querySelector('input')?.focus();
		} else if (ui.overlay !== 'palette' && dialog?.open) {
			dialog.close();
		}
	});

	function run(item: Item | undefined) {
		if (!item) return;
		if (!item.stay) ui.close();
		void item.run();
	}

	// Back to every command, from a list of choices such as Move.
	function back() {
		mode = 'root';
		query = '';
		dialog?.querySelector('input')?.focus();
	}

	function keys(e: KeyboardEvent) {
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			active = Math.min(active + 1, results.length - 1);
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			active = Math.max(active - 1, 0);
		} else if (e.key === 'Enter') {
			// An input method's Enter confirms what is being composed.
			if (e.isComposing) return;
			e.preventDefault();
			run(results[active]);
		} else if (mode !== 'root' && (e.key === 'Escape' || (e.key === 'Backspace' && !query))) {
			// Back to every command rather than closed.
			e.preventDefault();
			mode = 'root';
		}
	}
</script>

<!-- Escape frees the overlay at once (cancel). The close event comes later,
     at the next frame, and by then a key may have opened the dialog again:
     a close event on an open dialog is a stale one and changes nothing. -->
<dialog
	bind:this={dialog}
	class="palette"
	aria-label="Command palette"
	onclose={(e) => !e.currentTarget.open && ui.overlay === 'palette' && ui.close()}
	oncancel={() => ui.overlay === 'palette' && ui.close()}
>
	<!-- A phone has no Escape key, and the palette fills its screen: the
	     buttons are the way back and out. -->
	<div class="bar">
		{#if mode !== 'root'}
			<button class="icon" type="button" aria-label="Back to every command" onclick={back}
				><ArrowLeft size={18} /></button
			>
		{/if}
		<!-- svelte-ignore a11y_autofocus -->
		<input
			aria-label={mode === 'move'
				? `Move ${target?.title ?? 'the task'} to`
				: 'Search for a task, project or command'}
			placeholder={mode === 'move'
				? `Move “${target?.title ?? 'the task'}” to…`
				: 'Search tasks, projects and commands'}
			bind:value={query}
			onkeydown={keys}
			autofocus
			autocomplete="off"
			role="combobox"
			aria-expanded="true"
			aria-controls="palette-results"
			aria-activedescendant={results[active] ? `palette-${results[active].id}` : undefined}
		/>
		<button class="icon close" type="button" aria-label="Close search" onclick={() => ui.close()}
			><X size={18} /></button
		>
	</div>
	<ul id="palette-results" role="listbox" aria-label="Results">
		{#each results as item, i (item.id)}
			<li
				id={`palette-${item.id}`}
				role="option"
				aria-selected={i === active}
				onclick={() => run(item)}
				onmousemove={() => (active = i)}
				onkeydown={() => {}}
			>
				<item.icon size={16} />
				<span class="title">{item.title}</span>
				{#if item.hint}<span class="hint">{item.hint}</span>{/if}
				{#if item.command && keymap.label(item.command)}<span class="hint" aria-hidden="true"
						>{keymap.label(item.command)}</span
					>{/if}
			</li>
		{:else}
			<li class="empty" role="presentation">Nothing matches.</li>
		{/each}
	</ul>
	<!-- The page's live regions are inert behind a modal dialog. -->
	{#if ui.overlay === 'palette'}<Announcer inDialog />{/if}
</dialog>

<style>
	.palette {
		width: min(600px, calc(100vw - 32px));
		margin: 12vh auto auto;
		padding: 0;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
		overflow: hidden;
	}
	.palette::backdrop {
		/* With the value as written, for browsers whose ::backdrop does not inherit custom properties. */
		background: var(--scrim, oklch(0 0 0 / 0.25));
	}
	.bar {
		display: flex;
		align-items: center;
		border-bottom: var(--border-width) solid var(--border);
	}
	.bar .icon:first-child {
		margin-left: var(--space-2);
	}
	input {
		flex: 1;
		min-width: 0;
		width: 100%;
		height: 52px;
		padding: 0 var(--space-4);
		border: none;
		background: none;
		font-size: var(--text-lg);
	}
	.icon {
		display: grid;
		flex: none;
		place-items: center;
		width: max(var(--tap-min), 32px);
		height: max(var(--tap-min), 32px);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.icon:hover {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	/* Escape closes it on a desktop. */
	.close {
		display: none;
	}
	input:focus-visible {
		box-shadow: none;
	}
	ul {
		max-height: min(420px, 60vh);
		margin: 0;
		padding: var(--space-2);
		overflow-y: auto;
		list-style: none;
	}
	li {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		padding: var(--space-2) var(--space-3);
		border-radius: var(--radius-md);
		color: var(--text-secondary);
		cursor: pointer;
	}
	/* A bar as well as the tint marks the chosen option. */
	li[aria-selected='true'] {
		box-shadow: inset 3px 0 0 var(--accent);
		background: var(--accent-soft);
		color: var(--text-primary);
	}
	.title {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		color: var(--text-primary);
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.hint {
		font-size: var(--text-xs);
	}
	.empty {
		cursor: default;
	}
	/* A phone: the whole screen, as the Search tab, with the results
	   running down to the keyboard. */
	@media (max-width: 720px) {
		.palette[open] {
			display: flex;
			flex-direction: column;
			width: 100%;
			max-width: 100%;
			height: 100dvh;
			max-height: 100dvh;
			margin: 0;
			padding: env(safe-area-inset-top) 0 env(safe-area-inset-bottom);
			border: none;
			border-radius: 0;
		}
		ul {
			flex: 1;
			max-height: none;
		}
		.close {
			display: grid;
			margin-right: var(--space-2);
		}
		li {
			min-height: var(--touch-target);
		}
	}
</style>
