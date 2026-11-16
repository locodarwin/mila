// A made-up Notion export (Markdown & CSV), of the same shape as Notion
// writes (every name in it is invented), for the import's tests:
// notionimport.spec.ts and data.e2e.ts. A zip holding a Part zip, as a
// large export is given; pages under pages, links between them and to a
// page not in the export, an image and a file beside a page, a callout and
// a toggle, a code block, and two databases: a reading list with a page
// for one row, and tasks with a page for one row (with a callout whose icon
// is a picture, as Notion writes its own icons), another page with no row,
// a person (with the space after the name Notion leaves), tags, a Critical
// priority, and dates in two of Notion's ways.
import { zip } from './zip';

const id = (n: number) => `0f1e2d3c4b5a69788796a5b4c3d2e1${String(n).padStart(2, '0')}`;
const enc = (path: string) => path.split('/').map(encodeURIComponent).join('/');

export const IDS = {
	handbook: id(1),
	onboarding: id(2),
	reading: id(3),
	garden: id(4),
	tasks: id(5),
	offsite: id(6),
	untitled: id(7),
	gone: id(9)
};

const HANDBOOK = `Team handbook ${IDS.handbook}`;
const ONBOARDING = `${HANDBOOK}/Onboarding ${IDS.onboarding}`;
const READING = `${HANDBOOK}/Reading list ${IDS.reading}`;
const TASKS = `Tasks ${IDS.tasks}`;
const OFFSITE = `${TASKS}/Plan the offsite ${IDS.offsite}`;

/** A PNG of one pixel. */
export const PIXEL = Uint8Array.from(
	atob(
		'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=='
	),
	(c) => c.charCodeAt(0)
);

/** The export's files, by path. */
export function notionFiles(ada = 'Ada Park'): Record<string, string | Uint8Array> {
	return {
		[`${HANDBOOK}.md`]: `# Team handbook

Welcome to the team. Start with [Onboarding](${enc(`Team handbook ${IDS.handbook}/Onboarding ${IDS.onboarding}.md`)}), and the [old wiki](https://www.notion.so/Old-wiki-${IDS.gone}) is gone.

<aside>
💡 Ask in the team channel first.
</aside>

<details>
<summary>Office hours</summary>
Tuesdays at 10.
</details>

[Reading list](${enc(`Team handbook ${IDS.handbook}/Reading list ${IDS.reading}.csv`)})

\`\`\`html
<aside>kept as it is</aside>
\`\`\`
`,
		[`${ONBOARDING}.md`]: `# Onboarding

![Desk](${enc(`Onboarding ${IDS.onboarding}/desk.png`)})

The [checklist](${enc(`Onboarding ${IDS.onboarding}/checklist.pdf`)}) has the rest. Back to the [Team handbook](../${enc(`Team handbook ${IDS.handbook}.md`)}).
`,
		[`${ONBOARDING}/desk.png`]: PIXEL,
		[`${ONBOARDING}/checklist.pdf`]: '%PDF-1.4 a made-up file',
		[`${READING}.csv`]:
			'\uFEFFName,Author\nThe Quiet Garden,Lena Marsh\nRivers of Salt,Tomas Reyes\n',
		[`${READING}_all.csv`]:
			'\uFEFFName,Author,Read\nThe Quiet Garden,Lena Marsh,Yes\nRivers of Salt,Tomas Reyes,No\n',
		[`${READING}/The Quiet Garden ${IDS.garden}.md`]: `# The Quiet Garden

Author: Lena Marsh
Read: Yes

Note: a slow book, worth it.
`,
		[`${TASKS}.csv`]: '\uFEFFTask name,Status\nPlan the offsite,In progress\n',
		[`${TASKS}_all.csv`]: `\uFEFFTask name,Status,Assignee,Due date,Priority,Tags,Points
Plan the offsite,In progress,"${ada} ","April 30, 2025",High,"events, planning",3
Book the venue,Not started,Zed Quinn,05/02/2025,Critical,events,
Send invites,Done,,"May 1, 2025 → May 3, 2025",Low,,
`,
		[`${OFFSITE}.md`]: `# Plan the offsite

Status: In progress
Assignee: ${ada}
Due date: April 30, 2025

Read [Onboarding](../${enc(`Team handbook ${IDS.handbook}/Onboarding ${IDS.onboarding}.md`)}) first.

![Map](${enc(`Plan the offsite ${IDS.offsite}/map.png`)})

<aside>
<img src="https://app.notion.com/icons/notion_gray.svg" alt="https://app.notion.com/icons/notion_gray.svg" width="40px" />

**Budget**

Keep it under the team's allowance.

</aside>
`,
		[`${OFFSITE}/map.png`]: PIXEL,
		[`${TASKS}/Untitled ${IDS.untitled}.md`]: `# Untitled

Status: Not started
`
	};
}

/**
 * An export made without folders for subpages: every page at the top, each
 * subpage a link on a line of its own in its page. Recipes has Soups and
 * Breads; Soups has Stock, and mentions Breads in a sentence; Stock links
 * back to Recipes on a line of its own.
 */
export function notionFlatFiles(): Record<string, string | Uint8Array> {
	const page = (title: string, n: number) => `${title} ${id(20 + n)}.md`;
	const [recipes, soups, breads, stock] = [
		page('Recipes', 1),
		page('Soups', 2),
		page('Breads', 3),
		page('Stock', 4)
	];
	return {
		[recipes]: `# Recipes\n\n[Soups](${enc(soups)})\n\n[Breads](${enc(breads)})\n`,
		[soups]: `# Soups\n\nServe with one of the [Breads](${enc(breads)}).\n\n![Pot](pot.png)\n\n[Stock](${enc(stock)})\n`,
		'pot.png': PIXEL,
		[breads]: `# Breads\n\nFlour, water, salt.\n`,
		[stock]: `# Stock\n\nBack to [Recipes](${enc(recipes)}):\n\n[Recipes](${enc(recipes)})\n`
	};
}

/** The export as Notion gives it: a zip with a Part zip inside. */
export function notionExport(ada = 'Ada Park'): Uint8Array {
	return zip({ 'Export-0f1e2d3c-Part-1.zip': zip(notionFiles(ada)) });
}
