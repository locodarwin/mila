<script lang="ts">
	import { page } from '$app/state';
	import { weekStart } from '../dates';
	import { parseQuickAdd } from '../quickadd';
	import { TOKEN_LABELS } from '../quickaddTokens';
	import { session } from '../session.svelte';
	import { canOrganize } from '../roles';
	import { quickAdd } from '../sync/data';
	import { engine } from '../sync/engine.svelte';
	import { toasts } from '../toasts.svelte';
	import { ui } from '../ui.svelte';
	import Announcer from './Announcer.svelte';
	import Button from './Button.svelte';
	import HelpLink from './help/HelpLink.svelte';
	import Kbd from './Kbd.svelte';
	import Token from './help/Token.svelte';

	let dialog = $state<HTMLDialogElement>();
	let text = $state('');
	// The style guide gives it a sample workspace; the app, the one in view.
	let { ws: sample }: { ws?: string } = $props();
	const ws = $derived(sample ?? session.workspace?.id ?? '');
	const replica = $derived(engine.replica(ws));

	// Where the task goes when the text names no project: the project on
	// screen (a project, or a saved view of one), else the inbox. Added
	// from My Day, it lands in My Day too.
	const here = $derived.by(() => {
		const path = page.url.pathname;
		const project = path.match(/^\/projects\/([^/]+)/)?.[1];
		if (project) return project;
		const view = path.match(/^\/views\/([^/]+)/)?.[1];
		const of = view ? replica?.view(view)?.projectId : null;
		return of && replica?.project(of) ? of : null;
	});
	const inMyDay = $derived(page.url.pathname === '/today');

	// Read twice: where the task goes decides who it can be given to.
	const parsed = $derived.by(() => {
		const read = (members: { userId: string; name: string }[]) =>
			parseQuickAdd(text, {
				today: new Date(),
				weekStart: weekStart(),
				projects: replica?.projects() ?? [],
				labels: replica?.labels() ?? [],
				members: members.map((m) => ({ id: m.userId, name: m.name }))
			});
		const where = read(session.members(ws)).projectId ?? here;
		return read(session.reachable(ws, where));
	});
	// A guest has no inbox: the task needs a project.
	const needsProject = $derived(
		!!session.workspace && !canOrganize(session.workspace.role) && !(parsed.projectId ?? here)
	);

	$effect(() => {
		if (ui.overlay === 'quickadd' && dialog && !dialog.open) {
			text = '';
			dialog.showModal();
			// autofocus is not enough when a key opened it: the key's target keeps focus.
			dialog.querySelector('input')?.focus();
		} else if (ui.overlay !== 'quickadd' && dialog?.open) {
			dialog.close();
			// With nothing to go back to (focus was on the page), focus stays in the
			// closed field until the next frame, and a key pressed then is lost to it.
			if (dialog.contains(document.activeElement)) (document.activeElement as HTMLElement).blur();
		}
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!parsed.title || !ws) return;
		if (needsProject) {
			toasts.error('Name a project with #, or open one first.');
			return;
		}
		const p = parsed;
		ui.close();
		try {
			await quickAdd(ws, p, { projectId: here, myDayFor: inMyDay ? session.user?.id : undefined });
			const where = p.projectId ?? here;
			toasts.show(
				`Added "${p.title}"${where ? ` to ${replica?.project(where)?.name ?? 'the project'}` : ' to the inbox'}`
			);
		} catch (err) {
			toasts.error(err instanceof Error ? err.message : 'Could not add the task.');
		}
	}
</script>

<!-- Escape frees the overlay at once (cancel). The close event comes later,
     at the next frame, and by then a key may have opened the dialog again:
     a close event on an open dialog is a stale one and changes nothing. -->
<dialog
	bind:this={dialog}
	class="quickadd"
	aria-label="Add a task"
	onclose={(e) => !e.currentTarget.open && ui.overlay === 'quickadd' && ui.close()}
	oncancel={() => ui.overlay === 'quickadd' && ui.close()}
