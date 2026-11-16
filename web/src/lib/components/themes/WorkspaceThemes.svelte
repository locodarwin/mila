<script lang="ts">
	import { tick } from 'svelte';
	import { api, call } from '../../api/client';
	import { announce } from '../../announce.svelte';
	import { anchored } from '../../popover';
	import { preferences } from '../../preferences.svelte';
	import { canPublishThemes } from '../../roles';
	import { session } from '../../session.svelte';
	import { engine } from '../../sync/engine.svelte';
	import type { WorkspaceTheme } from '../../sync/entities';
	import { appearance } from '../../theme/appearance.svelte';
	import { floorsOf } from '../../theme/custom';
	import { LIMITS } from '../../theme/document';
	import { themes } from '../../theme/themes.svelte';
	import {
		duplicate,
		rename,
		suggest,
		unpublish,
		updateFrom,
		use
	} from '../../theme/workspaceActions';
	import {
		MAX_WORKSPACE_THEMES,
		suggestion,
		workspaceDoc
	} from '../../theme/workspaceThemes.svelte';
	import { toasts } from '../../toasts.svelte';
	import Button from '../Button.svelte';
	import PublishTheme from './PublishTheme.svelte';
	import ThemeCard from './ThemeCard.svelte';
	import { BASE_NAMES } from './names';

	// Settings, Workspace themes: the themes the workspace picked at the top
	// of Settings offers. Everyone uses or duplicates them; owners and admins
	// also publish one of their own, suggest one, update one from their
	// own theme, rename and unpublish.

	let { ws }: { ws: string } = $props();

	const workspace = $derived(session.workspaces.find((w) => w.id === ws));
	const admin = $derived(!!workspace && canPublishThemes(workspace.role));
	const list = $derived(engine.replica(ws)?.workspaceThemes() ?? []);
	const suggested = $derived(suggestion(list)?.id ?? null);
	const inUse = $derived(preferences.current.customTheme);
	const hue = $derived(appearance.current.accentHue);
	const scheme = $derived(appearance.scheme);
	const full = $derived(list.length >= MAX_WORKSPACE_THEMES);

	const docs = $derived(Object.fromEntries(list.map((t) => [t.id, workspaceDoc(t)])));
	// Whether each can be used by this person, on their accent.
	const floors = $derived(
		Object.fromEntries(
			list.map((t) => {
				const doc = docs[t.id];
				return [t.id, doc ? floorsOf(doc, { accentHue: hue }) : 'This theme cannot be read.'];
			})
		)
	);

	// How many use each, for admins; nothing while offline.
	let counts = $state<Record<string, number>>({});
	$effect(() => {
		if (!admin) return;
		const ids = list.map((t) => t.id).join();
		void ids;
		void inUse;
		let stale = false;
		call(api.GET('/workspaces/{workspaceId}/theme-use', { params: { path: { workspaceId: ws } } }))
			.then((r) => {
				if (!stale) counts = r.counts;
			})
			.catch(() => {});
		return () => {
			stale = true;
		};
	});

	const using = (n: number) => (n === 1 ? '1 person using it' : `${n} people using it`);

	function useIt(t: WorkspaceTheme) {
		const why = use(ws, t);
		if (why) toasts.error(why);
		else announce(`${t.name} is in use`);
	}

	function duplicateIt(t: WorkspaceTheme) {
		const doc = docs[t.id];
		if (!doc) return toasts.error('That theme cannot be read.');
		const copy = duplicate(doc);
		if (typeof copy === 'string') return toasts.error(copy);
		toasts.show(`Made ${copy.name}, one of your themes`);
	}

	async function suggestIt(t: WorkspaceTheme) {
		const on = suggested !== t.id;
		await suggest(ws, t, on);
		announce(on ? `Suggested ${t.name} to everyone` : `Stopped suggesting ${t.name}`);
	}

	async function updateIt(t: WorkspaceTheme) {
		const own = t.sourceId ? themes.get(t.sourceId) : undefined;
		if (!own) return;
		const why = await updateFrom(ws, t, own);
		if (why) toasts.error(why);
		else toasts.show(`Updated ${t.name} from ${own.name} for everyone using it`);
	}

	function unpublishIt(t: WorkspaceTheme) {
		unpublish(ws, t);
		document.getElementById('ws-themes-heading')?.focus();
	}

	// Renaming in place, as with the person's own themes.
	let renaming = $state<string | null>(null);
	let draft = $state('');

	async function startRename(t: WorkspaceTheme) {
		renaming = t.id;
		draft = t.name;
		await tick();
		document.getElementById(`ws-rename-${t.id}`)?.focus();
	}

	async function finishRename(t: WorkspaceTheme) {
		renaming = null;
		if (draft.trim() !== t.name) {
			const why = await rename(ws, t, draft);
			if (why) toasts.error(why);
			else announce(`Renamed to ${draft.trim()}`);
		}
		void tick().then(() => document.getElementById(`ws-more-${t.id}`)?.focus());
	}

	/** An admin's other actions on a theme; Update only while they have the theme it came from. */
	function menuFor(t: WorkspaceTheme, sourceName: string | undefined): [string, () => void][] {
		return [
			['Duplicate', () => duplicateIt(t)],
			...(sourceName
				? ([[`Update from ${sourceName}`, () => void updateIt(t)]] as [string, () => void][])
				: []),
			['Rename', () => void startRename(t)],
			['Unpublish', () => unpublishIt(t)]
		];
	}

	let publishing = $state(false);
</script>

