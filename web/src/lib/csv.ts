// A CSV reader (RFC 4180): quoted fields with commas, quotes and line
// breaks inside, CRLF or LF line ends, a byte order mark. Files saved by
// spreadsheets in regions that write decimals with a comma use semicolons
// or tabs between fields; the separator is taken from the first line.

/** The separator a file uses: whichever of , ; and tab its first line has most of, outside quotes. */
export function sniffSeparator(text: string): ',' | ';' | '\t' {
	const counts = { ',': 0, ';': 0, '\t': 0 };
	let quoted = false;
	for (const ch of text) {
		if (ch === '"') quoted = !quoted;
		else if (!quoted && (ch === '\n' || ch === '\r')) break;
		else if (!quoted && ch in counts) counts[ch as keyof typeof counts]++;
	}
	if (counts[';'] > counts[','] && counts[';'] >= counts['\t']) return ';';
	if (counts['\t'] > counts[',']) return '\t';
	return ',';
}

/** The rows of a CSV file, each a list of fields. Blank lines are left out. */
export function parseCsv(text: string, separator = sniffSeparator(text)): string[][] {
	if (text.charCodeAt(0) === 0xfeff) text = text.slice(1);
	const rows: string[][] = [];
	let row: string[] = [];
	let field = '';
	let quoted = false;
	let i = 0;
	const endRow = () => {
		row.push(field);
		field = '';
		if (row.length > 1 || row[0] !== '') rows.push(row);
		row = [];
	};
	while (i < text.length) {
		const ch = text[i];
		if (quoted) {
			if (ch === '"') {
				if (text[i + 1] === '"') {
					field += '"';
					i += 2;
					continue;
				}
				quoted = false;
			} else {
				field += ch;
			}
			i++;
			continue;
		}
		if (ch === '"' && field === '') {
			quoted = true;
		} else if (ch === separator) {
			row.push(field);
			field = '';
		} else if (ch === '\r' || ch === '\n') {
			endRow();
			if (ch === '\r' && text[i + 1] === '\n') i++;
		} else {
			field += ch;
		}
		i++;
	}
	if (field !== '' || row.length > 0) endRow();
	return rows;
}

/**
 * A CSV field, quoted when it must be, and kept from running as a formula:
 * one starting with = + - or @, after any spaces, or with a tab or CR, gets
 * a ' first, unless it is a plain negative number such as -5. A field with
 * ; or a tab is quoted too, since Excel in a ; locale splits lines on it,
 * and the part after one would otherwise be a cell of its own. The server's
 * CSV export (csvField in ownership/export.go) follows the same rule, held
 * to it by shared/csv-vectors.json.
 */
function field(v: string | number): string {
	let s = String(v);
	if (typeof v === 'string' && /^(?:[\t\r]|\s*[=+\-@])/.test(s) && !/^-\d+(?:\.\d+)?$/.test(s))
		s = `'${s}`;
	return /[",;\t\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

/** Rows as CSV (RFC 4180, CRLF line ends), a spreadsheet's formulas kept from running. */
export function writeCsv(rows: (string | number)[][]): string {
	return rows.map((r) => r.map(field).join(',')).join('\r\n') + '\r\n';
}
