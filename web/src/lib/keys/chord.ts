// What a key press is, and how a key binding is written.
//
// A binding is a string: a chord, [Mod+][Alt+][Shift+]KEY, or two plain
// keys pressed in turn, KEY KEY. KEY is one character (lowercase for
// letters: q, /, ?, 1, ü) or a key name as event.key gives it (ArrowUp,
// Enter, Delete, F2, Space). Mod is Ctrl, or Cmd on Apple devices. The
// server checks the same grammar (accounts/preferences.go), and
// shared/keybinding-vectors.json holds the two to it.

export type Binding = string;

/** The parts of a keydown event a binding is worked out from. */
export interface KeyLike {
	key: string;
	code: string;
	keyCode?: number;
	repeat?: boolean;
	ctrlKey: boolean;
	metaKey: boolean;
	altKey: boolean;
	shiftKey: boolean;
	isComposing?: boolean;
	getModifierState?(k: string): boolean;
}

const KEY = '(?:[^\\t\\n\\v\\f\\r ]|[A-Z][A-Za-z0-9]{1,11})';
const GRAMMAR = new RegExp(`^(?:(?:Mod\\+)?(?:Alt\\+)?(?:Shift\\+)?${KEY}|${KEY} ${KEY})$`, 'u');

/** Whether a binding is written as bindings are. */
export function valid(b: string): boolean {
	return GRAMMAR.test(b);
}

/** Keys that are no key on their own: modifiers, and what an IME or a dead key sends. */
const NOT_A_KEY = new Set([
	'Shift',
	'Control',
	'Alt',
	'Meta',
	'OS',
	'AltGraph',
	'CapsLock',
	'Fn',
	'FnLock',
	'Hyper',
	'Super',
	'Symbol',
	'SymbolLock',
	'NumLock',
	'ScrollLock',
	'Dead',
	'Process',
	'Unidentified'
]);

const chars = (s: string) => [...s].length;
const isLetter = (c: string) => c.toLowerCase() !== c.toUpperCase();

/**
 * The binding a key press makes, or null for a press that is no key of
 * its own (a lone modifier, a dead key, IME composition). The same rules
 * apply when a binding is recorded and when a press is matched.
 */
export function chordOf(e: KeyLike): string | null {
	if (e.isComposing || e.keyCode === 229) return null;
	if (!e.key || NOT_A_KEY.has(e.key)) return null;
	// AltGr (which Windows reports as Ctrl and Alt) types a character: the
	// character is the key, with no modifiers.
	const altGraph = e.getModifierState?.('AltGraph') ?? false;
	const mod = !altGraph && (e.ctrlKey || e.metaKey);
	const alt = !altGraph && e.altKey;
	let shift = e.shiftKey;
	let key = e.key;
	const letter = /^Key([A-Z])$/.exec(e.code)?.[1];
	const digit = /^Digit([0-9])$/.exec(e.code)?.[1];
	const asciiLetterOrDigit = /^[A-Za-z0-9]$/.test(key);
	if (e.code === 'Space' || key === ' ') key = 'Space';
	else if (chars(key) === 1 && /[^\x20-\x7e]/u.test(key) && letter && !alt && !altGraph) {
		// A Cyrillic, Greek or Hebrew layout: the Latin letter on the same
		// key. Not a character typed with AltGr (Polish ź on X), which is
		// that character.
		key = letter;
	} else if (alt && !asciiLetterOrDigit && (letter || digit)) {
		// Mac Option types a symbol (˚ for K): the key it is on. Where Alt
		// leaves the letter as printed (AZERTY A on Windows), it is that.
		key = letter ?? digit!;
	}
	if (chars(key) === 1) {
		// Letters are lowercase (Caps Lock), and keep Shift; Shift is how
		// other characters are typed (? is Shift and /), so it goes.
		if (isLetter(key)) key = key.toLowerCase();
		else shift = false;
	}
	const chord = `${mod ? 'Mod+' : ''}${alt ? 'Alt+' : ''}${shift ? 'Shift+' : ''}${key}`;
	return valid(chord) ? chord : null;
}

