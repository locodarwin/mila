// How the editor draws the formatting beyond Markdown that needs more than its
// HTML: a foldable section with its button, an image with
// its size, side and caption, a saved view's tasks, and a diagram drawn from
// its code block. Loaded with the editor.
import { Extension, type NodeViewRenderer } from '@tiptap/core';
import { Plugin } from '@tiptap/pm/state';
import { mount, unmount } from 'svelte';
import DiagramCard from '../components/doc/Diagram.svelte';
import DrawingCard from '../components/drawing/DrawingView.svelte';
import ReportEmbedCard from '../components/doc/ReportEmbed.svelte';
import ViewEmbedCard from '../components/doc/ViewEmbed.svelte';
import { editDiagram, editingIn, isDiagram, leaveDiagram } from './diagrams';
import { OPEN_FOLD } from './folds';

const CHEVRON =
	'<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m9 6 6 6-6 6"/></svg>';

/**
 * A foldable section. Whether it's open belongs to whoever reads it, on this
 * device: the document says only how it starts.
 */
export const foldView: NodeViewRenderer = ({ node, editor }) => {
	let current = node;
	let open = !!node.attrs.open;
	const dom = document.createElement('div');
	dom.className = 'fold';
	const toggle = document.createElement('button');
	toggle.type = 'button';
	toggle.className = 'fold-toggle';
	toggle.contentEditable = 'false';
	toggle.innerHTML = CHEVRON;
	const content = document.createElement('div');
	content.className = 'fold-content';
	dom.append(toggle, content);
	const draw = () => {
		dom.classList.toggle('open', open);
		const title = current.firstChild?.textContent.trim() || 'this section';
		toggle.setAttribute('aria-expanded', String(open));
		toggle.setAttribute('aria-label', `${open ? 'Close' : 'Open'} ${title}`);
		toggle.title = open ? 'Close this section' : 'Open this section';
		// In the text the button is reached by its tool in the formatting bar.
		toggle.tabIndex = editor.isEditable ? -1 : 0;
	};
	toggle.addEventListener('mousedown', (e) => e.preventDefault());
	toggle.addEventListener('click', () => {
		open = !open;
		draw();
	});
	dom.addEventListener(OPEN_FOLD, () => {
		open = true;
		draw();
	});
	draw();
	return {
		dom,
		contentDOM: content,
		update(next) {
			if (next.type !== current.type) return false;
			if (next.attrs.open !== current.attrs.open) open = !!next.attrs.open;
			current = next;
			draw();
			return true;
		},
		stopEvent: (e) => toggle.contains(e.target as Node),
		// Its button, and whether it's open, are this device's own, not edits.
		ignoreMutation: (m) =>
			m.type !== 'selection' &&
			(toggle.contains(m.target) || (m.type === 'attributes' && m.target === dom))
	};
};

/** Opens the folds around the caret, so that it's never somewhere hidden. */
export const FoldReveal = Extension.create({
	name: 'foldReveal',
	addProseMirrorPlugins() {
		return [
			new Plugin({
				view: () => ({
					update(view, prev) {
						if (prev && prev.selection.eq(view.state.selection)) return;
						const from = view.state.selection.$from;
						for (let d = from.depth; d > 0; d--) {
							if (from.node(d).type.name !== 'fold' || from.index(d) === 0) continue;
							const el = view.nodeDOM(from.before(d));
							if (el instanceof HTMLElement && !el.classList.contains('open'))
								el.dispatchEvent(new CustomEvent(OPEN_FOLD));
						}
					}
				})
			})
		];
	}
});

/** An image, at the size and on the side it was given, with its caption under it. */
export const imageView: NodeViewRenderer = ({ node }) => {
	let current = node;
	const dom = document.createElement('span');
	dom.className = 'doc-image';
	const img = document.createElement('img');
	const caption = document.createElement('span');
	caption.className = 'doc-image-caption';
	dom.append(img, caption);
	const draw = () => {
		const a = current.attrs;
		img.src = a.src ?? '';
		img.alt = a.alt ?? '';
		if (a.title) img.title = a.title;
		else img.removeAttribute('title');
		for (const [k, v] of [
			['data-size', a.size],
			['data-side', a.side]
		] as const) {
			if (v == null) dom.removeAttribute(k);
			else dom.setAttribute(k, String(v));
		}
		caption.textContent = a.caption ?? '';
		caption.hidden = !a.caption;
	};
	draw();
	return {
		dom,
		update(next) {
			if (next.type !== current.type) return false;
			current = next;
			draw();
			return true;
		},
		ignoreMutation: (m) => m.type !== 'selection'
	};
};

