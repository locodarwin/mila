<script lang="ts">
	import { focusOnShow } from '../focus';
	import Check from '@lucide/svelte/icons/check';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import Plus from '@lucide/svelte/icons/plus';
	import { api, call, messageOf } from '../api/client';
	import { layout } from '../layout.svelte';
	import { session } from '../session.svelte';
	import { toasts } from '../toasts.svelte';

	let menu = $state<HTMLElement>();
	let creating = $state(false);
	let name = $state('');

	const initial = $derived((session.workspace?.name ?? '?').slice(0, 1).toUpperCase());

	function pick(id: string) {
		session.select(id);
		menu?.hidePopover();
	}

	async function create(e: SubmitEvent) {
		e.preventDefault();
		try {
			const ws = await call(api.POST('/workspaces', { body: { name } }));
			await session.loadWorkspaces();
			session.select(ws.id);
			name = '';
			creating = false;
			menu?.hidePopover();
		} catch (err) {
			toasts.error(messageOf(err));
		}
	}
</script>

<button
	class="trigger"
	type="button"
	popovertarget="workspace-menu"
	aria-label="Switch workspace: {session.workspace?.name ?? ''}"
	title={layout.collapsed ? session.workspace?.name : undefined}
>
	<span class="avatar" aria-hidden="true">{initial}</span>
	<span class="name">{session.workspace?.name ?? 'Workspace'}</span>
	<ChevronDown size={14} />
</button>

<div id="workspace-menu" class="menu" popover bind:this={menu}>
	<p class="heading">Workspaces</p>
	<ul>
		{#each session.workspaces as ws (ws.id)}
			<li>
				<button
					type="button"
					onclick={() => pick(ws.id)}
					aria-current={ws.id === session.workspace?.id}
				>
					<span class="avatar small" aria-hidden="true">{ws.name.slice(0, 1).toUpperCase()}</span>
					<span class="label">{ws.name}</span>
					{#if ws.id === session.workspace?.id}<Check size={14} />{/if}
				</button>
			</li>
		{/each}
	</ul>
	{#if creating}
		<form onsubmit={create}>
			<input
				aria-label="New workspace name"
				placeholder="Workspace name"
				bind:value={name}
				use:focusOnShow
				required
			/>
		</form>
	{:else}
		<button type="button" class="new" onclick={() => (creating = true)}>
			<Plus size={14} />New workspace
		</button>
	{/if}
</div>

<style>
	.trigger {
		display: flex;
		flex: 1;
		align-items: center;
		gap: var(--space-2);
		min-width: 0;
		padding: var(--space-1) var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		font-weight: 600;
		cursor: pointer;
		anchor-name: --workspace-trigger;
	}
	.trigger:hover {
		background: var(--surface-3);
	}
	.name {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.avatar {
		display: grid;
		flex: none;
		place-items: center;
		width: 22px;
		height: 22px;
		border-radius: var(--radius-sm);
		background: var(--accent);
		color: var(--accent-contrast);
		font-size: var(--text-xs);
	}
	.avatar.small {
		width: 18px;
		height: 18px;
		background: var(--surface-3);
		color: var(--text-primary);
	}
	/* Under the trigger, which always sits at the sidebar's top left. */
	.menu {
		position: fixed;
		inset: auto;
		top: 44px;
		left: var(--space-2);
		width: 240px;
		margin: var(--space-1) 0 0;
		padding: var(--space-2);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface-0);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	@supports (anchor-name: --a) {
		.menu {
			position-anchor: --workspace-trigger;
			top: anchor(bottom);
			left: anchor(left);
		}
	}
	.heading {
		padding: var(--space-1) var(--space-2);
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-muted);
	}
	ul {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.menu button {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		width: 100%;
		padding: var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		text-align: left;
		cursor: pointer;
	}
	.menu button:hover {
		background: var(--surface-2);
	}
	.label {
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.new {
		color: var(--text-secondary);
	}
	form input {
		width: 100%;
		height: calc(32px * var(--control-scale));
		margin-top: var(--space-1);
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--accent);
		border-radius: var(--radius-md);
		background: var(--surface-0);
	}
</style>
