<script lang="ts">
	import { tick } from 'svelte';
	import KeyboardOff from '@lucide/svelte/icons/keyboard-off';
	import Lock from '@lucide/svelte/icons/lock';
	import X from '@lucide/svelte/icons/x';
	import Button from '#lib/components/Button.svelte';
	import Checkbox from '#lib/components/Checkbox.svelte';
	import Kbd from '#lib/components/Kbd.svelte';
	import KeyRecorder from '#lib/components/keys/KeyRecorder.svelte';
	import TextInput from '#lib/components/TextInput.svelte';
	import { announce } from '#lib/announce.svelte.js';
	import { assign, reset as resetKeys, type Overrides } from '#lib/keys/bindings.js';
	import { label, spoken, type Binding } from '#lib/keys/chord.js';
	import { COMMANDS, SCOPES, command, type CommandId } from '#lib/keys/commands.js';
	import { conflicts, swap, swapConflicts, type Conflict } from '#lib/keys/conflicts.js';
	import { keymap, list } from '#lib/keys/keymap.svelte.js';
	import { preferences } from '#lib/preferences.svelte.js';
	import { toasts } from '#lib/toasts.svelte.js';

	/** How many keys one command can have. */
	const MAX_KEYS = 4;

	let query = $state('');
	// The key being recorded: one to replace (old), or a new one.
	let recording = $state<{ id: CommandId; old?: Binding } | null>(null);
	// A key just pressed that something stands in the way of.
	// swappable: the command that has the key can take the one replaced.
	let problem = $state<{
		id: CommandId;
		old?: Binding;
		key: Binding;
		conflict: Conflict;
		swappable: boolean;
	} | null>(null);
	// A reset that takes back a default another command holds now.
	let resetting = $state<{ id: CommandId; next: Overrides; also: CommandId[] } | null>(null);
	// A key saved that does nothing until single-key shortcuts are on.
	let note = $state<CommandId | null>(null);

	const say = (b: Binding) => spoken(b, keymap.apple);
	const mod = $derived(keymap.apple ? 'Cmd' : 'Ctrl');

	// A key matches however it is written: "ctrl k", "Ctrl+K", "g i", "G then I".
	const plain = (s: string) => s.toLowerCase().replace(/\bthen\b|[\s+]/g, '');
	function matches(id: CommandId, q: string): boolean {
		if (!q.trim()) return true;
		if (command(id).name.toLowerCase().includes(q.trim().toLowerCase())) return true;
		const want = plain(q);
		return keymap
			.keys(id)
			.some((b) => plain(say(b)).includes(want) || plain(label(b, keymap.apple)).includes(want));
	}

	const groups = $derived(
		SCOPES.map((s) => ({
			...s,
			rows: COMMANDS.filter((c) => c.scope === s.id && matches(c.id, query))
		})).filter((g) => g.rows.length)
	);
	const shown = $derived(groups.reduce((n, g) => n + g.rows.length, 0));
	$effect(() => {
		if (query.trim())
			announce(
				shown ? `${shown} ${shown === 1 ? 'shortcut' : 'shortcuts'}` : 'No shortcut matches',
				500
			);
	});

	/** Focuses something in a command's row once the page has caught up. */
	async function focusIn(id: CommandId, selector: string) {
		await tick();
		const row = document.querySelector(`[data-row="${CSS.escape(id)}"]`);
		row?.querySelector<HTMLElement>(selector)?.focus();
	}

	const keyButton = (b: Binding) => `[data-key="${CSS.escape(b)}"]`;

	function keysOf(id: CommandId): string {
		const keys = keymap.map.get(id) ?? [];
		return keys.length ? list(keys.map(say)) : 'no key';
	}

	/** Saves new keys, says so, and offers to undo. */
	function save(next: Overrides, said: string, toast: string) {
		const before = preferences.current.shortcuts;
		preferences.set({ shortcuts: next });
		announce(said);
		toasts.show(toast, { onundo: () => preferences.set({ shortcuts: before }) });
	}

	function changedKeys(id: CommandId, next: Overrides, key?: Binding) {
		save(next, `${command(id).name}: ${keysOf(id)}.`, `Changed the keys for ${command(id).name}`);
		note = key && keymap.off(id, key) ? id : null;
		if (key) void focusIn(id, keyButton(key));
	}

	function start(id: CommandId, old?: Binding) {
		problem = null;
		resetting = null;
		note = null;
		recording = { id, old };
	}

	async function canceled(id: CommandId, old?: Binding) {
		recording = null;
		await tick();
		// Escape: back to what started it. Tab or a click has gone elsewhere.
		if (!document.activeElement || document.activeElement === document.body)
			void focusIn(id, old ? keyButton(old) : '[data-add]');
	}

	function record(id: CommandId, old: Binding | undefined, key: Binding) {
		recording = null;
		const mine = keymap.map.get(id) ?? [];
		if (key === old || keymap.keys(id).includes(key)) {
			announce(`${command(id).name} already has ${say(key)}.`);
			void focusIn(id, keyButton(key));
			return;
		}
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- a copy to ask about, never shown
		const without = new Map(keymap.map);
		without.set(
			id,
			mine.filter((k) => k !== old)
		);
		const found = conflicts(without, id, key, keymap.apple);
		if (found.length) {
			const conflict = found[0];
			const swappable =
				!!old &&
				!!conflict.other &&
				(conflict.kind === 'same-scope' || conflict.kind === 'overlap') &&
				!swapConflicts(without, { id, old }, { id: conflict.other, key }, keymap.apple).length;
			problem = { id, old, key, conflict, swappable };
			announce(message(problem));
			void focusIn(id, '[data-problem] button');
			return;
		}
		put(id, old, key);
	}

	/** Gives a command the key, in place of the old one or after its others. */
	function put(id: CommandId, old: Binding | undefined, key: Binding) {
		const mine = keymap.map.get(id) ?? [];
		const keys = old ? mine.map((k) => (k === old ? key : k)) : [...mine, key];
		problem = null;
		changedKeys(id, assign(preferences.current.shortcuts, id, keys), key);
	}

	/**
	 * Takes the key from the command that has it: a swap gives that one the
	 * key being replaced, where it can have it; a move leaves it without.
	 */
	function take(p: NonNullable<typeof problem>) {
		const other = p.conflict.other!;
		problem = null;
		const next = swap(
			preferences.current.shortcuts,
			{ id: p.id, old: p.old },
			{ id: other, key: p.key },
			p.swappable
		);
		changedKeys(p.id, next, p.key);
	}

	function message(p: NonNullable<typeof problem>): string {
		const other = p.conflict.other ? command(p.conflict.other).name : '';
		if (p.conflict.kind === 'same-scope' || p.conflict.kind === 'overlap')
			return (
				p.conflict.message +
				(p.swappable && p.old
					? ` Swap to give “${other}” ${say(p.old)} instead.`
					: p.old
						? ` Moving it here takes it from “${other}”, which can't have ${say(p.old)} instead.`
						: ` Moving it here takes it from “${other}”.`)
			);
		return p.conflict.message;
	}

	function retry(p: NonNullable<typeof problem>) {
		problem = null;
		start(p.id, p.old);
	}

	function dismiss(p: NonNullable<typeof problem>) {
		problem = null;
		announce('Nothing changed.');
		void focusIn(p.id, p.old ? keyButton(p.old) : '[data-add]');
	}

	function remove(id: CommandId, key: Binding) {
		const mine = keymap.map.get(id) ?? [];
		save(
			assign(
				preferences.current.shortcuts,
				id,
				mine.filter((k) => k !== key)
			),
			`Removed ${say(key)} from ${command(id).name}.`,
			`Removed ${say(key)} from ${command(id).name}`
		);
		void focusIn(id, '[data-add]');
	}

	function reset(id: CommandId) {
		problem = null;
		const { next, also } = resetKeys(preferences.current.shortcuts, id);
		if (also.length) {
			// Its defaults are another command's keys now: ask first.
			resetting = { id, next, also };
			announce(resetMessage(resetting));
			void focusIn(id, '[data-problem] button');
			return;
		}
		finishReset(id, next, []);
	}

	function resetMessage(r: NonNullable<typeof resetting>): string {
		const names = list(r.also.map((o) => `“${command(o).name}”`));
		return `${command(r.id).name}'s default keys belong to ${names} now. Resetting puts ${r.also.length === 1 ? 'that' : 'those'} back to ${r.also.length === 1 ? 'its' : 'their'} defaults too.`;
	}

	function finishReset(id: CommandId, next: Overrides, also: CommandId[]) {
		resetting = null;
		const names = [id, ...also].map((o) => command(o).name);
		save(
			next,
			`${list(names)} ${names.length === 1 ? 'is' : 'are'} back to the default keys.`,
			`Reset ${list(names)}`
		);
		void focusIn(id, '[data-add], [data-key]');
	}

	function keepKeys(id: CommandId) {
		resetting = null;
		announce('Nothing changed.');
		void focusIn(id, '[aria-label^="Reset"]');
	}

	function resetAll() {
		save({}, 'Every shortcut is back to its default keys.', 'Reset every shortcut');
	}

	const anyChanged = $derived(Object.keys(preferences.current.shortcuts).length > 0);
