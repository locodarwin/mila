// A wiki page's contents, worked out from its headings as they are drawn.

export interface Heading {
	level: number;
	text: string;
}

/** How far in each heading goes: the page's top level at none, three steps at most. */
export function indents(headings: Heading[]): number[] {
	const top = Math.min(...headings.map((h) => h.level));
	return headings.map((h) => Math.min(h.level - top, 3));
}

/**
 * Which heading the reader is at, by where each one's top is now: the last at
 * or above the line. At the end of a page that scrolled, the last heading,
 * which may never reach the line. A page that fits has no place to mark: -1.
 */
export function currentHeading(
	tops: number[],
	line: number,
	scroll: { top: number; height: number; client: number }
): number {
	if (!tops.length || scroll.height <= scroll.client) return -1;
	if (scroll.top + scroll.client >= scroll.height - 1) return tops.length - 1;
	let at = 0;
	tops.forEach((top, i) => {
		if (top <= line) at = i;
	});
	return at;
}
