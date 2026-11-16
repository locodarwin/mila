<script lang="ts">
	import { settingsWorkspace } from '#lib/settingsWorkspace.svelte.js';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import FileDown from '@lucide/svelte/icons/file-down';
	import Upload from '@lucide/svelte/icons/upload';
	import { api, ApiError, call, messageOf, type Schemas } from '#lib/api/client.js';
	import Button from '#lib/components/Button.svelte';
	import ExportList from '#lib/components/data/ExportList.svelte';
	import ImportList from '#lib/components/data/ImportList.svelte';
	import { session } from '#lib/session.svelte.js';
	import { toasts } from '#lib/toasts.svelte.js';

	type Format = Schemas['ExportFormat'];

	let exports = $state<Schemas['Export'][]>([]);
	let imports = $state<Schemas['Import'][]>([]);
	let format = $state<Format>('json');
	let busy = $state(false);
	let ids = $state<'fresh' | 'keep'>('fresh');
	let file = $state<File | null>(null);
	let picker = $state<HTMLInputElement>();

	const ws = $derived(settingsWorkspace());
	const formats: { value: Format; label: string; hint: string }[] = [
		{
			value: 'json',
			label: 'JSON',
			hint: 'Everything, the wikis too, with attachments and history. Mila can import it again.'
		},
		{
			value: 'csv',
			label: 'CSV',
			hint: 'A spreadsheet per project, a row per task, a column per field.'
		},
		{
			value: 'markdown',
			label: 'Markdown',
			hint: 'A folder per project, a file per task and per wiki page, for Obsidian or any editor.'
		}
	];

	async function load() {
		[exports, imports] = await Promise.all([
			call(api.GET('/me/exports')),
			call(api.GET('/me/imports'))
		]);
	}

	// Exports and imports run on the server; while any is on its way the
	// lists are looked at again every few seconds.
	const waiting = $derived(
		[...exports, ...imports].some((x) => x.status === 'queued' || x.status === 'running')
	);
	$effect(() => {
		load().catch((err) => toasts.error(messageOf(err)));
	});
	$effect(() => {
		if (!waiting) return;
		const timer = setInterval(() => {
			const before = imports.filter((i) => i.status === 'ready').length;
			load()
				.then(() => {
					if (imports.filter((i) => i.status === 'ready').length > before)
						void session.loadWorkspaces();
				})
				.catch(() => {});
		}, 3000);
		return () => clearInterval(timer);
	});

	async function startExport(e: SubmitEvent) {
		e.preventDefault();
		if (!ws) return;
		busy = true;
		try {
			await call(
				api.POST('/workspaces/{workspaceId}/exports', {
					params: { path: { workspaceId: ws.id } },
					body: { format }
				})
			);
			toasts.show('Export started. You will be told when it is ready.');
			await load();
		} catch (err) {
			toasts.error(messageOf(err));
		} finally {
			busy = false;
		}
	}

	async function removeExport(id: string) {
		try {
			await call(api.DELETE('/exports/{exportId}', { params: { path: { exportId: id } } }));
			await load();
		} catch (err) {
			toasts.error(messageOf(err));
		}
	}

	async function startImport(e: SubmitEvent) {
		e.preventDefault();
		if (!file) return;
		busy = true;
		const form = new FormData();
		form.append('ids', ids);
		form.append('file', file);
		try {
			let res: Response;
			try {
				res = await fetch('/api/v1/imports', { method: 'POST', body: form });
			} catch {
				throw new ApiError('network', 'Mila could not reach the server. Check your connection.', 0);
			}
			if (!res.ok) {
				const body = (await res.json().catch(() => ({}))) as Partial<Schemas['Error']>;
				throw new ApiError(
					body.error ?? 'http',
					body.message ?? 'The import was refused.',
					res.status
				);
			}
			toasts.show('Import started. The workspace appears when it is complete.');
			file = null;
			if (picker) picker.value = '';
			await load();
		} catch (err) {
			toasts.error(messageOf(err));
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Import and export · Mila</title></svelte:head>

<section class="half">
	<h2>Export</h2>
	<p class="hint">
		Take a copy of {ws ? ws.name : 'this workspace'} as you see it. It is made in the background; you
		get a notification when it is ready, and it can be downloaded for 7 days.
	</p>
	<form class="export" onsubmit={startExport}>
		<fieldset>
			<legend>Format</legend>
			{#each formats as f (f.value)}
				<label class="choice">
					<input type="radio" name="format" value={f.value} bind:group={format} />
					<span><strong>{f.label}</strong> <span class="sub">{f.hint}</span></span>
				</label>
			{/each}
		</fieldset>
		<div>
			<Button type="submit" variant="primary" disabled={busy || !ws}>
				<FileDown size={16} />Export {ws?.name ?? ''}
			</Button>
		</div>
	</form>
	<ExportList items={exports} onremove={removeExport} />
</section>

<section class="half">
	<h2>Import</h2>
	<p class="hint">
		Bring back a JSON export, from this Mila or another, as a new workspace that you own. Your own
		My Day, reminders and views come with it; other people's assignments do not, and what others
		wrote is credited to you. Links and images in notes and wiki pages point at what came in with
		them.
	</p>
	<form class="import" onsubmit={startImport}>
		<label class="file">
			<span>JSON export (.zip)</span>
			<input
				bind:this={picker}
				type="file"
				accept=".zip,application/zip"
				onchange={(e) => (file = (e.currentTarget as HTMLInputElement).files?.[0] ?? null)}
			/>
		</label>
		<fieldset>
			<legend>IDs</legend>
			<label class="choice">
				<input type="radio" name="ids" value="fresh" bind:group={ids} />
				<span
					><strong>New IDs</strong>
					<span class="sub">Works always, even while the original workspace is still here.</span
					></span
				>
			</label>
			<label class="choice">
				<input type="radio" name="ids" value="keep" bind:group={ids} />
				<span
					><strong>Keep the original IDs</strong>
					<span class="sub"
						>For moving a workspace to this Mila; refused if any of them is already here.</span
					></span
				>
			</label>
		</fieldset>
		<div>
			<Button
				title="Make a new workspace from a JSON export"
				type="submit"
				variant="primary"
				disabled={busy || !file}
			>
				<Upload size={16} />Import
			</Button>
		</div>
	</form>
	<ImportList items={imports} />
	<p class="hint">
		Coming from a spreadsheet or another app?
		<a class="link" href={resolve('/(app)/settings/w/[ws]/data/csv', { ws: page.params.ws ?? '' })}
			>Import a CSV file</a
		>
		or
		<a class="link" href={resolve('/(app)/settings/w/[ws]/data/apps', { ws: page.params.ws ?? '' })}
			>import from another app</a
		>, such as GitHub Issues, into {ws?.name ?? 'this workspace'}.
	</p>
</section>

<style>
	.export,
	.import {
		display: grid;
		gap: var(--space-3);
	}
	fieldset {
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
		align-items: flex-start;
		gap: var(--space-2);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.choice input {
		flex: none;
		width: max(var(--tap-min), 16px);
		height: max(var(--tap-min), 16px);
		margin: 2px 0 0;
		accent-color: var(--accent);
	}
	.file {
		display: grid;
		gap: var(--space-1);
		font-size: var(--text-sm);
	}
	.file span {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.file input {
		max-width: 100%;
		min-height: var(--tap-min);
	}
	.link {
		color: var(--accent-text);
		font-weight: 550;
	}
</style>
