// A small zip writer for the import tests' made-up files: Excel workbooks
// (monday.ts) and Notion exports (notion.ts).
import { crc32, deflateRawSync } from 'node:zlib';

/** A zip of the files given, each deflated, as a workbook is, or stored as it is. */
export function zip(
	files: Record<string, string | Uint8Array>,
	{ stored = false } = {}
): Uint8Array {
	const method = stored ? 0 : 8;
	const locals: Buffer[] = [];
	const centrals: Buffer[] = [];
	let offset = 0;
	for (const [name, text] of Object.entries(files)) {
		const data = typeof text === 'string' ? Buffer.from(text, 'utf8') : Buffer.from(text);
		const packed = stored ? data : deflateRawSync(data);
		const path = Buffer.from(name, 'utf8');
		const crc = crc32(data);
		const local = Buffer.alloc(30);
		local.writeUInt32LE(0x04034b50, 0);
		local.writeUInt16LE(20, 4);
		local.writeUInt16LE(method, 8);
		local.writeUInt32LE(crc, 14);
		local.writeUInt32LE(packed.length, 18);
		local.writeUInt32LE(data.length, 22);
		local.writeUInt16LE(path.length, 26);
		const central = Buffer.alloc(46);
		central.writeUInt32LE(0x02014b50, 0);
		central.writeUInt16LE(20, 4);
		central.writeUInt16LE(20, 6);
		central.writeUInt16LE(method, 10);
		central.writeUInt32LE(crc, 16);
		central.writeUInt32LE(packed.length, 20);
		central.writeUInt32LE(data.length, 24);
		central.writeUInt16LE(path.length, 28);
		central.writeUInt32LE(offset, 42);
		locals.push(local, path, packed);
		centrals.push(central, path);
		offset += local.length + path.length + packed.length;
	}
	const dir = Buffer.concat(centrals);
	const end = Buffer.alloc(22);
	end.writeUInt32LE(0x06054b50, 0);
	end.writeUInt16LE(Object.keys(files).length, 8);
	end.writeUInt16LE(Object.keys(files).length, 10);
	end.writeUInt32LE(dir.length, 12);
	end.writeUInt32LE(offset, 16);
	return new Uint8Array(Buffer.concat([...locals, dir, end]));
}
