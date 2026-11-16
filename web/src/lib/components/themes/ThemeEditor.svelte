<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { onMount, tick, untrack } from 'svelte';
	import ArrowLeft from '@lucide/svelte/icons/arrow-left';
	import Check from '@lucide/svelte/icons/check';
	import Ellipsis from '@lucide/svelte/icons/ellipsis';
	import Redo2 from '@lucide/svelte/icons/redo-2';
	import Search from '@lucide/svelte/icons/search';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import Undo2 from '@lucide/svelte/icons/undo-2';
	import { announce } from '../../announce.svelte';
	import { chordOf } from '../../keys/chord';
	import { keymap } from '../../keys/keymap.svelte';
	import { anchored } from '../../popover';
	import { preferences } from '../../preferences.svelte';
	import { appearance } from '../../theme/appearance.svelte';
	import type { Scheme } from '../../theme/color';
	import {
		check,
		clearsFloors,
		fix,
		measure,
		type Background,
		type Failure
	} from '../../theme/contrast';
	import { resolveTheme, SCHEMES } from '../../theme/custom';
	import {
		cleanName,
		LIMITS,
		parseTheme,
		TARGETS,
		uniqueName,
		validName,
		valueOf,
		type Target,
		type ThemeDoc,
		type ThemeValue
	} from '../../theme/document';
	import {
		ACCENT_TOKENS,
		changedIn,
		DURATIONS,
		groupTokens,
		manual,
		resetAll,
		SCALES,
		scaled,
		SPEEDS,
		withAccent,
		withoutGroup,
		withValue
	} from '../../theme/edit';
	import { ThemeHistory } from '../../theme/history.svelte';
	import type { Color, Resolved } from '../../theme/resolve';
	import { canonical, themes } from '../../theme/themes.svelte';
	import { byName, GROUPS, TOKENS } from '../../theme/tokens';
	import { toasts } from '../../toasts.svelte';
	import { ui } from '../../ui.svelte';
	import { uuid } from '../../uuid';
	import Button from '../Button.svelte';
	import Checkbox from '../Checkbox.svelte';
	import { BASE_NAMES } from './names';
	import { copy, download } from './share';
	import ThemePreview from './ThemePreview.svelte';
	import TokenRow from './TokenRow.svelte';

	// The theme editor: every token of a custom theme, by group, for light
	// and dark, with a live preview of the app's own parts, contrast
	// problems with Fix, and undo. Changes save as they are made; the app
	// follows when this is the theme in use, as long as its text stays
	// readable. (docs/PLAN.md section 3, Shawn's direction A of 2026-11-15:
	// the preview first, at the middle, showing the part being edited; the
	// settings a row each, opening in place; a click on the preview finds
	// the setting.) Wide, three columns that each scroll: parts, preview,
	// settings. Narrower, the preview sits on top and parts are a row.
	let { id }: { id: string } = $props();

	const manager = resolve('/(app)/settings/appearance');
	let ready = $state(false);
	onMount(() => {
		themes.editing = true;
		void themes.ensure().then(() => (ready = true));
		return () => {
			history?.settle();
			themes.editing = false;
		};
	});

	const stored = $derived(themes.get(id));
	let history = $state<ThemeHistory | null>(null);
	let unreadable = $state(false);
	// The stored version last seen, so only a change made elsewhere (another
	// device, through the themes message) comes into the editor.
	let seen = '';
	$effect(() => {
		const s = stored;
		if (!s) return;
		untrack(() => {
			const c = canonical(s);
			if (c === seen) return;
			seen = c;
			let theme: ThemeDoc;
			try {
				theme = parseTheme(s, 'stored').theme;
			} catch {
				// Not a theme this Mila can read (it says so below); nothing to edit.
				unreadable = !history;
				return;
			}
			unreadable = false;
			if (!history) history = new ThemeHistory(theme, (d) => themes.put(d));
			else history.external(theme);
		});
	});
	const doc = $derived(history?.doc ?? null);

	// What is edited, and how the page is laid out.
	let scheme = $state<Scheme>(untrack(() => appearance.scheme));
	let group = $state(GROUPS[0].id);
	const wideQuery = matchMedia('(min-width: 1180px)');
	const phoneQuery = matchMedia('(max-width: 720px)');
	let wide = $state(wideQuery.matches);
	let phone = $state(phoneQuery.matches);
	// The setting open in its row; a search across every part; only what
	// this theme changed.
	let expanded = $state<string | null>(null);
	let query = $state('');
	let changedOnly = $state(false);
	// What the preview shows: the part being edited or the whole app, in
	// light, dark or side by side; narrower, in the scheme being edited.
	let wholeApp = $state(false);
	let previewOpen = $state(true);
	$effect(() => {
		const w = (e: MediaQueryListEvent) => (wide = e.matches);
		const p = (e: MediaQueryListEvent) => (phone = e.matches);
		wideQuery.addEventListener('change', w);
		phoneQuery.addEventListener('change', p);
		return () => {
			wideQuery.removeEventListener('change', w);
			phoneQuery.removeEventListener('change', p);
		};
	});

	// Every token as the browser draws it, in both schemes, worked out at
	// most once a frame.
	const NAMES = [...TOKENS.map((t) => t.name), 'accent-hue'];
	const personHue = $derived(appearance.current.accentHue);
	const hueFor = (d: ThemeDoc) => (manual(d) ? null : personHue);
	let values = $state.raw<Record<Scheme, Map<string, Resolved>> | null>(null);
	$effect(() => {
		const d = doc;
		const hue = d ? hueFor(d) : null;
		void appearance.current.density;
		void appearance.current.fontSize;
		if (!d) return;
		const frame = requestAnimationFrame(() => {
			const out = {} as Record<Scheme, Map<string, Resolved>>;
			for (const s of SCHEMES) out[s] = resolveTheme(d, s, NAMES, { accentHue: hue }).values;
			values = out;
		});
		return () => cancelAnimationFrame(frame);
	});

	const target = $derived<Target>(doc?.target ?? 'AA');
	const found = $derived(
		values
			? { light: check(values.light, target), dark: check(values.dark, target) }
			: { light: [] as Failure[], dark: [] as Failure[] }
	);
	const total = $derived(found.light.length + found.dark.length);
	const floors = $derived(values ? clearsFloors(values.light) && clearsFloors(values.dark) : true);
	const measures = $derived(values ? measure(values[scheme], target) : []);
	const groupOf = (name: string) => byName(name)?.group;
	const bgNames = (bg: Background) => (typeof bg === 'string' ? [bg] : bg.mix.slice(0, 2));

	// The count of problems is said when it changes, after the first.
	let lastTotal: number | null = null;
	$effect(() => {
		const n = total;
		if (!values) return;
		untrack(() => {
			if (lastTotal !== null && n !== lastTotal)
				announce(
					n ? `${n} contrast ${n === 1 ? 'problem' : 'problems'}` : 'No contrast problems',
					800
				);
			lastTotal = n;
		});
	});

	function change(next: ThemeDoc, final = true) {
		history?.change(next, final);
	}
	const setToken = (name: string) => (v: ThemeValue | null, final: boolean) => {
		if (doc) change(withValue(doc, name, scheme, v), final);
	};
	const round = (n: number, places = 4) => Number(n.toFixed(places));

	function fixToken(name: string) {
		const v = values?.[scheme];
		if (!doc || !v) return;
		const lch = fix(v, name, target);
		if (!lch) return;
		const meta = byName(name)!;
		const was = v.get(name) as Color | undefined;
		const color: [number, number, number] = [
			round(Math.min(1, Math.max(0, lch[0]))),
			round(Math.min(0.4, Math.max(0, lch[1]))),
			round(Math.min(360, Math.max(0, lch[2])), 2)
		];
		// Nothing to do, or no color of this one's that helps: no step, and nothing claimed.
		if (was && color.every((n, i) => Math.abs(n - was.lch[i]) < 1e-3)) {
			announce(
				`${meta.label} cannot fix this on its own. Change the color it is read on, or read on it.`
			);
			return;
		}
		const value: ThemeValue =
			meta.alpha && was && was.alpha < 1 ? { oklch: color, alpha: was.alpha } : { oklch: color };
		change(withValue(doc, name, scheme, value));
		announce(`Fixed ${meta.label}`);
	}

	// Undo and redo from the keyboard, anywhere in the editor but a text
	// field, before the app's own undo hears the keys.
	const textEntry = (t: EventTarget | null) => {
		const el = t as HTMLElement | null;
		if (!el?.closest) return false;
		if (el.closest('textarea, select, [contenteditable="true"]')) return true;
		const input = el.closest('input');
		return !!input && !['range', 'radio', 'checkbox', 'color', 'button'].includes(input.type);
	};
	// Only for keys pressed in the editor (or with nothing focused): a task
	// opened over it keeps the app's own undo.
	let root = $state<HTMLElement>();
	function keys(e: KeyboardEvent) {
		if (!history || e.defaultPrevented || ui.overlay || textEntry(e.target)) return;
		const t = e.target as Node | null;
		const here = t === document.body || !!(t && root?.contains(t));
		if (!here) return;
		const chord = chordOf(e);
		if (!chord) return;
		if (keymap.keys('theme.undo').includes(chord)) {
			e.preventDefault();
			undo();
		} else if (keymap.keys('theme.redo').includes(chord)) {
			e.preventDefault();
			redo();
		}
	}
	$effect(() => {
		window.addEventListener('keydown', keys, { capture: true });
		return () => window.removeEventListener('keydown', keys, { capture: true });
	});
	function undo() {
		if (history?.undo()) announce('Undone');
	}
	function redo() {
		if (history?.redo()) announce('Redone');
	}

	// The name: typed live, kept when the field is left.
	function rename(text: string, final: boolean) {
		if (!doc) return;
		const name = cleanName(text);
		if (!final) return change({ ...doc, name: text }, false);
		if (!validName(name)) {
			toasts.error(`A name needs 1 to ${LIMITS.name} characters.`);
			history?.cancel();
			return;
		}
		change({ ...doc, name });
	}

	const inUse = $derived(preferences.current.customTheme === id);
	async function use() {
		const why = await themes.use(id);
		if (why) toasts.error(why);
		else announce(`${doc?.name} is in use`);
	}

	// The accent: worked out from a hue, or set by hand per scheme.
	const baseHue = $derived(Number(values?.[scheme].get('accent-hue')) || 262);
	const accentHue = $derived(doc && 'hue' in doc.accent ? (doc.accent.hue ?? baseHue) : baseHue);
	// The theme's own hue, remembered while the accent is set by hand, so
	// going back to a worked-out accent brings it back.
	let lastHue = $state<number | null>(null);
	$effect(() => {
		const a = doc?.accent;
		if (!a || 'manual' in a) return;
		lastHue = 'hue' in a && typeof a.hue === 'number' ? a.hue : null;
	});
	function generated() {
		if (doc) change(withAccent(doc, lastHue === null ? {} : { hue: lastHue }));
	}
	function byHand() {
		if (!doc || !values) return;
		const start = {} as Record<Scheme, Record<string, ThemeValue>>;
		for (const s of SCHEMES) {
			start[s] = {};
			for (const name of ACCENT_TOKENS) {
				const c = values[s].get(name) as Color | undefined;
				if (c) start[s][name] = { oklch: c.lch.map((n, i) => round(n, i === 2 ? 2 : 4)) as never };
			}
		}
		change(withAccent(doc, { manual: true }, start));
	}

	// Scales and speeds work from the base's values.
	let baseCache: { base: string; values: Map<string, Resolved> } | null = null;
	function baseValues(names: readonly string[]): Map<string, number> {
		if (!doc) return new Map();
		if (baseCache?.base !== doc.base) {
			const bare = resetAll(doc);
			baseCache = {
				base: doc.base,
				values: resolveTheme(bare, 'light', NAMES, { accentHue: null }).values
			};
		}
		// A plain answer for the caller, never state.
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const out = new Map<string, number>();
		for (const n of names) {
			const v = baseCache.values.get(n);
			if (typeof v === 'number') out.set(n, v);
		}
		return out;
	}
	function factorOf(name: string): number {
		const base = baseValues([name]).get(name);
		const now = values?.light.get(name);
		return base && typeof now === 'number' ? now / base : 1;
	}
	function scaleBy(names: readonly string[], factor: number, final: boolean) {
		if (doc) change(scaled(doc, scheme, baseValues(names), factor), final);
	}

	function resetGroup() {
		if (!doc) return;
		let next = withoutGroup(doc, group);
		if (group === 'accent') next = withAccent(next, {});
		change(next);
		announce(`${groupLabel(group)} is back to ${BASE_NAMES[doc.base]}`);
	}
	const groupLabel = (g: string) => GROUPS.find((x) => x.id === g)?.label ?? g;

	/** Opens a setting in its row, in its part, and goes to it. */
	async function goToToken(name: string, s: Scheme = scheme) {
		scheme = s;
		query = '';
		changedOnly = false;
		group = groupOf(name) ?? group;
		expanded = name;
		await tick();
		const row = document.getElementById(`token-${name}`);
		row?.scrollIntoView({ block: 'nearest' });
		row?.querySelector<HTMLElement>('.summary')?.focus({ preventScroll: true });
	}

	// A click on the preview: the first of the tokens drawing what was
	// clicked that the editor has (a mix's own tokens are its parts).
	function pickFromPreview(names: string[]) {
		const name = names.find((n) => byName(n));
		if (!name) return;
		announce(byName(name)!.label);
		void goToToken(name);
	}

	function pickGroup(g: string) {
		group = g;
		query = '';
		expanded = null;
	}

	// The rows shown: the part's, or every part's that a search finds, and
	// with Changed only, those this theme sets.
	const rows = $derived.by(() => {
		if (!doc) return [];
		const q = query.trim().toLowerCase();
		const metas = q
			? TOKENS.filter((t) => [t.label, t.used, t.name].some((x) => x.toLowerCase().includes(q)))
			: groupTokens(group);
		return changedOnly ? metas.filter((m) => valueOf(doc, m.name, scheme) !== null) : metas;
	});
	// A search's count, said as it changes.
	$effect(() => {
		const q = query.trim();
		const n = rows.length;
		if (!q) return;
		const t = setTimeout(() => announce(`${n} ${n === 1 ? 'setting' : 'settings'}`), 400);
		return () => clearTimeout(t);
	});

	function duplicate() {
		if (!doc) return;
		if (themes.list.length >= LIMITS.themes)
			return toasts.error(`You have ${LIMITS.themes} themes, the most Mila keeps.`);
		const copyDoc = {
			...doc,
			id: uuid(),
			name: uniqueName(
				`${doc.name} copy`,
				themes.list.map((t) => t.name)
			)
		};
		themes.put(copyDoc);
		toasts.show(`Made ${copyDoc.name}`);
		void goto(resolve('/(app)/settings/themes/[id]', { id: copyDoc.id }));
	}

	async function remove() {
		if (!doc) return;
		const name = doc.name;
		const undoIt = themes.remove(id);
		toasts.show(`Deleted ${name}`, { onundo: undoIt });
		await goto(manager);
	}

	function more(run: () => unknown) {
		return (e: Event) => {
			(e.currentTarget as HTMLElement).closest<HTMLElement>('[popover]')?.hidePopover();
			void run();
		};
	}

	let problemsOpen = $state(false);
	// The preview's scheme follows the one being edited, unless it shows both.
	let show = $state<'light' | 'dark' | 'both'>(untrack(() => scheme));
	$effect(() => {
		const s = scheme;
		untrack(() => {
			if (show !== 'both') show = s;
		});
	});
	const shown = $derived<Scheme[]>(!wide ? [scheme] : show === 'both' ? ['light', 'dark'] : [show]);
	let previews = $state<Record<string, ThemePreview | undefined>>({});
	function play() {
		for (const p of Object.values(previews)) p?.play();
	}
	const status = $derived(
		!values
			? 'Checking contrast'
			: total
				? `${total} ${total === 1 ? 'problem' : 'problems'} at ${target}`
				: `Reads at ${target}, light and dark`
	);

	const labelOf = (bg: Background) =>
		typeof bg === 'string' ? (byName(bg)?.label ?? bg) : bg.label;
	const ratio = (n: number) => `${n.toFixed(1)}:1`;
	const LOCKED = 'Worked out from the accent hue. Set the accent by hand to choose it yourself.';
