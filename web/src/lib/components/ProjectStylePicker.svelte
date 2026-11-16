<script lang="ts">
	import { PALETTE } from '../theme/palette';
	import ProjectIcon from './ProjectIcon.svelte';
	import { firstEmoji, iconSet, LUCIDE } from './project/iconSet.svelte';

	interface Props {
		/** The project's name, for the button's accessible name. */
		name: string;
		icon: string;
		color: string;
		onchange: (fields: { icon?: string; color?: string }) => void;
	}

	let { name, icon, color, onchange }: Props = $props();

	const id = $props.id();
	let trigger = $state<HTMLButtonElement>();
	let menu = $state<HTMLElement>();
	let emoji = $state('');
	const emojiIcon = $derived(icon && !icon.startsWith(LUCIDE) ? icon : '');

	// Positioned under the button: by CSS anchors where the browser has
	// them, else from the button's place on screen, kept inside it.
	function place(e: Event) {
		if ((e as ToggleEvent).newState !== 'open' || !menu || !trigger) return;
		void iconSet.load();
		emoji = emojiIcon;
		if (CSS.supports('anchor-name: --a')) return;
		const r = trigger.getBoundingClientRect();
		const width = menu.offsetWidth;
		menu.style.top = `${r.bottom + 4}px`;
		menu.style.left = `${Math.max(8, Math.min(r.left, innerWidth - width - 8))}px`;
	}

	function useEmoji() {
		const e = firstEmoji(emoji);
		if (e) onchange({ icon: e });
		else emoji = emojiIcon;
	}
</script>

<button
	bind:this={trigger}
	class="trigger"
	type="button"
	popovertarget={`${id}-menu`}
	data-project-style
	aria-label={`Icon and color of ${name}`}
	style:anchor-name={`--${id}`}
>
	<ProjectIcon {icon} {color} size={18} />
</button>

<div
	id={`${id}-menu`}
	class="menu"
	popover
	bind:this={menu}
	ontoggle={place}
	style:position-anchor={`--${id}`}
>
	<fieldset>
		<legend>Color</legend>
		<div class="swatches">
			{#each PALETTE as c (c)}
				<label class="swatch" title={c}>
					<input
						class="visually-hidden"
						type="radio"
						name={`${id}-color`}
						value={c}
						checked={color === c}
						aria-label={c}
						onchange={() => onchange({ color: c })}
					/>
					<span style:background={`var(--palette-${c})`}></span>
				</label>
			{/each}
		</div>
	</fieldset>

	<fieldset>
		<legend>Icon</legend>
		<div class="icons">
			<label class="choice" title="No icon">
				<input
					class="visually-hidden"
					type="radio"
					name={`${id}-icon`}
					checked={icon === ''}
					aria-label="No icon"
					onchange={() => onchange({ icon: '' })}
				/>
				<ProjectIcon icon="" {color} />
			</label>
			{#each Object.keys(iconSet.icons ?? {}) as n (n)}
				<label class="choice" title={n}>
					<input
						class="visually-hidden"
						type="radio"
						name={`${id}-icon`}
						checked={icon === LUCIDE + n}
						aria-label={n.replaceAll('-', ' ')}
						onchange={() => onchange({ icon: LUCIDE + n })}
					/>
					<ProjectIcon icon={LUCIDE + n} {color} />
				</label>
			{/each}
		</div>
		<form
			class="emoji"
			onsubmit={(e) => {
				e.preventDefault();
				useEmoji();
			}}
		>
			<label for={`${id}-emoji`}>Or an emoji</label>
			<input id={`${id}-emoji`} bind:value={emoji} autocomplete="off" onchange={useEmoji} />
		</form>
	</fieldset>
</div>

<style>
	.trigger {
		display: grid;
		flex: none;
		place-items: center;
		width: calc(30px * var(--ui-scale));
		height: calc(30px * var(--ui-scale));
		border: none;
		border-radius: var(--radius-md);
		background: none;
		cursor: pointer;
	}
	.trigger:hover {
		background: var(--surface-2);
	}
	.menu {
		position: fixed;
		inset: auto;
		width: min(296px, calc(100vw - 16px));
		max-height: min(420px, calc(100dvh - 16px));
		margin: var(--space-1) 0 0;
		padding: var(--space-3);
		overflow-y: auto;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface-0);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	@supports (anchor-name: --a) {
		.menu {
			top: anchor(bottom);
			left: anchor(left);
			position-try-fallbacks: flip-inline, flip-block;
		}
	}
	fieldset {
		display: grid;
		gap: var(--space-2);
		margin: 0 0 var(--space-3);
		padding: 0;
		border: none;
	}
	legend {
		margin-bottom: var(--space-2);
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.swatches,
	.icons {
		/* Holds the hidden radios, which are positioned absolutely: without a
		   positioned container they would stretch the page from inside a panel. */
		position: relative;
		display: grid;
		grid-template-columns: repeat(6, 1fr);
		gap: var(--space-1);
	}
	.icons {
		grid-template-columns: repeat(auto-fill, minmax(calc(32px * var(--ui-scale)), 1fr));
	}
	.swatch,
	.choice {
		display: grid;
		place-items: center;
		min-height: max(var(--tap-min), calc(32px * var(--ui-scale)));
		border: var(--border-width) solid transparent;
		border-radius: var(--radius-md);
		cursor: pointer;
	}
	.swatch span {
		width: calc(20px * var(--ui-scale));
		height: calc(20px * var(--ui-scale));
		border-radius: 50%;
	}
	.swatch:hover,
	.choice:hover {
		background: var(--surface-2);
	}
	.swatch:has(:checked),
	.choice:has(:checked) {
		border-color: var(--text-primary);
		background: var(--surface-2);
	}
	.swatch:has(:focus-visible),
	.choice:has(:focus-visible) {
		box-shadow: var(--focus-ring);
	}
	.emoji {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.emoji input {
		width: calc(64px * var(--ui-scale));
		height: calc(30px * var(--control-scale));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
	}
	.emoji input::placeholder {
		color: var(--input-placeholder);
	}
</style>
