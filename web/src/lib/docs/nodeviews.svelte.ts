// How the editor draws the formatting beyond Markdown that needs more than its
// HTML: a foldable section with its button, an image with
// its size, side and caption, and a saved view's tasks. Loaded with the editor.
import { Extension, type NodeViewRenderer } from '@tiptap/core';
import { Plugin } from '@tiptap/pm/state';
import { mount, unmount } from 'svelte';
import ReportEmbedCard from '../components/doc/ReportEmbed.svelte';
import ViewEmbedCard from '../components/doc/ViewEmbed.svelte';
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
