// The arithmetic behind cropping a picture to a circle (AvatarCrop.svelte).
// The stage is a square of `stage` CSS pixels with the circle inscribed in
// it; the picture sits on it at `scale` stage pixels per picture pixel,
// its top left corner at (x, y). What the stage shows is what is kept, so
// the picture must always cover the whole stage.

export interface View {
	scale: number;
	x: number;
	y: number;
}

export interface Size {
	w: number;
	h: number;
}

/** How far each arrow key press moves the picture, in stage pixels; with Shift, BIG. */
export const PAN_STEP = 8;
export const PAN_STEP_BIG = 40;
/** How much each plus or minus press zooms. */
export const ZOOM_STEP = 1.15;
/** The most the picture can be zoomed past just covering the stage. */
export const MAX_ZOOM = 8;
/** The largest picture sent; the server keeps 256 pixels at most. */
export const MAX_OUTPUT = 512;
/** The fewest picture pixels a crop can span: the server's smallest side. */
export const MIN_SIDE = 16;

/** The scale at which the picture's shorter side just spans the stage. */
export function coverScale(w: number, h: number, stage: number): number {
	return stage / Math.min(w, h);
}

/** The largest scale: MAX_ZOOM past covering, but never fewer than MIN_SIDE pixels across. */
export function maxScale(img: Size, stage: number): number {
	const cover = coverScale(img.w, img.h, stage);
	return Math.max(cover, Math.min(cover * MAX_ZOOM, stage / MIN_SIDE));
}

/** The picture centered and just covering the stage. */
export function initialView(img: Size, stage: number): View {
	const scale = coverScale(img.w, img.h, stage);
	return { scale, x: (stage - img.w * scale) / 2, y: (stage - img.h * scale) / 2 };
}

/** The view brought back within bounds: zoomed between covering and the most, and panned so the stage stays covered. */
export function clampPan(view: View, img: Size, stage: number): View {
	const scale = Math.min(
		maxScale(img, stage),
		Math.max(coverScale(img.w, img.h, stage), view.scale)
	);
	const x = Math.min(0, Math.max(stage - img.w * scale, view.x));
	const y = Math.min(0, Math.max(stage - img.h * scale, view.y));
	return { scale, x, y };
}

/** Zooms by factor, keeping the picture's pixel under point (in stage pixels) where it is. */
export function zoomAbout(view: View, factor: number, point: { x: number; y: number }): View {
	const scale = view.scale * factor;
	return {
		scale,
		x: point.x - ((point.x - view.x) / view.scale) * scale,
		y: point.y - ((point.y - view.y) / view.scale) * scale
	};
}

/** The square of the picture the stage shows, in picture pixels, inside the picture. */
export function cropRect(view: View, img: Size, stage: number) {
	const side = Math.min(stage / view.scale, img.w, img.h);
	const sx = Math.min(Math.max(0, -view.x / view.scale), img.w - side);
	const sy = Math.min(Math.max(0, -view.y / view.scale), img.h - side);
	return { sx, sy, side };
}

/** The side of the picture sent: the crop's own pixels, up to MAX_OUTPUT. */
export function outputSide(side: number): number {
	return Math.min(MAX_OUTPUT, Math.round(side));
}
