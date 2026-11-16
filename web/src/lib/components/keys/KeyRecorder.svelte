<script lang="ts">
	import { announce } from '../../announce.svelte';
	import { chordOf, format, parse, spoken, type Binding } from '../../keys/chord';
	import { keymap } from '../../keys/keymap.svelte';
	import { uuid } from '../../uuid';
	import Kbd from '../Kbd.svelte';

	interface Props {
		/** The command the key is for, as people read it. */
		name: string;
		/** Whether the command works anywhere, where two keys in turn make a sequence. */
		anywhere: boolean;
		onrecord: (b: Binding) => void;
		oncancel: () => void;
		/** Takes focus when it appears, as it does when a button starts it; not in the style guide. */
		focus?: boolean;
	}

	let { name, anywhere, onrecord: record, oncancel: cancel, focus = true }: Props = $props();

	// Once it has ended, focus leaving (to a conflict's buttons, say) is
	// no cancel.
	let ended = false;
	const onrecord = (b: Binding) => {
		ended = true;
		record(b);
	};
	const oncancel = () => {
		if (ended) return;
		ended = true;
		cancel();
	};

	// A text field, so screen readers switch to typing mode by themselves
	// and the app's own keys stay out of it; nothing is ever typed into it.
	let input = $state<HTMLInputElement>();
	// A plain key pressed for a command that works anywhere: Enter keeps it
	// alone, another plain key makes a sequence.
	let first = $state<string | null>(null);
	const id = `recorder-${uuid()}`;

	$effect(() => {
		if (focus) input?.focus();
	});

	const waiting = (chord: string) =>
		`Press Enter to use ${spoken(chord, keymap.apple)} alone, or another key for a sequence such as G I.`;

	const plain = (chord: string) => {
		const c = parse(chord);
		return !c.mod && !c.alt && !c.shift && [...c.key].length === 1;
	};

	function keydown(e: KeyboardEvent) {
		// Tab moves on as usual, and leaving cancels: no keyboard trap.
		if (e.key === 'Tab') return;
		// Every other key is the recorder's: not the palette's Ctrl+K, not
		// the open task's Escape.
		e.preventDefault();
		e.stopPropagation();
		if (e.repeat || e.isComposing || e.keyCode === 229) return;
		if (e.key === 'Escape') return oncancel();
		const chord = chordOf(e);
		if (!chord) return;
		if (first) {
			if (chord === 'Enter') return onrecord(first);
			const c = parse(chord);
			// A sequence's keys have no modifiers; one with Ctrl or Alt
			// starts again as a key of its own.
			if (c.mod || c.alt) return onrecord(chord);
			return onrecord(`${first} ${chord.replace('Shift+', '')}`);
		}
		if (anywhere && plain(chord)) {
			first = chord;
			// The prompt changes only in the field's description, which is
			// not read again by itself.
			announce(`${spoken(chord, keymap.apple)}. ${waiting(chord)}`);
		} else onrecord(chord);
	}
</script>

<span class="recorder">
	<span class="box">
		<input
			bind:this={input}
			class="field"
			aria-label={`Press the keys for ${name}`}
			aria-describedby={id}
			autocomplete="off"
			enterkeyhint="done"
			value=""
			oninput={(e) => (e.currentTarget.value = '')}
			onkeydown={keydown}
			onblur={() => oncancel()}
			placeholder={first ? '' : 'Press keys'}
		/>
		{#if first}<span class="steps" aria-hidden="true"
				><Kbd keys={format(first, keymap.apple)[0]} /></span
			>{/if}
	</span>
	<span class="instructions" {id}>
		{#if first}
			{waiting(first)}
		{:else if anywhere}
			Press a key or a combination. A single key waits: Enter keeps it alone, a second key makes a
			sequence. Escape cancels.
		{:else}
			Press a key or a combination. Escape cancels.
		{/if}
	</span>
</span>

<style>
	.recorder {
		position: relative;
		display: inline-flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
	.field {
		width: calc(120px * var(--ui-scale));
		height: max(var(--tap-min), calc(28px * var(--control-scale)));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
		font-size: var(--text-sm);
		caret-color: transparent;
	}
	.field::placeholder {
		color: var(--input-placeholder);
	}
	.box {
		position: relative;
		display: inline-flex;
		align-items: center;
	}
	.steps {
		position: absolute;
		left: var(--space-2);
		pointer-events: none;
	}
	.instructions {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
</style>
