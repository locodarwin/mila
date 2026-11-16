<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { tick } from 'svelte';
	import { announce } from '../../announce.svelte';
	import { anchored } from '../../popover';
	import { preferences } from '../../preferences.svelte';
	import { appearance, CHARACTERS, type Character } from '../../theme/appearance.svelte';
	import { floorsOf, problems } from '../../theme/custom';
	import { presets } from '../../theme/presets.svelte';
	import {
		blank,
		cleanName,
		LIMITS,
		parseTheme,
		ThemeError,
		uniqueName,
		validName,
		type ThemeDoc
	} from '../../theme/document';
	import { themes } from '../../theme/themes.svelte';
	import { toasts } from '../../toasts.svelte';
	import { uuid } from '../../uuid';
	import Button from '../Button.svelte';
	import ImportTheme from './ImportTheme.svelte';
	import ThemeCard from './ThemeCard.svelte';
	import { BASE_NAMES } from './names';
	import { copy, download } from './share';

	// Settings, Appearance: the person's own themes, each with what can be
	// done to it; starting a new one; and bringing one in from a file or
	// text. A workspace's themes are on its own page, Workspace themes.

	$effect(() => {
		void themes.ensure();
		void presets.ensure().catch(() => {});
	});

	const inUse = $derived(preferences.current.customTheme);
	const hue = $derived(appearance.current.accentHue);
	const scheme = $derived(appearance.scheme);
	const full = $derived(themes.list.length >= LIMITS.themes);
	const names = $derived(themes.list.map((t) => t.name));

	// Whether each theme can be used, and whether one aiming for AAA gets there.
	const status = $derived(
		Object.fromEntries(
			themes.list.map((t) => {
				const floors = floorsOf(t, { accentHue: hue });
				let below = false;
				if (!floors && t.target === 'AAA') {
					try {
						const p = problems(t, { accentHue: hue });
						below = p.light.length + p.dark.length > 0;
					} catch {
						// unreadable: floorsOf says so
					}
				}
				return [t.id, { floors, below }];
			})
		)
	);

	const editHref = (id: string) => resolve('/(app)/settings/themes/[id]', { id });

	async function use(t: ThemeDoc) {
		const why = await themes.use(t.id);
		if (why) toasts.error(why);
		else announce(`${t.name} is in use`);
	}

	/** A stored theme read again, as a fresh copy to change. */
	function copyOf(t: ThemeDoc): ThemeDoc {
		try {
			return parseTheme(t, 'stored').theme;
		} catch (err) {
			throw new ThemeError(err instanceof ThemeError ? err.message : 'This theme cannot be read.');
		}
	}

	/** Runs a change to a theme, saying so if the theme cannot be read. */
	function safely(run: () => unknown) {
		try {
			return run();
		} catch (err) {
			if (!(err instanceof ThemeError)) throw err;
			toasts.error(`${err.message} Delete it, or import it again.`);
		}
	}

	function duplicate(t: ThemeDoc) {
		if (full) return toasts.error(`You have ${LIMITS.themes} themes, the most Mila keeps.`);
		const copy = { ...copyOf(t), id: uuid(), name: uniqueName(`${t.name} copy`, names) };
		themes.put(copy);
		toasts.show(`Made ${copy.name}`);
		void focusRow(copy.id);
	}

	function remove(t: ThemeDoc) {
		const undo = themes.remove(t.id);
		toasts.show(`Deleted ${t.name}`, { onundo: undo });
		document.getElementById('themes-heading')?.focus();
	}

	// Renaming in place: the name becomes a field until Enter, or Escape.
	let renaming = $state<string | null>(null);
	let draft = $state('');

	async function startRename(t: ThemeDoc) {
		renaming = t.id;
		draft = t.name;
		await tick();
		document.getElementById(`rename-${t.id}`)?.focus();
	}

	function rename(t: ThemeDoc) {
		const name = cleanName(draft);
		renaming = null;
		if (!validName(name)) {
			toasts.error(`A name needs 1 to ${LIMITS.name} characters.`);
		} else if (name !== t.name) {
			const done = safely(() => {
				themes.put({ ...copyOf(t), name });
				return true;
			});
			if (done) announce(`Renamed to ${name}`);
		}
		void tick().then(() => document.getElementById(`more-${t.id}`)?.focus());
	}

	async function focusRow(id: string) {
		await tick();
		document.getElementById(`edit-${id}`)?.focus();
	}

	// New theme: a built-in, a copy of one made from Omarchy's (the way to
	// change one of those), or a copy of one of the person's own.
	let start = $state<string>('standard');
	const builtIn = (v: string): v is Character => (CHARACTERS as readonly string[]).includes(v);
	// A copy of a theme that has gone (deleted here or elsewhere) starts
	// from Standard instead, and says so.
	$effect(() => {
		if (!builtIn(start) && !themes.get(start) && !presets.get(start)) {
			start = 'standard';
			announce('That theme is gone, so a new one starts from Standard.');
		}
	});

	async function create() {
		if (full) return;
		const preset = presets.get(start);
		const own = themes.get(start) ?? preset;
		if (own && safely(() => copyOf(own)) === undefined) return;
		if (!own && !builtIn(start)) {
			start = 'standard';
			return;
		}
		// A copy of an Omarchy one keeps its name, which is the person's own now.
		const doc = own
			? {
					...copyOf(own),
					id: uuid(),
					name: uniqueName(preset ? own.name : `${own.name} copy`, names)
				}
			: blank(builtIn(start) ? start : 'standard', uniqueName('New theme', names));
		themes.put(doc);
		await goto(editHref(doc.id));
	}

	// Import: a file or pasted text, checked first (the dry run).
	let pasted = $state('');
	let fileInput = $state<HTMLInputElement>();
	let error = $state('');
	let checking = $state<{ theme: ThemeDoc; notes: string[] } | null>(null);

	function check(text: string) {
		error = '';
		try {
			checking = parseTheme(text, 'import', { taken: names });
		} catch (err) {
			checking = null;
			error = err instanceof ThemeError ? err.message : 'This is not a Mila theme Mila can read.';
		}
	}

	async function fromFile(e: Event & { currentTarget: HTMLInputElement }) {
		const file = e.currentTarget.files?.[0];
		e.currentTarget.value = '';
		if (!file) return;
		if (file.size > LIMITS.importBytes) {
			error = 'This file is too large to be a Mila theme (64 KiB at most).';
			return;
		}
		check(await file.text());
	}

	function added(t: ThemeDoc) {
		themes.put(t);
		pasted = '';
		toasts.show(`Added ${t.name}`);
		void focusRow(t.id);
	}
