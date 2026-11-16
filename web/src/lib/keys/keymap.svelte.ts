// The live keymap: each command's keys as the person has them, what works
// right now, and how to show and say a key. Everything that shows a key
// reads it here, never from COMMANDS, so it follows the person's changes
// and the single-key setting.
import { isApple } from '../device';
import { preferences } from '../preferences.svelte';
import { changed, effective } from './bindings';
import {
	aria,
	characterKey,
	format,
	label,
	spoken,
	steps,
	type Binding,
	type KeyLike
} from './chord';
import { command, type CommandId, type Scope } from './commands';
import { buildIndex, Matcher } from './match';

/** The words for a list: "A", "A and B", "A, B and C". */
export function list(items: string[]): string {
	return items.length < 2
		? (items[0] ?? '')
		: `${items.slice(0, -1).join(', ')} and ${items.at(-1)}`;
}

const lower = (s: string) => s.charAt(0).toLowerCase() + s.slice(1);

class Keymap {
	readonly apple = isApple();
	#map = $derived(effective(preferences.current.shortcuts));
	#index = $derived(buildIndex(this.#map, preferences.current.singleKeyShortcuts));
	#matcher = new Matcher(() => Date.now());

	/** Each command's changeable keys as they stand. */
	get map(): Map<CommandId, Binding[]> {
		return this.#map;
	}

	/** A command's keys: its changeable ones, then its fixed ones. */
	keys(id: CommandId): Binding[] {
		const c = command(id);
		return [...new Set([...(this.#map.get(id) ?? []), ...(c.fixed ?? [])])];
	}

	/** Whether a key is switched off by single-key shortcuts being off. */
	off(id: CommandId, b: Binding): boolean {
		return (
			command(id).scope === 'anywhere' && !preferences.current.singleKeyShortcuts && characterKey(b)
		);
	}

	/** The keys of a command that work now. */
	active(id: CommandId): Binding[] {
		return this.keys(id).filter((b) => !this.off(id, b));
	}

	/** Whether the person changed a command's keys. */
	changed(id: CommandId): boolean {
		return changed(this.#map, id);
	}

	/** The first key that works now, as keycaps for Kbd; null when none does. */
	hint(id: CommandId): string[][] | null {
		const b = this.active(id)[0];
		return b ? format(b, this.apple) : null;
	}

	/** The first key that works now as a short label ("Ctrl+K", "G I"); null when none does. */
	label(id: CommandId): string | null {
		const b = this.active(id)[0];
		return b ? label(b, this.apple) : null;
	}

	/** The first key that works now in words, for prose: "T", "Ctrl+K", "G then I". */
	say(id: CommandId): string | null {
		const b = this.active(id)[0];
		return b ? spoken(b, this.apple) : null;
	}

	/** aria-keyshortcuts for a command's keys that work now; sequences cannot be said there. */
	aria(id: CommandId): string | undefined {
		const all = this.active(id)
			.map((b) => aria(b, this.apple))
			.filter((a) => a !== null);
		return all.length ? all.join(' ') : undefined;
	}

	/**
	 * A sentence on the keys that move something, for hidden help text:
	 * "Alt and arrow keys move this one." while they are the defaults,
	 * else each key and what it does.
	 */
	describe(ids: CommandId[], what: string): string {
		const defaults = ids.every(
			(id) => !this.changed(id) && command(id).keys.every((b) => b.startsWith('Alt+Arrow'))
		);
		if (defaults) return `Alt and arrow keys ${what}.`;
		const parts = ids.flatMap((id) => {
			const key = this.say(id);
			return key ? [`${key} to ${lower(command(id).name)}`] : [];
		});
		return parts.length ? `${parts.join(', ')}.` : '';
	}

	/**
	 * The character keys anywhere that single-key shortcuts being off
	 * turns off, in words: "/, Q, ? and the G keys".
	 */
	characterKeys(): string {
		const singles: string[] = [];
		const starts: Record<string, string[]> = {};
		for (const [id, keys] of this.#map) {
			if (command(id).scope !== 'anywhere') continue;
			for (const b of keys.filter(characterKey)) {
				const s = steps(b);
				if (s.length === 1) singles.push(spoken(b, this.apple));
				else (starts[s[0]] ??= []).push(spoken(b, this.apple));
			}
		}
		const sequences = Object.entries(starts).map(([first, all]) =>
			all.length > 1 ? `the ${spoken(first, this.apple)} keys` : all[0]
		);
		return list([...new Set(singles), ...sequences]);
	}

	/** The command a press runs in a scope, if any; see Matcher.match. */
	match(scope: Scope, e: KeyLike, ctx?: { field?: boolean; handled?: boolean }): CommandId | null {
		return this.#matcher.match(this.#index, scope, e, ctx);
	}
}

export const keymap = new Keymap();
