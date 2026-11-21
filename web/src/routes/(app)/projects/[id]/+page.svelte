<script lang="ts">
	import { menu, projectActions } from '#lib/contextMenu.svelte.js';
	import { tick, untrack } from 'svelte';
	import { intents } from '#lib/intents.svelte.js';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import CalendarDays from '@lucide/svelte/icons/calendar-days';
	import CalendarPlus from '@lucide/svelte/icons/calendar-plus';
	import Kanban from '@lucide/svelte/icons/kanban';
	import List from '@lucide/svelte/icons/list';
	import Table2 from '@lucide/svelte/icons/table-2';
	import ChartGantt from '@lucide/svelte/icons/chart-gantt';
	import LayoutTemplate from '@lucide/svelte/icons/layout-template';
	import Archive from '@lucide/svelte/icons/archive';
	import ArchiveRestore from '@lucide/svelte/icons/archive-restore';
	import BookmarkPlus from '@lucide/svelte/icons/bookmark-plus';
	import BookOpen from '@lucide/svelte/icons/book-open';
	import Clock from '@lucide/svelte/icons/clock';
	import ListTodo from '@lucide/svelte/icons/list-todo';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import UserPlus from '@lucide/svelte/icons/user-plus';
	import { api, call, messageOf, type Schemas } from '#lib/api/client.js';
	import ShareProject from '#lib/components/ShareProject.svelte';
	import ProjectIcon from '#lib/components/ProjectIcon.svelte';
	import { canEditViews, canOrganize, canShare, canUseTemplates } from '#lib/roles.js';
	import EmptyState from '#lib/components/EmptyState.svelte';
	import ProjectStylePicker from '#lib/components/ProjectStylePicker.svelte';
	import StatusEditor from '#lib/components/StatusEditor.svelte';
	import TaskList from '#lib/components/TaskList.svelte';
	import { session } from '#lib/session.svelte.js';
	import {
		createView,
		renameProject,
		setProjectArchived,
		setProjectDeleted,
		updateProject
	} from '#lib/sync/data.js';
	import type { ViewDisplay, ViewType } from '#lib/sync/entities.js';
	import { engine } from '#lib/sync/engine.svelte.js';
	import { toasts } from '#lib/toasts.svelte.js';

	const ws = $derived(session.workspace?.id ?? '');
	const project = $derived(engine.replica(ws)?.project(page.params.id ?? ''));
	// The view lives in the URL, so a link or a reload keeps it.
	const view = $derived(page.url.searchParams.get('view') ?? 'list');

	// Tasks or Wiki: the address's, else the one this project was last on
	// on this device.
	const TAB = (id: string) => `mila.projectTab.${id}`;
	function storedTab(id: string): string | null {
		try {
			return localStorage.getItem(TAB(id));
		} catch {
			return null;
		}
	}
	// The project's tabs, in order: its tasks, its wiki, and the time on it.
	type Tab = 'tasks' | 'wiki' | 'time';
	const TABS: Tab[] = ['tasks', 'wiki', 'time'];
	const tab = $derived.by((): Tab => {
		const asked = page.url.searchParams.get('tab') ?? storedTab(page.params.id ?? '');
		return asked === 'wiki' || asked === 'time' ? asked : 'tasks';
	});
	function showTab(id: Tab) {
		if (!project) return;
		try {
			localStorage.setItem(TAB(project.id), id);
		} catch {
			// Kept for this visit only.
		}
		const u = new URL(page.url.href);
		u.searchParams.set('tab', id);
		if (id === 'tasks') u.searchParams.delete('page');
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- same page, new query
		void goto(u, { reset: false, replace: true });
	}
	function openPage(id: string) {
		const u = new URL(page.url.href);
		u.searchParams.set('tab', 'wiki');
		u.searchParams.set('page', id);
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- same page, new query
		return goto(u, { reset: false });
	}
	// The tabs: Left and Right move between them, as a tab list's do.
	function tabKeys(e: KeyboardEvent) {
		if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight' && e.key !== 'Home' && e.key !== 'End')
			return;
		e.preventDefault();
		const at = TABS.indexOf(tab);
		const step = e.key === 'ArrowRight' ? 1 : -1;
		const to =
			e.key === 'Home'
				? TABS[0]
				: e.key === 'End'
					? TABS[TABS.length - 1]
					: TABS[(at + step + TABS.length) % TABS.length];
		showTab(to);
		requestAnimationFrame(() => document.getElementById(`tab-${to}`)?.focus());
	}
	const views = [
		{ id: 'list', label: 'List', icon: List },
		{ id: 'board', label: 'Board', icon: Kanban },
		{ id: 'calendar', label: 'Calendar', icon: CalendarDays },
		{ id: 'table', label: 'Table', icon: Table2 },
		{ id: 'timeline', label: 'Timeline', icon: ChartGantt }
	];
	const projectTasks = $derived(
		(engine.replica(ws)?.allTasks() ?? []).filter((t) => t.projectId === project?.id)
	);

	function show(id: string) {
		const u = new URL(page.url.href);
		if (id === 'list') u.searchParams.delete('view');
		else u.searchParams.set('view', id);
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- same page, new query
		void goto(u, { reset: false, replace: true });
	}

	// Save as a template: its dialog loads the first time it opens.
	let templating = $state(false);
	let templateOpened = $state(false);

	// A guest works in the project, and leaves the project itself alone.
	const organize = $derived(!session.workspace || canOrganize(session.workspace.role));

	// Follows the project's name, and holds edits until they are saved.
	let name = $derived(project?.name ?? '');

	function save() {
		const n = name.trim();
		if (project && n && n !== project.name) void renameProject(ws, project, n);
		else name = project?.name ?? '';
	}

	/**
	 * Saves this project's current view to the sidebar, to filter it there;
	 * a table or timeline takes the layout kept on this device with it.
	 */
	async function saveView() {
		if (!project || !session.user) return;
		let display: ViewDisplay | undefined;
		if (view === 'table' || view === 'timeline')
			try {
				const key = view === 'table' ? 'mila.projectTable' : 'mila.projectTimeline';
				display = JSON.parse(localStorage.getItem(`${key}.${project.id}`) ?? '{}');
			} catch {
				display = undefined;
			}
		const id = await createView(ws, {
			name: `${project.name} ${view}`,
			type: view as ViewType,
			projectId: project.id,
			ownerId: session.user.id,
			display
		});
		await goto(resolve('/(app)/views/[id]', { id }));
	}

	// The description, held while it is edited like the name.
	let description = $derived(project?.description ?? '');

	function saveDescription() {
		if (project && description.trim() !== project.description)
			void updateProject(ws, project.id, { description });
	}

	// What a right-click menu elsewhere asked of this project (menus.ts).
	let nameField = $state<HTMLInputElement>();
	$effect(() => {
		void intents.pending;
		const id = project?.id;
		const what = untrack(() => intents.take('project', id));
		if (!what) return;
		void tick().then(() => {
			if (what === 'rename') {
				nameField?.focus();
				nameField?.select();
			} else if (what === 'share') openShare();
			else if (what === 'template') templating = templateOpened = true;
			else if (what === 'style')
				document.querySelector<HTMLButtonElement>('[data-project-style]')?.click();
		});
	});

	async function archive(on: boolean) {
		if (!project) return;
		const { id, name: gone } = project;
		await setProjectArchived(ws, id, on);
		if (on) {
			toasts.show(`Archived ${gone}`, { onundo: () => void setProjectArchived(ws, id, false) });
			await goto(resolve('/(app)/projects'));
		} else {
			toasts.show(`${gone} is back in use`);
		}
	}

	// Sharing with guests: only in a shared workspace, by its admins.
	const sharable = $derived(
		!session.local &&
			!!session.workspace &&
			!session.workspace.isPersonal &&
			canShare(session.workspace.role)
	);
	let sharing = $state(false);
	let guests = $state<Schemas['ProjectGuest'][] | null>(null);
	let pendingGuest = $state<string | null>(null);

	async function loadGuests() {
		if (!project) return;
		guests = await call(
			api.GET('/workspaces/{workspaceId}/projects/{projectId}/guests', {
				params: { path: { workspaceId: ws, projectId: project.id } }
			})
		);
	}

	function openShare() {
		guests = null;
		sharing = true;
		loadGuests().catch((err) => toasts.error(messageOf(err)));
	}

	async function share(guest: Schemas['ProjectGuest'], on: boolean) {
		if (!project) return;
		const path = { workspaceId: ws, projectId: project.id, userId: guest.userId };
		pendingGuest = guest.userId;
		try {
			await call(
				on
					? api.PUT('/workspaces/{workspaceId}/projects/{projectId}/guests/{userId}', {
							params: { path }
						})
					: api.DELETE('/workspaces/{workspaceId}/projects/{projectId}/guests/{userId}', {
							params: { path }
						})
			);
			toasts.show(
				on
					? `Shared ${project.name} with ${guest.name}`
					: `${project.name} is no longer shared with ${guest.name}`
			);
		} catch (err) {
			toasts.error(messageOf(err));
		} finally {
			pendingGuest = null;
		}
		await loadGuests().catch((err) => toasts.error(messageOf(err)));
	}

	async function remove() {
		if (!project) return;
		const { id, name: gone } = project;
		await setProjectDeleted(ws, id, true);
		toasts.show(`Deleted ${gone}`, { onundo: () => void setProjectDeleted(ws, id, false) });
		await goto(resolve('/(app)'));
	}
