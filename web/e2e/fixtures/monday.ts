// Made-up monday.com board exports, built as Excel workbooks of the same
// shape monday.com writes (every name and address in them is invented), for
// the import's tests: mondayimport.spec.ts and data.e2e.ts. `xlsx` is a
// small workbook writer for them.
import { zip } from './zip';

export { zip };

/** A cell: text, a number, a date (an Excel day number, in a date format), or empty. */
export type Cell = string | number | { date: number; format?: 'iso' | 'short' | 'time' } | null;

const FORMATS = { iso: 1, short: 2, time: 3 } as const;

const escape = (s: string) =>
	s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

const column = (i: number) => {
	let s = '';
	for (let n = i + 1; n > 0; n = Math.floor((n - 1) / 26))
		s = String.fromCharCode(65 + ((n - 1) % 26)) + s;
	return s;
};

/**
 * A workbook of these sheets. Text is written in the cell, as monday.com
 * writes it, or in shared strings, as Excel itself does.
 */
export function xlsx(
	sheets: { name: string; rows: Cell[][] }[],
	{ shared = false } = {}
): Uint8Array {
	const strings: string[] = [];
	const files: Record<string, string> = {
		'[Content_Types].xml':
			'<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>',
		'_rels/.rels':
			'<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>',
		'xl/styles.xml':
			'<?xml version="1.0" encoding="UTF-8" standalone="yes"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><numFmts count="1"><numFmt numFmtId="164" formatCode="yyyy-mm-dd"/></numFmts><cellXfs count="4"><xf numFmtId="0"/><xf numFmtId="164" applyNumberFormat="1"/><xf numFmtId="14" applyNumberFormat="1"/><xf numFmtId="22" applyNumberFormat="1"/></cellXfs></styleSheet>'
	};
	const rels: string[] = [];
	const list: string[] = [];
	sheets.forEach((sheet, i) => {
		const rows = sheet.rows
			.map((cells, r) => {
				const ref = (c: number) => `${column(c)}${r + 1}`;
				const xml = cells
					.map((v, c) => {
						if (v === null) return '';
						if (typeof v === 'number') return `<c r="${ref(c)}"><v>${v}</v></c>`;
						if (typeof v === 'object')
							return `<c r="${ref(c)}" s="${FORMATS[v.format ?? 'iso']}"><v>${v.date}</v></c>`;
						if (shared) {
							strings.push(v);
							return `<c r="${ref(c)}" t="s"><v>${strings.length - 1}</v></c>`;
						}
						return `<c r="${ref(c)}" t="str"><v>${escape(v)}</v></c>`;
					})
					.join('');
				return `<row r="${r + 1}">${xml}</row>`;
			})
			.join('');
		files[`xl/worksheets/sheet${i + 1}.xml`] =
			`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>${rows}</sheetData></worksheet>`;
		rels.push(
			`<Relationship Id="rId${i + 3}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet${i + 1}.xml"/>`
		);
		list.push(`<sheet sheetId="${i + 1}" name="${escape(sheet.name)}" r:id="rId${i + 3}"/>`);
	});
	if (shared)
		rels.push(
			'<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/>'
		);
	rels.push(
		'<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>'
	);
	files['xl/workbook.xml'] =
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>${list.join('')}</sheets></workbook>`;
	files['xl/_rels/workbook.xml.rels'] =
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">${rels.join('')}</Relationships>`;
	if (shared)
		files['xl/sharedStrings.xml'] =
			`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="${strings.length}" uniqueCount="${strings.length}">${strings.map((s) => `<si><t xml:space="preserve">${escape(s)}</t></si>`).join('')}</sst>`;
	return zip(files);
}

/** The sheets with Ada Park called by another name, as the person a test signs up. */
const named = (ada: string, sheets: { name: string; rows: Cell[][] }[]) =>
	sheets.map((s) => ({
		...s,
		rows: s.rows.map((r) =>
			r.map((c) => (typeof c === 'string' ? c.replaceAll('Ada Park', ada) : c))
		)
	}));

// Excel day numbers: 2026-11-06 is 46332.
const day = (n: number) => ({ date: 46332 + n });

