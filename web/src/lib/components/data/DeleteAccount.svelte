<script lang="ts">
	import { tick } from 'svelte';
	import Button from '../Button.svelte';
	import TextInput from '../TextInput.svelte';

	interface Props {
		/** The address to type; deleting is offered once it matches. */
		email: string;
		busy?: boolean;
		ondelete?: (typed: string) => void;
	}

	let { email, busy = false, ondelete }: Props = $props();
	let open = $state(false);
	let typed = $state('');
	let form = $state<HTMLFormElement>();
	const matches = $derived(typed.trim().toLowerCase() === email.toLowerCase());

	async function start() {
		open = true;
		await tick();
		form?.querySelector('input')?.focus();
	}

	function submit(e: SubmitEvent) {
		e.preventDefault();
		if (matches && !busy) ondelete?.(typed.trim());
	}
</script>

<div class="danger">
	<p>
		Deleting your account removes your personal workspace and every shared workspace where you are
		the only member, with everything in them. In workspaces you share, what you wrote stays,
		credited to "Deleted user", and you are taken off the tasks assigned to you. It cannot be
		undone; backups the server keeps may hold your data until they expire.
	</p>
	{#if !open}
		<div><Button variant="danger" onclick={start}>Delete my account…</Button></div>
	{:else}
		<form class="confirm" bind:this={form} onsubmit={submit}>
			<TextInput
				label="Type your email address, {email}, to confirm"
				autocomplete="off"
				spellcheck={false}
				bind:value={typed}
			/>
			<div class="buttons">
				<Button type="submit" variant="danger" disabled={!matches || busy}>
					Delete my account for good
				</Button>
				<Button variant="ghost" onclick={() => ((open = false), (typed = ''))}>Keep it</Button>
			</div>
		</form>
	{/if}
</div>

<style>
	.danger {
		display: grid;
		gap: var(--space-3);
		padding: var(--space-4);
		border: var(--border-width) solid var(--danger);
		border-radius: var(--radius-lg);
	}
	p {
		margin: 0;
		font-size: var(--text-sm);
	}
	.confirm {
		display: grid;
		gap: var(--space-3);
		max-width: 28rem;
	}
	.buttons {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
</style>
