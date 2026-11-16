// How the desktop layout is arranged on this device: whether the sidebar
// is collapsed, how wide it, the task panel, the wiki's parts, a board's
// columns and a timeline's task names are, whether
// the task panel fills the page, and which of its summary lines are open. Remembered per device, not synced: a laptop and a
// wide monitor want different widths.

export const SIDEBAR = { min: 180, max: 420, initial: 248 };
export const DETAIL = { min: 320, max: 900, initial: 460 };
export const WIKI_TREE = { min: 160, max: 480, initial: 240 };
// The widest is whatever the screen has; Wiki.svelte stops at the space there is.
export const WIKI_PAGE = { min: 420, max: 3840, initial: 760 };
// The panel beside the page shows only when at least this much is left over.
export const WIKI_SIDE_MIN = 200;
// A board's columns, all one width once set (until then they fit between 260
// and 300 px), and the task names beside a timeline.
export const BOARD_COLUMN = { min: 200, max: 560, initial: 280 };
export const TIMELINE_NAMES = { min: 160, max: 560, initial: 280 };

const KEYS = {
	collapsed: 'mila.sidebar.collapsed',
	sidebar: 'mila.sidebar.width',
	detail: 'mila.detail.width',
	expanded: 'mila.detail.expanded',
	sections: 'mila.detail.sections',
	folded: 'mila.sidebar.folded',
	wikiTree: 'mila.wiki.treeWidth',
	wikiPage: 'mila.wiki.pageWidth',
	wikiSide: 'mila.wiki.side',
	wikiSideOpen: 'mila.wiki.sideOpen',
	wikiPageAlone: 'mila.wiki.pageAloneWidth',
	boardColumn: 'mila.board.columnWidth',
	timelineNames: 'mila.timeline.namesWidth'
};

// What opens in the task panel's summary layout before anyone chooses.
const FIRST_OPEN = ['schedule', 'notes'];

/** A width kept between its limits, whole pixels. */
export function clampWidth(v: number, limits: { min: number; max: number }): number {
	return Math.round(Math.min(limits.max, Math.max(limits.min, v)));
}

/** What the panel beside a wiki page shows. */
export type WikiSide = 'contents' | 'pages' | 'comments';

function read(key: string): string | null {
	try {
		return localStorage.getItem(key);
	} catch {
		return null;
	}
}

function readWikiSide(): WikiSide {
	const side = read(KEYS.wikiSide);
	return side === 'pages' || side === 'comments' ? side : 'contents';
}

function write(key: string, value: string) {
	try {
		localStorage.setItem(key, value);
	} catch {
		// remembered for this visit only
	}
}

function width(key: string, limits: { min: number; max: number; initial: number }): number {
	const n = Number(read(key));
	return read(key) !== null && Number.isFinite(n) ? clampWidth(n, limits) : limits.initial;
}

function foldedLists(): string[] {
	try {
		const v = JSON.parse(read(KEYS.folded) ?? 'null');
		if (Array.isArray(v) && v.every((x) => typeof x === 'string')) return v;
	} catch {
		// nothing usable: all open
	}
	return [];
}

function openSections(): string[] {
	try {
		const v = JSON.parse(read(KEYS.sections) ?? 'null');
		if (Array.isArray(v) && v.every((x) => typeof x === 'string')) return v;
	} catch {
		// nothing usable: as it first is
	}
	return FIRST_OPEN;
}

