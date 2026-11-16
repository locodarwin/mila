<script lang="ts">
	import { resolve } from '$app/paths';
	import { announce } from '../../announce.svelte';
	import { exampleOf, requestPath, type Operation, type Spec } from '../../apidocs';
	import { session } from '../../session.svelte';
	import Button from '../Button.svelte';
	import Checkbox from '../Checkbox.svelte';

	interface Props {
		spec: Spec;
		op: Operation;
	}

	let { spec, op }: Props = $props();

	interface Answer {
		status: number;
		statusText: string;
		etag: string | null;
		location: string | null;
		text: string;
	}

	const json = $derived(op.body?.type === 'application/json');
	const fields = $derived(op.parameters.filter((p) => p.in !== 'cookie'));
	// Filled in once, then the person's to change.
	let values = $state<Record<string, string>>({});
	// The query parameters to send, empty or not.
	let include = $state<Record<string, boolean>>({});
	let body = $state('');
	let error = $state('');
	let busy = $state(false);
	let answer = $state<Answer | null>(null);

	$effect.pre(() => {
		if (op.body?.type === 'application/json' && body === '')
			body = JSON.stringify(exampleOf(spec, op.body.schema), null, 2);
	});
	$effect.pre(() => {
		if (!values.workspaceId && session.workspace) values.workspaceId = session.workspace.id;
	});

	async function send(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		const sent = new Set(Object.keys(include).filter((name) => include[name]));
		const { path, missing } = requestPath(op, values, sent);
		if (missing.length) {
			error = `Fill in ${missing.join(' and ')}.`;
			return;
		}
		const headers: Record<string, string> = {};
		for (const p of op.parameters) {
			const v = values[p.name]?.trim();
			if (p.in === 'header' && v) headers[p.name] = v;
		}
		let payload: string | undefined;
		if (json) {
			try {
				payload = JSON.stringify(JSON.parse(body));
			} catch {
				error = 'The body is not valid JSON.';
				return;
			}
			headers['Content-Type'] = 'application/json';
		}
		busy = true;
		try {
			// Same origin, so the browser sends the session cookie and the
			// Origin header the server asks of requests that change data.
			const res = await fetch('/api/v1' + path, { method: op.method, headers, body: payload });
			let text = await res.text();
			try {
				text = JSON.stringify(JSON.parse(text), null, 2);
			} catch {
				// not JSON: shown as it came
			}
			answer = {
				status: res.status,
				statusText: res.statusText,
				etag: res.headers.get('ETag'),
				location: res.headers.get('Location'),
				text
			};
			announce(`The server answered ${res.status} ${res.statusText}.`);
		} catch {
			error = 'The server could not be reached. Check your connection.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="try">
	{#if op.auth && !session.user}
		<p class="hint">
			<a class="link" href="{resolve('/(auth)/login')}?next=%2Fapi%2Fdocs">Sign in</a> to try this request
			as yourself.
		</p>
	{:else if op.body && !json}
		<p class="hint">This one takes a file; send it from a script.</p>
	{:else}
		<form onsubmit={send}>
			{#each fields as p (p.in + p.name)}
				<label class="field">
					<span class="label">{p.name} <span class="where">{p.in}</span></span>
					{#if p.name === 'workspaceId' && session.workspaces.length}
						<select bind:value={values[p.name]}>
							{#each session.workspaces as w (w.id)}
								<option value={w.id}>{w.name}</option>
							{/each}
						</select>
					{:else}
						<input
							bind:value={values[p.name]}
							autocomplete="off"
							spellcheck="false"
							oninput={() => {
								if (p.in === 'query') include[p.name] = values[p.name] !== '';
							}}
						/>
					{/if}
				</label>
				{#if p.in === 'query'}
					<!-- Ticked by typing; ticked with the field empty, it sends the
					     parameter empty (a filter's "none"). -->
					<Checkbox
						checked={include[p.name] ?? false}
						label="Send {p.name}"
						onchange={(on) => (include[p.name] = on)}
					/>
				{/if}
			{/each}
			{#if json}
				<label class="field">
					<span class="label">Body</span>
					<textarea
						bind:value={body}
						rows={Math.min(14, body.split('\n').length + 1)}
						spellcheck="false"></textarea>
				</label>
			{/if}
			{#if error}<p class="error" role="alert">{error}</p>{/if}
			<div>
				<Button type="submit" variant="primary" disabled={busy}>Send {op.method}</Button>
			</div>
		</form>
		{#if answer}
			<section class="answer" aria-label="Response to {op.method} {op.path}">
				<p class="status">
					<strong class="tabular">{answer.status}</strong>
					{answer.statusText}
				</p>
				{#if answer.etag}<p class="meta">ETag <code>{answer.etag}</code></p>{/if}
				{#if answer.location}<p class="meta">Location <code>{answer.location}</code></p>{/if}
				{#if answer.text}
					<!-- Focusable: it scrolls, and the keyboard needs to reach it. -->
					<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
					<pre tabindex="0" aria-label="Response body">{answer.text}</pre>
				{/if}
			</section>
		{/if}
	{/if}
</div>

<style>
	.try,
	form {
		display: grid;
		gap: var(--space-3);
	}
	.field {
		display: grid;
		gap: var(--space-1);
	}
	.label {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.where {
		font-weight: 400;
	}
	input,
	select,
	textarea {
		min-height: max(var(--tap-min), calc(34px * var(--control-scale)));
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
		font-family: var(--font-mono);
		font-size: var(--text-sm);
	}
	input::placeholder,
	textarea::placeholder {
		color: var(--input-placeholder);
	}
	textarea {
		padding: var(--space-2) var(--space-3);
		resize: vertical;
	}
	.hint {
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	.link {
		color: var(--accent-text);
		text-decoration: underline;
	}
	.error {
		color: var(--danger);
		font-size: var(--text-sm);
	}
	.answer {
		display: grid;
		gap: var(--space-2);
		padding: var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-1);
	}
	.meta {
		font-size: var(--text-sm);
		color: var(--text-secondary);
		overflow-wrap: anywhere;
	}
	pre {
		max-height: 24rem;
		margin: 0;
		overflow: auto;
		font-size: var(--text-xs);
	}
</style>
