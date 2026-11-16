<script lang="ts">
	import type { Role } from '../api/client';

	interface Props {
		value: Role;
		label: string;
		/** Roles to offer; the caller leaves out what the viewer cannot grant. */
		roles?: Role[];
		disabled?: boolean;
		onchange?: (role: Role) => void;
	}

	let {
		value = $bindable(),
		label,
		roles = ['owner', 'admin', 'member', 'guest'],
		disabled = false,
		onchange
	}: Props = $props();

	const names: Record<Role, string> = {
		owner: 'Owner',
		admin: 'Admin',
		member: 'Member',
		guest: 'Guest'
	};
</script>

<select aria-label={label} bind:value {disabled} onchange={() => onchange?.(value)}>
	{#each roles as role (role)}
		<option value={role}>{names[role]}</option>
	{/each}
</select>

<style>
	select {
		height: calc(30px * var(--control-scale));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		font-size: var(--text-sm);
	}
	select:disabled {
		opacity: 0.6;
	}
</style>
