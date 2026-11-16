// The tabs, periods and charts of a report (docs/PLAN.md section 3, Release
// 3.0), apart from reports.ts so a saved report view's page and the editor can
// name them without loading what the reports work out.
export type ReportTab = 'status' | 'workload' | 'flow';
export const REPORT_TABS: ReportTab[] = ['status', 'workload', 'flow'];

/**
 * Each chart a report draws, by the key a report in a page names it with
 * (`<!-- mila:report <view> <chart> -->`), in the order the tabs show them.
 */
export const REPORT_CHARTS = [
	{ key: 'status-numbers', tab: 'status', label: 'Headline numbers' },
	{ key: 'burnup', tab: 'status', label: 'Burnup' },
	{ key: 'where', tab: 'status', label: 'Where the open work is' },
	{ key: 'attention', tab: 'status', label: 'Needs attention' },
	{ key: 'by-status', tab: 'status', label: 'Tasks by status' },
	{ key: 'by-priority', tab: 'status', label: 'Open work by priority' },
	{ key: 'overdue', tab: 'status', label: 'How overdue' },
	{ key: 'by-label', tab: 'status', label: 'Open work by label' },
	{ key: 'by-project', tab: 'status', label: 'Open work by project' },
	{ key: 'by-person', tab: 'workload', label: 'Open work by person' },
	{ key: 'due-weeks', tab: 'workload', label: 'Due by week' },
	{ key: 'flow-numbers', tab: 'flow', label: 'Throughput numbers' },
	{ key: 'made-done', tab: 'flow', label: 'Made and done each week' },
	{ key: 'cycle', tab: 'flow', label: 'How long tasks took' },
	{ key: 'cfd', tab: 'flow', label: 'Cumulative flow' },
	{ key: 'dots', tab: 'flow', label: 'Cycle time, task by task' }
] as const satisfies readonly { key: string; tab: ReportTab; label: string }[];

export type ReportChart = (typeof REPORT_CHARTS)[number]['key'];

/** The chart a key names, or none for one this app doesn't know. */
export const chartOf = (key: unknown) => REPORT_CHARTS.find((c) => c.key === key);

export type PeriodKind = 'weeks6' | 'months3' | 'year' | 'range';
export const PERIOD_KINDS: PeriodKind[] = ['weeks6', 'months3', 'year', 'range'];
