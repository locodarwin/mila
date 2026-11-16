// How the Reports page is set up on this device, per workspace: the tab,
// the project, the period and the saved-view filter (docs/PLAN.md section 3, Release
// 3.0). Kept in localStorage under mila.reports.<ws>, as Inbox, My Day and
// Upcoming keep theirs (listViews.svelte.ts); signing out clears it.
import { PERIOD_KINDS, REPORT_TABS, type PeriodKind, type ReportTab } from './reportKinds';
import type { ViewEdit } from './sync/data';
import type { ViewDisplay, ViewFilter } from './sync/entities';
import { onSignOut } from './forgetting';

export interface ReportSetup {
	tab: ReportTab;
	/** The one project reported on, or null for all of them. */
	project: string | null;
	period: PeriodKind;
	/** The days picked, for a period of picked dates. */
	from: string | null;
	to: string | null;
	filter: ViewFilter;
}

export const freshReportSetup = (): ReportSetup => ({
	tab: 'status',
	project: null,
	period: 'weeks6',
	from: null,
	to: null,
	filter: {}
});

const keyOf = (ws: string) => `mila.reports.${ws}`;
const DAY = /^\d{4}-\d{2}-\d{2}$/;
/**
 * The earliest day a report's dates may start: a shared report view from the
 * year 1000 had every reader's device work through 375,000 days.
 */
export const EARLIEST_DAY = '2000-01-01';

/** A stored setup as far as it can be read; anything else is the page as it comes. */
export function readSetup(raw: string | null): ReportSetup {
	if (!raw) return freshReportSetup();
	try {
		return setupOf(JSON.parse(raw));
	} catch {
		return freshReportSetup();
	}
}

const isObject = (x: unknown): x is Record<string, unknown> =>
	typeof x === 'object' && x !== null && !Array.isArray(x);

/** A setup from what was stored, each part as far as it can be read. */
function setupOf(raw: unknown): ReportSetup {
	const fresh = freshReportSetup();
	if (!isObject(raw)) return fresh;
	const v = raw as Partial<ReportSetup>;
	const day = (x: unknown) =>
		typeof x === 'string' && DAY.test(x) && x >= EARLIEST_DAY && x <= '2999-12-31' ? x : null;
	return {
		tab: REPORT_TABS.includes(v.tab as ReportTab) ? (v.tab as ReportTab) : fresh.tab,
		project: typeof v.project === 'string' && v.project ? v.project : null,
		period: PERIOD_KINDS.includes(v.period as PeriodKind) ? (v.period as PeriodKind) : fresh.period,
		from: day(v.from),
		to: day(v.to),
		filter: isObject(v.filter) ? (v.filter as ViewFilter) : {}
	};
}

/**
 * A report view's setup: its tab, period and project from its display's
 * report part, its filter the view's own (docs/PLAN.md section 3, Release
 * 3.0). A view of a project reports on that project alone.
 */
export function viewReport(view: {
	projectId: string | null;
	filter: ViewFilter;
	display: ViewDisplay;
}): ReportSetup {
	const r = isObject(view.display.report) ? view.display.report : {};
	return {
		...setupOf({ ...r, filter: view.filter }),
		project: view.projectId ?? setupOf(r).project
	};
}

/** What a report view keeps in its display: the setup but its filter. */
export function reportDisplay(setup: ReportSetup): Record<string, unknown> {
	const { tab, period, from, to, project } = setup;
	return { tab, period, from, to, project };
}

/**
 * Where a report's setup comes from and goes to: this device's, for the
 * Reports page, or a report view's.
 */
export interface ReportSource {
	readonly setup: ReportSetup;
	change(patch: Partial<ReportSetup>): void;
	editFilter(edit: ViewEdit<ViewFilter, Partial<ViewFilter>>): void;
}

/** The Reports page's own: this device's, for the workspace. */
export function deviceReport(ws: string): ReportSource {
	return {
		get setup() {
			return reportSetups.get(ws);
		},
		change: (patch) => reportSetups.change(ws, patch),
		editFilter: (edit) => reportSetups.editFilter(ws, edit)
	};
}

class ReportSetups {
	/** Forgets every setup held in memory, at sign-out. */
	forget() {
		this.#setups = {};
	}

	#setups = $state<Record<string, ReportSetup>>({});

	get(ws: string): ReportSetup {
		const known = this.#setups[ws];
		if (known) return known;
		let raw: string | null = null;
		try {
			raw = localStorage.getItem(keyOf(ws));
		} catch {
			// Without storage the page comes as it is.
		}
		return readSetup(raw);
	}

	set(ws: string, setup: ReportSetup) {
		this.#setups[ws] = setup;
		try {
			localStorage.setItem(keyOf(ws), JSON.stringify(setup));
		} catch {
			// Kept for this visit only.
		}
	}

	change(ws: string, patch: Partial<ReportSetup>) {
		this.set(ws, { ...this.get(ws), ...patch });
	}

	/** A filter change, as a saved view's: the parts set, undefined taking one away. */
	editFilter(ws: string, edit: ViewEdit<ViewFilter, Partial<ViewFilter>>) {
		const setup = this.get(ws);
		const patch = typeof edit === 'function' ? edit(setup.filter) : edit;
		const filter: Record<string, unknown> = { ...setup.filter };
		for (const [k, v] of Object.entries(patch)) {
			if (v === undefined) delete filter[k];
			else filter[k] = v;
		}
		this.set(ws, { ...setup, filter: filter as ViewFilter });
	}
}

export const reportSetups = new ReportSetups();
onSignOut(() => reportSetups.forget());
