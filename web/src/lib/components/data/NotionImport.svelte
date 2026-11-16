<script lang="ts">
	import { settingsWorkspace } from '../../settingsWorkspace.svelte';
	// Notion's import (notionimport.ts): the export's zip, where its pages go,
	// how each database comes in, the dry run, and the import. The apps page
	// (routes/(app)/settings/w/[ws]/data/apps) loads this only when Notion is chosen.
	import { resolve } from '$app/paths';
	import FileUp from '@lucide/svelte/icons/file-up';
	import { onMount, untrack } from 'svelte';
	import { announce } from '../../announce.svelte';
	import { summaryLines, type Target } from '../../csvimport';
	import {
		MAX_EXPORT,
		NOTION,
		NotionError,
		openNotion,
		planNotion,
		readNotion,
		runNotion,
		type NotionExport,
		type NotionMade,
		type PagesGo,
		type PlannedPage
	} from '../../notionimport';
	import type { Context } from '../../importplan';
	import { session } from '../../session.svelte';
	import { deletePage, setTaskDeleted } from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import { toasts } from '../../toasts.svelte';
	import { pageHref } from '../../wiki';
	import Button from '../Button.svelte';
	import CsvMapping from './CsvMapping.svelte';
	import ImportPlan from './ImportPlan.svelte';

	const ws = $derived(settingsWorkspace());
	const replica = $derived(ws ? engine.replica(ws.id) : undefined);
	const projects = $derived(replica?.projects() ?? []);
	const fields = $derived(replica?.fields() ?? []);

	let exp = $state.raw<NotionExport | null>(null);
	let fileName = $state('');
	let where = $state('notion');
	let as = $state<Record<string, 'tasks' | 'pages'>>({});
	/** The task table's columns, once the person opens them, for the headings they were opened on. */
	let columns = $state<Target[]>([]);
	let columnsFor = $state<string | null>(null);
	let progress = $state<{ done: number; total: number } | null>(null);
	let made = $state<NotionMade | null>(null);

	const goes = $derived<PagesGo>(
		where === 'notion'
			? { kind: 'notion' }
			: where === 'top'
				? { kind: 'top' }
				: { kind: 'project', id: where }
	);
	const databases = $derived(exp ? [...exp.databases.values()] : []);
	const plan = $derived.by(() => {
		const e = exp;
		const w = goes;
		const a = { ...as };
		const cols = columnsFor === null ? undefined : [...columns];
		if (!e || !ws) return null;
		return untrack(() => {
			const r = engine.replica(ws.id);
			if (!r) return null;
			const first = planNotion(e, { where: w, as: a }, context(r), r.fields());
			// Columns chosen for other headings (another database's choice) no longer hold.
			if (!cols || columnsFor !== first.table[0].join('\u0001')) return first;
			return planNotion(e, { where: w, as: a, targets: cols }, context(r), r.fields());
		});
	});
	const headKey = $derived(plan?.table[0].join('\u0001') ?? '');
	const matching = $derived(columnsFor !== null && columnsFor === headKey);
	const taskCount = $derived(plan?.tasks?.tasks.length ?? 0);
	const pageCount = $derived((plan?.pages.length ?? 0) + (where === 'notion' ? 1 : 0));
	const summary = $derived(plan?.tasks ? summaryLines(plan.tasks, plan.targets, NOTION) : []);
	const placeName = $derived(
		where === 'notion'
			? 'under a new page, Notion, in the workspace wiki'
			: where === 'top'
				? 'at the top of the workspace wiki'
				: `in the wiki of ${projects.find((p) => p.id === where)?.name ?? 'the project'}`
	);
	/** The planned pages under each, by key; the root's under null. */
	const children = $derived.by(() => {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- made afresh by each derivation
		const m = new Map<string | null, PlannedPage[]>();
		for (const p of plan?.pages ?? []) m.set(p.parent, [...(m.get(p.parent) ?? []), p]);
		return m;
	});

	function context(r: NonNullable<ReturnType<typeof engine.replica>>): Context {
		const id = ws!.id;
		return {
			projects: r.projects(),
			statuses: (p: string) => r.statuses(p),
			labels: r.labels(),
			fields: r.fields(),
			members: session.members(id),
			reaches: (userId, projectId) =>
				session.reachable(id, projectId).some((m) => m.userId === userId)
		};
	}

	async function pick(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const f = input.files?.[0];
		exp = null;
		made = null;
		columnsFor = null;
		if (!f) return;
		if (f.size > MAX_EXPORT) {
			toasts.error(
				'That export is over 500 MB, more than an import takes. Export a part at a time.'
			);
			input.value = '';
			return;
		}
		try {
			const read = await readNotion(await openNotion(new Uint8Array(await f.arrayBuffer())));
			if (!read.pages.size && !read.databases.size)
				throw new NotionError('That zip has no Notion pages or databases in it.');
			fileName = f.name;
			as = Object.fromEntries([...read.databases.keys()].map((k) => [k, 'tasks' as const]));
			where = 'notion';
			exp = read;
			const p = plan;
			if (p) announce(`Dry run: ${pageCount} pages and ${taskCount} tasks would come in.`);
		} catch (err) {
			toasts.error(
				err instanceof NotionError ? err.message : 'That file could not be read as a Notion export.'
			);
			input.value = '';
		}
	}

	function matchColumns() {
		if (!plan) return;
		columns = [...plan.targets];
		columnsFor = headKey;
	}

	async function run() {
		if (!ws || !exp || !plan) return;
		const p = plan;
		progress = { done: 0, total: 1 };
		try {
			const out = await runNotion(
				ws.id,
				exp,
				p,
				goes,
				{
					page: pageHref,
					project: (id) => resolve('/(app)/projects/[id]', { id }),
					task: (id) => `${resolve('/(app)')}?task=${id}`
				},
				(done, total) => (progress = { done, total })
			);
			made = out;
			exp = null;
			toasts.show(`Imported ${out.pages.length} pages and ${out.tasks.length} tasks`);
			if (out.failed.length)
				toasts.error(
					`${out.failed.length} ${out.failed.length === 1 ? 'file' : 'files'} could not be uploaded: ${out.failed.slice(0, 3).join(', ')}`
				);
		} catch (err) {
			toasts.error(err instanceof Error ? err.message : 'The import stopped.');
		} finally {
			progress = null;
		}
	}

	async function trash() {
		if (!ws || !made) return;
		const m = made;
		made = null;
		for (const id of m.top) await deletePage(ws.id, id);
		for (const id of m.tasks) await setTaskDeleted(ws.id, id, true);
		toasts.show('Moved the imported pages and tasks to the trash');
	}

	/** Where the imported pages are: the first made at the top, or the root they went under. */
	const madeHref = $derived(
		made ? pageHref({ id: made.top[0] ?? made.root.id, projectId: made.root.projectId }) : ''
	);
	let heading = $state<HTMLElement>();
	onMount(() => () => (exp = null));
	$effect(() => {
		if (made) requestAnimationFrame(() => heading?.focus());
	});
