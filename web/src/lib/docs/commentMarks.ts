// Comments pinned to text in the editor. A thread is
// pinned by two Yjs relative positions, which follow their characters through
// everyone's edits. Its highlight is only drawn: the document and its Markdown
// never hold it. When the positions no longer enclose any text (the words were
// deleted, or the document was made again by a restore or an import), the thread
// goes by its words, if they appear exactly once, and otherwise counts as gone.
// Nothing is written for that: every device works it out.
import { Extension } from '@tiptap/core';
import type { Node as PMNode } from '@tiptap/pm/model';
import { Plugin, PluginKey, type EditorState, type Transaction } from '@tiptap/pm/state';
import { Decoration, DecorationSet, type EditorView } from '@tiptap/pm/view';
import {
	absolutePositionToRelativePosition,
	relativePositionToAbsolutePosition,
	ySyncPluginKey
} from '@tiptap/y-tiptap';
import * as Y from 'yjs';
import { DRAFT, REDRAW_PINS, type CommentPins, type Pin } from './commentPins.svelte';
import { revealInFolds } from './folds';

const commentMarks = new PluginKey<Drawn>('commentMarks');

/** The most of a thread's words kept; the server's limit. */
const MAX_QUOTE = 2000;

interface Range {
	from: number;
	to: number;
}

interface Drawn {
	marks: DecorationSet;
	ranges: Map<string, Range>;
	/** Whether the document is drawn from Yjs, so that a thread not found is gone. */
	ready: boolean;
	order: string[];
	gone: string[];
	/** Moved through a local edit that Yjs has not heard yet: to be worked out again after. */
	stale: boolean;
}

/** The document's text with white space collapsed, and each character's position. */
function flatten(doc: PMNode): { text: string; at: number[] } {
	let text = '';
	const at: number[] = [];
	const add = (ch: string, pos: number) => {
		const space = /\s/.test(ch);
		if (space && (!text || text.endsWith(' '))) return;
		text += space ? ' ' : ch;
		at.push(pos);
	};
	doc.descendants((node, pos) => {
		if (node.isText) {
			const words = node.text ?? '';
			for (let i = 0; i < words.length; i++) add(words[i], pos + i);
		} else if (node.isBlock || node.isLeaf) {
			add('\n', pos);
		}
		return true;
	});
	return { text, at };
}

const collapse = (s: string) => s.replace(/\s+/g, ' ').trim();

/** Where some words are in the document, if they are there exactly once. */
function findQuote(flat: { text: string; at: number[] }, quote: string): Range | null {
	const words = collapse(quote);
	if (!words) return null;
	const first = flat.text.indexOf(words);
	if (first < 0 || flat.text.indexOf(words, first + 1) >= 0) return null;
	return { from: flat.at[first], to: flat.at[first + words.length - 1] + 1 };
}

/** The range a pin's positions hold now, or null if they hold no text. */
function resolve(state: EditorState, anchor: string): Range | null {
	if (!anchor) return null;
	const binding = ySyncPluginKey.getState(state)?.binding;
	if (!binding) return null;
	try {
		const [a, b] = JSON.parse(anchor) as [unknown, unknown];
		const position = (json: unknown) =>
			relativePositionToAbsolutePosition(
				binding.doc,
				binding.type,
				Y.createRelativePositionFromJSON(json),
				binding.mapping
			);
		const from = position(a);
		const to = position(b);
		if (from == null || to == null || from >= to || to > state.doc.content.size) return null;
		return state.doc.textBetween(from, to, ' ', ' ').trim() ? { from, to } : null;
	} catch {
		// Not a pin this app can read: it goes by its words.
		return null;
	}
}

function draw(state: EditorState, pins: CommentPins): Drawn {
	const binding = ySyncPluginKey.getState(state)?.binding;
	const ready = !!binding?.mapping && binding.mapping.size > 0;
	const ranges = new Map<string, Range>();
	const gone: string[] = [];
	const threads: Pin[] = [...pins.pins];
	if (pins.draft) threads.push({ id: DRAFT, ...pins.draft });
	let flat: ReturnType<typeof flatten> | undefined;
	if (ready) {
		for (const t of threads) {
			let range = resolve(state, t.anchor);
			if (!range) {
				flat ??= flatten(state.doc);
				range = findQuote(flat, t.quote);
			}
			if (range) ranges.set(t.id, range);
			else gone.push(t.id);
		}
	}
	const order = [...ranges]
		.filter(([id]) => id !== DRAFT)
		.sort((a, b) => a[1].from - b[1].from)
		.map(([id]) => id);
	return { marks: decorate(state.doc, ranges, pins), ranges, ready, order, gone, stale: false };
}

