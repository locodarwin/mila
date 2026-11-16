// Which command a key press runs. Pure, with the clock passed in, so the
// window for a sequence's second key can be tested.
import { characterKey, chordOf, parse, steps, type Binding, type KeyLike } from './chord';
import { fits } from './bindings';
import { COMMANDS, command, type CommandId, type Scope } from './commands';

/** How long a sequence (G then I) waits for its second key. */
export const SEQUENCE_MS = 1200;

/** The keys that work now, by scope. */
export interface Index {
	keys: Map<Scope, Map<string, CommandId>>;
	/** The first keys of the sequences that work now. */
	starts: Set<string>;
}

/**
 * The keys that work now, from each command's keys (and fixed keys). With
 * single-key shortcuts off, character keys that work anywhere are left
 * out (WCAG 2.1.4); keys that work only on a focused task, card or entry
 * stay, as the exception for focused components allows. Sequences work
 * only anywhere, and help-only commands keep their handlers to themselves.
 */
export function buildIndex(map: Map<CommandId, Binding[]>, singleKeys: boolean): Index {
	const index: Index = { keys: new Map(), starts: new Set() };
	for (const c of COMMANDS) {
		if (c.scope === 'elsewhere') continue;
		const fixed: readonly Binding[] = 'fixed' in c ? c.fixed : [];
		for (const b of [...(map.get(c.id) ?? []), ...fixed]) {
			const sequence = steps(b).length > 1;
			// A key that cannot work here is never matched, whatever the map holds.
			if (!fixed.includes(b) && !fits(c.id, b)) continue;
			if (c.scope === 'anywhere' && !singleKeys && characterKey(b)) continue;
			let table = index.keys.get(c.scope);
			if (!table) index.keys.set(c.scope, (table = new Map()));
			if (!table.has(b)) table.set(b, c.id);
			if (sequence) index.starts.add(steps(b)[0]);
		}
	}
	return index;
}

/** The chord, then (for a chord without Mod) the same without Shift: Shift+Q still adds a task. */
function lenient(chord: string): string[] {
	const c = parse(chord);
	return !c.mod && c.shift ? [chord, chord.replace('Shift+', '')] : [chord];
}

export class Matcher {
	#now: () => number;
	#pending: { step: string; at: number } | null = null;

	constructor(now: () => number) {
		this.#now = now;
	}

	/**
	 * The command a press runs in a scope, or null. Focused scopes (a row,
	 * a card) are asked first, by their own handlers; anywhere is asked by
	 * the window's, which only it keeps a sequence's first key for.
	 * field: the press is typed into a field or an overlay, so only
	 * commands that work there count. handled: a focused handler took it.
	 */
	match(
		index: Index,
		scope: Scope,
		e: KeyLike,
		ctx: { field?: boolean; handled?: boolean } = {}
	): CommandId | null {
		const chord = chordOf(e);
		if (!chord) return null;
		const table = index.keys.get(scope);
		const lookup = (keys: string[]) => {
			for (const k of keys) {
				const id = table?.get(k);
				if (id) return id;
			}
			return null;
		};
		const allowed = (id: CommandId | null) =>
			id && (!e.repeat || 'repeat' in command(id)) ? id : null;
		const pending =
			this.#pending && this.#now() - this.#pending.at < SEQUENCE_MS ? this.#pending.step : null;
		const second = (step: string) => lenient(chord).map((k) => `${step} ${k}`);

		if (scope !== 'anywhere') {
			// The second key of a sequence belongs to the window's handler.
			const anywhere = index.keys.get('anywhere');
			if (pending && second(pending).some((k) => anywhere?.has(k))) return null;
			return allowed(lookup(lenient(chord)));
		}

		if (ctx.field || ctx.handled) {
			if (ctx.handled) this.#pending = null;
			const id = lookup([chord]);
			return id && 'inFields' in command(id) ? allowed(id) : null;
		}
		this.#pending = null;
		if (pending) {
			const id = lookup(second(pending));
			if (id) return allowed(id);
		}
		// The exact chord first (a Shift+G of its own), then without Shift.
		// A held start key keeps its sequence waiting.
		for (const k of lenient(chord)) {
			if (index.starts.has(k)) {
				this.#pending = { step: k, at: this.#now() };
				return null;
			}
			const id = lookup([k]);
			if (id) return allowed(id);
		}
		return null;
	}
}
