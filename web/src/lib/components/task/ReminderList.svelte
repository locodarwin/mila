<script lang="ts">
	import Bell from '@lucide/svelte/icons/bell';
	import Plus from '@lucide/svelte/icons/plus';
	import X from '@lucide/svelte/icons/x';
	import Button from '../Button.svelte';
	import { describeReminder, firesAt, localMoment, presets, type Preset } from '../../reminders';
	import type { Reminder, Task } from '../../sync/entities';
	import { toasts } from '../../toasts.svelte';
	import { anchored } from '../../popover';
	import { uuid } from '../../uuid';

	interface Props {
		task: Pick<Task, 'dueDate' | 'dueTime'>;
		/** The viewer's reminders on the task; nobody else's are synced to them. */
		reminders: Reminder[];
		/** Adds one: minutes before the due moment, or at a fixed time. */
		onadd: (when: { offset: number } | { at: string }) => void;
		onremove: (id: string) => void;
	}

	let { task, reminders, onadd, onremove }: Props = $props();

	const choices = $derived(presets(task));
	const menuId = `reminders-${uuid()}`;
	let menu = $state<HTMLElement>();
	let trigger = $state<HTMLButtonElement>();
	let custom = $state(false);
	let when = $state('');
	// Focus follows the switch between the menu and the date and time
	// field: into the field when it opens, back to the button when it closes.
	let refocus = false;
	function setCustom(on: boolean) {
		refocus = !on;
		custom = on;
	}
	const focusNow = (el: HTMLElement) => el.focus();
	function focusIfBack(el: HTMLElement) {
		if (refocus) {
			refocus = false;
			el.focus();
		}
	}

	/** A local time as a datetime-local value: tomorrow at 9:00 to start. */
	function local(d: Date): string {
		const pad = (n: number) => String(n).padStart(2, '0');
		return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
	}

	// The choices are buttons, not a select: arrowing through a closed
	// select changes it, which here would add a reminder at every key.
	function pick(p: Preset | 'custom') {
		menu?.hidePopover();
		if (p === 'custom') {
			const t = new Date();
			when = local(new Date(t.getFullYear(), t.getMonth(), t.getDate() + 1, 9, 0));
			setCustom(true);
			return;
		}
		onadd(p.offset !== undefined ? { offset: p.offset } : { at: p.at! });
		trigger?.focus();
	}

	/** Into the menu when it opens; the arrow keys move through it. */
	function opened(e: ToggleEvent) {
		if (e.newState === 'open') menu?.querySelector('button')?.focus();
	}
	function arrows(e: KeyboardEvent) {
		if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
		e.preventDefault();
		const items = [...(menu?.querySelectorAll('button') ?? [])];
		const i = items.indexOf(document.activeElement as HTMLButtonElement);
		const step = e.key === 'ArrowDown' ? 1 : items.length - 1;
		items[(i + step) % items.length]?.focus();
	}

	function addCustom(e: SubmitEvent) {
		e.preventDefault();
		const [date, time] = when.split('T');
		if (!date || !time) return;
		const at = localMoment(date, time);
		if (at <= new Date()) {
			toasts.error('Pick a time that is still to come.');
			return;
		}
		onadd({ at: at.toISOString() });
		setCustom(false);
	}
</script>

<div class="reminders">
	{#if reminders.length}
		<ul aria-label="Reminders">
			{#each reminders as r (r.id)}
				{@const text = describeReminder(r, task)}
				<li class:past={firesAt(r, task) <= new Date()}>
					<Bell size={14} aria-hidden="true" />
					<span class="text">{text}</span>
					<button
						class="remove"
						type="button"
						aria-label={`Remove reminder: ${text}`}
						onclick={() => onremove(r.id)}><X size={14} /></button
					>
				</li>
			{/each}
		</ul>
	{/if}
	{#if custom}
		<form class="custom" onsubmit={addCustom}>
			<input
				type="datetime-local"
				aria-label="Reminder date and time"
				required
				bind:value={when}
				{@attach focusNow}
				onkeydown={(e) => {
					if (e.key === 'Escape') {
						// Closes the field, not the task.
						e.preventDefault();
						setCustom(false);
					}
				}}
			/>
			<Button variant="primary" size="sm" type="submit">Add reminder</Button>
			<Button variant="ghost" size="sm" onclick={() => setCustom(false)}>Cancel</Button>
		</form>
	{:else}
		<button
			class="add"
			type="button"
			popovertarget={menuId}
			bind:this={trigger}
			{@attach focusIfBack}><Plus size={14} aria-hidden="true" />Add a reminder</button
		>
	{/if}
</div>

<div id={menuId} popover class="menu" bind:this={menu} ontoggle={opened} use:anchored>
	<p class="heading">Remind me</p>
	<ul aria-label="Reminder times">
		{#each choices as c (c.id)}
			<li>
				<button type="button" class="option" onclick={() => pick(c)} onkeydown={arrows}
					>{c.label}</button
				>
			</li>
		{/each}
		<li>
			<button type="button" class="option" onclick={() => pick('custom')} onkeydown={arrows}
				>Pick a date and time…</button
			>
		</li>
	</ul>
</div>

<style>
	.reminders {
		display: grid;
		gap: var(--space-1);
		justify-items: start;
	}
	ul {
		display: grid;
		gap: 2px;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	li {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-primary);
	}
	li :global(svg) {
		flex: none;
		color: var(--text-secondary);
	}
	li.past .text {
		color: var(--text-secondary);
	}
	.remove {
		display: grid;
		place-items: center;
		width: 24px;
		height: 24px;
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.remove:hover {
		background: var(--surface-2);
		color: var(--danger);
	}
	.custom {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
		align-items: center;
	}
	input {
		height: 30px;
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		font-size: var(--text-sm);
	}
	input:hover {
		border-color: var(--border-strong);
	}
	.add {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		height: 30px;
		padding: 0 var(--space-2);
		border: var(--border-width) dashed var(--border-strong);
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.add:hover {
		border-color: var(--accent);
		color: var(--text-primary);
	}
	.menu ul {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.menu .option {
		width: 100%;
		min-height: max(var(--tap-min), 32px);
		border: none;
		background: none;
		color: inherit;
		font: inherit;
		font-size: var(--text-sm);
		text-align: left;
	}
</style>