</script>

<section aria-labelledby="themes-heading">
	<h2 id="themes-heading" tabindex="-1">Your themes</h2>
	<p class="hint">
		A theme of your own starts from a built-in one and changes what you choose: colors, corners,
		type, spacing and motion. Only you see it. Pick one above, or here.
	</p>
	{#if themes.list.length}
		<ul class="rows" aria-label="Your themes">
			{#each themes.list as t (t.id)}
				{@const s = status[t.id]}
				{@const why = themes.notSaved[t.id]}
				<li>
					<span class="card"><ThemeCard base={t.base} doc={t} {scheme} accentHue={hue} /></span>
					<span class="grow">
						{#if renaming === t.id}
							<input
								id="rename-{t.id}"
								class="rename"
								aria-label="Name of {t.name}"
								maxlength={LIMITS.name}
								bind:value={draft}
								onkeydown={(e) => {
									if (e.key === 'Enter') {
										e.preventDefault();
										rename(t);
									} else if (e.key === 'Escape') {
										e.preventDefault();
										renaming = null;
										void tick().then(() => document.getElementById(`more-${t.id}`)?.focus());
									}
								}}
								onblur={() => renaming === t.id && rename(t)}
							/>
						{:else}
							<span class="name">{t.name}</span>
						{/if}
						<span class="sub">
							Based on {BASE_NAMES[t.base]}, contrast goal {t.target}{#if inUse === t.id}<span
									class="badge">In use</span
								>{/if}{#if s?.below}<span class="badge">Below AAA</span>{/if}
						</span>
						{#if s?.floors}<span class="sub warn" id="floors-{t.id}">{s.floors}</span>{/if}
						{#if why}
							<span class="sub warn">
								Not saved: {why.code === 'too_many_themes'
									? `you have the most themes Mila keeps (${LIMITS.themes}).`
									: why.message}
							</span>
						{/if}
					</span>
					<span class="actions">
						<Button
							size="sm"
							variant={inUse === t.id ? 'ghost' : 'secondary'}
							disabled={inUse === t.id || !!s?.floors}
							aria-describedby={s?.floors ? `floors-${t.id}` : undefined}
							aria-label={inUse === t.id ? `${t.name} is in use` : `Use ${t.name}`}
							onclick={() => void use(t)}>{inUse === t.id ? 'In use' : 'Use'}</Button
						>
						<a class="button-link" id="edit-{t.id}" href={editHref(t.id)} aria-label="Edit {t.name}"
							>Edit</a
						>
						<button
							type="button"
							class="more"
							id="more-{t.id}"
							popovertarget="menu-{t.id}"
							aria-label="More for {t.name}">More</button
						>
						<div class="menu" popover="auto" id="menu-{t.id}" use:anchored>
							{#each [['Rename', () => void startRename(t)], ['Duplicate', () => safely( () => duplicate(t) )], ['Export file', () => download(t)], ['Copy as text', () => void copy(t)], ['Delete', () => remove(t)]] as const as [label, run] (label)}
								<button
									type="button"
									class="option"
									class:danger={label === 'Delete'}
									onclick={(e) => {
										(e.currentTarget.closest('[popover]') as HTMLElement | null)?.hidePopover();
										run();
									}}>{label}</button
								>
							{/each}
						</div>
					</span>
				</li>
			{/each}
		</ul>
	{:else}
		<p class="hint">You have no themes of your own yet.</p>
	{/if}
</section>

<section aria-labelledby="new-heading">
	<h2 id="new-heading">New theme</h2>
	<form
		class="inline-form"
		onsubmit={(e) => {
			e.preventDefault();
			void create();
		}}
	>
		<label class="field">
			<span class="label">Start from</span>
			<select bind:value={start}>
				<optgroup label="Built-in">
					{#each CHARACTERS as c (c)}<option value={c}>{BASE_NAMES[c]}</option>{/each}
				</optgroup>
				{#if presets.list.length}
					<optgroup label="A copy of one from Omarchy">
						{#each presets.list as p (p.doc.id)}<option value={p.doc.id}>{p.doc.name}</option
							>{/each}
					</optgroup>
				{/if}
				{#if themes.list.length}
					<optgroup label="A copy of yours">
						{#each themes.list as t (t.id)}<option value={t.id}>{t.name}</option>{/each}
					</optgroup>
				{/if}
			</select>
		</label>
		<Button type="submit" variant="primary" disabled={full}>New theme</Button>
	</form>
	{#if full}
		<p class="hint">
			You have {LIMITS.themes} themes, the most Mila keeps. Delete one to make another.
		</p>
	{/if}
</section>

<section aria-labelledby="import-heading">
	<h2 id="import-heading">Import a theme</h2>
	<p class="hint">
		From a file someone exported (.mila-theme.json), or its text pasted below. Mila shows what it
		would bring before adding it.
	</p>
	<div class="inline-form">
		<input
			bind:this={fileInput}
			type="file"
			accept=".json,application/json"
			class="visually-hidden"
			aria-label="Theme file to import"
			tabindex="-1"
			id="theme-file"
			onchange={fromFile}
		/>
		<Button onclick={() => fileInput?.click()}>Choose a file</Button>
	</div>
	<label class="field">
		<span class="label">Or paste a theme</span>
		<textarea rows="4" bind:value={pasted} spellcheck="false"></textarea>
	</label>
	<div>
		<Button
			title="See what the pasted theme holds before adding it"
			disabled={!pasted.trim()}
			onclick={() => check(pasted)}>Check the pasted theme</Button
		>
	</div>
	<p class="error" role="status">{error}</p>
</section>

{#if checking}
	<ImportTheme
		theme={checking.theme}
		notes={checking.notes}
		{full}
		onadd={added}
		onclose={() => (checking = null)}
	/>
{/if}

<style>
	section {
		position: relative;
	}
	h2:focus-visible {
		outline: none;
		box-shadow: none;
	}
	.card {
		flex: none;
		width: 88px;
	}
	.name {
		font-weight: 550;
		overflow-wrap: anywhere;
	}
	.sub {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
	.warn {
		color: var(--danger);
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
	.button-link,
	.more {
		display: inline-flex;
		align-items: center;
		height: calc(26px * var(--control-scale));
		/* Edit is a short word: a finger still gets a whole target. */
		min-width: var(--tap-min);
		justify-content: center;
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--button-secondary-border);
		border-radius: var(--button-radius);
		background: var(--button-secondary-bg);
		color: var(--button-secondary-text);
		font-size: var(--text-xs);
		font-weight: 550;
		cursor: pointer;
	}
	.button-link:hover,
	.more:hover {
		background: var(--button-ghost-hover);
	}
	.menu .option {
		width: 100%;
		min-height: max(var(--tap-min), calc(30px * var(--control-scale)));
		border: none;
		background: none;
		color: var(--text-primary);
		font: inherit;
		text-align: left;
	}
	.menu .option.danger {
		color: var(--danger);
	}
	.rename {
		width: 100%;
		max-width: 320px;
		height: calc(30px * var(--control-scale));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
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
	select,
	textarea {
		padding: var(--space-1) var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
	}
	select {
		height: calc(32px * var(--control-scale));
	}
	textarea {
		font-family: var(--font-mono);
		font-size: var(--text-xs);
		resize: vertical;
	}
	.error {
		color: var(--danger);
		font-size: var(--text-sm);
	}
</style>