class Layout {
	collapsed = $state(read(KEYS.collapsed) === '1');
	sidebarWidth = $state(width(KEYS.sidebar, SIDEBAR));
	detailWidth = $state(width(KEYS.detail, DETAIL));
	/** The task panel fills the page instead of peeking in beside the list. */
	expanded = $state(read(KEYS.expanded) === '1');
	#sections = $state.raw(openSections());
	#folded = $state.raw(foldedLists());
	wikiTreeWidth = $state(width(KEYS.wikiTree, WIKI_TREE));
	wikiPageWidth = $state(width(KEYS.wikiPage, WIKI_PAGE));
	/** Null until someone resizes them: the board's own fit. */
	boardColumnWidth = $state<number | null>(
		read(KEYS.boardColumn) ? width(KEYS.boardColumn, BOARD_COLUMN) : null
	);
	timelineNamesWidth = $state(width(KEYS.timelineNames, TIMELINE_NAMES));
	/** What the panel beside a wiki page shows. */
	wikiSide = $state<WikiSide>(readWikiSide());
	wikiSideOpen = $state(read(KEYS.wikiSideOpen) !== 'closed');
	/** The page's width with that panel hidden; null takes all the room there is. */
	wikiPageAlone = $state<number | null>(
		read(KEYS.wikiPageAlone) ? width(KEYS.wikiPageAlone, WIKI_PAGE) : null
	);

	toggleSidebar() {
		this.collapsed = !this.collapsed;
		write(KEYS.collapsed, this.collapsed ? '1' : '0');
	}

	setSidebarWidth(px: number) {
		this.sidebarWidth = clampWidth(px, SIDEBAR);
		write(KEYS.sidebar, String(this.sidebarWidth));
	}

	setDetailWidth(px: number) {
		this.detailWidth = clampWidth(px, DETAIL);
		write(KEYS.detail, String(this.detailWidth));
	}

	setWikiTreeWidth(px: number) {
		this.wikiTreeWidth = clampWidth(px, WIKI_TREE);
		write(KEYS.wikiTree, String(this.wikiTreeWidth));
	}

	setWikiPageWidth(px: number) {
		this.wikiPageWidth = clampWidth(px, WIKI_PAGE);
		write(KEYS.wikiPage, String(this.wikiPageWidth));
	}

	/** Null puts the board back to its own fit. */
	setBoardColumnWidth(px: number | null) {
		this.boardColumnWidth = px === null ? null : clampWidth(px, BOARD_COLUMN);
		write(KEYS.boardColumn, this.boardColumnWidth === null ? '' : String(this.boardColumnWidth));
	}

	setTimelineNamesWidth(px: number) {
		this.timelineNamesWidth = clampWidth(px, TIMELINE_NAMES);
		write(KEYS.timelineNames, String(this.timelineNamesWidth));
	}

	setWikiSide(side: WikiSide) {
		this.wikiSide = side;
		write(KEYS.wikiSide, side);
	}

	setWikiSideOpen(open: boolean) {
		this.wikiSideOpen = open;
		write(KEYS.wikiSideOpen, open ? 'open' : 'closed');
	}

	setWikiPageAlone(px: number | null) {
		this.wikiPageAlone = px === null ? null : clampWidth(px, WIKI_PAGE);
		write(KEYS.wikiPageAlone, this.wikiPageAlone === null ? '' : String(this.wikiPageAlone));
	}

	/**
	 * Whether a list in the sidebar shows. A collapsed sidebar has no heading to open
	 * one with, so its lists always show.
	 */
	sidebarListOpen(id: string): boolean {
		return this.collapsed || !this.#folded.includes(id);
	}

	setSidebarList(id: string, open: boolean) {
		if (open !== this.#folded.includes(id)) return;
		this.#folded = open ? this.#folded.filter((s) => s !== id) : [...this.#folded, id];
		write(KEYS.folded, JSON.stringify(this.#folded));
	}

	/** Whether a section of the task panel's summary layout is open. */
	sectionOpen(id: string): boolean {
		return this.#sections.includes(id);
	}

	setSection(id: string, open: boolean) {
		if (open === this.sectionOpen(id)) return;
		this.#sections = open ? [...this.#sections, id] : this.#sections.filter((s) => s !== id);
		write(KEYS.sections, JSON.stringify(this.#sections));
	}

	toggleExpanded() {
		this.expanded = !this.expanded;
		write(KEYS.expanded, this.expanded ? '1' : '0');
	}
}

export const layout = new Layout();
