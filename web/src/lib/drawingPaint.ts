// How a drawing is painted: live in the page and the editor, in the
// theme's colors through its tokens, and as the picture saved with the
// page for other apps and exports, in the Standard light theme's colors
// written out. Loaded only with a drawing.
import { DRAWING_TOKENS, drawingSvg, type Measure, type Paint, type Shape } from './drawing';
import { resolveTokens } from './theme/resolve';
import { colorCss, tokenVar } from './theme/css';
import { fileUrl, uploadFile } from './uploads';
import { engine } from './sync/engine.svelte';

let canvas: CanvasRenderingContext2D | null | undefined;

/** Measures text in a font, as the browser lays it out. */
export function measurer(font: string): Measure {
	canvas ??=
		typeof document === 'undefined' ? null : document.createElement('canvas').getContext('2d');
	const cache = new Map<string, number>();
	return (text, size, bold) => {
		const key = `${size}|${bold ? 1 : 0}|${text}`;
		const hit = cache.get(key);
		if (hit !== undefined) return hit;
		let w = text.length * size * 0.55;
		if (canvas) {
			canvas.font = `${bold ? 600 : 400} ${size}px ${font}`;
			w = canvas.measureText(text).width;
		}
		if (cache.size > 5000) cache.clear();
		cache.set(key, w);
		return w;
	};
}

/** The paint for a drawing shown in the app, in the theme's own colors. */
export function livePaint(ws: string, font: string): Paint {
	return {
		color: tokenVar,
		measure: measurer(font),
		font,
		image: (id) => fileUrl(ws, id)
	};
}

/** An element carrying the Standard light theme, for reading its values. */
function standardProbe(): HTMLElement {
	const el = document.createElement('div');
	el.dataset.theme = 'standard';
	el.dataset.scheme = 'light';
	el.dataset.density = 'comfortable';
	el.dataset.fontSize = 'm';
	el.setAttribute('aria-hidden', 'true');
	el.style.cssText = 'position:absolute;visibility:hidden;pointer-events:none;';
	document.body.append(el);
	return el;
}

async function asDataUrl(url: string): Promise<string | null> {
	try {
		const res = await fetch(url);
		if (!res.ok) return null;
		const blob = await res.blob();
		if (blob.size > 2 * 1024 * 1024) return null;
		return await new Promise((done) => {
			const reader = new FileReader();
			reader.onload = () => done(typeof reader.result === 'string' ? reader.result : null);
			reader.onerror = () => done(null);
			reader.readAsDataURL(blob);
		});
	} catch {
		return null;
	}
}

/**
 * The picture of a drawing saved with the page: an SVG file of its own, in
 * the Standard light theme, its pictures written into it, so it shows
 * the same anywhere.
 */
export async function pictureOf(ws: string, shapes: Shape[], title: string): Promise<string> {
	const probe = standardProbe();
	try {
		const colors = resolveTokens(probe, DRAWING_TOKENS);
		const font = getComputedStyle(probe).getPropertyValue('--font-page').trim() || 'sans-serif';
		const images = new Map<string, string>();
		for (const s of shapes)
			if (s.kind === 'image' && s.imageId && !images.has(s.imageId)) {
				const data = await asDataUrl(fileUrl(ws, s.imageId));
				if (data) images.set(s.imageId, data);
			}
		const color = (token: string) => {
			const c = colors.get(token);
			return c && typeof c === 'object' ? colorCss(c.lch, c.alpha) : 'currentColor';
		};
		const paint: Paint = {
			color,
			measure: measurer(font),
			font,
			image: (id) => images.get(id) ?? null
		};
		return drawingSvg(shapes, paint, title, color('surface-0'));
	} finally {
		probe.remove();
	}
}

const ATTACHMENT = /\/api\/v1\/workspaces\/[0-9a-f-]{36}\/attachments\/([0-9a-f-]{36})\/file$/;

/** The page file a picture's address names, if it is one. */
export function attachmentOf(src: string): string | null {
	return ATTACHMENT.exec(src)?.[1] ?? null;
}

/**
 * Saves a new picture of a drawing when it differs from the one the page
 * has, and takes the old one out of the page's files. The new picture's
 * address, or null when nothing changed or it could not be saved (offline:
 * the next time the drawing is edited tries again).
 */
export async function refreshPicture(
	ws: string,
	pageId: string,
	shapes: Shape[],
	name: string,
	src: string
): Promise<string | null> {
	const svg = await pictureOf(ws, shapes, name);
	if (src) {
		try {
			const res = await fetch(src);
			if (res.ok && (await res.text()) === svg) return null;
		} catch {
			return null;
		}
	}
	const file = new File(
		[svg],
		`${name.replace(/[^\p{L}\p{N} _-]+/gu, '').trim() || 'Drawing'}.svg`,
		{
			type: 'image/svg+xml'
		}
	);
	let id: string;
	try {
		id = await uploadFile(ws, { kind: 'page', id: pageId }, file);
	} catch {
		return null;
	}
	const old = attachmentOf(src);
	if (old && old !== id)
		void engine.mutate(
			ws,
			'page_attachment',
			old,
			{ deleted_at: new Date().toISOString() },
			{ undoable: false }
		);
	return fileUrl(ws, id);
}
