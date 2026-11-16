<script lang="ts">
	import { settingsWorkspace } from '../../settingsWorkspace.svelte';
	// Confluence's import (confluenceimport.ts): the files its Export to
	// Markdown writes, where their pages go, the dry run, and the import. The
	// apps page loads it only when Confluence is chosen.
	import FileUp from '@lucide/svelte/icons/file-up';
	import { onMount } from 'svelte';
	import { announce } from '../../announce.svelte';
	import {
		MAX_FILE,
		MAX_PAGES,
		planConfluence,
		runConfluence,
		type ConfluenceFile,
		type ConfluenceMade,
		type PagesGo
	} from '../../confluenceimport';
	import { deletePage } from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import { toasts } from '../../toasts.svelte';
	import { pageHref } from '../../wiki';
	import Button from '../Button.svelte';

	const ws = $derived(settingsWorkspace());
	const replica = $derived(ws ? engine.replica(ws.id) : undefined);
	const projects = $derived(replica?.projects() ?? []);

	let files = $state.raw<ConfluenceFile[]>([]);
	let skipped = $state.raw<{ file: string; reason: string }[]>([]);
	let where = $state('confluence');
	let progress = $state<{ done: number; total: number } | null>(null);
	let made = $state<ConfluenceMade | null>(null);

	const goes = $derived<PagesGo>(
		where === 'confluence'
			? { kind: 'confluence' }
			: where === 'top'
				? { kind: 'top' }
				: { kind: 'project', id: where }
	);
	// The titles already in the wiki the pages go to, to say when one repeats.
	const existing = $derived(
		(replica?.pages(goes.kind === 'project' ? goes.id : null) ?? []).map((p) => p.title)
	);
	const plan = $derived(
		files.length || skipped.length ? planConfluence(files, existing, skipped) : null
	);
	const count = $derived(plan?.pages.length ?? 0);
	const placeName = $derived(
		where === 'confluence'
			? 'under a new page, Confluence, in the workspace wiki'
			: where === 'top'
				? 'at the top of the workspace wiki'
				: `in the wiki of ${projects.find((p) => p.id === where)?.name ?? 'the project'}`
	);
	const wikiName = $derived(
		goes.kind === 'project'
			? `the wiki of ${projects.find((p) => p.id === where)?.name ?? 'the project'}`
			: 'the workspace wiki'
	);
	const pages = (n: number) => (n === 1 ? '1 page' : `${n} pages`);

	async function pick(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const chosen = [...(input.files ?? [])];
		made = null;
		const read: ConfluenceFile[] = [];
		const left: { file: string; reason: string }[] = [];
		for (const f of chosen) {
			if (!/\.(md|markdown)$/i.test(f.name))
				left.push({ file: f.name, reason: 'not a Markdown file' });
			else if (f.size > MAX_FILE) left.push({ file: f.name, reason: 'over 20 MB' });
			else
				try {
					read.push({ name: f.name, text: await f.text() });
				} catch {
					left.push({ file: f.name, reason: 'could not be read' });
				}
		}
		files = read;
		skipped = left;
		where = 'confluence';
		if (!read.length) {
			toasts.error('None of those files is a Markdown file from Confluence.');
			input.value = '';
			return;
		}
		announce(`Dry run: ${pages(read.length)} would come in.`);
	}

	async function run() {
		if (!ws || !plan) return;
		progress = { done: 0, total: count };
		try {
			const out = await runConfluence(
				ws.id,
				plan,
				goes,
				(done, total) => (progress = { done, total })
			);
			made = out;
			files = [];
			skipped = [];
			toasts.show(`Imported ${pages(plan.pages.length)}`);
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
		toasts.show('Moved the imported pages to the trash');
	}

	const madeHref = $derived(
		made ? pageHref({ id: made.top[0] ?? made.root.id, projectId: made.root.projectId }) : ''
	);
	let heading = $state<HTMLElement>();
	onMount(() => () => (files = []));
	$effect(() => {
		if (made) requestAnimationFrame(() => heading?.focus());
	});
</script>

<ol class="steps">
	<li>In Confluence, open a page, then its menu of three dots, Export, Export to Markdown.</li>
	<li>
		Do the same for each page you want. Pages under it are not in its file, so export those too.
	</li>
	<li>
		Choose all the files below at once. Each becomes a page in the wiki. Nothing changes until you
		have seen what would come in.
	</li>
</ol>

<label class="file">
	<span>Confluence pages (Markdown files)</span>
	<input
		type="file"
		accept=".md,.markdown,text/markdown"
		multiple
		onchange={pick}
		disabled={!!progress}
	/>
</label>

{#if plan && count}
	<section>
		<h2>What would come in</h2>
		<label class="field">
			<span>Put the pages in</span>
			<select bind:value={where} disabled={!!progress}>
				<option value="confluence">A new page, Confluence, in the workspace wiki</option>
				<option value="top">The top of the workspace wiki</option>
				{#each projects as p (p.id)}<option value={p.id}>The wiki of {p.name}</option>{/each}
			</select>
		</label>

		<div class="plan" role="region" aria-label="The pages the import would make">
			<ul class="pages">
				{#each plan.pages as p (p.file)}
					<li>
						<span class="t">{p.title}</span>
						<span class="n"
							>{p.words === 1 ? '1 word' : `${p.words.toLocaleString()} words`}{#if p.tables}
								· {p.tables === 1 ? '1 table' : `${p.tables} tables`}{/if}</span
						>
						{#if plan.repeats.has(p.file)}
							<span class="w warn"
								>A page called {p.title} is already in {wikiName}, or among these files; this makes
								another.</span
							>
						{/if}
						{#if p.confluenceLinks}
							<span class="w"
								>{p.confluenceLinks === 1
									? '1 link goes to Confluence, and keeps going there.'
									: `${p.confluenceLinks} links go to Confluence, and keep going there.`}</span
							>
						{/if}
						{#if p.cut}<span class="w warn">Longer than a page holds, so cut at its end.</span>{/if}
					</li>
				{/each}
			</ul>
			<p class="headline">{pages(count)} would come in, {placeName}.</p>
			<ul class="notes">
				<li>
					Pictures and attached files are not in Confluence's Markdown, so add them in Mila
					afterwards.
				</li>
				<li>
					The pages go side by side, in title order: drag them into a tree afterwards. Nothing is
					changed in Confluence.
				</li>
				{#each plan.skipped as s (s.file)}<li>{s.file}: left out, {s.reason}</li>{/each}
			</ul>
		</div>
		{#if plan.over}
			<p class="hint">
				That is more than {MAX_PAGES.toLocaleString()} pages, more than an import makes.
			</p>
		{/if}
		<div class="actions">
			<Button variant="primary" onclick={run} disabled={plan.over || !!progress}>
				<FileUp size={16} />Import {pages(count)}
			</Button>
		</div>
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
			Imported {pages(made.pages.length - (made.top[0] === made.root.id ? 1 : 0))}
		</h2>
		<div class="actions">
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- pageHref resolves it -->
			<a class="link" href={madeHref}>Open the pages</a>
			<Button title="Move the pages this import made to the trash" variant="danger" onclick={trash}
				>Move them to the trash</Button
			>
		</div>
	</section>
{/if}

<style>
	.steps {
		display: grid;
		gap: var(--space-2);
		margin: 0;
		padding-left: var(--space-5);
		font-size: var(--text-sm);
	}
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
	.field select {
		max-width: 28rem;
		height: max(var(--tap-min), calc(34px * var(--control-scale)));
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
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
	.pages {
		margin: 0;
		padding: 0;
		list-style: none;
		display: grid;
		gap: var(--space-2);
	}
	.pages li {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		gap: 2px var(--space-3);
	}
	.t {
		font-weight: 550;
		overflow-wrap: anywhere;
	}
	.n {
		font-size: var(--text-xs);
		color: var(--text-secondary);
		white-space: nowrap;
	}
	.w {
		grid-column: 1 / -1;
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.w.warn {
		color: var(--danger);
	}
	.headline {
		margin: 0;
		font-weight: 600;
	}
	.notes {
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
