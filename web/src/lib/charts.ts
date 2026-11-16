// The arithmetic of the Reports page's charts (components/reports/), pure:
// a value axis of round numbers, the shapes of bars with a rounded data
// end, and moving between a chart's marks from the keyboard.

/** An axis from 0 to a round number at or above the largest value, in a few round steps. */
export function niceScale(
	largest: number,
	steps = 4
): { max: number; step: number; ticks: number[] } {
	if (!(largest > 0))
		return { max: steps, step: 1, ticks: Array.from({ length: steps + 1 }, (_, i) => i) };
	const raw = largest / steps;
	const mag = 10 ** Math.floor(Math.log10(raw));
	// Counts are whole: never a step below 1, nor one of 2.5.
	const step = Math.max(
		1,
		[1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => s >= raw && Number.isInteger(s)) ?? 10 * mag
	);
	const n = Math.ceil(largest / step);
	return { max: step * n, step, ticks: Array.from({ length: n + 1 }, (_, i) => round(i * step)) };
}

const round = (n: number) => Math.round(n * 1000) / 1000;

/** A column from the baseline up to `top`, square at the baseline and rounded at the top. */
export function columnPath(
	x: number,
	top: number,
	width: number,
	base: number,
	radius = 4
): string {
	const h = base - top;
	if (h <= 0 || width <= 0) return '';
	const r = Math.min(radius, h, width / 2);
	return (
		`M${round(x)},${round(base)}V${round(top + r)}` +
		`Q${round(x)},${round(top)} ${round(x + r)},${round(top)}` +
		`H${round(x + width - r)}` +
		`Q${round(x + width)},${round(top)} ${round(x + width)},${round(top + r)}` +
		`V${round(base)}Z`
	);
}

/** A bar from the axis at `x` out to `x + length`, square at the axis and rounded at its end. */
export function barPath(x: number, y: number, length: number, height: number, radius = 4): string {
	if (length <= 0 || height <= 0) return '';
	const r = Math.min(radius, length, height / 2);
	const end = x + length;
	return (
		`M${round(x)},${round(y)}H${round(end - r)}` +
		`Q${round(end)},${round(y)} ${round(end)},${round(y + r)}` +
		`V${round(y + height - r)}` +
		`Q${round(end)},${round(y + height)} ${round(end - r)},${round(y + height)}` +
		`H${round(x)}Z`
	);
}

/**
 * Where a key moves a chart's highlighted mark among `count`: the arrows
 * step (on either axis), Home and End go to the ends; null for any other
 * key. From nothing highlighted the first step lands on the first or last.
 */
export function stepMark(key: string, at: number | null, count: number): number | null {
	if (!count) return null;
	switch (key) {
		case 'ArrowRight':
		case 'ArrowDown':
			return at === null ? 0 : Math.min(count - 1, at + 1);
		case 'ArrowLeft':
		case 'ArrowUp':
			return at === null ? count - 1 : Math.max(0, at - 1);
		case 'Home':
			return 0;
		case 'End':
			return count - 1;
	}
	return null;
}

/** Which of `count` evenly spread places a point along a length is nearest. */
export function nearest(x: number, start: number, length: number, count: number): number {
	if (count <= 1) return 0;
	const i = Math.round(((x - start) / length) * (count - 1));
	return Math.max(0, Math.min(count - 1, i));
}

/** Which labels along an axis to write so that none overlap: every nth, and the last. */
export function labelEvery(count: number, room: number, labelWidth: number): number {
	if (count <= 1 || room <= 0) return 1;
	return Math.max(1, Math.ceil((count * labelWidth) / room));
}

/**
 * The largest of `floor` and the values, in a loop: `Math.max(...values)`
 * passes each value as an argument, which throws past about 100,000.
 */
export function largest(floor: number, ...lists: readonly number[][]): number {
	let most = floor;
	for (const list of lists) for (const v of list) if (v > most) most = v;
	return most;
}
