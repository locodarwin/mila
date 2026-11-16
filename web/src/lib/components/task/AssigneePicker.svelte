<script lang="ts">
	import Plus from '@lucide/svelte/icons/plus';
	import { session } from '../../session.svelte';
	import { setAssignee } from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import { anchored } from '../../popover';
	import { uuid } from '../../uuid';
	import Avatar from './Avatar.svelte';

	let { ws, taskId, assigned }: { ws: string; taskId: string; assigned: string[] } = $props();
	const id = `assignees-${uuid()}`;
	const on = $derived(new Set(assigned));
	// Who can reach the task, and anyone given it before they lost it, so
	// they can be taken off.
	const members = $derived.by(() => {
		const projectId = engine.replica(ws)?.task(taskId)?.projectId ?? null;
		const can = new Set(session.reachable(ws, projectId).map((m) => m.userId));
		return session.members(ws).filter((m) => can.has(m.userId) || on.has(m.userId));
	});
</script>

<div class="people">
	{#each assigned as userId (userId)}
		{@const who = session.personOf(ws, userId)}
		<span class="person"><Avatar name={who.name} src={who.avatar} />{who.name}</span>
	{/each}
	<button class="add" type="button" popovertarget={id} aria-label="Edit assignees"
		><Plus size={14} /></button
	>
</div>

<div {id} popover class="menu" use:anchored>
	<p class="heading">Assignees</p>
	{#each members as m (m.userId)}
		<label class="option">
			<input
				type="checkbox"
				checked={on.has(m.userId)}
				onchange={(e) => void setAssignee(ws, taskId, m.userId, e.currentTarget.checked)}
			/>
			<Avatar name={m.name} src={m.avatar} />{m.name}
		</label>
	{/each}
</div>

<style>
	.people {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
		align-items: center;
	}
	.person {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		font-size: var(--text-sm);
	}
	.add {
		display: grid;
		place-items: center;
		width: 24px;
		height: 24px;
		border: var(--border-width) dashed var(--border-strong);
		border-radius: var(--radius-full);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.add:hover {
		border-color: var(--accent);
		color: var(--text-primary);
	}
</style>
