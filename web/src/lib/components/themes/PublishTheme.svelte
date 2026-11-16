<script lang="ts">
	import { resolve } from '$app/paths';
	import { onMount, tick } from 'svelte';
	import Announcer from '../Announcer.svelte';
	import Button from '../Button.svelte';
	import { announce } from '../../announce.svelte';
	import { appearance } from '../../theme/appearance.svelte';
	import { LIMITS, uniqueName } from '../../theme/document';
	import { themes } from '../../theme/themes.svelte';
	import { cannotPublish, publish } from '../../theme/workspaceActions';
	import { toasts } from '../../toasts.svelte';
	import ThemeCard from './ThemeCard.svelte';
	import { BASE_NAMES } from './names';

	// Publishing one of the admin's own themes to the workspace: which
	// one, its name there, and whether to suggest it to everyone. A theme
	// below the contrast floors cannot be chosen, and says why.
	interface Props {
		ws: string;
		wsName: string;
		/** The names the workspace's themes have already. */
		taken: string[];
		onclose: () => void;
	}

	let { ws, wsName, taken, onclose }: Props = $props();

	let dialog = $state<HTMLDialogElement>();
	const scheme = $derived(appearance.scheme);
	// Checked on each theme's own accent, as most people will see it.
	const why = $derived(Object.fromEntries(themes.list.map((t) => [t.id, cannotPublish(t)])));
	let chosen = $state<string | null>(null);
	let name = $state('');
	let suggest = $state(false);
	let error = $state('');
	let busy = $state(false);

	function choose(id: string) {
		chosen = id;
		const t = themes.get(id);
		if (t) name = uniqueName(t.name, taken);
	}

	onMount(() => {
		const first = themes.list.find((t) => !why[t.id]);
		if (first) choose(first.id);
		dialog?.showModal();
		void tick().then(() =>
			dialog?.querySelector<HTMLElement>('input[type="radio"]:checked, [data-first]')?.focus()
		);
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		const t = chosen ? themes.get(chosen) : undefined;
		if (!t) return;
		busy = true;
		const done = await publish(ws, t, { name, suggest });
		busy = false;
		if ('error' in done) {
			error = done.error;
			return;
		}
		dialog?.close();
		toasts.show(
			suggest
				? `Published ${name.trim()} to ${wsName} and suggested it to everyone`
				: `Published ${name.trim()} to ${wsName}`
		);
		announce(`Published ${name.trim()}`);
	}
</script>

<dialog
	bind:this={dialog}
	class="publish"
	aria-labelledby="publish-title"
	aria-describedby="publish-lede"
	onclose={() => onclose()}
>
	<form onsubmit={submit}>
		<h2 id="publish-title">Publish a theme to {wsName}</h2>
		<p class="lede" id="publish-lede">
			Everyone in the workspace can use it, guests too. Nobody is switched to it.
		</p>
		<fieldset>
			<legend>Which of your themes</legend>
			<div class="choices">
				{#each themes.list as t (t.id)}
					{@const bad = why[t.id]}
					<label class="choice" class:bad={!!bad}>
						<input
							type="radio"
							name="publish-theme"
							value={t.id}
							checked={chosen === t.id}
							disabled={!!bad}
							aria-describedby="publish-why-{t.id}"
							onchange={() => choose(t.id)}
						/>
						<span class="card"
							><ThemeCard
								base={t.base}
								doc={t}
								{scheme}
								accentHue={appearance.current.accentHue}
							/></span
						>
						<span class="text">
							<span class="name">{t.name}</span>
							<span class="sub" id="publish-why-{t.id}">
								{#if bad}
									{bad}
									<a href={resolve('/(app)/settings/themes/[id]', { id: t.id })}
										>Edit it to publish it</a
									>
								{:else}
									Based on {BASE_NAMES[t.base]}, contrast goal {t.target}
								{/if}
							</span>
						</span>
					</label>
				{/each}
			</div>
		</fieldset>
		<label class="field">
			<span class="label">Name in the workspace</span>
			<input
				type="text"
				bind:value={name}
				maxlength={LIMITS.name}
				required
				disabled={!chosen}
				autocomplete="off"
			/>
		</label>
		<label class="check">
			<input type="checkbox" bind:checked={suggest} disabled={!chosen} />
			<span>
				Suggest it to everyone
				<span class="sub"
					>Each person is asked once whether to use it. Suggesting it stops suggesting any other.</span
				>
			</span>
		</label>
		<p class="error" role="status">{error}</p>
		<div class="actions">
			<Button variant="ghost" onclick={() => dialog?.close()}>Cancel</Button>
			<Button type="submit" variant="primary" data-first disabled={!chosen || !name.trim() || busy}
				>Publish</Button
			>
		</div>
	</form>
	<Announcer inDialog />
</dialog>

<style>
	.publish {
		width: min(calc(480px * var(--ui-scale)), calc(100vw - 32px));
		max-height: 90vh;
		padding: var(--space-5);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	.publish::backdrop {
		/* With the value as written, for browsers whose ::backdrop does not inherit custom properties. */
		background: var(--scrim, oklch(0 0 0 / 0.25));
	}
	form {
		display: grid;
		gap: var(--space-3);
	}
	h2 {
		font-size: var(--text-xl);
		overflow-wrap: anywhere;
	}
	.lede,
	.sub {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.sub {
		display: block;
		font-size: var(--text-xs);
	}
	fieldset {
		display: grid;
		gap: var(--space-2);
		min-width: 0;
		margin: 0;
		padding: 0;
		border: none;
	}
	legend,
	.label {
		margin-bottom: var(--space-1);
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.choices {
		display: grid;
		gap: var(--space-1);
		max-height: 40vh;
		overflow: auto;
	}
	.choice {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		padding: var(--space-2);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		cursor: pointer;
	}
	.choice:has(:checked) {
		border-color: var(--accent);
	}
	.choice.bad {
		cursor: not-allowed;
	}
	.choice.bad .sub {
		color: var(--danger);
	}
	.choice input {
		flex: none;
	}
	.card {
		flex: none;
		width: 64px;
	}
	.text {
		min-width: 0;
	}
	.name {
		font-weight: 550;
		overflow-wrap: anywhere;
	}
	/* A link inside text is underlined, not told apart by color alone. */
	a {
		color: var(--accent-text);
		text-decoration: underline;
	}
	.field {
		display: grid;
		gap: var(--space-1);
	}
	.field input {
		height: calc(32px * var(--control-scale));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
	}
	.check {
		display: flex;
		align-items: flex-start;
		gap: var(--space-2);
		font-size: var(--text-sm);
	}
	.check input {
		margin-top: 3px;
	}
	.error {
		color: var(--danger);
		font-size: var(--text-sm);
	}
	.error:empty {
		display: none;
	}
	.actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-2);
	}
	@media (max-width: 720px) {
		.publish {
			width: 100%;
			max-width: none;
			height: 100%;
			max-height: none;
			margin: 0;
			padding: var(--space-4) var(--space-4) calc(var(--space-4) + env(safe-area-inset-bottom));
			border: none;
			border-radius: 0;
		}
	}
</style>
