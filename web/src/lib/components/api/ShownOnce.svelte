<script lang="ts">
	// A secret the server gives once: a new API token, or a webhook's signing secret.
	import Copy from '@lucide/svelte/icons/copy';
	import { copyText } from '../../clipboard';
	import { uuid } from '../../uuid';
	import Button from '../Button.svelte';

	interface Props {
		/** What it is called, as "Your new token". */
		title: string;
		/** What it is for, and that it shows only now. */
		note: string;
		value: string;
		ondone: () => void;
	}

	let { title, note, value, ondone }: Props = $props();
	let input = $state<HTMLInputElement>();
	let status = $state('');
	const id = `once-${uuid()}`;

	/** Moves focus to it, whose description says it shows only now. */
	export function focus() {
		input?.focus();
	}

	async function copy() {
		if (await copyText(value, input)) status = 'Copied';
		else {
			input?.select();
			status = 'Copying did not work here. It is selected; copy it from the field.';
		}
	}
</script>

<div class="once" role="group" aria-labelledby="{id}-title">
	<p class="title" id="{id}-title">{title}</p>
	<p class="hint" id="{id}-note">{note}</p>
	<div class="row">
		<input
			bind:this={input}
			readonly
			{value}
			aria-label={title}
			aria-describedby="{id}-note"
			spellcheck="false"
			onfocus={(e) => e.currentTarget.select()}
		/>
		<Button title="Copy it" onclick={copy}><Copy size={15} />Copy</Button>
	</div>
	<p class="status" role="status">{status}</p>
	<div><Button variant="ghost" size="sm" title="Close this" onclick={ondone}>Done</Button></div>
</div>

<style>
	.once {
		display: grid;
		gap: var(--space-2);
		padding: var(--space-3) var(--space-4);
		border: var(--border-width) solid var(--accent);
		border-radius: var(--radius-md);
	}
	.title {
		font-weight: 600;
	}
	.row {
		display: flex;
		gap: var(--space-2);
	}
	input {
		flex: 1;
		min-width: 0;
		height: calc(32px * var(--control-scale));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-md);
		background: var(--surface-1);
		font-family: var(--font-mono);
		font-size: var(--text-sm);
	}
	.status {
		min-height: 1.2em;
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
</style>
