// The keys each command has: the defaults in COMMANDS, with the person's
// changes (preferences.current.shortcuts) on top. Only changes are
// stored, so a default changed in a later version still reaches everyone
// who kept it.
import { parse, steps, valid, type Binding } from './chord';
import { COMMANDS, command, overlapping, type CommandId, type Scope } from './commands';

/** The person's changed keys, by command id; [] leaves a command without one. */
export type Overrides = Record<string, Binding[]>;

const sameList = (a: readonly Binding[], b: readonly Binding[]) =>
	a.length === b.length && a.every((x, i) => x === b[i]);

/**
 * Whether two keys in two scopes get in each other's way: the same key in
 * one scope, the same key in scopes that overlap, or one key starting the
 * other's sequence where both are heard.
 */
export function clash(
	a: { scope: Scope; b: Binding },
	b: { scope: Scope; b: Binding }
): 'same' | 'overlap' | 'prefix' | null {
	const together = a.scope === b.scope || overlapping(a.scope, b.scope);
	if (!together) return null;
	if (a.b === b.b) return a.scope === b.scope ? 'same' : 'overlap';
	const sa = steps(a.b);
	const sb = steps(b.b);
	if (
		(sa.length === 2 && sb.length === 1 && sa[0] === sb[0]) ||
		(sb.length === 2 && sa.length === 1 && sb[0] === sa[0])
	)
		return 'prefix';
	return null;
}

/**
 * Whether a key can work for a command at all: a sequence only anywhere
 * (a focused scope has no second key to wait for), and a command that
 * works while typing only with Mod, Alt or an F key, since a plain key
 * belongs to the field. Fixed keys are the registry's and always fit.
 */
export function fits(id: CommandId, b: Binding): boolean {
	const c = command(id);
	const s = steps(b);
	if (s.length > 1 && c.scope !== 'anywhere') return false;
	if ('inFields' in c) {
		const p = parse(s[0]);
		return s.length === 1 && (p.mod || p.alt || /^F\d+$/.test(p.key));
	}
	return true;
}

/** A command's fixed keys. */
export function fixedOf(id: CommandId): readonly Binding[] {
	return command(id).fixed ?? [];
}

/**
 * Each command's keys as they stand: the defaults with the overrides on
 * top. Unknown ids, bindings that are not valid, keys that cannot work
 * for the command (fits) and its own fixed keys are passed over (and left
 * in storage, for a newer build on another device). A default that
 * has the same key as one the person chose in the same scope, or starts
 * or is started by it, is dropped, so a default added later never takes a
 * key someone picked; the same key in an overlapping scope stays, since
 * the narrower scope wins where they meet (Settings, Keyboard's "Keep
 * both"). Any duplicates left go to the command first in COMMANDS.
 * Help-only commands have none: only fixed keys.
 */
export function effective(overrides: Overrides): Map<CommandId, Binding[]> {
	const map = new Map<CommandId, Binding[]>();
	const chosen: { scope: Scope; b: Binding }[] = [];
	for (const c of COMMANDS) {
		const o = Object.hasOwn(overrides, c.id) ? overrides[c.id] : undefined;
		if ('remappable' in c || !Array.isArray(o)) continue;
		// A key that cannot work for it, or that it always has, is passed over.
		const list = [
			...new Set(
				o.filter(
					(b) => typeof b === 'string' && valid(b) && fits(c.id, b) && !fixedOf(c.id).includes(b)
				)
			)
		];
		map.set(c.id, list);
		chosen.push(...list.map((b) => ({ scope: c.scope as Scope, b })));
	}
	for (const c of COMMANDS) {
		if ('remappable' in c) map.set(c.id, []);
		if (map.has(c.id)) continue;
		map.set(
			c.id,
			c.keys.filter((b) =>
				chosen.every((k) => {
					const kind = clash({ scope: c.scope, b }, k);
					return kind !== 'same' && kind !== 'prefix';
				})
			)
		);
	}
	// In the order of COMMANDS, which is the order they are shown.
	const out = new Map<CommandId, Binding[]>();
	const taken: { scope: Scope; b: Binding }[] = [];
	for (const c of COMMANDS) {
		const kept = map.get(c.id)!.filter((b) => {
			const mine = { scope: c.scope as Scope, b };
			if (taken.some((k) => clash(mine, k) === 'same')) return false;
			taken.push(mine);
			return true;
		});
		out.set(c.id, kept);
	}
	return out;
}

/**
 * The overrides with one command's keys set. An entry equal to the
 * command's defaults is removed, so later changes to the defaults still
 * reach the person. Ids are kept in sorted order, as the server writes
 * them, so a change that only reorders is no change; the keys' own order
 * is kept, since the first is the one hints show.
 */
export function assign(overrides: Overrides, id: CommandId, keys: Binding[]): Overrides {
	const next: Overrides = { ...overrides };
	if (sameList(keys, command(id).keys)) delete next[id];
	else next[id] = [...keys];
	return Object.fromEntries(
		Object.keys(next)
			.sort()
			.map((k) => [k, next[k]])
	);
}

/** Whether a command's keys differ from its defaults. */
export function changed(map: Map<CommandId, Binding[]>, id: CommandId): boolean {
	return !sameList(map.get(id) ?? [], command(id).keys);
}

/**
 * The overrides with a command back on its defaults. A default another
 * command's chosen key now holds (after a swap, say) would stay dropped,
 * so that command goes back to its defaults too, and so on until every
 * command reset has all its defaults; also names the others reset.
 */
export function reset(overrides: Overrides, id: CommandId): { next: Overrides; also: CommandId[] } {
	let next = assign(overrides, id, [...command(id).keys]);
	const done: CommandId[] = [id];
	for (;;) {
		const map = effective(next);
		const short = done.filter((d) => !sameList(map.get(d) ?? [], command(d).keys));
		if (!short.length) break;
		const holders = COMMANDS.filter(
			(c) =>
				Object.hasOwn(next, c.id) &&
				!done.includes(c.id) &&
				short.some((d) =>
					command(d).keys.some((b) =>
						(map.get(c.id) ?? []).some((k) => {
							const kind = clash({ scope: command(d).scope, b }, { scope: c.scope, b: k });
							return kind === 'same' || kind === 'prefix';
						})
					)
				)
		);
		if (!holders.length) break;
		for (const h of holders) {
			next = assign(next, h.id, [...h.keys]);
			done.push(h.id);
		}
	}
	return { next, also: done.slice(1) };
}
