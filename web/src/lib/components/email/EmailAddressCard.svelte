<script lang="ts">
	// One of a person's addresses for email to task: the
	// address to copy, where its tasks go, and who it accepts mail from.
	import X from '@lucide/svelte/icons/x';
	import type { Schemas } from '../../api/client';
	import { copyText } from '../../clipboard';
	import { toasts } from '../../toasts.svelte';
	import Button from '../Button.svelte';

	type Address = Schemas['EmailAddress'];

	interface Props {
		address: Address;
		/** The projects its tasks can go to. */
		projects: { id: string; name: string }[];
		/** The person's sign-in address, always accepted. */
		own: string;
		onsave: (changes: { projectId: string | null; assignToMe: boolean; senders: string[] }) => void;
		onreplace: () => void;
		ondelete: () => void;
	}

	let { address, projects, own, onsave, onreplace, ondelete }: Props = $props();
	const uid = $props.id();
	let adding = $state('');
	let confirming = $state<'replace' | 'delete' | null>(null);

	const place = $derived(address.projectId ?? '');
	const current = () => ({
		projectId: address.projectId ?? null,
		assignToMe: address.assignToMe,
		senders: address.senders
	});

	async function copy() {
		await copyText(address.address);
		toasts.show('Address copied');
	}

	function addSender(e: SubmitEvent) {
		e.preventDefault();
		const sender = adding.trim();
		if (!sender) return;
		onsave({ ...current(), senders: [...address.senders, sender] });
		adding = '';
	}

	function confirmed(action: 'replace' | 'delete') {
		if (confirming !== action) {
			confirming = action;
			return;
		}
		confirming = null;
		if (action === 'replace') onreplace();
		else ondelete();
	}
</script>

<article
	class="card"
	aria-label={`Address going to ${place ? (projects.find((p) => p.id === place)?.name ?? 'a project') : 'the Inbox'}`}
>
	<div class="top">
		<code class="address">{address.address}</code>
		<Button size="sm" title="Copy this address" onclick={copy}>Copy</Button>
		<Button
			size="sm"
			title="Give this a new address. Mail to the old one is refused from then on."
			onclick={() => confirmed('replace')}
			>{confirming === 'replace' ? 'Confirm new address' : 'Make a new one'}</Button
		>
		<Button
			size="sm"
			variant="danger"
			title="Stop this address working, at once"
			onclick={() => confirmed('delete')}
			>{confirming === 'delete' ? 'Confirm delete' : 'Delete'}</Button
		>
	</div>
	<div class="fields">
		<div class="field">
			<label for="{uid}-place">Tasks go to</label>
			<select
				id="{uid}-place"
				value={place}
				onchange={(e) => onsave({ ...current(), projectId: e.currentTarget.value || null })}
			>
				<option value="">Inbox</option>
				{#each projects as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
			</select>
			<label class="check">
				<input
					type="checkbox"
					checked={address.assignToMe}
					onchange={(e) => onsave({ ...current(), assignToMe: e.currentTarget.checked })}
				/>
				Assign them to me
			</label>
		</div>
		<div class="field">
			<span class="label" id="{uid}-senders">Accept mail from</span>
			<ul class="chips" aria-labelledby="{uid}-senders">
				<li class="chip" title="Your sign-in address, always accepted">{own}</li>
				{#each address.senders as s (s)}
					<li class="chip">
						{s}
						<button
							type="button"
							aria-label={`Stop accepting mail from ${s}`}
							title={`Stop accepting mail from ${s}`}
							onclick={() =>
								onsave({ ...current(), senders: address.senders.filter((x) => x !== s) })}
							><X size={12} /></button
						>
					</li>
				{/each}
			</ul>
			<form class="add" onsubmit={addSender}>
				<input
					type="email"
					aria-label="Another address to accept mail from"
					placeholder="Add an address"
					bind:value={adding}
				/>
				<Button
					size="sm"
					type="submit"
					title="Accept mail from this address too"
					disabled={!adding.trim()}>Add</Button
				>
			</form>
		</div>
	</div>
</article>

<style>
	.card {
		display: grid;
		gap: var(--space-3);
		padding: var(--space-3) var(--space-4);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
	}
	.top {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
	.address {
		flex: 1;
		min-width: 14rem;
		padding: var(--space-1) var(--space-2);
		border-radius: var(--radius-md);
		background: var(--surface-2);
		font-family: var(--font-mono);
		font-size: var(--text-sm);
		overflow-wrap: anywhere;
	}
	.fields {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
		gap: var(--space-3) var(--space-5);
	}
	.field {
		display: grid;
		align-content: start;
		gap: var(--space-1);
		min-width: 0;
		font-size: var(--text-sm);
	}
	.field > label:not(.check),
	.label {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	select,
	.add input {
		width: 100%;
		min-width: 0;
		height: calc(34px * var(--control-scale));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-md);
		background: var(--surface-0);
	}
	.check {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: max(var(--tap-min), 28px);
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1);
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.chip {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-width: 0;
		padding: 2px var(--space-2);
		border-radius: var(--radius-lg);
		background: var(--surface-2);
		font-size: var(--text-xs);
		overflow-wrap: anywhere;
	}
	.chip button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: max(var(--tap-min), 18px);
		min-height: max(var(--tap-min), 18px);
		padding: 0;
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.chip button:hover {
		background: var(--surface-3);
		color: var(--text-primary);
	}
	.add {
		display: flex;
		gap: var(--space-2);
		margin-top: var(--space-1);
	}
</style>
