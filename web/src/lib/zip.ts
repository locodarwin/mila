// Reading a zip file in the browser, for imports whose file is one (an
// Excel workbook, xlsx.ts; Notion's export, notionimport.ts): its directory
// read by hand, and a part inflated by the browser's own decompression,
// only up to a limit, since the file is untrusted.

/** Why a zip could not be read: not a zip, a password on it, or too large inflated. */
export class ZipError extends Error {
	constructor(readonly reason: 'unreadable' | 'password' | 'large') {
		super(reason);
	}
}

export interface Entry {
	method: number;
	/** Its size in the zip, and once inflated. */
	size: number;
	unpacked: number;
	offset: number;
}

/** The zip's directory: each part's path, how it is stored and where. */
export function readZip(b: Uint8Array): Map<string, Entry> {
	const view = new DataView(b.buffer, b.byteOffset, b.byteLength);
	const u16 = (at: number) => view.getUint16(at, true);
	const u32 = (at: number) => view.getUint32(at, true);
	// The end of the directory is in the last 64 KB and a bit.
	let end = -1;
	for (let i = b.length - 22; i >= Math.max(0, b.length - 65_557); i--)
		if (u32(i) === 0x06054b50) {
			end = i;
			break;
		}
	if (end < 0) throw new ZipError('unreadable');
	const count = u16(end + 10);
	let at = u32(end + 16);
	const entries = new Map<string, Entry>();
	const names = new TextDecoder();
	for (let n = 0; n < count; n++) {
		if (at + 46 > b.length || u32(at) !== 0x02014b50) throw new ZipError('unreadable');
		const flags = u16(at + 8);
		const method = u16(at + 10);
		const size = u32(at + 20);
		const unpacked = u32(at + 24);
		const nameLength = u16(at + 28);
		const skip = u16(at + 30) + u16(at + 32);
		const local = u32(at + 42);
		const name = names.decode(b.subarray(at + 46, at + 46 + nameLength));
		at += 46 + nameLength + skip;
		// An encrypted part cannot be read.
		if (flags & 1) throw new ZipError('password');
		if (local + 30 > b.length || u32(local) !== 0x04034b50) throw new ZipError('unreadable');
		const offset = local + 30 + u16(local + 26) + u16(local + 28);
		if (offset + size > b.length) throw new ZipError('unreadable');
		entries.set(name, { method, size, unpacked, offset });
	}
	// Parts that share their bytes are never in a real zip; allowed, one
	// small stream read under many names inflates to any size.
	const spans = [...entries.values()].filter((e) => e.size > 0).sort((x, y) => x.offset - y.offset);
	for (let i = 1; i < spans.length; i++)
		if (spans[i].offset < spans[i - 1].offset + spans[i - 1].size) throw new ZipError('unreadable');
	return entries;
}

/** A part's bytes, inflated where they are compressed, up to `limit` either way. */
export async function inflate(b: Uint8Array, e: Entry, limit: number): Promise<Uint8Array> {
	const raw = b.subarray(e.offset, e.offset + e.size);
	if (e.method === 0) {
		// A part stored as it is: the size its directory claims is the
		// file's word, so the limit holds to its bytes.
		if (raw.length > limit) throw new ZipError('large');
		return raw;
	}
	if (e.method !== 8) throw new ZipError('unreadable');
	const stream = new Blob([raw as BlobPart])
		.stream()
		.pipeThrough(new DecompressionStream('deflate-raw'));
	const reader = stream.getReader();
	const chunks: Uint8Array[] = [];
	let total = 0;
	try {
		for (;;) {
			const { done, value } = await reader.read();
			if (done) break;
			total += value.length;
			if (total > limit) {
				await reader.cancel();
				throw new ZipError('large');
			}
			chunks.push(value);
		}
	} catch (err) {
		throw err instanceof ZipError ? err : new ZipError('unreadable');
	}
	const out = new Uint8Array(total);
	let at = 0;
	for (const c of chunks) {
		out.set(c, at);
		at += c.length;
	}
	return out;
}
