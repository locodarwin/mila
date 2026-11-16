<script lang="ts">
	import { TARGETS, type Target } from '../../csvimport';
	import type { CustomField } from '../../sync/entities';

	interface Props {
		headers: string[];
		/** The first rows below the headings, to show what each column holds. */
		samples: string[][];
		fields: CustomField[];
		targets: Target[];
	}

	let { headers, samples, fields, targets = $bindable() }: Props = $props();

	const sample = (i: number) =>
		samples
			.map((r) => (r[i] ?? '').trim())
			.filter(Boolean)
			.slice(0, 2)
			.join(' · ');

	/** A target other than a field is taken by one column at a time. */
	function choose(i: number, t: Target) {
		targets = targets.map((x, j) =>
			j === i ? t : x === t && t !== 'skip' && !t.startsWith('field:') ? 'skip' : x
		);
	}
</script>

<table class="mapping">
	<caption class="visually-hidden">What each column of the file goes to</caption>
	<thead>
		<tr>
			<th scope="col">Column</th>
			<th scope="col">Holds, for example</th>
			<th scope="col">Goes to</th>
		</tr>
	</thead>
	<tbody>
		{#each headers as header, i (i)}
			<tr>
				<th scope="row">{header || `Column ${i + 1}`}</th>
				<td class="sample">{sample(i) || '(empty)'}</td>
				<td>
					<select
						aria-label="{header || `Column ${i + 1}`} goes to"
						value={targets[i]}
						onchange={(e) => choose(i, (e.currentTarget as HTMLSelectElement).value as Target)}
					>
						{#each TARGETS as t (t.value)}<option value={t.value}>{t.label}</option>{/each}
						{#if fields.length}
							<optgroup label="Fields">
								{#each fields as f (f.id)}<option value="field:{f.id}">{f.name}</option>{/each}
							</optgroup>
						{/if}
					</select>
				</td>
			</tr>
		{/each}
	</tbody>
</table>

<style>
	.mapping {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--text-sm);
	}
	th,
	td {
		padding: var(--space-2);
		border-bottom: 1px solid var(--border);
		text-align: left;
		vertical-align: middle;
	}
	thead th {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	tbody th {
		font-weight: 550;
		overflow-wrap: anywhere;
	}
	.sample {
		max-width: 16rem;
		overflow: hidden;
		color: var(--text-secondary);
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	select {
		width: 100%;
		min-width: 9rem;
		height: max(var(--tap-min), calc(32px * var(--control-scale)));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
	}
	@media (max-width: 560px) {
		.sample {
			display: none;
		}
		thead th:nth-child(2) {
			display: none;
		}
	}
</style>
