// Motion for Svelte transitions and animations, from the same tokens CSS
// uses (tokens.css): the theme's durations and curve, so Calm is slower and
// Playful springs, and a plain crossfade when the OS asks for less motion.
// CSS transitions read the tokens directly; this is for what Svelte draws.
import { flip as svelteFlip } from 'svelte/animate';
import { fade, fly, slide, type TransitionConfig } from 'svelte/transition';

export type Duration = 'micro' | 'panel' | 'view' | 'fade';
export type Easing = (t: number) => number;

const reducedQuery =
	typeof matchMedia === 'function' ? matchMedia('(prefers-reduced-motion: reduce)') : null;

/** True when the OS asks for less motion: movement becomes a crossfade. */
export function reducedMotion(): boolean {
	return reducedQuery?.matches ?? false;
}

const FALLBACK: Record<Duration, number> = { micro: 120, panel: 200, view: 320, fade: 150 };
let cache: { durations: Record<Duration, number>; ease: Easing; spring: Easing } | null = null;

/** Forgets the tokens read so far; appearance.apply() calls it when the theme changes. */
export function refreshMotion() {
	cache = null;
}
reducedQuery?.addEventListener('change', refreshMotion);

/**
 * The motion tokens as an element has them: the page's (cached until the
 * theme changes), or a theme preview container's, for its Play motion.
 */
export function tokens(el?: Element) {
	if (!el && cache) return cache;
	if (typeof document === 'undefined')
		return { durations: FALLBACK, ease: cubicBezier(0.2, 0.8, 0.2, 1), spring: linearEase };
	const style = getComputedStyle(el ?? document.documentElement);
	const read = (name: string) => style.getPropertyValue(name).trim();
	const durations = { ...FALLBACK };
	for (const d of Object.keys(FALLBACK) as Duration[]) {
		const ms = parseDuration(read(`--dur-${d}`));
		if (ms !== null) durations[d] = ms;
	}
	const ease = parseEasing(read('--ease')) ?? cubicBezier(0.2, 0.8, 0.2, 1);
	const out = { durations, ease, spring: parseEasing(read('--ease-spring')) ?? ease };
	if (!el) cache = out;
	return out;
}

/** A duration token in milliseconds, as the current theme sets it. */
export function duration(name: Duration): number {
	return tokens().durations[name];
}

/** The theme's curve; the spring one where it has one (Playful). */
export function easing(spring = false): Easing {
	const t = tokens();
	return spring ? t.spring : t.ease;
}

/** "200ms" or "0.2s" in milliseconds; null if it is neither. */
export function parseDuration(v: string): number | null {
	const m = /^(-?[\d.]+)(ms|s)$/.exec(v.trim());
	if (!m) return null;
	const n = Number(m[1]) * (m[2] === 's' ? 1000 : 1);
	return Number.isFinite(n) && n >= 0 ? n : null;
}

/** A CSS cubic-bezier() or linear() easing as a function, or null if it is neither. */
export function parseEasing(v: string): Easing | null {
	v = v.trim();
	if (v === 'linear') return linearEase;
	if (v === 'ease-out') return cubicBezier(0, 0, 0.58, 1);
	const bezier = /^cubic-bezier\(([^)]*)\)$/.exec(v);
	if (bezier) {
		const n = bezier[1].split(',').map(Number);
		return n.length === 4 && n.every(Number.isFinite) ? cubicBezier(n[0], n[1], n[2], n[3]) : null;
	}
	const linear = /^linear\((.*)\)$/.exec(v);
	if (linear) return linearStops(linear[1]);
	return null;
}

function linearEase(t: number): number {
	return t;
}

/** The curve CSS draws for cubic-bezier(x1, y1, x2, y2). */
export function cubicBezier(x1: number, y1: number, x2: number, y2: number): Easing {
	const at = (a: number, b: number, t: number) =>
		3 * a * (1 - t) ** 2 * t + 3 * b * (1 - t) * t ** 2 + t ** 3;
	const slope = (a: number, b: number, t: number) =>
		3 * a * (1 - t) ** 2 + 6 * (b - a) * (1 - t) * t + 3 * (1 - b) * t ** 2;
	return (x: number) => {
		if (x <= 0) return 0;
		if (x >= 1) return 1;
		// Newton's method for the t whose x is the input, then bisection if it stalls.
		let t = x;
		for (let i = 0; i < 8; i++) {
			const err = at(x1, x2, t) - x;
			if (Math.abs(err) < 1e-6) return at(y1, y2, t);
			const d = slope(x1, x2, t);
			if (Math.abs(d) < 1e-6) break;
			t -= err / d;
		}
		let lo = 0;
		let hi = 1;
		t = x;
		for (let i = 0; i < 30; i++) {
			const v = at(x1, x2, t);
			if (Math.abs(v - x) < 1e-6) break;
			if (v < x) lo = t;
			else hi = t;
			t = (lo + hi) / 2;
		}
		return at(y1, y2, t);
	};
}

