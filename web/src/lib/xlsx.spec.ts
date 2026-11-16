import { describe, expect, it } from 'vitest';
import { xlsx, zip } from '../../e2e/fixtures/monday';
import { XlsxError, excelDate, readXlsx, walk } from './xlsx';

const sheetXml = (cells: string, x = '') =>
	`<?xml version="1.0"?><${x}worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><${x}sheetData>${cells}</${x}sheetData></${x}worksheet>`;
const workbook = (sheet: string, extra: Record<string, string> = {}) =>
	zip({
		'xl/workbook.xml':
			'<workbook xmlns:r="r"><workbookPr date1904="0"/><sheets><sheet name="One" r:id="rId1"/></sheets></workbook>',
		'xl/_rels/workbook.xml.rels':
			'<Relationships><Relationship Id="rId1" Type="x/worksheet" Target="/xl/worksheets/a.xml"/></Relationships>',
		'xl/worksheets/a.xml': sheet,
		...extra
	});

describe('An Excel workbook', () => {
	it('is read a sheet at a time, its text, numbers and dates as text', async () => {
		const sheets = await readXlsx(
			xlsx(
				[
					{
						name: 'Board & more',
						rows: [
							['Name', 'Count', 'Day', 'Short', 'At'],
							[
								'Tom & Jerry <3',
								12.5,
								{ date: 46332 },
								{ date: 46332, format: 'short' },
								{ date: 46332.75, format: 'time' }
							],
							[],
							[null, null, 'third']
						]
					},
					{ name: 'Second', rows: [['x']] }
				],
				{ shared: true }
			)
		);
		expect(sheets.map((s) => s.name)).toEqual(['Board & more', 'Second']);
		expect(sheets[0].rows).toEqual([
			['Name', 'Count', 'Day', 'Short', 'At'],
			['Tom & Jerry <3', '12.5', '2026-11-06', '2026-11-06', '2026-11-06 18:00'],
			[],
			['', '', 'third']
		]);
	});

	it('reads inline text, booleans, errors, escapes and prefixed names', async () => {
		const [sheet] = await readXlsx(
			workbook(
				sheetXml(
					'<x:row r="2"><x:c r="B2" t="inlineStr"><x:is><x:r><x:t>Two</x:t></x:r><x:r><x:t xml:space="preserve"> runs</x:t></x:r><x:rPh><x:t>ignored</x:t></x:rPh></x:is></x:c>' +
						'<x:c r="C2" t="b"><x:v>1</x:v></x:c><x:c r="D2" t="e"><x:v>#N/A</x:v></x:c>' +
						'<x:c r="E2" t="str"><x:v>a_x000D__x000A_b&#x1;&#65;&lt;</x:v></x:c><x:c r="F2" t="d"><x:v>2026-11-06</x:v></x:c></x:row>',
					'x:'
				)
			)
		);
		expect(sheet.rows).toEqual([[], ['', 'Two runs', 'TRUE', '', 'a\nbA<', '2026-11-06']]);
	});

	it('is refused when it is no workbook, has a password or is too large', async () => {
		await expect(readXlsx(new TextEncoder().encode('Name,Status\nA,B'))).rejects.toThrow(
			'could not be read as an Excel workbook'
		);
		await expect(readXlsx(zip({ 'a.txt': 'x' }))).rejects.toThrow(XlsxError);
		// The first entry of the zip's directory marked encrypted.
		const locked = workbook(sheetXml(''));
		const view = new DataView(locked.buffer);
		let central = 0;
		while (view.getUint32(central, true) !== 0x02014b50) central++;
		view.setUint16(central + 8, 1, true);
		await expect(readXlsx(locked)).rejects.toThrow('has a password');
		const big = workbook(sheetXml(`<row r="1"><c t="str"><v>${'x'.repeat(5000)}</v></c></row>`));
		await expect(readXlsx(big, { limit: 1000 })).rejects.toThrow('larger than an import takes');
		await expect(
			readXlsx(workbook(sheetXml('<row r="100001"><c r="A100001"><v>1</v></c></row>')))
		).rejects.toThrow('larger than an import takes');
	});
});

describe("Excel's day numbers", () => {
	it('are dates, with the time where there is one', () => {
		expect(excelDate(46332)).toBe('2026-11-06');
		expect(excelDate(46332.5)).toBe('2026-11-06 12:00');
		expect(excelDate(46332 + 1 / 86400)).toBe('2026-11-06 00:00:01');
		expect(excelDate(1)).toBe('1900-01-01');
		expect(excelDate(61)).toBe('1900-03-01');
		expect(excelDate(0, true)).toBe('1904-01-01');
		expect(excelDate(-1)).toBeNull();
		expect(excelDate(Number.NaN)).toBeNull();
	});
});

describe('The XML scanner', () => {
	it('gives each element, its attributes and its text, in order', () => {
		const seen: string[] = [];
		walk(
			'<?xml version="1.0"?><!-- note --><a x="1 > 0" y=\'q&amp;\'>t&lt;<b/><![CDATA[<raw>]]></a>',
			{
				open: (n, a, empty) => seen.push(`open ${n} ${JSON.stringify(a)} ${empty}`),
				close: (n) => seen.push(`close ${n}`),
				text: (s) => seen.push(`text ${s}`)
			}
		);
		expect(seen).toEqual([
			'open a {"x":"1 > 0","y":"q&"} false',
			'text t<',
			'open b {} true',
			'close b',
			'text <raw>',
			'close a'
		]);
		expect(() => walk('<a x="1', {})).toThrow(XlsxError);
	});
});

describe('a tag a stranger made', () => {
	it('has its attributes read in time in line with its length', async () => {
		const { attributes } = await import('./xlsx');
		const start = performance.now();
		attributes('sheet ' + 'a'.repeat(200_000));
		expect(performance.now() - start).toBeLessThan(1000);
		expect(attributes(`sheet name="One &amp; two" r:id='rId1' broken =`)).toEqual({
			name: 'One & two',
			'r:id': 'rId1'
		});
	});
});
