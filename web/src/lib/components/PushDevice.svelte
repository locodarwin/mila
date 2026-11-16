<script lang="ts">
	import type { PushSupport } from '../push';
	import Button from './Button.svelte';
	import Checkbox from './Checkbox.svelte';

	interface Props {
		/** What this browser can do; null while that is being found out. */
		support: PushSupport | null;
		/** Whether push is on in this browser. */
		on?: boolean;
		/** How many of the person's devices have push on. */
		devices: number;
		/** Turning push on or off is under way. */
		busy?: boolean;
		/** A test push is under way. */
		testing?: boolean;
		ontoggle: (on: boolean) => void;
		ontest: () => void;
	}

	let {
		support,
		on = $bindable(false),
		devices,
		busy = false,
		testing = false,
		ontoggle,
		ontest
	}: Props = $props();

	const blocked: Record<Exclude<PushSupport, 'available'>, string> = {
		insecure:
			'Push needs a secure connection. Open Mila at its https:// address, or at localhost on the computer that runs it, to turn it on here.',
		unsupported:
			'This browser cannot receive push notifications. On an iPhone or iPad, add Mila to the Home Screen (Share, then Add to Home Screen) and turn push on from there.',
		denied:
			'Notifications are blocked for Mila in this browser. Allow them in the site settings, then reload this page.'
	};

	const reason = $derived(support && support !== 'available' ? blocked[support] : undefined);
	const hint = $derived(
		support === null
			? 'Checking this browser…'
			: (reason ??
					(on
						? 'Pushes reach this device even when Mila is closed.'
						: 'Your browser asks for permission when you turn it on.'))
	);
	// The checkbox and button are disabled while their work runs, and a
	// control that is disabled loses focus to the page. Put it back after.
	let root = $state<HTMLDivElement>();
	let returnTo: 'box' | 'test' | null = null;

	function remember(control: 'box' | 'test') {
		returnTo = root?.contains(document.activeElement) ? control : null;
	}

	$effect(() => {
		if (busy || testing || !root || !returnTo) return;
		const control = returnTo;
		returnTo = null;
		if (document.activeElement && document.activeElement !== document.body) return;
		const box = root.querySelector<HTMLInputElement>('input[type="checkbox"]');
		const test = root.querySelector<HTMLButtonElement>('.test button');
		const target = control === 'test' && test && !test.disabled ? test : box;
		if (target && !target.disabled) target.focus();
	});

	const count = $derived(
		devices === 0
			? 'Push is not on for any of your devices.'
			: `Push is on for ${devices} of your devices.`
	);
</script>

<div class="push" bind:this={root}>
	<Checkbox
		label="Push on this device"
		bind:checked={on}
		disabled={support !== 'available' || busy}
		title={reason}
		onchange={(checked) => {
			remember('box');
			ontoggle(checked);
		}}
	/>
	<p class="hint" class:blocked={reason}>{hint}</p>
	<div class="test">
		<Button
			onclick={() => {
				remember('test');
				ontest();
			}}
			disabled={devices === 0 || testing}>Send a test push</Button
		>
		<span class="hint">{count}</span>
	</div>
</div>

<style>
	.push {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		/* The checkbox's input is positioned absolutely. */
		position: relative;
	}
	.hint {
		margin: 0;
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.blocked {
		color: var(--text-primary);
	}
	.test {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-3);
		align-items: center;
		margin-top: var(--space-2);
	}
</style>
