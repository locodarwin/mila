<script lang="ts">
	import { settingsWorkspace } from '../../settingsWorkspace.svelte';
	// The CSV import's column screen, its dry run and the import, for any CSV
	// file (Settings, Import and export, Import a CSV file) or an app's (ClickUp's,
	// Asana's, Todoist's or monday.com's, from Import from another app), whose
	// dialect matches its columns first, after making the app's files one
	// table where they are not (Todoist's and monday.com's, several at once;
	// monday.com's are Excel, read by its own reader). The page holds the file
	// input and passes the files to `load`.
	import FileUp from '@lucide/svelte/icons/file-up';
	import { announce } from '../../announce.svelte';
	import { parseCsv } from '../../csv';
	import {
		ANY_CSV,
		guessTargets,
		plan as makePlan,
		summaryLines,
		type Dialect,
		type Reshaped,
		type Target
	} from '../../csvimport';
	import { runImport, type Destination, type Plan } from '../../importplan';
	import { session } from '../../session.svelte';
	import { engine } from '../../sync/engine.svelte';
	import { toasts } from '../../toasts.svelte';
	import Button from '../Button.svelte';
	import CsvMapping from './CsvMapping.svelte';
	import ImportDone from './ImportDone.svelte';
	import ImportPlan from './ImportPlan.svelte';

	interface Props {
		/** How the file writes its values; any CSV file's by default. */
		dialect?: Dialect;
		/** The choice that puts each row in the project its Project column names. */
		ownLabel?: string;
		/** A new project's name when the file's own name gives none. */
		fallbackName?: string;
		/** Whether the headings are the ones the dialect expects; a hint says when not. */
		recognized?: (headers: string[]) => boolean;
	}

	let {
		dialect = ANY_CSV,
		ownLabel = 'The project each row names',
		fallbackName = 'Imported',
		recognized = () => true
	}: Props = $props();

	const ws = $derived(settingsWorkspace());
	const replica = $derived(ws ? engine.replica(ws.id) : undefined);

	let fileName = $state('');
	let rows = $state<string[][]>([]);
	/** For files the dialect made into one table: each line's place in them, and what was left out first. */
	let shaped = $state<{ where: string[]; left: Plan['skipped']; said: string[] } | null>(null);
	let targets = $state<Target[]>([]);
	let where = $state<'own' | 'new' | 'inbox' | string>('new');
	let newName = $state('');
	let checked = $state<Plan | null>(null);
	let checkedFor = $state('');
	let progress = $state<{ done: number; total: number } | null>(null);
	let made = $state<{ ids: string[]; project: string | null } | null>(null);

	const headers = $derived(rows[0] ?? []);
	const fields = $derived(replica?.fields() ?? []);
	const projects = $derived(replica?.projects() ?? []);
	const byColumn = $derived(targets.includes('project'));
	// Without a Project column, there is no project each row names.
	$effect(() => {
		if (where === 'own' && !byColumn) where = 'new';
	});
	const newProject = $derived(newName.trim() || fallbackName);
	const destination = $derived<Destination>(
		where === 'new' || where === 'own'
			? { kind: 'new', name: newProject }
			: where === 'inbox'
				? { kind: 'inbox' }
				: { kind: 'project', id: where }
	);
	const ownProjects = $derived(where === 'own' && byColumn);
	// The dry run holds only for the choices it was made with.
	const choices = $derived(JSON.stringify([targets, destination, ownProjects]));
	const current = $derived(checked && checkedFor === choices ? checked : null);
	const summary = $derived(
		current ? [...summaryLines(current, targets, dialect), ...(shaped?.said ?? [])] : []
	);
	// Where the tasks go when they all go to one place: the chosen place, or
	// the project every row names.
	const placeName = $derived.by(() => {
		const first = current?.tasks[0];
		const ref = first
			? first.project
			: destination.kind === 'new'
				? { make: `project:${destination.name}` }
				: destination.kind === 'project'
					? { id: destination.id }
					: null;
		if (!ref) return 'the inbox';
		if ('make' in ref) return `a new project, ${ref.make.slice('project:'.length)}`;
		return projects.find((p) => p.id === ref.id)?.name ?? 'the project';
	});

	/** Reads the chosen files; false, with a toast saying why, when they can't be imported. */
	export async function load(chosen: File | File[] | undefined): Promise<boolean> {
		checked = null;
		made = null;
		rows = [];
		shaped = null;
		const files = chosen === undefined ? [] : Array.isArray(chosen) ? chosen : [chosen];
		if (!files.length) return false;
		if (files.reduce((sum, f) => sum + f.size, 0) > 20 << 20) {
			toasts.error(
				`${files.length === 1 ? 'That file is' : 'Those files are'} over 20 MB, more than an import takes.`
			);
			return false;
		}
		let table: Reshaped | null;
		let parsed: string[][];
		if (dialect.readFiles) {
			// A file that is not CSV, read by its app's own reader.
			try {
				table = await dialect.readFiles(files);
			} catch (err) {
				toasts.error(err instanceof Error ? err.message : 'That file could not be read.');
				return false;
			}
			if (!table) {
				toasts.error(
					`${files.length === 1 ? 'That is not a file' : 'Those are not files'} ${dialect.app ?? 'the app'} exports.`
				);
				return false;
			}
			parsed = table.rows;
		} else {
			const read = await Promise.all(
				files.map(async (f) => ({ name: f.name, rows: parseCsv(await f.text()) }))
			);
			table = dialect.reshape?.(read) ?? null;
			if (!table && files.length > 1) {
				toasts.error(
					`Those are not ${dialect.app ?? 'an app'}'s files, so choose one at a time and match its columns.`
				);
				return false;
			}
			parsed = table?.rows ?? read[0].rows;
		}
		if (parsed.length < 2) {
			toasts.error(
				table
					? `${files.length === 1 ? 'That file has' : 'Those files have'} no tasks in them.`
					: 'That file has no rows below its headings.'
			);
			return false;
		}
		fileName = files.length === 1 ? files[0].name : `${files[0].name} and ${files.length - 1} more`;
		newName = files[0].name.replace(/\.(csv|tsv|txt|xlsx)$/i, '').slice(0, 200);
		if (table) shaped = { where: table.where, left: table.left, said: table.said ?? [] };
		rows = parsed;
		targets = guessTargets(parsed[0], replica?.fields() ?? [], dialect);
		if (!targets.includes('title')) targets[0] = 'title';
		where = targets.includes('project') ? 'own' : 'new';
		return true;
	}

	function check() {
		if (!ws || !replica) return;
		checked = makePlan(
			rows,
			targets,
			destination,
			{
				projects: replica.projects(),
				statuses: (id) => replica.statuses(id),
				labels: replica.labels(),
				fields: replica.fields(),
				members: session.members(ws.id),
				reaches: (userId, projectId) =>
					session.reachable(ws.id, projectId).some((m) => m.userId === userId)
			},
			{ dialect, byColumn: ownProjects }
		);
		if (shaped?.left.length) checked.skipped = [...checked.skipped, ...shaped.left];
		checkedFor = choices;
		announce(`Dry run: ${checked.tasks.length} tasks would come in.`);
	}

	async function run() {
		if (!ws || !current) return;
		const p = current;
		progress = { done: 0, total: p.tasks.length };
		try {
			const ids = await runImport(ws.id, p, (done, total) => (progress = { done, total }));
			const first = ids.length ? engine.replica(ws.id)?.task(ids[0]) : undefined;
			made = { ids, project: first?.projectId ?? null };
			checked = null;
			rows = [];
			toasts.show(`Imported ${ids.length} ${ids.length === 1 ? 'task' : 'tasks'}`);
		} catch (err) {
			toasts.error(err instanceof Error ? err.message : 'The import stopped.');
		} finally {
			progress = null;
		}
	}