/** CSS linear(0, 0.18 6%, ..., 1): straight lines between the stops. */
function linearStops(body: string): Easing | null {
	const stops: { v: number; at: number | null }[] = [];
	for (const part of body.split(',')) {
		const [value, pos] = part.trim().split(/\s+/);
		const v = Number(value);
		if (!Number.isFinite(v)) return null;
		stops.push({ v, at: pos?.endsWith('%') ? Number(pos.slice(0, -1)) / 100 : null });
	}
	if (stops.length < 2) return null;
	stops[0].at ??= 0;
	stops[stops.length - 1].at ??= 1;
	// Stops without a position share the space between their neighbors.
	for (let i = 1; i < stops.length - 1; i++) {
		if (stops[i].at !== null) continue;
		let j = i;
		while (stops[j].at === null) j++;
		const from = stops[i - 1].at!;
		const step = (stops[j].at! - from) / (j - i + 1);
		for (let k = i; k < j; k++) stops[k].at = from + step * (k - i + 1);
	}
	return (t: number) => {
		if (t <= 0) return stops[0].v;
		for (let i = 1; i < stops.length; i++) {
			const a = stops[i - 1];
			const b = stops[i];
			if (t <= b.at!)
				return b.at === a.at ? b.v : a.v + ((t - a.at!) / (b.at! - a.at!)) * (b.v - a.v);
		}
		return stops[stops.length - 1].v;
	};
}

/** A crossfade at the fade duration: what every movement becomes under reduced motion. */
function crossfade(node: Element): TransitionConfig {
	return fade(node, { duration: duration('fade') });
}

/** A row or section opening or closing: slides its height, or fades. */
export function collapse(node: Element): () => TransitionConfig {
	const config = reducedMotion()
		? crossfade(node)
		: slide(node, { duration: duration('panel'), easing: easing() });
	const batch = (crowd ??= { n: 0 });
	batch.n++;
	queueMicrotask(() => (crowd = null));
	// Svelte asks for the config a microtask later, once every row in the
	// same change has been counted.
	return () => (batch.n > CROWD ? { duration: 0 } : config);
}

// Rows that come or go together in numbers (a filter, a long list filling
// in) are simply there or gone: animating hundreds of heights at once costs
// frames and shows nothing anyone could follow.
const CROWD = 8;
let crowd: { n: number } | null = null;

/**
 * A new view (onNavigate in the app layout): the page rises a little and
 * fades in, or only fades with reduced motion, while everything around it
 * holds still. An animation of the page itself, not a view transition:
 * one of those holds the new view back two frames to capture the old one,
 * over the 50 ms budget for switching views (docs/PLAN.md section 13), and
 * keeps taps from the page while it runs.
 */
export function arrive(node: HTMLElement): Animation | undefined {
	if (typeof node.animate !== 'function') return undefined;
	const ease = getComputedStyle(document.documentElement).getPropertyValue('--ease').trim();
	const frames: Keyframe[] = reducedMotion()
		? [{ opacity: 0 }, { opacity: 1 }]
		: [
				{ opacity: 0, transform: 'translateY(8px)' },
				{ opacity: 1, transform: 'none' }
			];
	return node.animate(frames, { duration: duration('view'), easing: ease || 'ease-out' });
}

/** Something arriving from a little way off (a toast): rises into place, or fades. */
export function rise(node: Element, { y = 16 }: { y?: number } = {}): TransitionConfig {
	if (reducedMotion()) return crossfade(node);
	return fly(node, { y, duration: duration('panel'), easing: easing(true) });
}

/** Reordering: neighbors slide to their new places (FLIP), or simply are there. */
export function flip(node: Element, rects: { from: DOMRect; to: DOMRect }) {
	return svelteFlip(node, rects, {
		duration: reducedMotion() ? 0 : duration('panel'),
		easing: easing(true)
	});
}

/**
 * The task panel. On a phone it is a sheet that slides up from the bottom
 * and back down; beside the list it slides in a little from the side and
 * leaves at once, since the list takes its room back in the same moment.
 * Use it as separate in: and out: directives: transition: asks for both
 * directions at once, which never tells it the panel is leaving.
 */
export function sheet(
	node: Element,
	{ phone }: { phone: boolean },
	options?: { direction?: 'in' | 'out' | 'both' }
): TransitionConfig {
	if (!phone && options?.direction === 'out') return { duration: 0 };
	if (reducedMotion()) return crossfade(node);
	if (phone)
		return fly(node, { y: '100%', opacity: 1, duration: duration('view'), easing: easing() });
	return fly(node, { x: 16, duration: duration('panel'), easing: easing() });
}
