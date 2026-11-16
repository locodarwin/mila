import { describe, expect, it } from 'vitest';
import vectors from '../../../../shared/keybinding-vectors.json';
import {
	aria,
	characterKey,
	chordOf,
	format,
	label,
	spoken,
	steps,
	valid,
	type KeyLike
} from './chord';

/** A keydown as a test writes it: the key, and anything else that differs. */
function press(key: string, rest: Partial<KeyLike> = {}): KeyLike {
	const code =
		rest.code ??
		(/^[a-z]$/i.test(key)
			? `Key${key.toUpperCase()}`
			: /^[0-9]$/.test(key)
				? `Digit${key}`
				: key === ' '
					? 'Space'
					: key);
	return {
		key,
		code,
		ctrlKey: false,
		metaKey: false,
		altKey: false,
		shiftKey: false,
		...rest
	};
}

describe('chordOf', () => {
	it.each([
		['AZERTY q (on the A key) is q', press('q', { code: 'KeyA' }), 'q'],
		['Cyrillic й is q, the key it is on', press('й', { code: 'KeyQ' }), 'q'],
		['Mac Option œ is Alt+q', press('œ', { code: 'KeyQ', altKey: true }), 'Alt+q'],
		['Mac Option ˚ is Alt+k', press('˚', { code: 'KeyK', altKey: true }), 'Alt+k'],
		['? keeps no Shift', press('?', { code: 'Slash', shiftKey: true }), '?'],
		['? on AZERTY is the same', press('?', { code: 'KeyM', shiftKey: true }), '?'],
		['Shift and a letter keep Shift', press('Q', { shiftKey: true }), 'Shift+q'],
		['Caps Lock is no Shift', press('Q'), 'q'],
		['Ctrl+K', press('k', { ctrlKey: true }), 'Mod+k'],
		['Cmd+K', press('k', { metaKey: true }), 'Mod+k'],
		['Ctrl+K on a Russian layout', press('л', { code: 'KeyK', ctrlKey: true }), 'Mod+k'],
		['Ctrl+Shift+Z', press('Z', { ctrlKey: true, shiftKey: true }), 'Mod+Shift+z'],
		['Alt+Shift+ArrowUp', press('ArrowUp', { altKey: true, shiftKey: true }), 'Alt+Shift+ArrowUp'],
		[
			'AltGr @ is @',
			press('@', {
				code: 'KeyQ',
				ctrlKey: true,
				altKey: true,
				getModifierState: (k) => k === 'AltGraph'
			}),
			'@'
		],
		['Space', press(' '), 'Space'],
		['Mac Option and Space', press(' ', { code: 'Space', altKey: true }), 'Alt+Space'],
		['a named key', press('Delete'), 'Delete'],
		['Ctrl and +', press('+', { code: 'Equal', ctrlKey: true, shiftKey: true }), 'Mod++'],
		['German ü stays ü', press('ü', { code: 'BracketLeft' }), 'ü'],
		[
			'Polish AltGr ź stays ź, not X',
			press('ź', {
				code: 'KeyX',
				ctrlKey: true,
				altKey: true,
				getModifierState: (k) => k === 'AltGraph'
			}),
			'ź'
		],
		[
			'AZERTY Alt+A on Windows or Linux is Alt+a, not Alt+q',
			press('a', { code: 'KeyQ', altKey: true }),
			'Alt+a'
		],
		['Mac Option å is Alt+a', press('å', { code: 'KeyA', altKey: true }), 'Alt+a']
	])('%s', (_, e, want) => {
		expect(chordOf(e)).toBe(want);
		expect(valid(want)).toBe(true);
	});

	it.each([
		['a dead key', press('Dead', { code: 'Quote' })],
		['IME processing', press('Process', { code: 'KeyA' })],
		['composition', press('a', { isComposing: true })],
		["Safari's composition start", press('a', { keyCode: 229 })],
		['a lone Shift', press('Shift', { code: 'ShiftLeft', shiftKey: true })],
		['a lone Control', press('Control', { code: 'ControlLeft', ctrlKey: true })],
		['an unidentified key', press('Unidentified', { code: '' })],
		['a key name too long to store', press('AudioVolumeMute', { code: 'AudioVolumeMute' })]
	])('gives nothing for %s', (_, e) => {
		expect(chordOf(e)).toBeNull();
	});
});

describe('valid', () => {
	it('holds to the grammar the server checks (shared/keybinding-vectors.json)', () => {
		expect(vectors.good.length).toBeGreaterThan(10);
		for (const b of vectors.good) expect(valid(b), b).toBe(true);
		for (const b of vectors.bad) expect(valid(b), JSON.stringify(b)).toBe(false);
	});
});

describe('showing and saying keys', () => {
	it('formats keycaps on Apple devices and elsewhere', () => {
		expect(format('Mod+k', false)).toEqual([['Ctrl', 'K']]);
		expect(format('Mod+k', true)).toEqual([['⌘', 'K']]);
		expect(format('Alt+Shift+ArrowUp', false)).toEqual([['Alt', 'Shift', '↑']]);
		expect(format('Alt+ArrowUp', true)).toEqual([['⌥', '↑']]);
		expect(format('g i', false)).toEqual([['G'], ['I']]);
		expect(format('Mod++', false)).toEqual([['Ctrl', '+']]);
		expect(label('Mod+k', false)).toBe('Ctrl+K');
		expect(label('Mod+k', true)).toBe('⌘K');
		expect(label('g i', false)).toBe('G I');
	});

	it('says keys in words', () => {
		expect(spoken('Mod+k', false)).toBe('Ctrl+K');
		expect(spoken('Mod+k', true)).toBe('Cmd+K');
		expect(spoken('g i', false)).toBe('G then I');
		expect(spoken('Alt+ArrowUp', true)).toBe('Option+Up arrow');
		expect(spoken('t', false)).toBe('T');
		expect(spoken('?', false)).toBe('?');
	});

	it('writes aria-keyshortcuts, and nothing for a sequence', () => {
		expect(aria('Mod+k', false)).toBe('Control+K');
		expect(aria('Mod+k', true)).toBe('Meta+K');
		expect(aria('Alt+Shift+ArrowDown', false)).toBe('Alt+Shift+ArrowDown');
		expect(aria('q', false)).toBe('Q');
		expect(aria('g i', false)).toBeNull();
	});

	it('knows character keys (WCAG 2.1.4)', () => {
		for (const b of ['q', '?', 'Shift+t', 'g i', 'Space', '1'])
			expect(characterKey(b), b).toBe(true);
		for (const b of ['Mod+k', 'Alt+n', 'F2', 'Delete', 'ArrowDown', 'Alt+ArrowUp'])
			expect(characterKey(b), b).toBe(false);
		expect(steps('g i')).toEqual(['g', 'i']);
	});
});