/** A saved view's tasks, drawn by ViewEmbed.svelte. */
export function viewEmbedView(ws: string): NodeViewRenderer {
	return ({ node }) => {
		let current = node;
		const dom = document.createElement('div');
		dom.className = 'view-embed';
		dom.contentEditable = 'false';
		const props = $state({
			ws,
			id: String(node.attrs.id ?? ''),
			name: String(node.attrs.name ?? ''),
			layout: (node.attrs.layout as string | null) ?? null
		});
		const card = mount(ViewEmbedCard, { target: dom, props });
		return {
			dom,
			update(next) {
				if (next.type !== current.type) return false;
				current = next;
				props.id = String(next.attrs.id ?? '');
				props.name = String(next.attrs.name ?? '');
				props.layout = (next.attrs.layout as string | null) ?? null;
				return true;
			},
			// Its buttons and links are its own, and so is dragging a board's cards;
			// the rest picks the block.
			stopEvent: (e) => {
				const at = e.target as HTMLElement | null;
				if (/^drag|^drop$/.test(e.type) && at?.closest?.('.board')) return true;
				return !!at?.closest?.('button, a, input') && e.type !== 'dragstart';
			},
			ignoreMutation: (m) => m.type !== 'selection',
			destroy: () => void unmount(card)
		};
	};
}

/** One chart of a report view, drawn by ReportEmbed.svelte. */
export function reportEmbedView(ws: string): NodeViewRenderer {
	return ({ node }) => {
		let current = node;
		const dom = document.createElement('div');
		dom.className = 'report-embed';
		dom.contentEditable = 'false';
		const props = $state({
			ws,
			id: String(node.attrs.id ?? ''),
			chart: String(node.attrs.chart ?? ''),
			name: String(node.attrs.name ?? '')
		});
		const card = mount(ReportEmbedCard, { target: dom, props });
		return {
			dom,
			update(next) {
				if (next.type !== current.type) return false;
				current = next;
				props.id = String(next.attrs.id ?? '');
				props.chart = String(next.attrs.chart ?? '');
				props.name = String(next.attrs.name ?? '');
				return true;
			},
			// Its buttons and links are its own (a chart's table, a task in the
			// list), and so are the arrow keys on a focused chart; the rest picks
			// the block.
			stopEvent: (e) => {
				const at = e.target as HTMLElement | null;
				if (e.type === 'keydown' && at?.closest?.('[role="group"][tabindex]')) return true;
				return !!at?.closest?.('button, a, input') && e.type !== 'dragstart';
			},
			ignoreMutation: (m) => m.type !== 'selection',
			destroy: () => void unmount(card)
		};
	};
}

/**
 * A code block, and a diagram when its language is `mermaid`: the drawing,
 * and under or beside it the source, the block's own text, shown while the
 * caret is in it (diagrams.ts marks the block `editing`).
 */
export const codeBlockView: NodeViewRenderer = ({ node, editor, getPos, decorations }) => {
	let current = node;
	const language = (n: typeof node) => (n.attrs.language as string | null) || null;
	if (!isDiagram(node.attrs.language)) {
		const pre = document.createElement('pre');
		const code = document.createElement('code');
		if (language(node)) code.className = `language-${language(node)}`;
		pre.append(code);
		return {
			dom: pre,
			contentDOM: code,
			update(next) {
				if (next.type !== current.type || isDiagram(next.attrs.language)) return false;
				if (language(next) !== language(current)) return false;
				current = next;
				return true;
			}
		};
	}
	const dom = document.createElement('div');
	dom.className = 'diagram-block';
	const figure = document.createElement('div');
	figure.className = 'diagram-figure';
	figure.contentEditable = 'false';
	const pre = document.createElement('pre');
	pre.className = 'diagram-source';
	pre.setAttribute('aria-label', 'Diagram source');
	const code = document.createElement('code');
	code.className = 'language-mermaid';
	code.spellcheck = false;
	pre.append(code);
	const inner = document.createElement('div');
	inner.className = 'diagram-inner';
	inner.append(figure, pre);
	dom.append(inner);
	const props = $state({
		source: node.textContent,
		editing: editingIn(decorations),
		editable: editor.isEditable,
		onedit: () => {
			const pos = typeof getPos === 'function' ? getPos() : undefined;
			if (typeof pos === 'number') editDiagram(editor.view, pos);
		},
		ondone: () => {
			const pos = typeof getPos === 'function' ? getPos() : undefined;
			if (typeof pos === 'number') leaveDiagram(editor.view, pos);
		}
	});
	const card = mount(DiagramCard, { target: figure, props });
	return {
		dom,
		contentDOM: code,
		update(next, decos) {
			if (next.type !== current.type || !isDiagram(next.attrs.language)) return false;
			current = next;
			if (props.source !== next.textContent) props.source = next.textContent;
			props.editing = editingIn(decos);
			props.editable = editor.isEditable;
			return true;
		},
		// The drawing's buttons and full screen are its own; the rest is the text.
		stopEvent: (e) => figure.contains(e.target as Node) && e.type !== 'dragstart',
		ignoreMutation: (m) =>
			m.type !== 'selection' &&
			(figure.contains(m.target) || (m.type === 'attributes' && m.target === dom)),
		destroy: () => void unmount(card)
	};
};

