// Why a key cannot go to a command, or what it would take from another:
// for Settings, Keyboard only, so it loads with that page and not the app.
import { assign, clash, effective, fits, fixedOf, type Overrides } from './bindings';
import { spoken, steps, type Binding } from './chord';
import { COMMANDS, command, scope, type CommandId, type Scope } from './commands';

export interface Conflict {
	kind: 'reserved' | 'fixed' | 'field' | 'sequence' | 'same-scope' | 'overlap' | 'prefix';
	/** The command that has the key now. */
	other?: CommandId;
	message: string;
}

const ALL = 'all' as const;
const SHELL: Scope[] = ['anywhere', 'panel'];
/** The theme editor's keys, heard in the capture phase wherever focus is. */
const EDITOR: CommandId[] = ['theme.undo', 'theme.redo'];
/** The app's own undo, which the editor's stands in for on purpose. */
const EDITOR_YIELDS: CommandId[] = ['undo'];

/**
 * Keys Mila never takes: ones it needs itself, ones the browser or the
 * system acts on before the page hears them, and ones kept for the
 * browser's own use. Arrows, Space and paging scroll the page, so they
 * stay out of the shell's scopes; a focused row, card or entry may use
 * them, since its handler takes them.
 */
export const RESERVED: { chord: string; why: string; scopes: Scope[] | typeof ALL }[] = [
	{ chord: 'Escape', why: 'closes things', scopes: ALL },
	{ chord: 'Tab', why: 'moves between controls', scopes: ALL },
	{ chord: 'Shift+Tab', why: 'moves between controls', scopes: ALL },
	{ chord: 'Enter', why: 'opens and confirms', scopes: ALL },
	...['w', 'Shift+w'].map((k) => ({
		chord: `Mod+${k}`,
		why: 'closes the browser tab',
		scopes: ALL
	})),
	...['t', 'Shift+t', 'n', 'Shift+n'].map((k) => ({
		chord: `Mod+${k}`,
		why: 'opens a browser tab or window',
		scopes: ALL
	})),
	...['Mod+Tab', 'Mod+Shift+Tab', 'Alt+Tab'].map((chord) => ({
		chord,
		why: 'switches tabs or windows',
		scopes: ALL
	})),
	{ chord: 'Mod+q', why: 'quits the browser', scopes: ALL },
	{ chord: 'Alt+F4', why: 'closes the window', scopes: ALL },
	...['Mod+h', 'Mod+m'].map((chord) => ({
		chord,
		why: 'hides or minimizes the window on a Mac',
		scopes: ALL
	})),
	...[
		'Mod+l',
		'Mod+r',
		'Mod+Shift+r',
		'F5',
		'Mod+f',
		'Mod+p',
		'Mod+d',
		...Array.from({ length: 10 }, (_, i) => `Mod+${i}`),
		'Mod+=',
		'Mod++',
		'Mod+-',
		'F6',
		'F7',
		'F11',
		'F12',
		'Mod+Shift+i',
		'Mod+Shift+j',
		'Mod+Shift+c',
		'Alt+d',
		'Alt+e',
		'Alt+f',
		'Mod+c',
		'Mod+v',
		'Mod+x',
		'Mod+a'
	].map((chord) => ({ chord, why: 'is kept for the browser', scopes: ALL })),
	...['Alt+ArrowLeft', 'Alt+ArrowRight'].map((chord) => ({
		chord,
		why: 'goes back or forward in the browser',
		scopes: SHELL
	})),
	...[
		'Space',
		'Shift+Space',
		'ArrowUp',
		'ArrowDown',
		'ArrowLeft',
		'ArrowRight',
		'PageUp',
		'PageDown',
		'Home',
		'End'
	].map((chord) => ({ chord, why: 'scrolls the page', scopes: SHELL }))
];

const lower = (s: string) => s.charAt(0).toLowerCase() + s.slice(1);
const where = (s: Scope) => lower(scope(s).name);

/**
 * What stands in the way of giving a command a key, most serious first:
 * a reserved key, a fixed key (its own or another command's), a key that
 * cannot work for it (a plain key where a field would take it, a sequence
 * on a focused task), then another command's key in the same scope, in an overlapping one, or
 * one that starts or is started by the other's sequence. The map is each
 * command's keys as they stand, without the key being replaced.
 */