function decorate(doc: PMNode, ranges: Map<string, Range>, pins: CommentPins): DecorationSet {
	const marks = [...ranges].map(([id, { from, to }]) =>
		Decoration.inline(from, to, {
			class: id === pins.active || id === DRAFT ? 'comment-mark active' : 'comment-mark',
			'data-thread': id
		})
	);
	return DecorationSet.create(doc, marks);
}

/**
 * The highlights through an edit made here. Yjs hears of it only when the view
 * updates, so positions read from it now would land on the old text.
 */
function carried(old: Drawn, tr: Transaction, pins: CommentPins): Drawn {
	const ranges = new Map<string, Range>();
	for (const [id, { from, to }] of old.ranges) {
		const next = { from: tr.mapping.map(from, 1), to: tr.mapping.map(to, -1) };
		if (next.from < next.to) ranges.set(id, next);
	}
	return { ...old, marks: decorate(tr.doc, ranges, pins), ranges, stale: true };
}

/** The thread whose words a position is in: the shortest, when they overlap. */
function threadAt(drawn: Drawn, pos: number): string | null {
	let best: string | null = null;
	let size = Infinity;
	for (const [id, { from, to }] of drawn.ranges) {
		if (id !== DRAFT && from <= pos && pos <= to && to - from < size) {
			best = id;
			size = to - from;
		}
	}
	return best;
}

function publish(drawn: Drawn, pins: CommentPins) {
	if (!drawn.ready) return;
	pins.ready = true;
	if (pins.order.join() !== drawn.order.join()) pins.order = drawn.order;
	if (pins.gone.size !== drawn.gone.length || drawn.gone.some((id) => !pins.gone.has(id))) {
		pins.gone = new Set(drawn.gone);
	}
}

/** The selection as a new thread's pin and words, or null if it holds no words. */
export function pinSelection(state: EditorState): { anchor: string; quote: string } | null {
	const { from, to } = state.selection;
	const quote = state.doc.textBetween(from, to, '\n', '\n').trim().slice(0, MAX_QUOTE);
	const binding = ySyncPluginKey.getState(state)?.binding;
	if (!quote || !binding) return null;
	const json = (pos: number) =>
		Y.relativePositionToJSON(
			absolutePositionToRelativePosition(pos, binding.type, binding.mapping)
		);
	return { anchor: JSON.stringify([json(from), json(to)]), quote };
}

/** Brings a thread's highlight into view. */
export function revealMark(view: EditorView, id: string) {
	const mark = view.dom.querySelector(`[data-thread="${CSS.escape(id)}"]`);
	// A thread in a closed section opens it, so its words are there to see.
	revealInFolds(mark);
	mark?.scrollIntoView({ block: 'center' });
}

export const CommentMarks = Extension.create<{ pins: CommentPins | null }>({
	name: 'commentMarks',
	addOptions() {
		return { pins: null };
	},
	addProseMirrorPlugins() {
		const pins = this.options.pins;
		if (!pins) return [];
		return [
			new Plugin<Drawn>({
				key: commentMarks,
				state: {
					init: (_, state) => draw(state, pins),
					apply(tr, old, _before, state) {
						if (tr.getMeta(REDRAW_PINS) || tr.getMeta(ySyncPluginKey)) return draw(state, pins);
						return tr.docChanged ? carried(old, tr, pins) : old;
					}
				},
				props: {
					decorations: (state) => commentMarks.getState(state)?.marks,
					handleClick(view, pos, event) {
						if (event.button !== 0) return false;
						const drawn = commentMarks.getState(view.state);
						const id = drawn && threadAt(drawn, pos);
						if (!id) return false;
						pins.active = id;
						pins.onshow();
						return false;
					}
				},
				view: (view) => {
					const first = commentMarks.getState(view.state);
					if (first) publish(first, pins);
					return {
						update(view, before) {
							const drawn = commentMarks.getState(view.state);
							if (!drawn) return;
							publish(drawn, pins);
							// Yjs has it by the time this view update is over.
							if (drawn.stale && !view.composing) {
								queueMicrotask(() => {
									if (!view.isDestroyed && commentMarks.getState(view.state)?.stale) {
										view.dispatch(view.state.tr.setMeta(REDRAW_PINS, true));
									}
								});
							}
							// The caret reaching a highlight picks its thread, for the keyboard.
							const { selection } = view.state;
							if (selection.empty && !selection.eq(before.selection)) {
								const id = threadAt(drawn, selection.head);
								if (id && pins.active !== id) pins.active = id;
							}
						}
					};
				}
			})
		];
	}
});
