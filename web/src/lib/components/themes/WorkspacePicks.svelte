<script lang="ts">
	import type { WorkspaceTheme } from '../../sync/entities';
	import type { Scheme } from '../../theme/color';
	import { floorsOf } from '../../theme/custom';
	import { use } from '../../theme/workspaceActions';
	import { suggestion, workspaceDoc } from '../../theme/workspaceThemes.svelte';
	import { toasts } from '../../toasts.svelte';
	import ThemeCard from './ThemeCard.svelte';

	// The theme picker's group for one workspace's themes, between the
	// built-ins and the person's own: radio buttons of the picker's group,
	// loaded only when a workspace offers some.
	interface Props {
		/** The picker's radio group. */
		group: string;
		ws: string;
		wsName: string;
		list: WorkspaceTheme[];
		current: string;
		scheme: Scheme;
		hue: number | null;
		/** Puts the picker's choice back after one that did not take. */
		onrefused: () => void;
	}

	let { group, ws, wsName, list, current, scheme, hue, onrefused }: Props = $props();

	const suggested = $derived(suggestion(list)?.id ?? null);
	const entries = $derived(
		list.map((t) => {
			const doc = workspaceDoc(t);
			return { t, doc, floors: doc ? floorsOf(doc, { accentHue: hue }) : 'Cannot be read' };
		})
	);

	function choose(t: WorkspaceTheme) {
		const why = use(ws, t);
		if (why) {
			toasts.error(why);
			onrefused();
		}
	}
</script>

<p class="sub">From {wsName}</p>
<div class="cards">
	{#each entries as { t, doc, floors } (t.id)}
		<label class="card" class:active={current === t.id} class:off={!!floors}>
			<input
				type="radio"
				name={group}
				value={t.id}
				checked={current === t.id}
				aria-describedby={floors || suggested === t.id ? `${group}-${t.id}` : undefined}
				onchange={() => choose(t)}
				class="visually-hidden"
			/>
			<ThemeCard base={doc?.base ?? 'standard'} {doc} {scheme} accentHue={hue} />
			<span class="name">{t.name}</span>
			{#if floors}
				<span class="note" id="{group}-{t.id}">Cannot be used: text would be unreadable</span>
			{:else if suggested === t.id}
				<span class="note" id="{group}-{t.id}">Suggested</span>
			{/if}
		</label>
	{/each}
</div>

<style>
	/* The picker's own look: ThemePicker.svelte draws its other groups the same. */
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
	.note {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.sub {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
</style>