</script>

<label class="file">
	<span>Notion export (zip)</span>
	<input type="file" accept=".zip,application/zip" onchange={pick} disabled={!!progress} />
</label>

{#snippet tree(parent: string | null)}
	{@const list = children.get(parent) ?? []}
	{#if list.length}
		<ul>
			{#each list as p (p.key)}
				<li>
					<span>{p.title}</span>
					{#if p.key.startsWith('db:')}<span class="kind">from a database</span>{/if}
					{@render tree(p.key)}
				</li>
			{/each}
		</ul>
	{/if}
{/snippet}

{#if exp && plan}
	<section>
		<h2>What would come in</h2>
		<p class="hint">
			{fileName}: {exp.pages.size}
			{exp.pages.size === 1 ? 'page' : 'pages'} and {databases.length}
			{databases.length === 1 ? 'database' : 'databases'}.
		</p>
		<div class="choices">
			<label class="field">
				<span>Put the pages in</span>
				<select bind:value={where} disabled={!!progress}>
					<option value="notion">A new page, Notion, in the workspace wiki</option>
					<option value="top">The top of the workspace wiki</option>
					{#each projects as p (p.id)}<option value={p.id}>The wiki of {p.name}</option>{/each}
				</select>
			</label>
			{#each databases as db (db.key)}
				<label class="field">
					<span>Database: {db.name}, {db.rows.length} {db.rows.length === 1 ? 'row' : 'rows'}</span>
					<select bind:value={as[db.key]} disabled={!!progress}>
						<option value="tasks">Tasks, in a project named {db.name}</option>
						<option value="pages">Pages in the wiki, under its page</option>
					</select>
				</label>
			{/each}
		</div>

		<div class="tree" role="group" aria-label="The pages, as they would sit in the wiki">
			<h3>Pages</h3>
			{#if plan.pages.length}
				<ul>
					<li>
						<span class="root"
							>{where === 'notion'
								? 'Notion (a new page)'
								: where === 'top'
									? 'Workspace Wiki'
									: (projects.find((p) => p.id === where)?.name ?? 'The project')}</span
						>
						{@render tree(null)}
					</li>
				</ul>
			{:else}
				<p class="hint">No pages: every database comes in as tasks.</p>
			{/if}
		</div>

		{#if plan.tasks}
			<div class="columns">
				{#if matching}
					<h3>The databases' columns</h3>
					<CsvMapping
						headers={plan.table[0]}
						samples={plan.table
							.slice(1, 4)
							.map((r) => r.map((c) => c.replace(/\(mila-notion:\d+\)/g, '')))}
						{fields}
						bind:targets={columns}
					/>
					<p class="hint">Columns left out come in as a table at the top of each task's notes.</p>
				{:else}
					<Button onclick={matchColumns} disabled={!!progress}>Match columns</Button>
				{/if}
			</div>
		{/if}

		<div class="plan" role="region" aria-label="The pages the import would make">
			<p class="headline">
				{pageCount}
				{pageCount === 1 ? 'page' : 'pages'} would come in, {placeName}.
			</p>
			<ul>
				{#if plan.images || plan.uploads.length}
					<li>
						{plan.uploads.length}
						{plan.uploads.length === 1 ? 'file' : 'files'}, {plan.images}
						{plan.images === 1 ? 'of them an image' : 'of them images'}, uploaded as the pages' and
						tasks' own files
					</li>
				{/if}
				{#if plan.tooLarge.length}
					<li>
						{plan.tooLarge.length}
						{plan.tooLarge.length === 1 ? 'file is' : 'files are'} over 25 MB and left out: {plan.tooLarge
							.slice(0, 5)
							.join(', ')}
					</li>
				{/if}
				{#if plan.links}
					<li>
						{plan.links}
						{plan.links === 1 ? 'link' : 'links'} between pages, pointed at the new pages and tasks
					</li>
				{/if}
				{#if plan.missing}
					<li>
						{plan.missing}
						{plan.missing === 1 ? 'link goes' : 'links go'} to Notion pages not in the export, and
						{plan.missing === 1 ? 'becomes' : 'become'} plain text
					</li>
				{/if}
				{#if plan.callouts}
					<li>
						{plan.callouts}
						{plan.callouts === 1 ? 'callout becomes a note box' : 'callouts become note boxes'}
					</li>
				{/if}
				{#if plan.toggles}
					<li>
						{plan.toggles}
						{plan.toggles === 1 ? 'toggle becomes its heading' : 'toggles become their headings'} with
						what was inside below
					</li>
				{/if}
				{#each plan.problems.slice(0, 5) as p, i (i)}<li>{p.at}: {p.reason}</li>{/each}
				<li>Comments and page history are not in Notion's export, so they stay in Notion</li>
			</ul>
		</div>
		{#if plan.tasks}
			<ImportPlan
				plan={plan.tasks}
				destination="a project for each database"
				item={(line) => {
					const r = plan.table[line - 1];
					return r ? `${r[1]}, ${r[2]}` : `Row ${line}`;
				}}
			>
				{#if summary.length}
					<ul>
						{#each summary.filter((l) => !l.startsWith('Comments and page history')) as line, i (i)}
							<li>{line}</li>
						{/each}
					</ul>
				{/if}
			</ImportPlan>
		{/if}
		{#if plan.over}
			<p class="hint">
				That is more than 5,000 pages and tasks, more than an import makes. Export a part at a time.
			</p>
		{/if}
		<div class="actions">
			<Button
				variant="primary"
				onclick={run}
				disabled={plan.over || pageCount + taskCount === 0 || !!progress}
			>
				<FileUp size={16} />Import {pageCount}
				{pageCount === 1 ? 'page' : 'pages'} and {taskCount}
				{taskCount === 1 ? 'task' : 'tasks'}
			</Button>
		</div>
		<p class="hint">
			Importing the same export again makes a second copy of everything; the dry run cannot tell.
			Files are uploaded, so the import needs a connection.
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
	<section>
		<h2 tabindex="-1" bind:this={heading}>
			Imported {made.pages.length}
			{made.pages.length === 1 ? 'page' : 'pages'} and {made.tasks.length}
			{made.tasks.length === 1 ? 'task' : 'tasks'}
		</h2>
		<div class="actions">
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- pageHref resolves it -->
			<a class="link" href={madeHref}>Open the pages</a>
			<Button
				title="Move the pages and tasks this import made to the trash"
				variant="danger"
				onclick={trash}>Move them to the trash</Button
			>
		</div>
	</section>
{/if}

<style>
	.file,
	.field {
		display: grid;
		gap: var(--space-1);
		font-size: var(--text-sm);
	}
	.file span,
	.field > span {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.file input {
		max-width: 100%;
		min-height: var(--tap-min);
	}
	.choices {
		display: grid;
		gap: var(--space-3);
	}
	.field select {
		max-width: 28rem;
		height: max(var(--tap-min), calc(34px * var(--control-scale)));
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
	}
	h3 {
		font-size: var(--text-sm);
	}
	.tree ul {
		margin: 0;
		padding-left: var(--space-4);
		list-style: none;
		font-size: var(--text-sm);
	}
	.tree > ul {
		padding-left: 0;
	}
	.tree li {
		padding-block: 2px;
	}
	.tree .root {
		font-weight: 600;
	}
	.kind {
		margin-left: var(--space-2);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.plan {
		display: grid;
		gap: var(--space-2);
		padding: var(--space-4);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface-1);
		font-size: var(--text-sm);
	}
	.plan .headline {
		margin: 0;
		font-weight: 600;
	}
	.plan ul {
		margin: 0;
		padding-left: var(--space-5);
		color: var(--text-secondary);
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
