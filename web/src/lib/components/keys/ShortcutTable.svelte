<script lang="ts">
	import { isDesktop } from '../../desktop';
	import { COMMANDS, SCOPES, command, type CommandId, type Scope } from '../../keys/commands';
	import { keymap } from '../../keys/keymap.svelte';
	import { score } from '../../search';
	import Kbd from '../Kbd.svelte';

	interface Props {
		/** The scopes to show, in SCOPES order; every one when left out. */
		scopes?: Scope[];
		/** The ? sheet's form: smaller headings, and no rows for commands without a key. */
		compact?: boolean;
		/** Only the commands whose names match this. */
		query?: string;
	}

	let { scopes, compact = false, query = '' }: Props = $props();
	const q = $derived(query.trim().toLowerCase());

	// What each command's keys are now: the person's, with the defaults
	// shown only beside a changed one, marked as such.
	const groups = $derived(
		SCOPES.filter((s) => (!scopes || scopes.includes(s.id)) && (s.id !== 'system' || isDesktop()))
			.map((s) => ({
				...s,
				rows: COMMANDS.filter((c) => c.scope === s.id)
					.map((c) => ({
						id: c.id as CommandId,
						name: c.name,
						help: compact ? undefined : command(c.id).help,
						keys: keymap.keys(c.id)
					}))
					.filter((r) => (!compact || r.keys.length) && score(r.name, q) > 0)
			}))
			.filter((g) => g.rows.length)
	);
</script>

<div class="tables" class:compact>
	{#each groups as g (g.id)}
		<!-- Help search links to a scope's table (#keys-list), which takes focus
		     there; -1 only, never a Tab stop. -->
		<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
		<table id={compact ? undefined : `keys-${g.id}`} tabindex={compact ? undefined : -1}>
			<caption>{g.name}</caption>
			<!-- The same widths in every table, so their columns line up. -->
			<colgroup><col /><col class="keys-col" /></colgroup>
			<thead>
				<tr><th scope="col">Action</th><th scope="col">Keys</th></tr>
			</thead>
			<tbody>
				{#each g.rows as row (row.id)}
					<tr>
						<th scope="row">
							{row.name}
							{#if keymap.changed(row.id)}<span class="tag">Changed</span>{/if}
						</th>
						<td>
							<div class="keys">
								{#each row.keys as b, i (b)}
									{#if i}<span class="or">or</span>{/if}
									{#if keymap.off(row.id, b)}<span class="off">Off</span>{:else}<Kbd
											binding={b}
										/>{/if}
								{:else}
									<span class="off">No key</span>
								{/each}
							</div>
							{#if !compact && keymap.changed(row.id)}
								{@const defaults = command(row.id).keys}
								<div class="default">
									default: {#each defaults as b, i (b)}{#if i}<span class="or">or</span>{/if}<Kbd
											binding={b}
										/>{:else}no key{/each}
								</div>
							{/if}
						</td>
					</tr>
					<!-- A row of its own, so the sentence is not read again as the
					     header of the keys cell. -->
					{#if row.help}
						<tr class="note"><td colspan="2" class="about">{row.help}</td></tr>
					{/if}
				{/each}
			</tbody>
		</table>
	{/each}
</div>

<style>
	.tables {
		position: relative;
		display: grid;
		gap: var(--space-4);
	}
	table {
		width: 100%;
		table-layout: fixed;
		border-collapse: collapse;
		scroll-margin-top: var(--space-4);
	}
	/* Focused only when a link lands on it; the caption marks the place. */
	table:focus-visible {
		outline: none;
		box-shadow: none;
	}
	table:focus-visible caption {
		color: var(--text-primary);
		text-decoration: underline;
	}
	caption {
		padding-bottom: var(--space-1);
		font-family: var(--font-ui);
		font-weight: 600;
		text-align: left;
		color: var(--text-secondary);
	}
	.keys-col {
		width: 35%;
	}
	th,
	td {
		padding: var(--space-1) 0;
		text-align: left;
		vertical-align: middle;
	}
	thead th {
		border-bottom: var(--border-width) solid var(--border);
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	tbody th {
		padding-right: var(--space-4);
		font-weight: 400;
	}
	.keys {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-1) var(--space-2);
	}
	.note td {
		padding-top: 0;
	}
	tr:has(+ .note) > * {
		padding-bottom: 0;
	}
	.about {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.default {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-1) var(--space-2);
		margin-top: var(--space-1);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.or,
	.off {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.tag {
		margin-left: var(--space-2);
		padding: 1px var(--space-2);
		border-radius: var(--radius-full);
		background: var(--accent-soft);
		font-size: var(--text-xs);
		font-weight: 600;
	}
	/* The sheet: the scope names stand as headings, without column heads. */
	.compact caption {
		font-size: var(--text-sm);
	}
	.compact thead {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}
	@media (max-width: 480px) {
		/* Rows stack here, so the shared column widths would only narrow them. */
		table {
			table-layout: auto;
		}
		.keys-col {
			width: auto;
		}
		tbody tr {
			display: grid;
			gap: var(--space-1);
			padding: var(--space-1) 0;
		}
		tbody th,
		td {
			padding: 0;
		}
	}
</style>
