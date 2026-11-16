// How links show and behave in the editor (docs/PLAN.md section 3): a
// click follows a link, and a link to a task or page in the app is
// followed by a short note of what it points at now, the task's status or
// "In the trash". Only the view: the document holds the links as they are.
import { Extension } from '@tiptap/core';
import type { Node as PMNode } from '@tiptap/pm/model';
import { Plugin, PluginKey } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';

export interface LinkViewOptions {
	/** What to say after a link, or null for nothing. */
	noteFor: (href: string) => string | null;
	/** Goes where a link points. */
	follow: (href: string) => void;
}

/** A transaction with this meta set draws the notes again (what they say changed). */
export const linkNotes = new PluginKey('linkNotes');

/** The end of each run of text under one link in a text block, with its address. */
function linkRuns(block: PMNode, start: number): { href: string; end: number }[] {
	const out: { href: string; end: number }[] = [];
	let href: string | null = null;
	let end = 0;
	block.forEach((child, offset) => {
		const h = (child.marks.find((m) => m.type.name === 'link')?.attrs.href as string) ?? null;
		if (h !== href && href) out.push({ href, end });
		href = h;
		end = start + offset + child.nodeSize;
	});
	if (href) out.push({ href, end });
	return out;
}

export const LinkView = Extension.create<LinkViewOptions>({
	name: 'linkView',
	addOptions() {
		return { noteFor: () => null, follow: () => {} };
	},
	addProseMirrorPlugins() {
		const { noteFor, follow } = this.options;
		return [
			new Plugin({
				key: linkNotes,
				props: {
					decorations(state) {
						const marks: Decoration[] = [];
						state.doc.descendants((node, pos) => {
							if (!node.isTextblock) return true;
							for (const run of linkRuns(node, pos + 1)) {
								const note = noteFor(run.href);
								if (!note) continue;
								marks.push(
									Decoration.widget(
										run.end,
										() => {
											const el = document.createElement('span');
											el.className = 'link-note';
											el.textContent = note;
											return el;
										},
										{ side: 1, key: `${run.href} ${note}`, ignoreSelection: true }
									)
								);
							}
							return false;
						});
						return DecorationSet.create(state.doc, marks);
					},
					handleClick(view, _pos, event) {
						if (event.button !== 0 || event.shiftKey) return false;
						const a = (event.target as HTMLElement | null)?.closest('a[href]');
						if (!a || !view.dom.contains(a)) return false;
						event.preventDefault();
						follow(a.getAttribute('href')!);
						return true;
					}
				}
			})
		];
	}
});