>
	<form onsubmit={submit}>
		<!-- svelte-ignore a11y_autofocus -->
		<input
			aria-label="Quick add"
			placeholder="Call Dana tomorrow 3pm #project !high @person +label"
			bind:value={text}
			autofocus
			autocomplete="off"
		/>
		<ul class="chips" aria-live="polite">
			{#each parsed.tokens as t (t.start)}
				<li class={t.kind}><span>{TOKEN_LABELS[t.kind]}</span> {text.slice(t.start, t.end)}</li>
			{/each}
		</ul>
		<!-- The rules beside the field: following a link closes quick add,
		     and it opens empty again. -->
		<details class="syntax">
			<summary title="Show what you can type to set a date, labels and more">Syntax</summary>
			<ul>
				<li><Token kind="project" text="#work" /> a project, by the start of its name</li>
				<li><Token kind="priority" text="!high" /> or <code>!1</code> to <code>!4</code></li>
				<li><Token kind="person" text="@dana" /> and <Token kind="label" text="+phone" /></li>
				<li>
					<Token kind="date" text="fri" />
					<Token kind="time" text="3pm" /> or <code>tomorrow</code>,
					<code>nov 14</code>, <code>in 3 days</code>
				</li>
				<li>
					<Token kind="repeat" text="every mon" /> or <code>daily</code>, <code>every 2 weeks</code>
				</li>
				<li>
					<Token kind="reminder" text="remind 1h before" /> or <code>remind tomorrow 9am</code>
				</li>
			</ul>
			<HelpLink topic="quick-add" onclick={() => ui.close()}>More about quick add</HelpLink>
		</details>
		<div class="foot">
			<p class="hint">
				{#if needsProject}
					Name a project with #, or open one first.
				{:else}
					<Kbd command="quickadd.add" /> to add{here
						? ` to ${replica?.project(here)?.name}`
						: ' to the inbox'}{inMyDay ? ' and My Day' : ''}
				{/if}
				<span class="esc">· <Kbd command="panel.close" /> to cancel</span>
			</p>
			<!-- A touch screen has no Escape key. -->
			<span class="cancel"
				><Button variant="ghost" size="sm" onclick={() => ui.close()}>Cancel</Button></span
			>
		</div>
	</form>
	<!-- The page's live regions are inert behind a modal dialog. -->
	{#if ui.overlay === 'quickadd'}<Announcer inDialog />{/if}
</dialog>

<style>
	.quickadd {
		width: min(560px, calc(100vw - 32px));
		margin: 15vh auto auto;
		padding: 0;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	.quickadd::backdrop {
		/* With the value as written, for browsers whose ::backdrop does not inherit custom properties. */
		background: var(--scrim, oklch(0 0 0 / 0.25));
	}
	form {
		display: grid;
		gap: var(--space-2);
		padding: var(--space-3);
	}
	input {
		height: 44px;
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
		font-size: var(--text-lg);
	}
	input::placeholder {
		color: var(--input-placeholder);
	}
	/* A border and a ring inside it, two pixels of accent in all. */
	input:focus-visible {
		border-color: var(--accent);
		box-shadow: inset 0 0 0 1px var(--accent);
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1);
		min-height: 24px;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.chips li {
		padding: 2px var(--space-2);
		border-radius: var(--radius-full);
		background: var(--accent-soft);
		font-size: var(--text-xs);
		font-weight: 600;
	}
	.chips span {
		color: var(--text-secondary);
		font-weight: 500;
	}
	.syntax {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.syntax summary {
		width: fit-content;
		cursor: pointer;
	}
	.syntax ul {
		display: grid;
		gap: var(--space-1);
		margin: var(--space-2) 0;
		padding: 0;
		list-style: none;
	}
	.syntax li {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-1);
	}
	.syntax code {
		font-family: var(--font-mono);
		color: var(--text-primary);
	}
	.foot {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-2);
	}
	.hint {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.cancel {
		display: none;
	}
	@media (pointer: coarse) {
		.esc {
			display: none;
		}
		.cancel {
			display: block;
		}
	}
</style>