</script>

<svelte:head><title>{project?.name ?? 'Project'} · Mila</title></svelte:head>

{#if project}
	<div class="project">
		<header class="page" use:menu={projectActions(ws, project.id)}>
			<!-- The name is a field; the page still needs a heading named after it. -->
			<h1 class="visually-hidden">{project.name}</h1>
			{#if organize}
				<ProjectStylePicker
					name={project.name}
					icon={project.icon}
					color={project.color}
					onchange={(fields) => project && void updateProject(ws, project.id, fields)}
				/>
			{:else}
				<ProjectIcon icon={project.icon} color={project.color} size={16} />
			{/if}
			<input
				class="name"
				aria-label="Project name"
				bind:this={nameField}
				readonly={!organize}
				bind:value={name}
				onblur={save}
				onkeydown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
			/>
			<!-- One row with the name when there is room; a row of its own,
			     scrolled sideways, when there is not. -->
			<div class="tools">
				{#if tab === 'tasks'}
					<!-- Toggle buttons, not tabs: each shows the same tasks another way,
				     and an ordinary Tab stop each is simpler than a tab pattern. -->
					<div class="views" role="group" aria-label="View">
						{#each views as v (v.id)}
							<button
								type="button"
								aria-label={v.label}
								aria-pressed={view === v.id}
								onclick={() => show(v.id)}
							>
								<v.icon size={15} /><span>{v.label}</span>
							</button>
						{/each}
					</div>
					{#if organize}<StatusEditor {ws} projectId={project.id} />{/if}
					{#if !session.workspace || canEditViews(session.workspace.role)}
						<button class="icon save" type="button" aria-label="Save as a view" onclick={saveView}>
							<BookmarkPlus size={16} />
						</button>
					{/if}
					{#if session.workspace && canUseTemplates(session.workspace.role)}
						<button
							class="icon save"
							type="button"
							aria-label="Save as a template"
							title="Save as a template"
							onclick={() => (templating = templateOpened = true)}
						>
							<LayoutTemplate size={16} />
						</button>
					{/if}
					<!-- eslint-disable svelte/no-navigation-without-resolve -- resolved, with a query -->
					<a
						class="icon save"
						href={`${resolve('/(app)/settings/w/[ws]/calendar', { ws })}?project=${project.id}`}
						aria-label="Subscribe in a calendar app"
						title="Subscribe in a calendar app"
					>
						<CalendarPlus size={16} />
					</a>
					<!-- eslint-enable svelte/no-navigation-without-resolve -->
				{/if}
				{#if sharable}
					<button
						class="icon save"
						type="button"
						aria-label={`Share ${project.name} with guests`}
						title="Share with guests"
						onclick={openShare}
					>
						<UserPlus size={16} />
					</button>
				{/if}
				{#if organize}
					{#if project.archivedAt}
						<button
							class="icon save"
							type="button"
							aria-label={`Unarchive ${project.name}`}
							title="Unarchive"
							onclick={() => archive(false)}
						>
							<ArchiveRestore size={16} />
						</button>
					{:else}
						<button
							class="icon save"
							type="button"
							aria-label={`Archive ${project.name}`}
							title="Archive"
							onclick={() => archive(true)}
						>
							<Archive size={16} />
						</button>
					{/if}
					<button class="icon" type="button" aria-label={`Delete ${project.name}`} onclick={remove}>
						<Trash2 size={16} />
					</button>
				{/if}
			</div>
		</header>
		{#if project.archivedAt}
			<p class="archived">
				This project is archived: it is out of the sidebar and lists until it is back in use.
			</p>
		{/if}
		{#if sharable}
			<ShareProject
				bind:open={sharing}
				projectName={project.name}
				{guests}
				pending={pendingGuest}
				onchange={share}
			/>
		{/if}
		{#if templating || templateOpened}
			{#await import('#lib/components/templates/SaveTemplate.svelte') then { default: SaveTemplate }}
				<SaveTemplate
					bind:open={templating}
					{ws}
					kind="project"
					sourceId={project.id}
					sourceName={project.name}
				/>
			{/await}
		{/if}
		<div class="tabs" role="tablist" aria-label={project.name}>
			<button
				id="tab-tasks"
				type="button"
				role="tab"
				aria-selected={tab === 'tasks'}
				aria-controls="tabpanel"
				tabindex={tab === 'tasks' ? 0 : -1}
				onclick={() => showTab('tasks')}
				onkeydown={tabKeys}><ListTodo size={15} />Tasks</button
			>
			<button
				id="tab-wiki"
				type="button"
				role="tab"
				aria-selected={tab === 'wiki'}
				aria-controls="tabpanel"
				tabindex={tab === 'wiki' ? 0 : -1}
				onclick={() => showTab('wiki')}
				onkeydown={tabKeys}><BookOpen size={15} />Project Wiki</button
			>
			<button
				id="tab-time"
				type="button"
				role="tab"
				aria-selected={tab === 'time'}
				aria-controls="tabpanel"
				tabindex={tab === 'time' ? 0 : -1}
				onclick={() => showTab('time')}
				onkeydown={tabKeys}><Clock size={15} />Time</button
			>
		</div>
		{#if tab === 'time'}
			<div class="view" id="tabpanel" role="tabpanel" aria-labelledby="tab-time">
				<!-- The time loads only when its tab is shown. -->
				{#await import('#lib/components/time/ProjectTime.svelte') then { default: ProjectTime }}
					<ProjectTime {ws} projectId={project.id} logNow={page.url.hash === '#log'} />
				{/await}
			</div>
		{:else if tab === 'wiki'}
			<div class="view" id="tabpanel" role="tabpanel" aria-labelledby="tab-wiki">
				<!-- The wiki loads only when its tab is shown. -->
				{#await import('#lib/components/wiki/Wiki.svelte') then { default: Wiki }}
					<Wiki
						{ws}
						projectId={project.id}
						pageId={page.url.searchParams.get('page')}
						heading="h2"
						onopen={openPage}
					/>
				{/await}
			</div>
		{:else}
			<textarea
				class="description"
				aria-label="Project description"
				readonly={!organize}
				placeholder="Add a description"
				rows="1"
				bind:value={description}
				onblur={saveDescription}></textarea>
			<div class="view" id="tabpanel" role="tabpanel" aria-labelledby="tab-tasks">
				<!-- The board and the calendar, like the table and timeline, load only when shown. -->
				{#if view === 'board'}
					{#await import('#lib/components/views/Board.svelte') then { default: Board }}
						<Board {ws} projectId={project.id} />
					{/await}
				{:else if view === 'calendar'}
					{#await import('#lib/components/views/Calendar.svelte') then { default: Calendar }}
						<Calendar {ws} tasks={projectTasks} />
					{/await}
				{:else if view === 'timeline'}
					<!-- The timeline loads only when it is shown. -->
					{#await import('#lib/components/views/ProjectTimeline.svelte') then { default: ProjectTimeline }}
						<ProjectTimeline {ws} projectId={project.id} />
					{/await}
				{:else if view === 'table'}
					<!-- The table, its sorting and summaries load only when one is shown. -->
					{#await import('#lib/components/views/ProjectTable.svelte') then { default: ProjectTable }}
						<ProjectTable {ws} projectId={project.id} />
					{/await}
				{:else}
					<TaskList
						{ws}
						projectId={project.id}
						emptyTitle="No tasks yet"
						emptyHint="Add the first one above."
					/>
				{/if}
			</div>
		{/if}
	</div>
{:else}
	<header class="page"><h1>Project</h1></header>
	<EmptyState
		title="This project isn't here"
		hint="It may have been deleted, or belongs to another workspace."
	/>
{/if}

<style>
	.views {
		position: relative;
		display: inline-flex;
		padding: 2px;
		border-radius: var(--radius-md);
		background: var(--surface-2);
	}
	.views button {
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
	.views button[aria-pressed='true'] {
		background: var(--surface-0);
		color: var(--text-primary);
		box-shadow: var(--shadow-sm);
		font-weight: 550;
	}
	/* One screen tall, less what the layout puts over it, so a board or
	   calendar scrolls inside the view and the page itself never does. */
	.project {
		display: flex;
		flex-direction: column;
		height: calc(100% - var(--above, 0px));
	}
	.project > :global(.page) {
		flex: none;
	}
	.view {
		flex: 1;
		min-height: 0;
	}
	.tabs {
		flex: none;
		display: flex;
		gap: var(--space-1);
		padding: 0 var(--space-4);
		border-bottom: var(--border-width) solid var(--border);
	}
	.tabs button {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		min-height: max(var(--tap-min), 36px);
		margin-bottom: calc(-1 * var(--border-width));
		padding: 0 var(--space-2);
		border: none;
		border-bottom: 2px solid transparent;
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		font-weight: 550;
		cursor: pointer;
	}
	.tabs button:hover {
		color: var(--text-primary);
	}
	.tabs button[aria-selected='true'] {
		border-bottom-color: var(--accent);
		color: var(--text-primary);
	}
	.tabs button:focus-visible {
		outline: none;
		box-shadow: inset 0 0 0 2px var(--accent);
	}
	.description {
		display: block;
		width: 100%;
		min-height: max(var(--tap-min), calc(32px * var(--control-scale)));
		padding: var(--space-2) var(--space-5);
		border: none;
		border-bottom: var(--border-width) solid var(--border);
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		resize: none;
		field-sizing: content;
		/* A long description scrolls rather than pushing the tasks off
		   screen: three lines at rest, more while it is edited. */
		max-height: calc(3lh + 2 * var(--space-2));
		overflow-y: auto;
	}
	.description:focus {
		max-height: calc(12lh + 2 * var(--space-2));
	}
	.description:focus-visible {
		box-shadow: inset 2px 0 0 var(--accent);
		color: var(--text-primary);
	}
	.archived {
		padding: var(--space-2) var(--space-5);
		background: var(--surface-2);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.tools {
		display: contents;
	}
	/* Measured on the page itself, so a desktop window with the task panel
	   open narrows it as a phone does. */
	.project {
		container-type: inline-size;
	}
	/* A task panel open takes the room from here, whatever the window's width:
	   the views give up their words before the name gives up its room. */
	@container (max-width: 960px) {
		.views span {
			display: none;
		}
	}
	@container (max-width: 720px) {
		/* Too many controls for one row: the icon and name keep the first,
		   and the rest scroll sideways on the second rather than wrapping. */
		.project > :global(.page) {
			flex-wrap: wrap;
			height: auto;
			padding-block: var(--space-2);
			row-gap: var(--space-2);
		}
		.tools {
			display: flex;
			flex: 1 0 100%;
			align-items: center;
			gap: var(--space-3);
			/* Room for focus rings, which the scrolling would clip. */
			padding: 3px;
			margin: -3px;
			overflow-x: auto;
			scrollbar-width: none;
		}
		.tools > :global(*) {
			flex: none;
		}
	}
	.name {
		flex: 1;
		min-width: 0;
		padding: 2px var(--space-1);
		margin-left: calc(-1 * var(--space-1));
		border: var(--border-width) solid transparent;
		border-radius: var(--radius-sm);
		background: none;
		font-family: var(--font-heading);
		font-size: var(--text-lg);
		font-weight: var(--heading-weight);
	}
	.name:hover {
		border-color: var(--border);
	}
	/* A border and a ring inside it, two pixels of accent in all. */
	.name:focus-visible {
		border-color: var(--accent);
		box-shadow: inset 0 0 0 1px var(--accent);
	}
	.icon {
		display: grid;
		place-items: center;
		width: 30px;
		height: 30px;
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.icon:hover {
		background: var(--surface-2);
		color: var(--danger);
	}
	.icon.save:hover {
		color: var(--text-primary);
	}
</style>
