<script lang="ts">
	import { settingsWorkspace } from '#lib/settingsWorkspace.svelte.js';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import FileUp from '@lucide/svelte/icons/file-up';
	import { tick, untrack } from 'svelte';
	import { announce } from '#lib/announce.svelte.js';
	import Button from '#lib/components/Button.svelte';
	import CsvImport from '#lib/components/data/CsvImport.svelte';
	import type { Dialect } from '#lib/csvimport.js';
	import ImportDone from '#lib/components/data/ImportDone.svelte';
	import ImportPlan from '#lib/components/data/ImportPlan.svelte';
	import {
		IssueFileError,
		MAX_BYTES,
		planIssues,
		readIssues,
		repoName,
		type IssueFile
	} from '#lib/githubimport.js';
	import { runImport, type Destination, type Plan } from '#lib/importplan.js';
	import { session } from '#lib/session.svelte.js';
	import { engine } from '#lib/sync/engine.svelte.js';
	import { toasts } from '#lib/toasts.svelte.js';

	// The apps a file can come from. Each has its own steps for getting the
	// file and its own reader; the dry run and the import are shared. A CSV
	// app's file opens the CSV import's column screen, its columns matched.
	const APPS = [
		{ id: 'github', name: 'GitHub Issues' },
		{ id: 'clickup', name: 'ClickUp' },
		{ id: 'asana', name: 'Asana' },
		{ id: 'todoist', name: 'Todoist' },
		{ id: 'monday', name: 'monday.com' },
		{ id: 'notion', name: 'Notion' },
		{ id: 'confluence', name: 'Confluence' }
	] as const;
	type Reader = { dialect: Dialect; recognized: (headers: string[]) => boolean };
	const CSV = { kind: 'CSV', accept: '.csv,text/csv' };
	// The apps whose file opens the column screen, and how it starts. Each
	// app's reader, and its steps, load only when the app is chosen.
	const CSV_APPS = {
		clickup: {
			...CSV,
			load: async (): Promise<Reader> => {
				const m = await import('#lib/clickupimport.js');
				return { dialect: m.CLICKUP, recognized: m.isClickUp };
			},
			ownLabel: 'A project for each list',
			steps: () => import('#lib/components/data/ClickupSteps.svelte'),
			several: false
		},
		asana: {
			...CSV,
			load: async (): Promise<Reader> => {
				const m = await import('#lib/asanaimport.js');
				return { dialect: m.ASANA, recognized: m.isAsana };
			},
			ownLabel: 'The project each task is in',
			steps: () => import('#lib/components/data/AsanaSteps.svelte'),
			several: false
		},
		// A file for each project, chosen together.
		todoist: {
			...CSV,
			load: async (): Promise<Reader> => {
				const m = await import('#lib/todoistimport.js');
				return { dialect: m.TODOIST, recognized: m.isTodoist };
			},
			ownLabel: 'A project for each file',
			steps: () => import('#lib/components/data/TodoistSteps.svelte'),
			several: true
		},
		// A file for each board, chosen together; Excel, read by its own reader.
		monday: {
			kind: 'Excel',
			accept: '.xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
			load: async (): Promise<Reader> => {
				const m = await import('#lib/mondayimport.js');
				return { dialect: m.MONDAY, recognized: () => true };
			},
			ownLabel: 'A project for each board',
			steps: () => import('#lib/components/data/MondaySteps.svelte'),
			several: true
		}
	} as const;

	const ws = $derived(settingsWorkspace());
	const replica = $derived(ws ? engine.replica(ws.id) : undefined);
	const guest = $derived(ws?.role === 'guest');

	let app = $state<(typeof APPS)[number]['id']>('github');
	const csvApp = $derived(
		app === 'github' || app === 'notion' || app === 'confluence' ? null : CSV_APPS[app]
	);
	// Notion's and Confluence's imports have screens of their own, loaded only when chosen.
	const notionScreen = () => import('#lib/components/data/NotionImport.svelte');
	const confluenceScreen = () => import('#lib/components/data/ConfluenceImport.svelte');
	/** The chosen app's reader, once it has loaded, and its loading. */
	let reader = $state<{ app: string; reader: Reader } | null>(null);
	const loaded = $derived(reader?.app === app ? reader.reader : null);
	let loading: Promise<unknown> = Promise.resolve();
	let fileName = $state('');
	let file = $state<IssueFile | null>(null);
	let where = $state<'new' | 'inbox' | string>('new');
	let newName = $state('');
	let progress = $state<{ done: number; total: number } | null>(null);
	let made = $state<{ ids: string[]; project: string | null } | null>(null);

	const appName = $derived(APPS.find((a) => a.id === app)?.name ?? '');
	const projects = $derived(replica?.projects() ?? []);
	const destination = $derived<Destination>(
		where === 'new'
			? { kind: 'new', name: newName.trim() || 'GitHub issues' }
			: where === 'inbox'
				? { kind: 'inbox' }
				: { kind: 'project', id: where }
	);
	// The dry run follows the file and the place chosen. It reads the
	// workspace as it is then, and not again as the import itself changes it.
	const plan = $derived.by((): Plan | null => {
		const f = file;
		const d = destination;
		if (!f || !ws) return null;
		return untrack(() => {
			const r = engine.replica(ws.id);
			if (!r) return null;
			return planIssues(f, d, {
				projects: r.projects(),
				statuses: (id) => r.statuses(id),
				labels: r.labels(),
				fields: r.fields(),
				members: session.members(ws.id),
				reaches: (userId, projectId) =>
					session.reachable(ws.id, projectId).some((m) => m.userId === userId)
			});
		});
	});
	const placeName = $derived(
		destination.kind === 'new'
			? `a new project, ${destination.name}`
			: destination.kind === 'inbox'
				? 'the inbox'
				: (projects.find((p) => p.id === where)?.name ?? 'the project')
	);
	const counts = $derived.by(() => {
		const tasks = plan?.tasks ?? [];
		const closed = tasks.filter((t) => t.completedAt).length;
		return {
			open: tasks.length - closed,
			closed,
			comments: tasks.reduce((n, t) => n + t.comments.length, 0)
		};
	});

	let csvImport = $state<ReturnType<typeof CsvImport>>();

	function choose(id: (typeof APPS)[number]['id']) {
		app = id;
		file = null;
		made = null;
		if (id === 'github' || id === 'notion' || id === 'confluence' || reader?.app === id) return;
		loading = CSV_APPS[id]
			.load()
			.then((r) => (reader = { app: id, reader: r }))
			.catch(() =>
				toasts.error(`The ${appName} import could not be loaded. Try again when online.`)
			);
	}

	async function pickCsv(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const files = [...(input.files ?? [])];
		// A file chosen before the app's reader has loaded waits for it.
		await loading;
		await tick();
		if (!(await csvImport?.load(files))) input.value = '';
	}

	async function pick(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const f = input.files?.[0];
		file = null;
		made = null;
		if (!f) return;
		if (f.size > MAX_BYTES) {
			toasts.error('That file is over 20 MB, more than an import takes.');
			input.value = '';
			return;
		}
		try {
			const read = readIssues(await f.text());
			if (!read.entries) throw new IssueFileError('That file has no issues in it.');
			fileName = f.name;
			newName = (repoName(read.repo) ?? 'GitHub issues').slice(0, 200);
			where = 'new';
			file = read;
			const p = plan;
			if (p) announce(`Dry run: ${p.tasks.length} tasks would come in.`);
		} catch (err) {
			toasts.error(
				err instanceof IssueFileError ? err.message : 'That file could not be read as issues.'
			);
			input.value = '';
		}
	}

	async function run() {
		if (!ws || !plan) return;
		const p = plan;
		progress = { done: 0, total: p.tasks.length };
		try {
			const ids = await runImport(ws.id, p, (done, total) => (progress = { done, total }));
			const first = ids.length ? engine.replica(ws.id)?.task(ids[0]) : undefined;
			made = { ids, project: first?.projectId ?? null };
			file = null;
			toasts.show(`Imported ${ids.length} ${ids.length === 1 ? 'task' : 'tasks'}`);
		} catch (err) {
			toasts.error(err instanceof Error ? err.message : 'The import stopped.');
		} finally {
			progress = null;
		}
	}
