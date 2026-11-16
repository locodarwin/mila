<script lang="ts">
	// New project from a template (docs/PLAN.md section 3): which one, its
	// name and its start date, with where the dated tasks will land.
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import Announcer from '../Announcer.svelte';
	import Button from '../Button.svelte';
	import { formatDate } from '../../dates';
	import { localToday, setProjectDeleted } from '../../sync/data';
	import { makeProjectFromTemplate } from '../../templateActions';
	import { engine } from '../../sync/engine.svelte';
	import { contents, lastDay } from '../../templateInfo';
	import { toasts } from '../../toasts.svelte';
	import { uuid } from '../../uuid';

	interface Props {
		open: boolean;
		ws: string;
		/** The template chosen as it opens, or the first. */
		templateId?: string | null;
	}

	let { open = $bindable(), ws, templateId = null }: Props = $props();
	const id = `new-from-${uuid()}`;
	let dialog = $state<HTMLDialogElement>();
	let chosen = $state('');
	let name = $state('');
	let start = $state(localToday());
	let making = $state(false);

	const templates = $derived(
		(engine.replica(ws)?.templates() ?? []).filter((t) => t.kind === 'project')
	);
	const template = $derived(templates.find((t) => t.id === chosen));

	$effect(() => {
		if (open && dialog && !dialog.open) {
			chosen = (templates.some((t) => t.id === templateId) ? templateId : templates[0]?.id) ?? '';
			name = templates.find((t) => t.id === chosen)?.name ?? '';
			start = localToday();
			dialog.showModal();
		} else if (!open && dialog?.open) dialog.close();
	});

	function pick(t: { id: string; name: string }) {
		chosen = t.id;
		name = t.name;
	}

	const addDays = (iso: string, n: number) => {
		const [y, m, d] = iso.split('-').map(Number);
		return new Date(Date.UTC(y, m - 1, d + n)).toISOString().slice(0, 10);
	};
	const span = $derived.by(() => {
		if (!template || !start) return '';
		const dated = template.data.tasks.filter((t) => t.start !== null || t.due !== null).length;
		const loose = template.data.tasks.length - dated;
		const last = lastDay(template.data);
		const rest = loose ? `; ${loose} ${loose === 1 ? 'has' : 'have'} no dates` : '';
		if (!dated || last === null) return `No task has a date${rest}.`;
		return `${dated} dated ${dated === 1 ? 'task runs' : 'tasks run'} from ${formatDate(start)} to ${formatDate(addDays(start, last))}${rest}.`;
	});

	async function make(e: SubmitEvent) {
		e.preventDefault();
		if (!template || !name.trim() || !start) return;
		making = true;
		const made = await makeProjectFromTemplate(ws, template.id, { name, start });
		making = false;
		open = false;
		const left = made.dropped
			? `; ${made.dropped} ${made.dropped === 1 ? 'assignee who left was' : 'assignees who left were'} left off`
			: '';
		toasts.show(`Made ${name.trim()} from ${template.name}${left}`, {
			onundo: () => void setProjectDeleted(ws, made.id, true)
		});
		await goto(resolve('/(app)/projects/[id]', { id: made.id }));
	}
</script>

<dialog bind:this={dialog} class="new" aria-labelledby="{id}-title" onclose={() => (open = false)}>
	<form onsubmit={make}>
		<h2 id="{id}-title">New project from a template</h2>
		{#if templates.length}
			<fieldset>
				<legend class="visually-hidden">Template</legend>
				{#each templates as t (t.id)}
					<label class="choice" class:on={t.id === chosen}>
						<input
							type="radio"
							name="{id}-template"
							value={t.id}
							checked={t.id === chosen}
							onchange={() => pick(t)}
						/>
						<span class="what"
							><span class="name">{t.name}</span><span class="meta">{contents(t)}</span></span
						>
					</label>
				{/each}
			</fieldset>
			<label class="field">
				<span>Project name</span>
				<input bind:value={name} maxlength="200" required />
			</label>
			<label class="field">
				<span>Start date</span>
				<input type="date" bind:value={start} required />
			</label>
			<p class="span">{span}</p>
		{:else}
			<p class="span">
				There are no project templates yet. Save one from a project's menu: Save as a template.
			</p>
		{/if}
		<div class="foot">
			<Button onclick={() => (open = false)}>Cancel</Button>
			{#if templates.length}
				<Button type="submit" variant="primary" disabled={making || !name.trim() || !start}
					>Make {name.trim() || 'project'}</Button
				>
			{/if}
		</div>
	</form>
	{#if open}<Announcer inDialog />{/if}
</dialog>

<style>
	.new {
		width: min(480px, calc(100vw - 32px));
		max-height: 85vh;
		padding: var(--space-5);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	.new::backdrop {
		/* With the value as written, for browsers whose ::backdrop does not inherit custom properties. */
		background: var(--scrim, oklch(0 0 0 / 0.25));
	}
	form {
		display: grid;
		gap: var(--space-3);
	}
	h2 {
		font-size: var(--text-xl);
	}
	fieldset {
		display: grid;
		gap: var(--space-2);
		max-height: 40vh;
		margin: 0;
		padding: 0;
		overflow-y: auto;
		border: none;
	}
	.choice {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		min-height: max(var(--tap-min), 44px);
		padding: var(--space-2) var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		cursor: pointer;
	}
	.choice.on {
		border-color: var(--accent);
		background: var(--accent-soft);
	}
	.what {
		display: grid;
	}
	.name {
		font-weight: 600;
	}
	.meta {
		color: var(--text-secondary);
		font-size: var(--text-sm);
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
	.field input {
		min-height: max(var(--tap-min), 32px);
		padding: var(--space-1) var(--space-2);
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-sm);
		background: var(--surface-0);
	}
	.span {
		margin: 0;
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	.foot {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-2);
	}
</style>