</script>

<svelte:head><title>{doc?.name ?? 'Theme'} · Themes · Mila</title></svelte:head>

{#snippet segmented(
	legend: string,
	name: string,
	options: readonly (readonly [string, string])[],
	value: string,
	pick: (v: string) => void
)}
	<fieldset class="segmented">
		<legend>{legend}</legend>
		<div>
			{#each options as [option, label] (option)}
				<label class:active={value === option}>
					<input
						type="radio"
						{name}
						value={option}
						checked={value === option}
						onchange={() => pick(option)}
						class="visually-hidden"
					/>
					{label}
				</label>
			{/each}
		</div>
	</fieldset>
{/snippet}

{#if !doc}
	<header class="page"><h1>Theme</h1></header>
	<div class="missing">
		{#if unreadable}
			<p>This theme cannot be read, so it cannot be edited. Delete it in Settings, Appearance.</p>
		{:else if ready}
			<p>This theme is not here. It may have been deleted, or belong to someone else.</p>
		{:else}
			<p>Loading the theme.</p>
		{/if}
		<a class="back" href={manager}><ArrowLeft size={16} />Back to themes</a>
	</div>
{:else}
	<div class="editor" class:wide class:phone bind:this={root}>
		<header class="toolbar">
			<a class="back" href={manager} aria-label={phone ? 'Back to themes' : undefined}
				><ArrowLeft size={16} /><span class="word">Back to themes</span></a
			>
			<h1 class="visually-hidden">{doc.name}</h1>
			<label class="name">
				<span class="visually-hidden">Theme name</span>
				<input
					value={doc.name}
					maxlength={LIMITS.name}
					spellcheck="false"
					oninput={(e) => rename(e.currentTarget.value, false)}
					onchange={(e) => rename(e.currentTarget.value, true)}
				/>
			</label>
			{#if !phone}<span class="based">Based on {BASE_NAMES[doc.base]}</span>{/if}
			{#if !phone}
				{@render segmented(
					'Editing',
					'editing-scheme',
					[
						['light', 'Light'],
						['dark', 'Dark']
					] as const,
					scheme,
					(s) => (scheme = s as Scheme)
				)}
			{/if}
			<div class="history" role="group" aria-label="History">
				<button
					type="button"
					class="icon"
					title="Undo"
					aria-label="Undo"
					disabled={!history?.canUndo}
					aria-keyshortcuts={keymap.aria('theme.undo')}
					onclick={undo}><Undo2 size={17} /></button
				>
				<button
					type="button"
					class="icon"
					title="Redo"
					aria-label="Redo"
					disabled={!history?.canRedo}
					aria-keyshortcuts={keymap.aria('theme.redo')}
					onclick={redo}><Redo2 size={17} /></button
				>
			</div>
			<span class="spacer"></span>
			{#if !phone}{@render statusChip()}{@render useButton()}{/if}
			<button type="button" class="icon" title="More" aria-label="More" popovertarget="editor-more"
				><Ellipsis size={18} /></button
			>
			{#if phone}
				<!-- A phone: the state of the theme and its use, under the bar (the
				     app's own add button sits over the bottom right). -->
				<div class="actions">
					{@render statusChip()}<span class="spacer"></span>{@render useButton()}
				</div>
			{/if}
			<div class="menu" popover="auto" id="editor-more" use:anchored>
				<button type="button" class="option" onclick={more(duplicate)}>Duplicate</button>
				<button type="button" class="option" onclick={more(() => download(doc))}>Export file</button
				>
				<button type="button" class="option" onclick={more(() => copy(doc))}>Copy as text</button>
				<button
					type="button"
					class="option"
					disabled={!history?.changed}
					onclick={more(() => {
						history?.revert();
						announce('Back to how it was when you opened it');
					})}>Revert to when I opened it</button
				>
				<button
					type="button"
					class="option"
					onclick={more(() => {
						change(resetAll(doc));
						announce(`Everything is back to ${BASE_NAMES[doc.base]}`);
					})}>Reset all to {BASE_NAMES[doc.base]}</button
				>
				<button type="button" class="option danger" onclick={more(remove)}>Delete</button>
			</div>
		</header>

		{#if !floors}
			<p class="banner" id="use-why">
				<TriangleAlert size={16} aria-hidden="true" />
				{#if appearance.notApplied === id}Not applied:{/if} text or the focus ring would be unreadable:
				each needs 3:1 on the page in light and dark. {inUse
					? 'The app keeps the last readable version until you fix it.'
					: 'Fix that to use this theme.'}
			</p>
		{/if}
		{#if themes.notSaved[id]}
			<p class="banner">Not saved: {themes.notSaved[id].message}</p>
		{/if}
		{#if ready && !stored}
			<p class="banner">This theme was deleted. Change anything, or undo, to keep it.</p>
		{/if}

		{#if problemsOpen}
			<section class="problems" id="problems" aria-labelledby="problems-heading">
				<div class="problems-head">
					<h2 id="problems-heading" tabindex="-1">Contrast</h2>
					<label class="goal">
						<span>Contrast goal</span>
						<select
							value={doc.target}
							onchange={(e) => change({ ...doc, target: e.currentTarget.value as Target })}
						>
							{#each TARGETS as t (t)}<option value={t}>{t}</option>{/each}
						</select>
					</label>
				</div>
				{#if !values}
					<p class="hint">Checking the contrast.</p>
				{:else if !total}
					<p class="hint">Everything reads at {target}, in light and dark.</p>
				{/if}
				{#each SCHEMES as s (s)}
					{#if found[s].length}
						<h3>{s === 'light' ? 'Light' : 'Dark'}</h3>
						<ul>
							{#each found[s] as f, i (i)}
								<li>
									<button
										type="button"
										class="problem"
										onclick={() => void goToToken(f.pair.fg, s)}
									>
										{f.pair.label} on {labelOf(f.bg)}: {ratio(f.ratio)}; {target} needs {f.need}
									</button>
								</li>
							{/each}
						</ul>
					{/if}
				{/each}
			</section>
		{/if}

		<div class="body">
			{#if wide}
				<nav class="rail" aria-label="Parts of the theme">
					{@render finder()}
					<ul>
						{#each GROUPS as g (g.id)}
							{@const changes = changedIn(doc, g.id, scheme)}
							{@const issues = found[scheme].filter((f) => groupOf(f.pair.fg) === g.id).length}
							<li>
								<button
									type="button"
									id="group-{g.id}"
									aria-current={!query && group === g.id ? 'true' : undefined}
									onclick={() => pickGroup(g.id)}
								>
									<span class="group-name">{g.label}</span>
									{#if changes}<span class="count">{changes} changed</span>{/if}
									{#if issues}<span class="count warn"
											>{issues} {issues === 1 ? 'problem' : 'problems'}</span
										>{/if}
								</button>
							</li>
						{/each}
					</ul>
				</nav>
				{@render stage()}
				{@render settings()}
			{:else}
				{@render stage()}
				<div class="narrow">
					{@render finder()}
					<nav class="chips" aria-label="Parts of the theme">
						{#each GROUPS as g (g.id)}
							{@const changes = changedIn(doc, g.id, scheme)}
							<button
								type="button"
								id="group-{g.id}"
								class="chip"
								aria-current={!query && group === g.id ? 'true' : undefined}
								onclick={() => pickGroup(g.id)}
								>{g.label}{#if changes}<span class="chip-count"
										><span aria-hidden="true">·</span>
										{changes}<span class="visually-hidden"> changed</span></span
									>{/if}</button
							>
						{/each}
					</nav>
					{@render settings()}
				</div>
			{/if}
		</div>
	</div>
{/if}

{#snippet statusChip()}
	<button
		type="button"
		class="status"
		class:warn={total > 0}
		aria-expanded={problemsOpen}
		aria-controls="problems"
		onclick={async () => {
			problemsOpen = !problemsOpen;
			// Into the list, so the next Tab reaches the first problem.
			if (problemsOpen) {
				await tick();
				document.getElementById('problems-heading')?.focus();
			}
		}}
	>
		{#if total}<TriangleAlert size={14} aria-hidden="true" />{:else}<Check
				size={14}
				aria-hidden="true"
			/>{/if}
		{status}
	</button>
{/snippet}

{#snippet useButton()}
	{#if inUse}
		<span class="in-use"><Check size={14} aria-hidden="true" />In use</span>
	{:else}
		<Button
			size="sm"
			variant="primary"
			disabled={!floors}
			aria-describedby={floors ? undefined : 'use-why'}
			onclick={() => void use()}>Use this theme</Button
		>
	{/if}
{/snippet}

{#snippet finder()}
	<label class="find">
		<Search size={14} aria-hidden="true" />
		<span class="visually-hidden">Find a setting</span>
		<input
			type="search"
			placeholder="Find a setting"
			bind:value={query}
			onkeydown={(e) => {
				// Clears the search, and nothing under it.
				if (e.key === 'Escape' && query) {
					e.preventDefault();
					query = '';
				}
			}}
		/>
	</label>
{/snippet}

{#snippet stage()}
	{#if doc}
		<section class="stage" class:closed={!previewOpen} aria-labelledby="preview-heading">
			<div class="stage-head">
				<h2 id="preview-heading" class:visually-hidden={wide}>Preview</h2>
				{#if previewOpen}
					{@render segmented(
						'Shows',
						'preview-part',
						[
							['part', query ? 'This part' : groupLabel(group)],
							['all', 'Whole app']
						] as const,
						wholeApp || query ? 'all' : 'part',
						(v) => (wholeApp = v === 'all')
					)}
					{#if wide}
						{@render segmented(
							'Scheme',
							'preview-scheme',
							[
								['light', 'Light'],
								['dark', 'Dark'],
								['both', 'Side by side']
							] as const,
							show,
							(v) => (show = v as typeof show)
						)}
					{:else if phone}
						{@render segmented(
							'Editing',
							'editing-scheme-phone',
							[
								['light', 'Light'],
								['dark', 'Dark']
							] as const,
							scheme,
							(s) => (scheme = s as Scheme)
						)}
					{/if}
					<Button title="Play the theme's animations in the preview" size="sm" onclick={play}
						>Play motion</Button
					>
				{/if}
				<span class="spacer"></span>
				{#if wide}
					<span class="hint">Click anything in the preview to find its setting</span>
				{:else}
					<Button size="sm" variant="ghost" onclick={() => (previewOpen = !previewOpen)}
						>{previewOpen ? 'Hide preview' : 'Show preview'}</Button
					>
				{/if}
			</div>
			{#if previewOpen}
				<div class="previews" class:pair={shown.length > 1}>
					{#each shown as s (s)}
						<ThemePreview
							bind:this={previews[s]}
							theme={doc}
							scheme={s}
							accentHue={hueFor(doc)}
							caption={shown.length > 1 ? (s === 'light' ? 'Light' : 'Dark') : undefined}
							part={wholeApp || query ? null : group}
							onpick={pickFromPreview}
						/>
					{/each}
				</div>
			{/if}
		</section>
	{/if}
{/snippet}

{#snippet settings()}
	{#if doc}
		<section class="settings" aria-labelledby="group-heading">
			<div class="group-head">
				<h2 id="group-heading" tabindex="-1">
					{query ? `Settings matching ${query.trim()}` : groupLabel(group)}
				</h2>
				<span class="scheme-note">{scheme === 'light' ? 'light' : 'dark'}</span>
				<span class="spacer"></span>
				<Checkbox
					label="Changed only"
					checked={changedOnly}
					onchange={(on) => (changedOnly = on)}
				/>
				{#if !query}
					<Button
						size="sm"
						variant="ghost"
						disabled={!changedIn(doc, group, 'light') &&
							!changedIn(doc, group, 'dark') &&
							!(group === 'accent' && Object.keys(doc.accent).length)}
						onclick={resetGroup}>Reset {groupLabel(group)}</Button
					>
				{/if}
			</div>

			{#if !query && group === 'accent'}
				<fieldset class="knob">
					<legend>How the accent is made</legend>
					<label class="choice">
						<input type="radio" name="accent-mode" checked={!manual(doc)} onchange={generated} />
						Worked out from a hue, readable on every surface
					</label>
					<label class="choice">
						<input type="radio" name="accent-mode" checked={manual(doc)} onchange={byHand} />
						Set by hand, for light and dark
					</label>
				</fieldset>
				{#if !manual(doc)}
					<div class="knob">
						<label for="accent-hue">Accent hue</label>
						<input
							id="accent-hue"
							type="range"
							min="0"
							max="359"
							value={accentHue}
							aria-valuetext="Accent hue {Math.round(accentHue)}"
							oninput={(e) =>
								change({ ...doc, accent: { hue: Number(e.currentTarget.value) } }, false)}
							onchange={(e) => change({ ...doc, accent: { hue: Number(e.currentTarget.value) } })}
						/>
						<Button
							size="sm"
							variant="ghost"
							disabled={!('hue' in doc.accent)}
							onclick={() => change({ ...doc, accent: {} })}
							>Use {BASE_NAMES[doc.base]}'s hue</Button
						>
						{#if personHue !== null}
							<p class="hint">
								Your own accent hue in Appearance is in use, so it shows here instead. Set it back
								to Theme default there to see this one.
							</p>
						{/if}
					</div>
				{/if}
			{/if}

			{#if !query && SCALES[group]}
				{@const sc = SCALES[group]}
				{@const ref = sc.names.includes('space-4') ? 'space-4' : 'type-base'}
				{@const pct = Math.round(factorOf(ref) * 100)}
				<div class="knob">
					<label for="scale-{group}">{sc.label}</label>
					<input
						id="scale-{group}"
						type="range"
						min="50"
						max="200"
						step="5"
						value={pct}
						aria-valuetext="{sc.label} {pct} percent"
						oninput={(e) => scaleBy(sc.names, Number(e.currentTarget.value) / 100, false)}
						onchange={(e) => scaleBy(sc.names, Number(e.currentTarget.value) / 100, true)}
					/>
					<span class="readout">{pct}%</span>
				</div>
			{/if}

			{#if !query && group === 'motion'}
				{@const f = factorOf('dur-panel')}
				<fieldset class="knob">
					<legend>Speed</legend>
					{#each SPEEDS as sp (sp.id)}
						<label class="choice">
							<input
								type="radio"
								name="motion-speed"
								checked={Math.abs(f - sp.factor) < 0.03}
								onchange={() => scaleBy(DURATIONS, sp.factor, true)}
							/>
							{sp.label}
						</label>
					{/each}
				</fieldset>
				<p class="hint">
					When your device asks for less motion, Mila keeps things still whatever the theme says.
				</p>
			{/if}

			<div class="rows">
				{#each rows as meta (meta.name)}
					<TokenRow
						{meta}
						value={valueOf(doc, meta.name, scheme)}
						resolved={values?.[scheme].get(meta.name)}
						baseName={BASE_NAMES[doc.base]}
						locked={!manual(doc) && (ACCENT_TOKENS as readonly string[]).includes(meta.name)
							? LOCKED
							: undefined}
						measures={measures.filter((m) => m.pair.fg === meta.name)}
						under={found[scheme].filter((f) => bgNames(f.bg).includes(meta.name))}
						{target}
						expanded={expanded === meta.name}
						part={query ? groupLabel(meta.group) : undefined}
						ontoggle={() => (expanded = expanded === meta.name ? null : meta.name)}
						onchange={setToken(meta.name)}
						onfix={() => fixToken(meta.name)}
					/>
				{:else}
					<p class="hint empty">
						{query ? 'No setting matches that.' : 'This theme changes nothing here yet.'}
					</p>
				{/each}
			</div>
		</section>
	{/if}
{/snippet}

<style>
	/* The editor fills the page below the app's own bar; wide, its three
	   columns scroll each on their own. */
	.editor {
		display: flex;
		flex-direction: column;
		height: 100%;
		min-height: 0;
	}
	.toolbar {
		display: flex;
		flex: none;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2) var(--space-3);
		padding: var(--space-2) var(--space-4);
		border-bottom: var(--border-width) solid var(--border);
	}
	.spacer {
		flex: 1;
	}
	.wide .toolbar {
		flex-wrap: nowrap;
	}
	.actions {
		display: flex;
		flex-basis: 100%;
		align-items: center;
		gap: var(--space-2);
	}
	.back {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-1);
		min-width: max(var(--tap-min), 28px);
		min-height: max(var(--tap-min), 28px);
		padding: 0 var(--space-1);
		color: var(--accent-text);
		font-size: var(--text-sm);
		font-weight: 550;
		text-decoration: none;
	}
	.phone .back .word {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}
	/* The name takes what room the bar has, up to a point. */
	.name {
		flex: 0 1 240px;
		min-width: 0;
	}
	.phone .name {
		flex: 1 1 0;
	}
	.name input {
		width: 100%;
		height: calc(34px * var(--control-scale));
		padding: 0 var(--space-2);
		border: var(--border-width) solid transparent;
		border-radius: var(--input-radius);
		background: transparent;
		color: var(--input-text);
		font-family: var(--font-heading);
		font-size: var(--text-lg);
		font-weight: var(--heading-weight);
	}
	.name input:hover {
		border-color: var(--input-border);
	}
	.name input:focus {
		border-color: var(--accent);
		background: var(--input-bg);
		outline: none;
	}
	.based {
		font-size: var(--text-sm);
		color: var(--text-secondary);
		white-space: nowrap;
	}
	.history {
		display: flex;
	}
	.icon {
		display: inline-grid;
		place-items: center;
		width: max(var(--tap-min), 32px);
		height: max(var(--tap-min), 32px);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.icon:hover:not(:disabled) {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	.icon:disabled {
		color: var(--text-muted);
		cursor: not-allowed;
	}
	.status {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-height: max(var(--tap-min), 30px);
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-full);
		background: var(--surface-1);
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-weight: 550;
		cursor: pointer;
	}
	.status.warn {
		border-color: var(--danger);
		color: var(--danger);
	}
	.in-use {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		font-size: var(--text-sm);
		font-weight: 600;
		color: var(--text-secondary);
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
	.menu .option:disabled {
		color: var(--text-muted);
		cursor: not-allowed;
	}
	.menu .option.danger {
		color: var(--danger);
	}
	.segmented {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		margin: 0;
		padding: 0;
		border: none;
	}
	.segmented legend {
		float: left;
		margin-right: var(--space-2);
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.segmented div {
		display: inline-flex;
		padding: 2px;
		border-radius: var(--radius-md);
		background: var(--surface-2);
	}
	.segmented label {
		display: inline-flex;
		align-items: center;
		min-height: max(var(--tap-min), 26px);
		padding: 0 var(--space-3);
		border-radius: calc(var(--radius-md) - 2px);
		font-size: var(--text-sm);
		color: var(--text-secondary);
		cursor: pointer;
	}
	.segmented label.active {
		background: var(--surface-0);
		color: var(--text-primary);
		box-shadow: var(--shadow-sm);
		font-weight: 550;
	}
	.segmented label:has(:focus-visible) {
		box-shadow: var(--focus-ring);
	}
	.banner {
		display: flex;
		flex: none;
		align-items: center;
		gap: var(--space-2);
		margin: var(--space-2) var(--space-4) 0;
		padding: var(--space-2) var(--space-3);
		border: var(--border-width) solid var(--danger);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
	}
	.problems {
		display: grid;
		flex: none;
		gap: var(--space-2);
		max-height: 40vh;
		margin: var(--space-2) var(--space-4) 0;
		padding: var(--space-3);
		overflow-y: auto;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface-1);
	}
	.problems-head {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}
	.problems h2 {
		font-size: var(--text-md);
	}
	.problems h2:focus-visible {
		outline: none;
		box-shadow: none;
	}
	.problems h3 {
		font-size: var(--text-sm);
	}
	.problems ul {
		display: grid;
		gap: 2px;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.goal {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	select {
		min-height: max(var(--tap-min), calc(28px * var(--control-scale)));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
		font-size: var(--text-sm);
	}
	.problem {
		width: 100%;
		min-height: max(var(--tap-min), 28px);
		padding: var(--space-1) var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-primary);
		font-size: var(--text-sm);
		text-align: left;
		text-decoration: underline;
		cursor: pointer;
	}
	.problem:hover {
		background: var(--surface-2);
	}
	.hint {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}

	/* The body: wide, parts | preview | settings; else the preview on top. */
	.body {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
	}
	.wide .body {
		display: grid;
		grid-template-columns: 200px minmax(0, 1fr) minmax(340px, 400px);
		overflow: hidden;
	}
	.wide .body > * {
		min-height: 0;
		overflow-y: auto;
	}
	.rail {
		display: grid;
		align-content: start;
		gap: var(--space-2);
		padding: var(--space-3) var(--space-2);
		border-right: var(--border-width) solid var(--border);
		background: var(--surface-1);
	}
	.rail ul {
		display: grid;
		gap: 1px;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.rail button {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 2px var(--space-2);
		width: 100%;
		min-height: max(var(--tap-min), calc(30px * var(--control-scale)));
		padding: var(--space-1) var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-primary);
		font-size: var(--text-sm);
		text-align: left;
		cursor: pointer;
	}
	.rail button:hover {
		background: var(--surface-2);
	}
	.rail button[aria-current='true'] {
		background: var(--accent-soft);
		font-weight: 600;
	}
	.group-name {
		flex: 1;
	}
	.count {
		font-size: var(--text-xs);
		color: var(--text-secondary);
		font-weight: 400;
	}
	.count.warn {
		color: var(--danger);
		font-weight: 600;
	}
	.find {
		min-width: 0;
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: max(var(--tap-min), calc(32px * var(--control-scale)));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--text-secondary);
	}
	.find:focus-within {
		border-color: var(--accent);
	}
	.find input {
		flex: 1;
		width: 100%;
		min-width: 0;
		border: none;
		background: none;
		color: var(--input-text);
		font-size: var(--text-sm);
		outline: none;
	}

	/* The preview: the middle of the page, wide; on top, narrower. */
	.stage {
		display: grid;
		align-content: start;
		gap: var(--space-3);
		padding: var(--space-3) var(--space-4);
		background: var(--surface-2);
	}
	.stage-head {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2) var(--space-3);
	}
	.stage-head h2 {
		font-size: var(--text-md);
	}
	.previews {
		display: grid;
		gap: var(--space-3);
	}
	.previews.pair {
		grid-template-columns: repeat(2, minmax(0, 1fr));
	}
	/* Narrower, the preview stays in view above the settings, a part of
	   the screen high, and scrolls within that. */
	.editor:not(.wide) .stage {
		position: sticky;
		top: 0;
		z-index: 2;
		max-height: 45vh;
		overflow-y: auto;
		border-bottom: var(--border-width) solid var(--border);
	}
	.editor.phone .stage {
		max-height: 33vh;
	}
	/* A setting gone to stops clear of the preview over it. */
	.editor:not(.wide) .settings :global(.token) {
		scroll-margin-top: calc(45vh + var(--space-3));
	}
	.editor.phone .settings :global(.token) {
		scroll-margin-top: calc(33vh + var(--space-3));
	}
	.editor:not(.wide) .stage.closed {
		max-height: none;
	}
	.narrow {
		display: grid;
		gap: var(--space-3);
		padding: var(--space-3) var(--space-4);
	}
	.chips {
		display: flex;
		gap: var(--space-2);
		overflow-x: auto;
		padding-bottom: var(--space-1);
	}
	.chip {
		flex: none;
		min-height: max(var(--tap-min), 32px);
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-full);
		background: var(--surface-0);
		color: var(--text-primary);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.chip[aria-current='true'] {
		border-color: var(--accent);
		background: var(--accent-soft);
		font-weight: 600;
	}
	.chip-count {
		color: var(--text-secondary);
		font-weight: 400;
	}

	/* The settings: a row each. */
	.settings {
		display: grid;
		align-content: start;
		gap: var(--space-2);
		min-width: 0;
	}
	.wide .settings {
		padding: var(--space-3) var(--space-3) var(--space-6);
		border-left: var(--border-width) solid var(--border);
	}
	.group-head {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		padding-bottom: var(--space-2);
		border-bottom: var(--border-width) solid var(--border);
	}
	.group-head h2 {
		font-size: var(--text-lg);
	}
	.group-head h2:focus-visible {
		outline: none;
		box-shadow: none;
	}
	.scheme-note {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.rows {
		display: grid;
	}
	.empty {
		padding: var(--space-3) var(--space-2);
	}
	.knob {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2) var(--space-3);
		margin: 0;
		padding: var(--space-2);
		border: none;
		border-bottom: var(--border-width) solid var(--border);
		font-size: var(--text-sm);
	}
	fieldset.knob {
		display: grid;
	}
	.knob legend,
	.knob > label {
		font-weight: 600;
	}
	.knob legend {
		float: left;
		margin-bottom: var(--space-1);
	}
	.knob input[type='range'] {
		flex: 1;
		min-width: 120px;
		min-height: max(var(--tap-min), 20px);
		accent-color: var(--accent);
	}
	.knob .hint {
		flex-basis: 100%;
	}
	.choice {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: max(var(--tap-min), 28px);
		cursor: pointer;
	}
	.choice input {
		accent-color: var(--accent);
	}
	.readout {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.missing {
		display: grid;
		gap: var(--space-3);
		padding: var(--space-5);
	}
	.missing .back {
		justify-self: start;
	}
	@media (max-width: 720px) {
		.toolbar {
			padding: var(--space-1) var(--space-2);
		}
		.banner,
		.problems {
			margin: var(--space-2) var(--space-3) 0;
		}
		.stage {
			padding: var(--space-2) var(--space-3);
		}
		/* Room to scroll the last setting clear of the app's add button. */
		.narrow {
			padding: var(--space-2) var(--space-3) calc(var(--space-7) + 56px);
		}
	}
</style>