export function conflicts(
	map: Map<CommandId, Binding[]>,
	id: CommandId,
	b: Binding,
	apple = false
): Conflict[] {
	const me = command(id);
	const say = (x: Binding) => spoken(x, apple);
	const out: Conflict[] = [];

	for (const r of RESERVED) {
		const hit = b === r.chord || steps(b).includes(r.chord);
		if (hit && (r.scopes === ALL || r.scopes.includes(me.scope))) {
			out.push({ kind: 'reserved', message: `${say(r.chord)} ${r.why}; Mila can't use it.` });
			break;
		}
	}
	if (fixedOf(id).includes(b))
		out.push({
			kind: 'fixed',
			other: id,
			message: `${say(b)} is always one of “${me.name}”'s keys already.`
		});
	for (const c of COMMANDS) {
		if (!('fixed' in c) || c.id === id) continue;
		const fixed: readonly Binding[] = c.fixed;
		// The theme editor hears its undo keys before anything else, so
		// they take a key from any command of the shell while it is open.
		const editor = EDITOR.includes(c.id) && SHELL.includes(me.scope) && !EDITOR_YIELDS.includes(id);
		const theirs = editor ? me.scope : c.scope;
		if (fixed.some((f) => clash({ scope: me.scope, b }, { scope: theirs, b: f }) !== null)) {
			out.push({
				kind: 'fixed',
				other: c.id,
				message: editor
					? `${say(b)} is always ${c.id === 'theme.undo' ? 'Undo' : 'Redo'} in the theme editor, so “${me.name}” would not work there; that can't change.`
					: `${say(b)} is always “${c.name}” ${where(c.scope)}, and that can't change.`
			});
			break;
		}
	}
	if (!fits(id, b)) {
		if ('inFields' in me)
			out.push({
				kind: 'field',
				message: `${me.name} works while you type, so its key needs ${apple ? 'Cmd' : 'Ctrl'}, ${apple ? 'Option' : 'Alt'} or an F key.`
			});
		else
			out.push({
				kind: 'sequence',
				message: `Two keys in turn work only anywhere, not ${where(me.scope)}.`
			});
	}

	const found = { same: [] as CommandId[], overlap: [] as CommandId[], prefix: [] as CommandId[] };
	for (const [other, keys] of map) {
		if (other === id) continue;
		for (const k of keys) {
			const kind = clash({ scope: me.scope, b }, { scope: command(other).scope, b: k });
			if (kind && !found[kind].includes(other)) found[kind].push(other);
		}
	}
	for (const other of found.same) {
		out.push({
			kind: 'same-scope',
			other,
			message: `${say(b)} already does “${command(other).name}” ${where(me.scope)}.`
		});
	}
	for (const other of found.overlap) {
		const theirs = command(other);
		const [narrow, broad] = me.scope === 'anywhere' ? [theirs, me] : [me, theirs];
		out.push({
			kind: 'overlap',
			other,
			message: `${scope(narrow.scope).name}, ${say(b)} would do “${narrow.name}” instead of “${broad.name}”.`
		});
	}
	if (found.prefix.length) {
		const [first, ...more] = found.prefix;
		const theirs = map
			.get(first)!
			.find(
				(k) => clash({ scope: me.scope, b }, { scope: command(first).scope, b: k }) === 'prefix'
			)!;
		const message =
			steps(b).length > 1
				? `${say(steps(b)[0])} already does “${command(first).name}”, so it can't start a sequence.`
				: `${say(b)} starts “${command(first).name}” (${say(theirs)})${more.length ? ` and ${more.length} more` : ''}.`;
		out.push({ kind: 'prefix', other: first, message });
	}
	return out;
}

/**
 * Whether a swap would leave the command that had the key with a key it
 * can have: the key being replaced goes to it, so it is asked of that
 * command as of any other. Empty when the swap is fine; otherwise only a
 * move (it loses the key and gets nothing) is offered.
 */
export function swapConflicts(
	map: Map<CommandId, Binding[]>,
	a: { id: CommandId; old: Binding },
	b: { id: CommandId; key: Binding },
	apple = false
): Conflict[] {
	const after = new Map(map);
	after.set(
		a.id,
		(map.get(a.id) ?? []).map((k) => (k === a.old ? b.key : k))
	);
	after.set(
		b.id,
		(map.get(b.id) ?? []).filter((k) => k !== b.key)
	);
	return conflicts(after, b.id, a.old, apple);
}

/**
 * The overrides after a key moves from one command to another. With give,
 * the command that had it gets the key being replaced in its place, if
 * there is one (a swap); otherwise it simply loses it (a move).
 */
export function swap(
	overrides: Overrides,
	a: { id: CommandId; old?: Binding },
	b: { id: CommandId; key: Binding },
	give = true
): Overrides {
	const map = effective(overrides);
	const mine = map.get(a.id) ?? [];
	const theirs = map.get(b.id) ?? [];
	const nextMine =
		a.old && mine.includes(a.old)
			? mine.map((k) => (k === a.old ? b.key : k))
			: [...mine.filter((k) => k !== b.key), b.key];
	const nextTheirs =
		a.old && give
			? theirs.map((k) => (k === b.key ? a.old! : k))
			: theirs.filter((k) => k !== b.key);
	return assign(assign(overrides, b.id, [...new Set(nextTheirs)]), a.id, nextMine);
}
