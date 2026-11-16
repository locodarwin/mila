<script lang="ts">
	// Save as a template (docs/PLAN.md section 3): a project, a task or a page, as
	// a new template or over one of the same kind, and what comes along.
	import Announcer from '../Announcer.svelte';
	import Button from '../Button.svelte';
	import { formatDate } from '../../dates';
	import { localToday } from '../../sync/data';
	import { saveTemplate } from '../../templateActions';
	import { engine } from '../../sync/engine.svelte';
	import { session } from '../../session.svelte';
	import type { TemplateKind } from '../../templates';
	import { toasts } from '../../toasts.svelte';
	import { uuid } from '../../uuid';

	interface Props {
		open: boolean;
		ws: string;
		kind: TemplateKind;
		sourceId: string;
		sourceName: string;
		/** A page's text as it stands in the editor, ahead of what the device has synced. */
		pageText?: () => string | null;
	}

	let { open = $bindable(), ws, kind, sourceId, sourceName, pageText }: Props = $props();
	const id = `save-tpl-${uuid()}`;
	let dialog = $state<HTMLDialogElement>();
	let mode = $state<'new' | 'replace'>('new');
	let replaceId = $state('');
	let name = $state('');
	let keepAssignees = $state(false);
	let subpages = $state(true);
	let under = $state(0);
	let error = $state('');
	let saving = $state(false);
	let summary = $state<{ comes: string; stays: string; dates: string } | null>(null);

	const same = $derived((engine.replica(ws)?.templates() ?? []).filter((t) => t.kind === kind));

	$effect(() => {
		if (open && dialog && !dialog.open) {
			name = sourceName;
			mode = 'new';
			replaceId = same.find((t) => t.name === sourceName)?.id ?? same[0]?.id ?? '';
			keepAssignees = false;
			subpages = true;
			under = 0;
			error = '';
			summary = null;
			dialog.showModal();
			void describe();
		} else if (!open && dialog?.open) dialog.close();
	});

	// What comes along, worked out from the device's copy as the dialog opens.
	async function describe() {
		const { snapshotProject, snapshotTask, summarize, summarizePage } =
			await import('../../templatePlan');
		const replica = engine.replica(ws);
		if (!replica) return;
		if (kind === 'page') {
			const s = summarizePage(replica, sourceId, subpages);
			under = s.under;
			summary = {
				comes: s.comes,
				stays: s.stays,
				dates: 'Write {{today}} in the page for the date it is used on.'
			};
			return;
		}
		const snap =
			kind === 'project'
				? snapshotProject(replica, sourceId, { keepAssignees: false, me: session.user?.id ?? '' })
				: snapshotTask(replica, sourceId, { keepAssignees: false, today: localToday() });
		if (!snap) return;
		const s = summarize(snap, kind);
		const root = snap.data.tasks[0];
		summary = {
			...s,
			dates:
				kind === 'project'
					? snap.anchor
						? `Dates are kept relative to ${formatDate(snap.anchor)}, the earliest date in the project.`
						: 'No task has a date yet.'
					: root?.due !== null && root?.due !== undefined
						? `Each use is due ${root.due === 0 ? 'the day it is made' : `${root.due} ${root.due === 1 ? 'day' : 'days'} after it is made`}.`
						: 'It has no due date.'
		};
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		if (!name.trim()) return;
		saving = true;
		const res = await saveTemplate(ws, {
			kind,
			sourceId,
			name,
			keepAssignees,
			subpages,
			body: pageText?.() ?? undefined,
			replace: mode === 'replace' ? replaceId || null : null
		});
		saving = false;
		if ('error' in res) {
			error = res.error;
			return;
		}
		open = false;
		toasts.show(
			mode === 'replace'
				? `Replaced the template ${name.trim()}`
				: `Saved ${name.trim()} as a template`
		);
	}
</script>

<dialog bind:this={dialog} class="save" aria-labelledby="{id}-title" onclose={() => (open = false)}>
	<form onsubmit={save}>
		<h2 id="{id}-title">Save {sourceName} as a template</h2>
		{#if same.length}
			<fieldset>
				<legend class="visually-hidden">Save as</legend>
				<label class="radio"
					><input type="radio" name="{id}-mode" value="new" bind:group={mode} /> A new template</label
				>
				<label class="radio"
					><input type="radio" name="{id}-mode" value="replace" bind:group={mode} /> Replace a template</label
				>
				<select
					aria-label="Template to replace"
					bind:value={replaceId}
					disabled={mode !== 'replace'}
				>
					{#each same as t (t.id)}<option value={t.id}>{t.name}</option>{/each}
				</select>
			</fieldset>
		{/if}
		<label class="field">
			<span>Template name</span>
			<input bind:value={name} maxlength="120" required />
		</label>
		{#if kind === 'page'}
			{#if under}
				<label class="radio"
					><input type="checkbox" bind:checked={subpages} onchange={() => void describe()} />
					Include
					{under === 1 ? 'its subpage' : `its ${under} subpages`}</label
				>
			{/if}
		{:else}
			<label class="radio"
				><input type="checkbox" bind:checked={keepAssignees} /> Keep assignees</label
			>
		{/if}
		{#if summary}
			<div class="keeps">
				<p><strong>Comes along:</strong> {summary.comes}.</p>
				<p><strong>Stays behind:</strong> {summary.stays}.</p>
				<p>{summary.dates}</p>
			</div>
		{/if}
		{#if error}<p class="error" role="alert">{error}</p>{/if}
		<div class="foot">
			<Button onclick={() => (open = false)}>Cancel</Button>
			<Button type="submit" variant="primary" disabled={saving || !name.trim()}
				>Save template</Button
			>
		</div>
	</form>
	{#if open}<Announcer inDialog />{/if}
</dialog>

<style>
	.save {
		width: min(480px, calc(100vw - 32px));
		max-height: 85vh;
		padding: var(--space-5);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	.save::backdrop {
		/* With the value as written, for browsers whose ::backdrop does not inherit custom properties. */
		background: var(--scrim, oklch(0 0 0 / 0.25));
	}
	form {
		display: grid;
		gap: var(--space-3);
	}
	h2 {
		font-size: var(--text-xl);
		overflow-wrap: anywhere;
	}
	fieldset {
		display: grid;
		gap: var(--space-2);
		margin: 0;
		padding: 0;
		border: none;
	}
	.radio {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: max(var(--tap-min), 28px);
	}
	select,
	input:not([type]) {
		min-height: max(var(--tap-min), 32px);
		padding: var(--space-1) var(--space-2);
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-sm);
		background: var(--surface-0);
	}
	select {
		margin-left: var(--space-6);
	}
	.field {
		display: grid;
		gap: var(--space-1);
	}
	.field span {
		color: var(--text-secondary);
		font-size: var(--text-sm);
		font-weight: 600;
	}
	.keeps {
		display: grid;
		gap: var(--space-1);
		padding: var(--space-3);
		border-radius: var(--radius-md);
		background: var(--surface-1);
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	.keeps p {
		margin: 0;
	}
	.keeps strong {
		color: var(--text-primary);
	}
	.error {
		margin: 0;
		color: var(--danger);
		font-size: var(--text-sm);
	}
	.foot {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-2);
	}
</style>