</script>

<svelte:head><title>Import from another app · Mila</title></svelte:head>

<section>
	<h2>Import from another app</h2>
	<p class="hint">
		Work from another app, into {ws?.name ?? 'this workspace'}, from a file you export there. You
		see what would come in before anything changes.
		<a class="link" href={resolve('/(app)/settings/w/[ws]/data', { ws: page.params.ws ?? '' })}
			>Back to Import and export</a
		>
	</p>
	{#if guest}
		<p class="hint">Guests cannot import tasks here.</p>
	{:else}
		<div class="apps" role="group" aria-label="App to import from">
			{#each APPS as a (a.id)}
				<button
					type="button"
					class="app"
					title="Import from {a.name}"
					aria-pressed={app === a.id}
					onclick={() => choose(a.id)}>{a.name}</button
				>
			{/each}
		</div>
		<h3>Get your file from {appName}</h3>
		{#if app === 'github'}
			{#await import('#lib/components/data/GithubSteps.svelte') then { default: GithubSteps }}
				<GithubSteps />
			{/await}
			<label class="file">
				<span>Issues file (JSON)</span>
				<input type="file" accept=".json,application/json" onchange={pick} />
			</label>
		{:else if app === 'notion'}
			{#await import('#lib/components/data/NotionSteps.svelte') then { default: NotionSteps }}
				<NotionSteps />
			{/await}
			{#await notionScreen()}
				<p class="hint">Loading the Notion import…</p>
			{:then { default: NotionImport }}
				<NotionImport />
			{:catch}
				<p class="hint">The Notion import could not be loaded. Try again when online.</p>
			{/await}
		{:else if app === 'confluence'}
			{#await confluenceScreen()}
				<p class="hint">Loading the Confluence import…</p>
			{:then { default: ConfluenceImport }}
				<ConfluenceImport />
			{:catch}
				<p class="hint">The Confluence import could not be loaded. Try again when online.</p>
			{/await}
		{:else if csvApp}
			{#await csvApp.steps() then { default: Steps }}
				<Steps />
			{/await}
			{#key app}
				<label class="file">
					<span>{appName} {csvApp.several ? 'files' : 'file'} ({csvApp.kind})</span>
					<input type="file" accept={csvApp.accept} multiple={csvApp.several} onchange={pickCsv} />
				</label>
			{/key}
		{/if}
	{/if}
</section>

{#if csvApp && loaded && !guest}
	{#key app}
		<CsvImport
			bind:this={csvImport}
			dialect={loaded.dialect}
			ownLabel={csvApp.ownLabel}
			fallbackName={appName}
			recognized={loaded.recognized}
		/>
	{/key}
{/if}

{#if file && plan && !guest}
	<section>
		<h2>What would come in</h2>
		<p class="hint">
			{fileName}: {file.entries}
			{file.entries === 1 ? 'entry' : 'entries'}{file.repo ? `, from ${file.repo}` : ''}.
		</p>
		<div class="where">
			<label class="field">
				<span>Put it in</span>
				<select bind:value={where} disabled={!!progress}>
					<option value="new">A new project</option>
					{#each projects as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
					<option value="inbox">The inbox</option>
				</select>
			</label>
			{#if where === 'new'}
				<label class="field">
					<span>New project's name</span>
					<input bind:value={newName} maxlength="200" disabled={!!progress} />
				</label>
			{/if}
		</div>
		<ImportPlan
			{plan}
			destination={placeName}
			item={(line) => file?.names.get(line) ?? `Entry ${line}`}
			unit={['entry', 'entries']}
		>
			<ul>
				<li>{counts.open} open and {counts.closed} closed; closed ones come in done</li>
				{#if counts.comments}
					<li>
						{counts.comments}
						{counts.comments === 1 ? 'comment' : 'comments'}, each starting with who wrote it and
						when
					</li>
				{/if}
				<li>Each task's notes start with a link back to its issue</li>
				<li>
					Pull requests, reactions and project boards are not in the file, so they stay behind
				</li>
			</ul>
		</ImportPlan>
		<div class="actions">
			<Button variant="primary" onclick={run} disabled={plan.tasks.length === 0 || !!progress}>
				<FileUp size={16} />Import {plan.tasks.length}
				{plan.tasks.length === 1 ? 'task' : 'tasks'}
			</Button>
		</div>
		<p class="hint">
			Importing the same file again makes a second copy of each task; the dry run cannot tell.
		</p>
		{#if progress}
			<p class="hint" role="status">
				<progress max={progress.total} value={progress.done}></progress>
				Imported {progress.done} of {progress.total}
			</p>
		{/if}
	</section>
{/if}

{#if made && ws}
	<ImportDone ws={ws.id} ids={made.ids} project={made.project} ontrash={() => (made = null)} />
{/if}

<style>
	.apps {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.app {
		min-height: max(var(--tap-min), calc(32px * var(--control-scale)));
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--button-secondary-border);
		border-radius: var(--button-radius);
		background: var(--button-secondary-bg);
		color: var(--button-secondary-text);
		font-size: var(--text-sm);
		font-weight: 550;
		cursor: pointer;
	}
	.app[aria-pressed='true'] {
		border-color: var(--accent);
		background: var(--accent-soft);
	}
	h3 {
		font-size: var(--text-sm);
	}
	.file,
	.field {
		display: grid;
		gap: var(--space-1);
		font-size: var(--text-sm);
	}
	.file span,
	.field span {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.file input {
		max-width: 100%;
		min-height: var(--tap-min);
	}
	.where {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-3);
	}
	.field input,
	.field select {
		max-width: 24rem;
		height: max(var(--tap-min), calc(34px * var(--control-scale)));
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-3);
	}
	.link {
		color: var(--accent-text);
		font-weight: 550;
	}
	progress {
		vertical-align: middle;
	}
</style>
