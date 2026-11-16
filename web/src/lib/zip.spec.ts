import { describe, expect, it } from 'vitest';
import { zip } from '../../e2e/fixtures/zip';
import { ZipError, inflate, readZip } from './zip';

/** The zip with its nth directory entry's local offset changed to that of the first. */
function sharing(b: Uint8Array, n: number): Uint8Array {
	const out = new Uint8Array(b);
	const view = new DataView(out.buffer);
	const at: number[] = [];
	for (let i = 0; i + 4 <= out.length; i++) if (view.getUint32(i, true) === 0x02014b50) at.push(i);
	view.setUint32(at[n] + 42, view.getUint32(at[0] + 42, true), true);
	return out;
}

describe('a zip', () => {
	it('is read part by part', () => {
		const parts = readZip(zip({ 'a.md': 'a'.repeat(100), 'b.md': 'b' }));
		expect([...parts.keys()]).toEqual(['a.md', 'b.md']);
	});

	it('whose parts share their bytes is refused: one stream read under many names', () => {
		const b = sharing(zip({ 'a.md': 'a'.repeat(100), 'b.md': 'b'.repeat(100) }), 1);
		expect(() => readZip(b)).toThrow(ZipError);
	});

	it('holds a part stored as it is to the limit, as it does one deflated', async () => {
		for (const stored of [false, true]) {
			const b = zip({ 'a.md': 'a'.repeat(100) }, { stored });
			const e = readZip(b).get('a.md')!;
			expect(e.method).toBe(stored ? 0 : 8);
			expect(new TextDecoder().decode(await inflate(b, e, 100))).toBe('a'.repeat(100));
			await expect(inflate(b, e, 99)).rejects.toThrow(new ZipError('large'));
		}
	});
});
