<script lang="ts">
	import { resolve } from '$app/paths';
	import { appearance, CHARACTERS, type Character } from '../../theme/appearance.svelte';
	import { presets } from '../../theme/presets.svelte';
	import { themes } from '../../theme/themes.svelte';
	import { workspaceThemes, type Offered } from '../../theme/workspaceThemes.svelte';
	import { toasts } from '../../toasts.svelte';
	import { uuid } from '../../uuid';
	import ThemeCard from './ThemeCard.svelte';
	import { BASE_NAMES } from './names';

	// The theme: the four built-ins, those made from Omarchy's, the
	// workspace's, then the person's own, as a group of radio buttons drawn
	// as swatches. With preview (the style guide) a choice shows on this
	// page only.
	let { preview = false }: { preview?: boolean } = $props();

	// Two pickers on one page are two groups.
	const group = `theme-${uuid()}`;

	$effect(() => {
		if (!preview) void themes.ensure();
		void presets.ensure().catch(() => {});
	});

	const current = $derived(appearance.custom ? appearance.custom.id : appearance.current.character);
	const scheme = $derived(appearance.scheme);
	const hue = $derived(appearance.current.accentHue);

	// Whether each custom theme can be used, and whether one aiming for AAA
	// gets there: worked out once the serializer has loaded.
	let status = $state<Record<string, { floors: string | null; belowAAA: boolean }>>({});
	$effect(() => {
		const list = themes.list;
		const accentHue = hue;
		if (!list.length) return;
		let stale = false;
		void import('../../theme/custom').then(({ floorsOf, problems }) => {
			if (stale) return;
			const out: typeof status = {};
			for (const t of list) {
				const floors = floorsOf(t, { accentHue });
				let belowAAA = false;
				if (!floors && t.target === 'AAA') {
					try {
						const p = problems(t, { accentHue });
						belowAAA = p.light.length + p.dark.length > 0;
					} catch {
						// unreadable: floorsOf says so
					}
				}
				out[t.id] = { floors, belowAAA };
			}
			status = out;
		});
		return () => {
			stale = true;
		};
	});

	// The workspace in view's themes, and the one in use if another
	// workspace offers it (or offered it), alone under that one's name.
	const offered = $derived.by((): Offered[] => {
		if (preview) return [];
		const out = workspaceThemes.offered ? [workspaceThemes.offered] : [];
		const k = workspaceThemes.kept;
		if (k && k.doc.id === appearance.current.customTheme && !out.some((g) => g.ws === k.ws))
			out.push({
				ws: k.ws,
				wsName: k.wsName,
				list: [
					{
						id: k.doc.id,
						name: k.doc.name,
						body: k.doc,
						sourceId: null,
						suggestedAt: null,
						publishedBy: null,
						publishedAt: null,
						sortKey: ''
					}
				]
			});
		return out;
	});

	let fieldset = $state<HTMLFieldSetElement>();

	/** Puts the radio buttons back on what is in use, after a choice that did not take. */
	function restore() {
		for (const input of fieldset?.querySelectorAll<HTMLInputElement>('input[type="radio"]') ?? [])
			input.checked = input.value === current;
	}

	function chooseBuiltin(character: Character) {
		if (preview) appearance.preview({ character, doc: null });
		else appearance.set({ character });
	}

	async function chooseCustom(id: string) {
		if (preview) {
			appearance.preview({ doc: themes.get(id) ?? presets.get(id) ?? null });
			return;
		}
		const why = await (presets.get(id) ? presets.use(id) : themes.use(id));
		if (why) {
			toasts.error(why);
			restore();
		}
	}
</script>

<fieldset class="picker" bind:this={fieldset}>
	<legend>Theme</legend>
	<p class="sub">Built in</p>
	<div class="cards">
		{#each CHARACTERS as character (character)}
			<label class="card" class:active={current === character}>
				<input
					type="radio"
					name={group}
					value={character}
					checked={current === character}
					onchange={() => chooseBuiltin(character)}
					class="visually-hidden"
				/>
				<ThemeCard base={character} {scheme} accentHue={hue} />
				<span class="name">{BASE_NAMES[character]}</span>
			</label>
		{/each}
	</div>
	{#if presets.list.length}
		<p class="sub">From Omarchy</p>
		<div class="cards">
			{#each presets.list as p (p.doc.id)}
				<label class="card" class:active={current === p.doc.id}>
					<input
						type="radio"
						name={group}
						value={p.doc.id}
						checked={current === p.doc.id}
						onchange={() => void chooseCustom(p.doc.id)}
						class="visually-hidden"
					/>
					<ThemeCard base={p.doc.base} doc={p.doc} {scheme} accentHue={hue} />
					<span class="name">{p.doc.name}</span>
					{#if p.mode !== 'both'}
						<span class="note">{p.mode === 'dark' ? 'Dark' : 'Light'}</span>
					{/if}
				</label>
			{/each}
		</div>
	{/if}
	{#if offered.length}
		{#await import('./WorkspacePicks.svelte') then { default: WorkspacePicks }}
			{#each offered as g (g.ws)}
				<WorkspacePicks
					{group}
					ws={g.ws}
					wsName={g.wsName}
					list={g.list}
					{current}
					{scheme}
					{hue}
					onrefused={restore}
				/>
			{/each}
		{/await}
	{/if}
	{#if themes.list.length}
		<p class="sub">Your themes</p>
		<div class="cards">
			{#each themes.list as theme (theme.id)}
				{@const s = status[theme.id]}
				<label class="card" class:active={current === theme.id} class:off={!!s?.floors}>
					<input
						type="radio"
						name={group}
						value={theme.id}
						checked={current === theme.id}
						aria-describedby={s?.floors || s?.belowAAA ? `${group}-${theme.id}` : undefined}
						onchange={() => void chooseCustom(theme.id)}
						class="visually-hidden"
					/>
					<ThemeCard base={theme.base} doc={theme} {scheme} accentHue={hue} />
					<span class="name">{theme.name}</span>
					{#if s?.floors}
						<span class="note" id="{group}-{theme.id}"
							>Cannot be used: text would be unreadable</span
						>
					{:else if s?.belowAAA}
						<span class="note" id="{group}-{theme.id}">Below AAA</span>
					{/if}
				</label>
			{/each}
		</div>
	{/if}
	<a class="manage" href={resolve('/(app)/settings/appearance')}>Manage themes</a>
</fieldset>

<style>
	.picker {
		display: grid;
		gap: var(--space-2);
		min-width: 0;
		margin: 0;
		padding: 0;
		border: none;
	}
	legend {
		margin-bottom: var(--space-1);
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
		gap: var(--space-2);
	}
	.card {
		position: relative;
		display: grid;
		align-content: start;
		gap: var(--space-1);
		padding: var(--space-1);
		border: 2px solid transparent;
		border-radius: var(--radius-lg);
		cursor: pointer;
	}
	.card:hover {
		background: var(--surface-1);
	}
	.card.active {
		border-color: var(--accent);
	}
	.card:has(:focus-visible) {
		box-shadow: var(--focus-ring);
	}
	.card.off {
		cursor: not-allowed;
	}
	.name {
		overflow: hidden;
		font-size: var(--text-sm);
		font-weight: 550;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.active .name {
		color: var(--text-primary);
	}
	.note {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.sub {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.manage {
		justify-self: start;
		color: var(--accent-text);
		font-size: var(--text-sm);
		font-weight: 550;
	}
</style>