/** A binding's steps: one chord, or the two keys of a sequence. */
export function steps(b: Binding): string[] {
	return b.split(' ');
}

export interface Chord {
	mod: boolean;
	alt: boolean;
	shift: boolean;
	key: string;
}

/** A chord's modifiers and key. The key + is written Mod++. */
export function parse(chord: string): Chord {
	const out: Chord = { mod: false, alt: false, shift: false, key: chord };
	for (const m of ['mod', 'alt', 'shift'] as const) {
		const prefix = m === 'mod' ? 'Mod+' : m === 'alt' ? 'Alt+' : 'Shift+';
		if (out.key.length > prefix.length && out.key.startsWith(prefix)) {
			out[m] = true;
			out.key = out.key.slice(prefix.length);
		}
	}
	return out;
}

const SHOWN: Record<string, string> = {
	ArrowUp: '↑',
	ArrowDown: '↓',
	ArrowLeft: '←',
	ArrowRight: '→',
	Escape: 'Esc',
	PageUp: 'Page Up',
	PageDown: 'Page Down'
};

const SAID: Record<string, string> = {
	ArrowUp: 'Up arrow',
	ArrowDown: 'Down arrow',
	ArrowLeft: 'Left arrow',
	ArrowRight: 'Right arrow',
	PageUp: 'Page Up',
	PageDown: 'Page Down'
};

const capOf = (key: string) => SHOWN[key] ?? (chars(key) === 1 ? key.toUpperCase() : key);

/** The keycaps of each step, for Kbd: [['⌘','K']], [['Ctrl','K']], [['G'],['I']]. */
export function format(b: Binding, apple: boolean): string[][] {
	return steps(b).map((s) => {
		const c = parse(s);
		const caps: string[] = [];
		if (c.mod) caps.push(apple ? '⌘' : 'Ctrl');
		if (c.alt) caps.push(apple ? '⌥' : 'Alt');
		if (c.shift) caps.push(apple ? '⇧' : 'Shift');
		caps.push(capOf(c.key));
		return caps;
	});
}

/** A binding as one short label: "Ctrl+K", "⌘K", "G I". */
export function label(b: Binding, apple: boolean): string {
	return format(b, apple)
		.map((caps) => caps.join(apple ? '' : '+'))
		.join(' ');
}

/** A binding in words, for prose and screen readers: "Ctrl+K", "G then I". */
export function spoken(b: Binding, apple: boolean): string {
	return steps(b)
		.map((s) => {
			const c = parse(s);
			const words: string[] = [];
			if (c.mod) words.push(apple ? 'Cmd' : 'Ctrl');
			if (c.alt) words.push(apple ? 'Option' : 'Alt');
			if (c.shift) words.push('Shift');
			words.push(SAID[c.key] ?? capOf(c.key));
			return words.join('+');
		})
		.join(' then ');
}

/** The binding for aria-keyshortcuts: "Control+K", "Meta+K"; null for a sequence, which it cannot say. */
export function aria(b: Binding, apple: boolean): string | null {
	if (steps(b).length > 1) return null;
	const c = parse(b);
	const parts: string[] = [];
	if (c.mod) parts.push(apple ? 'Meta' : 'Control');
	if (c.alt) parts.push('Alt');
	if (c.shift) parts.push('Shift');
	parts.push(chars(c.key) === 1 ? c.key.toUpperCase() : c.key);
	return parts.join('+');
}

/**
 * Whether a binding is made of character keys alone (WCAG 2.1.4): no step
 * has Mod or Alt, and each is one printable character or Space. These are
 * the keys single-key shortcuts being off turns off.
 */
export function characterKey(b: Binding): boolean {
	return steps(b).every((s) => {
		const c = parse(s);
		return !c.mod && !c.alt && (chars(c.key) === 1 || c.key === 'Space');
	});
}