</script>

<svelte:head><title>Keyboard · Mila</title></svelte:head>

<p class="hint">
	These follow you to every device you sign in on. <span class="touch"
		>Shortcuts need a keyboard.</span
	>
</p>

<section>
	<h2>Single-key shortcuts</h2>
	<Checkbox
		label="Single-key shortcuts"
		checked={preferences.current.singleKeyShortcuts}
		onchange={(on) => preferences.set({ singleKeyShortcuts: on })}
	/>
	<p class="hint">
		{#if keymap.characterKeys()}{keymap.characterKeys()} work anywhere a field is not being typed in.
		{/if}Turn them off if keys get pressed by accident or speech input sets them off; keys with {mod}
		or {keymap.apple ? 'Option' : 'Alt'}, and the keys on a focused task, keep working.
	</p>
</section>

<section>
	<h2>Shortcuts</h2>
	<div class="find">
		<TextInput
			label="Find a shortcut"
			type="search"
			placeholder="A name or a key, such as {mod} K"
			bind:value={query}
		/>
	</div>
	{#each groups as g (g.id)}
		<div class="scope">
			<h3 id={`scope-${g.id}`}>{g.name}</h3>
			<ul class="rows" aria-labelledby={`scope-${g.id}`}>
				{#each g.rows as c (c.id)}
					{@const mine = keymap.map.get(c.id) ?? []}
					{@const fixed: readonly Binding[] = 'fixed' in c ? c.fixed : []}
					{@const editable = !('remappable' in c)}
					<li data-row={c.id}>
						<span class="grow name">
							{c.name}
							{#if keymap.changed(c.id)}<span class="badge">Changed</span>{/if}
						</span>
						<span class="keys">
							{#each mine as b (b)}
								{#if recording?.id === c.id && recording.old === b}
									<KeyRecorder
										name={c.name}
										anywhere={c.scope === 'anywhere'}
										onrecord={(key) => record(c.id, b, key)}
										oncancel={() => canceled(c.id, b)}
									/>
								{:else}
									<span class="binding" class:dim={keymap.off(c.id, b)}>
										<button
											type="button"
											class="key"
											data-key={b}
											aria-label={`${say(b)}, shortcut for ${c.name}, change`}
											onclick={() => start(c.id, b)}><Kbd binding={b} /></button
										><button
											type="button"
											class="icon"
											aria-label={`Remove ${say(b)} from ${c.name}`}
											onclick={() => remove(c.id, b)}><X size={14} /></button
										>
										{#if keymap.off(c.id, b)}
											<span class="sub off"
												><KeyboardOff size={12} aria-hidden="true" /> off while single-key shortcuts are
												off</span
											>
										{/if}
									</span>
								{/if}
							{/each}
							{#each fixed as b (b)}
								<span class="fixed"
									><Lock size={12} aria-hidden="true" /><Kbd binding={b} /><span class="sub"
										>always</span
									></span
								>
							{/each}
							{#if editable}
								{#if recording?.id === c.id && !recording.old}
									<KeyRecorder
										name={c.name}
										anywhere={c.scope === 'anywhere'}
										onrecord={(key) => record(c.id, undefined, key)}
										oncancel={() => canceled(c.id)}
									/>
								{:else if mine.length < MAX_KEYS}
									<Button
										size="sm"
										variant="ghost"
										data-add
										aria-label={`Add a key for ${c.name}`}
										onclick={() => start(c.id)}>Add a key</Button
									>
								{/if}
								{#if keymap.changed(c.id)}
									<Button
										size="sm"
										variant="ghost"
										aria-label={`Reset ${c.name}`}
										onclick={() => reset(c.id)}>Reset</Button
									>
								{/if}
							{/if}
						</span>
						{#if problem?.id === c.id}
							{@const p = problem}
							{@const kind = p.conflict.kind}
							<div class="problem" data-problem>
								<p id="key-problem">{message(p)}</p>
								<div class="actions">
									{#if kind === 'same-scope' || kind === 'overlap'}
										<Button size="sm" aria-describedby="key-problem" onclick={() => take(p)}
											>{p.swappable ? 'Swap' : 'Move it here'}</Button
										>
										{#if kind === 'overlap'}
											<Button
												size="sm"
												aria-describedby="key-problem"
												onclick={() => put(p.id, p.old, p.key)}>Keep both</Button
											>
										{/if}
									{:else}
										<Button size="sm" aria-describedby="key-problem" onclick={() => retry(p)}
											>Try another key</Button
										>
									{/if}
									<Button size="sm" variant="ghost" onclick={() => dismiss(p)}>Cancel</Button>
								</div>
							</div>
						{/if}
						{#if resetting?.id === c.id}
							{@const r = resetting}
							<div class="problem" data-problem>
								<p id="reset-problem">{resetMessage(r)}</p>
								<div class="actions">
									<Button
										size="sm"
										aria-describedby="reset-problem"
										onclick={() => finishReset(r.id, r.next, r.also)}
										>{r.also.length === 1 ? 'Reset both' : `Reset all ${r.also.length + 1}`}</Button
									>
									<Button size="sm" variant="ghost" onclick={() => keepKeys(r.id)}>Cancel</Button>
								</div>
							</div>
						{/if}
						{#if note === c.id}
							<p class="sub note">
								This works once single-key shortcuts are on, or pick a key with {mod} or {keymap.apple
									? 'Option'
									: 'Alt'}.
							</p>
						{/if}
					</li>
				{/each}
			</ul>
		</div>
	{:else}
		<p class="hint">No shortcut matches “{query.trim()}”.</p>
	{/each}
	<div>
		<Button disabled={!anyChanged} onclick={resetAll}>Reset all shortcuts</Button>
	</div>
</section>

<style>
	.find {
		max-width: 320px;
	}
	.scope {
		display: grid;
		gap: var(--space-2);
	}
	h3 {
		font-family: var(--font-ui);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.name {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
	}
	.keys {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
	.binding,
	.fixed {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
	}
	.fixed {
		color: var(--text-secondary);
	}
	.key,
	.icon {
		display: inline-grid;
		place-items: center;
		min-width: max(var(--tap-min), calc(28px * var(--control-scale)));
		min-height: max(var(--tap-min), calc(28px * var(--control-scale)));
		padding: 0 var(--space-1);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.key:hover,
	.icon:hover {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	/* Off: a dashed edge, beside the icon and the words that say so. */
	.dim .key {
		outline: 1px dashed var(--border-strong);
		outline-offset: -2px;
	}
	.off {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
	}
	.problem,
	.note {
		flex-basis: 100%;
	}
	.problem {
		display: grid;
		gap: var(--space-2);
		padding: var(--space-2) var(--space-3);
		border-left: 3px solid var(--accent);
		background: var(--surface-1);
		font-size: var(--text-sm);
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.touch {
		display: none;
	}
	@media (pointer: coarse) {
		.touch {
			display: inline;
		}
	}
</style>
