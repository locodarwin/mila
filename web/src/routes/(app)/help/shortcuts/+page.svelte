<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import Kbd from '#lib/components/Kbd.svelte';
	import ShortcutTable from '#lib/components/keys/ShortcutTable.svelte';
	import TextInput from '#lib/components/TextInput.svelte';
	import { announce } from '#lib/announce.svelte.js';
	import { COMMANDS, command, type CommandId } from '#lib/keys/commands.js';
	import { keymap } from '#lib/keys/keymap.svelte.js';
	import { preferences } from '#lib/preferences.svelte.js';
	import { score } from '#lib/search.js';

	let query = $state('');
	const shown = $derived.by(() => {
		const q = query.trim().toLowerCase();
		return COMMANDS.filter((c) => score(c.name, q) > 0).length;
	});
	$effect(() => {
		if (query.trim())
			announce(
				shown ? `${shown} ${shown === 1 ? 'shortcut' : 'shortcuts'}` : 'No shortcut matches',
				500
			);
	});

	// A link to one scope's table (#keys-list) gives it focus there.
	afterNavigate(({ to }) => {
		const id = to?.url.hash.slice(1);
		const table = id ? document.getElementById(id) : null;
		if (!(table instanceof HTMLElement)) return;
		table.scrollIntoView({ block: 'start' });
		table.focus({ preventScroll: true });
	});
</script>

<svelte:head><title>Keyboard shortcuts · Mila</title></svelte:head>

<!-- A command's default keys (or one of them), not the person's: marked
     Off while single-key shortcuts are off, so none is offered that does
     nothing. -->
{#snippet defaults(id: CommandId, only?: number)}
	{@const keys = command(id).keys.filter((_, i) => only === undefined || i === only)}
	{#each keys as b, i (b)}{#if i}<span class="or">or</span>{/if}{#if keymap.off(id, b)}<span
				class="off">Off</span
			>{:else}<Kbd binding={b} />{/if}{/each}
{/snippet}

<header class="page">
	<nav class="crumbs" aria-label="Breadcrumb">
		<a href={resolve('/(app)/help')}>Help</a><span class="sep" aria-hidden="true">/</span>
	</nav>
	<h1>Keyboard shortcuts</h1>
</header>
<div class="column">
	<p class="hint">
		These are your keys, as you have them. You can give a command other keys, more keys or none,
		except the few shown with a lock.
	</p>
	{#if !preferences.current.singleKeyShortcuts}
		<p class="banner">
			Single-key shortcuts are off, so the keys marked Off do nothing for now. Turn them back on in
			<a href={resolve('/(app)/settings/keyboard')}>Settings, Keyboard</a>.
		</p>
	{/if}
	<div class="tools">
		<div class="find">
			<TextInput label="Find a shortcut" type="search" autocomplete="off" bind:value={query} />
		</div>
		<a class="button" href={resolve('/(app)/settings/keyboard')}>Change shortcuts</a>
	</div>
	<ShortcutTable {query} />
	{#if !shown}<p class="hint">No shortcut matches.</p>{/if}
	<p class="hint">
		Keys that move between things, open, close and confirm always work and can't be changed. So are
		the keys that send a comment, move sidebar items, resize the sidebar and the task panel, move
		and zoom a picture you crop, and undo and redo in the theme editor. Settings, Keyboard shows
		them with a lock.
	</p>

	<section class="origins" aria-labelledby="origins">
		<h2 id="origins">Where the default keys come from</h2>
		<p class="hint">
			The keys Mila starts with, which yours may differ from, are ones people may know from other
			apps.
		</p>
		<ul>
			<li>{@render defaults('quick-add')} to add a task: Todoist's key for quick add.</li>
			<li>
				{@render defaults('go.inbox')} and the other Go to keys, {@render defaults('help', 0)} for this
				list and {@render defaults('search')} to search: Gmail and GitHub.
			</li>
			<li>{@render defaults('help', 1)} for this list: Slack and Google Docs.</li>
			<li>
				{@render defaults('task.next', 0)} and {@render defaults('task.prev', 0)} for the next and previous
				task: the vi editor, by way of Gmail.
			</li>
			<li>{@render defaults('palette')} for the command palette: Linear and Slack.</li>
			<li>
				{@render defaults('task.complete')} to complete: the key Gmail and Linear use to select a row,
				which Mila uses to complete one.
			</li>
			<li>{@render defaults('task.my-day')} for My Day: the first letter of Today.</li>
			<li>
				{@render defaults('task.priority.urgent')} to {@render defaults('task.priority.low')} for priority:
				Todoist's p1 to p4.
			</li>
		</ul>
	</section>
</div>

<style>
	.banner {
		padding: var(--space-3) var(--space-4);
		border-radius: var(--radius-md);
		background: var(--surface-2);
		font-size: var(--text-sm);
	}
	.banner a,
	.button {
		color: var(--accent-text);
		font-weight: 550;
	}
	.banner a {
		text-decoration: underline;
	}
	.tools {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		justify-content: space-between;
		gap: var(--space-3);
	}
	.find {
		flex: 1;
		max-width: 320px;
	}
	.find :global(input) {
		font-size: max(16px, var(--text-md));
	}
	.button {
		display: inline-flex;
		align-items: center;
		height: max(var(--tap-min), calc(32px * var(--control-scale)));
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		font-size: var(--text-sm);
	}
	.origins {
		display: grid;
		gap: var(--space-3);
		padding-top: var(--space-4);
		border-top: var(--border-width) solid var(--border);
	}
	.origins h2 {
		font-size: var(--text-lg);
	}
	.or,
	.off {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.off {
		padding: 0 var(--space-1);
		border: var(--border-width) dashed var(--border);
		border-radius: var(--radius-sm);
	}
	.origins ul {
		display: grid;
		gap: var(--space-2);
		margin: 0;
		padding-left: var(--space-5);
		line-height: 1.8;
	}
</style>