const HEADINGS = [
	'Name',
	'Subitems',
	'Owner',
	'Status',
	'Due date',
	'Priority',
	'Files',
	'Tags',
	'Timeline - Start',
	'Timeline - End',
	'Budget',
	'Notes',
	'Item ID'
];

/**
 * A board with two groups, subitems under one item (a column further in,
 * under headings of their own), each group's totals, an item with only a
 * name, tags, two items of one name, and a sheet of updates: one written
 * as HTML with a reply, one with no item ID, one on an item not in the file, and one on
 * the name two items have.
 */
export function gardenBoard(ada = 'Ada Park'): Uint8Array {
	return xlsx(
		named(ada, [
			{
				name: 'Garden Club',
				rows: [
					['Garden Club'],
					['What the club is growing this season.'],
					[],
					['This week'],
					HEADINGS,
					[
						'Order seeds',
						'Pick varieties, Check the budget',
						'Ada Park',
						'Working on it',
						day(2),
						'High',
						'https://files.example.com/seeds.pdf',
						'spring',
						day(1),
						day(3),
						120,
						'From the spring catalog.',
						5001
					],
					['Subitems', 'Name', 'Owner', 'Status', 'Date', 'Item ID'],
					[null, 'Pick varieties', 'Ada Park', 'Done', day(1), 5011],
					[null, 'Check the budget', 'Zed Quinn', null, null, 5012],
					[
						'Build a cold frame',
						null,
						'Zed Quinn, Ada Park',
						'Stuck',
						{ date: 46339, format: 'short' },
						'Critical ⚠️',
						null,
						null,
						null,
						null,
						300,
						null,
						5002
					],
					[
						'Water the beds',
						null,
						null,
						null,
						null,
						null,
						null,
						null,
						null,
						null,
						null,
						null,
						5003
					],
					[null, null, null, null, null, null, null, null, day(1), day(3), 420],
					[],
					['Later'],
					HEADINGS,
					[
						'Plant bulbs',
						null,
						'Ada Park',
						'Done',
						day(30),
						'Low',
						null,
						'autumn, bulbs',
						null,
						null,
						null,
						null,
						5004
					],
					['Mend the hose'],
					['Water the beds', null, null, null, null, null, null, null, null, null, null, null, 5005]
				]
			},
			{
				// As monday.com writes it: a title row, then a row an update or
				// reply, its time in UTC.
				name: 'updates',
				rows: [
					['Garden Club', 'Updates'],
					[
						'Item ID',
						'Item Name',
						'Content Type',
						'Content Type',
						'User',
						'Created At',
						'Update Content',
						'Likes Count',
						'Asset IDs',
						'Post ID',
						'Parent Post ID'
					],
					[
						5001,
						'Order seeds',
						'Update',
						null,
						'Ada Park',
						'02/October/2026  10:30:00 AM',
						'<p>Ordered from <b>Green &amp; Co</b>.</p><p>Arrives Friday.</p>',
						0,
						null,
						7001,
						null
					],
					[
						5001,
						'Order seeds',
						null,
						'Reply',
						'Zed Quinn',
						{ date: 46297.5, format: 'time' },
						'Thanks!',
						0,
						null,
						7002,
						7001
					],
					[
						null,
						'Build a cold frame',
						'Update',
						null,
						'Zed Quinn',
						'2026-10-03 09:00:00 UTC',
						'Need hinges.'
					],
					[9999, 'An old item', 'Update', null, 'Zed Quinn', null, 'Gone now.'],
					[null, 'Water the beds', 'Update', null, 'Ada Park', null, 'Which beds?']
				]
			}
		])
	);
}

/**
 * A small board in shared strings, as Excel itself writes them, with no
 * description, IDs or updates, and an item naming a subitem the file
 * leaves out.
 */
export function errandsBoard(ada = 'Ada Park'): Uint8Array {
	return xlsx(
		named(ada, [
			{
				name: 'Errands',
				rows: [
					['Errands'],
					[],
					['Town'],
					['Name', 'Subitems', 'Person', 'Status', 'Date'],
					['Post the parcel', 'Find some tape', 'Ada Park', '', day(-1)],
					['Return the books', null, null, 'Done', null]
				]
			}
		]),
		{ shared: true }
	);
}