<section aria-labelledby="ws-themes-heading">
	<h2 id="ws-themes-heading" tabindex="-1">Workspace themes in {workspace?.name ?? ''}</h2>
	<p class="hint">
		{#if admin}
			Themes {workspace?.name} offers its members and guests. Nobody is switched to one; they choose it
			if they like it.
		{:else}
			Themes {workspace?.name} offers. Use one, or duplicate it to make it your own.
		{/if}
	</p>
	{#if list.length}
		<ul class="rows" aria-label="Workspace themes">
			{#each list as t (t.id)}
				{@const doc = docs[t.id]}
				{@const why = floors[t.id]}
				{@const source = t.sourceId ? themes.get(t.sourceId) : undefined}
				<li>
					<span class="card"
						><ThemeCard base={doc?.base ?? 'standard'} {doc} {scheme} accentHue={hue} /></span
					>
					<span class="grow">
						{#if renaming === t.id}
							<input
								id="ws-rename-{t.id}"
								class="rename"
								aria-label="Name of {t.name}"
								maxlength={LIMITS.name}
								bind:value={draft}
								onkeydown={(e) => {
									if (e.key === 'Enter') {
										e.preventDefault();
										void finishRename(t);
									} else if (e.key === 'Escape') {
										e.preventDefault();
										renaming = null;
										void tick().then(() => document.getElementById(`ws-more-${t.id}`)?.focus());
									}
								}}
								onblur={() => renaming === t.id && void finishRename(t)}
							/>
						{:else}
							<span class="name">{t.name}</span>
						{/if}
						<span class="sub">
							{#if doc}Based on {BASE_NAMES[doc.base]}{/if}{#if admin && t.publishedBy}, published
								by {session.nameOf(
									ws,
									t.publishedBy
								)}{/if}{#if admin && counts[t.id] !== undefined},
								{using(counts[t.id])}{/if}{#if suggested === t.id}<span class="badge"
									>Suggested</span
								>{/if}{#if inUse === t.id}<span class="badge">In use</span>{/if}
						</span>
						{#if why}<span class="sub warn" id="ws-floors-{t.id}">{why}</span>{/if}
					</span>
					<span class="actions">
						<Button
							size="sm"
							variant={inUse === t.id ? 'ghost' : 'secondary'}
							disabled={inUse === t.id || !!why}
							aria-describedby={why ? `ws-floors-${t.id}` : undefined}
							aria-label={inUse === t.id ? `${t.name} is in use` : `Use ${t.name}`}
							onclick={() => useIt(t)}>{inUse === t.id ? 'In use' : 'Use'}</Button
						>
						{#if admin}
							<Button
								size="sm"
								aria-label={suggested === t.id
									? `Stop suggesting ${t.name}`
									: `Suggest ${t.name} to everyone`}
								onclick={() => void suggestIt(t)}
								>{suggested === t.id ? 'Stop suggesting' : 'Suggest'}</Button
							>
							<button
								type="button"
								class="more"
								id="ws-more-{t.id}"
								popovertarget="ws-menu-{t.id}"
								aria-label="More for {t.name}">More</button
							>
							<div class="menu" popover="auto" id="ws-menu-{t.id}" use:anchored>
								{#each menuFor(t, source?.name) as [label, run] (label)}
									<button
										type="button"
										class="option"
										class:danger={label === 'Unpublish'}
										onclick={(e) => {
											(e.currentTarget.closest('[popover]') as HTMLElement | null)?.hidePopover();
											run();
										}}>{label}</button
									>
								{/each}
							</div>
						{:else}
							<Button size="sm" aria-label="Duplicate {t.name}" onclick={() => duplicateIt(t)}
								>Duplicate</Button
							>
						{/if}
					</span>
				</li>
			{/each}
		</ul>
	{:else}
		<p class="hint">
			{admin
				? 'No workspace themes yet. Publish one of yours to offer it to everyone here.'
				: 'No workspace themes yet.'}
		</p>
	{/if}
	{#if admin}
		<div class="inline-form">
			<Button
				variant="primary"
				disabled={full || !themes.list.length}
				onclick={() => (publishing = true)}>Publish one of my themes</Button
			>
			<span class="hint">{list.length} of {MAX_WORKSPACE_THEMES}</span>
		</div>
		{#if !themes.list.length}
			<p class="hint">Make a theme of your own below first; then publish it here.</p>
		{:else if full}
			<p class="hint">
				A workspace offers {MAX_WORKSPACE_THEMES} themes at most. Unpublish one to publish another.
			</p>
		{/if}
	{/if}
</section>

{#if publishing && workspace}
	<PublishTheme
		{ws}
		wsName={workspace.name}
		taken={list.map((t) => t.name)}
		onclose={() => (publishing = false)}
	/>
{/if}

<style>
	section {
		position: relative;
	}
	h2:focus-visible {
		outline: none;
		box-shadow: none;
	}
	.card {
		flex: none;
		width: 88px;
	}
	.name {
		font-weight: 550;
		overflow-wrap: anywhere;
	}
	.sub {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
	.warn {
		color: var(--danger);
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
	.more {
		display: inline-flex;
		align-items: center;
		height: calc(26px * var(--control-scale));
		min-width: var(--tap-min);
		justify-content: center;
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--button-secondary-border);
		border-radius: var(--button-radius);
		background: var(--button-secondary-bg);
		color: var(--button-secondary-text);
		font-size: var(--text-xs);
		font-weight: 550;
		cursor: pointer;
	}
	.more:hover {
		background: var(--button-ghost-hover);
	}
	.menu .option {
		width: 100%;
		min-height: max(var(--tap-min), calc(30px * var(--control-scale)));
		border: none;
		background: none;
		color: var(--text-primary);
		font: inherit;
		text-align: left;
	}
	.menu .option.danger {
		color: var(--danger);
	}
	.rename {
		width: 100%;
		max-width: 320px;
		height: calc(30px * var(--control-scale));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
	}
</style>