</script>

{#if rows.length > 1}
	<section>
		<h2>Columns</h2>
		<p class="hint">
			{fileName}: {rows.length - 1}
			{shaped ? (rows.length === 2 ? 'task' : 'tasks') : rows.length === 2 ? 'row' : 'rows'}. One
			column must be the title.
			{#if dialect.app && !shaped && !recognized(headers)}
				These headings are not the ones {dialect.app} writes, so choose what each column goes to.
			{/if}
		</p>
		<CsvMapping {headers} samples={rows.slice(1, 4)} {fields} bind:targets />
		<fieldset class="where">
			<legend>Put the tasks in</legend>
			{#if byColumn}
				<label class="choice">
					<input type="radio" name="where" value="own" bind:group={where} />
					<span>{ownLabel}</span>
				</label>
				{#if where === 'own'}
					<p class="hint">Rows that name none go to a new project, {newProject}.</p>
				{/if}
			{/if}
			<label class="choice">
				<input type="radio" name="where" value="new" bind:group={where} />
				<span>A new project</span>
			</label>
			{#if where === 'new'}
				<label class="name">
					<span>New project's name</span>
					<input bind:value={newName} maxlength="200" />
				</label>
			{/if}
			{#each projects as p (p.id)}
				<label class="choice">
					<input type="radio" name="where" value={p.id} bind:group={where} />
					<span>{p.name}</span>
				</label>
			{/each}
			<label class="choice">
				<input type="radio" name="where" value="inbox" bind:group={where} />
				<span>The inbox</span>
			</label>
		</fieldset>
		<div class="actions">
			<Button
				title="Show what the import would do, without doing it"
				onclick={check}
				disabled={!targets.includes('title') || !!progress}
			>
				Check the import
			</Button>
			<Button
				variant="primary"
				onclick={run}
				disabled={!current || current.tasks.length === 0 || !!progress}
			>
				<FileUp size={16} />Import {current ? current.tasks.length : ''}
				{current?.tasks.length === 1 ? 'task' : 'tasks'}
			</Button>
		</div>
		{#if !targets.includes('title')}
			<p class="hint">Choose the column that holds each task's title.</p>
		{:else if !current}
			<p class="hint">Check the import first; nothing changes until you import.</p>
		{/if}
		{#if current}
			<ImportPlan
				plan={current}
				destination={placeName}
				item={(line) => shaped?.where[line] ?? `Row ${line}`}
			>
				{#if summary.length}
					<ul>
						{#each summary as line, i (i)}<li>{line}</li>{/each}
					</ul>
				{/if}
			</ImportPlan>
			<p class="hint">
				Importing the same file again makes a second copy of each task; the dry run cannot tell.
			</p>
		{/if}
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
	.name {
		display: grid;
		gap: var(--space-1);
		font-size: var(--text-sm);
	}
	.name span {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.name input {
		max-width: 24rem;
		height: max(var(--tap-min), calc(34px * var(--control-scale)));
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
	}
	.name input::placeholder {
		color: var(--input-placeholder);
	}
	.where {
		display: grid;
		gap: var(--space-2);
		margin: 0;
		padding: 0;
		border: none;
	}
	legend {
		margin-bottom: var(--space-1);
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.choice {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.choice input {
		flex: none;
		width: max(var(--tap-min), 16px);
		height: max(var(--tap-min), 16px);
		margin: 0;
		accent-color: var(--accent);
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-3);
	}
	progress {
		vertical-align: middle;
	}
</style>