/** Drawings just made here, to open in the editor as soon as they are drawn. */
// eslint-disable-next-line svelte/prefer-svelte-reactivity -- plain: read once, as a block is drawn
const opening = new Set<string>();
export function openWhenDrawn(id: string) {
	opening.add(id);
}

/**
 * A drawing in a page, drawn by DrawingView.svelte from the page's rows.
 * When its editor closes, the picture saved with the page (for other apps
 * and exports) is made again if the drawing changed, and the block takes
 * its address and the drawing's name.
 */
export function drawingEmbedView(ws: string, pageId: string | null): NodeViewRenderer {
	return ({ node, editor, getPos }) => {
		let current = node;
		const dom = document.createElement('div');
		dom.className = 'drawing-embed';
		dom.contentEditable = 'false';
		const id = String(node.attrs.id ?? '');
		const openAtStart = opening.delete(id);
		const props = $state({
			ws,
			pageId: pageId ?? '',
			id,
			name: String(node.attrs.name ?? ''),
			src: String(node.attrs.src ?? ''),
			size: (node.attrs.size as number | null) ?? null,
			editable: editor.isEditable && !!pageId,
			openAtStart,
			onclosed: (changed: boolean) => void closed(changed)
		});
		async function closed(changed: boolean) {
			if (!pageId || !editor.isEditable) return;
			const [{ shapesOf }, { refreshPicture }, { engine }] = await Promise.all([
				import('../sync/drawings.svelte'),
				import('../drawingPaint'),
				import('../sync/engine.svelte')
			]);
			const row = engine.replica(ws)?.get('page_drawing', id);
			const name =
				typeof row?.values.name === 'string' ? row.values.name : String(current.attrs.name ?? '');
			const src = String(current.attrs.src ?? '');
			const picture =
				changed || !src
					? await refreshPicture(ws, pageId, shapesOf(ws, pageId, id), name, src)
					: null;
			if (editor.isDestroyed) return;
			const pos = typeof getPos === 'function' ? getPos() : undefined;
			if (typeof pos !== 'number') return;
			const at = editor.state.doc.nodeAt(pos);
			if (!at || at.type.name !== 'drawingEmbed' || at.attrs.id !== id) return;
			if (name === at.attrs.name && !picture) return;
			editor.view.dispatch(
				editor.state.tr.setNodeMarkup(pos, undefined, {
					...at.attrs,
					name,
					src: picture ?? at.attrs.src
				})
			);
		}
		const card = mount(DrawingCard, { target: dom, props });
		// The editor opens inside the block, which the browser lets be dragged
		// (the node is draggable): a drag that starts while it is open, such as
		// moving a shape, would carry a picture of the whole block with the
		// pointer. The drag starts at the block itself, so it is stopped here.
		const editorOpen = () => !!dom.querySelector('dialog[open]');
		dom.addEventListener('dragstart', (e) => {
			if (editorOpen()) e.preventDefault();
		});
		return {
			dom,
			update(next) {
				if (next.type !== current.type || next.attrs.id !== current.attrs.id) return false;
				current = next;
				props.name = String(next.attrs.name ?? '');
				props.src = String(next.attrs.src ?? '');
				props.size = (next.attrs.size as number | null) ?? null;
				props.editable = editor.isEditable && !!pageId;
				return true;
			},
			// Its buttons and the editor over the page are its own; the rest picks
			// the block, and dragging it by its buttons moves it. While its editor
			// is open nothing drags the block (see the dragstart listener above).
			stopEvent: (e) => {
				const at = e.target as HTMLElement | null;
				if (editorOpen() || at?.closest?.('dialog')) return true;
				return !!at?.closest?.('button, a, input, textarea') && e.type !== 'dragstart';
			},
			ignoreMutation: (m) => m.type !== 'selection',
			destroy: () => void unmount(card)
		};
	};
}
