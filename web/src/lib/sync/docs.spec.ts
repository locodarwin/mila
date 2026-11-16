import { describe, expect, it } from 'vitest';
import * as Y from 'yjs';
import { editable, fromBase64, LiveDoc, toBase64 } from './docs';
import type { Row } from './replica.svelte';

const row = (id: string, update: Uint8Array): Row => ({
	entity: 'doc_update',
	id,
	values: { task_id: 't1', data: toBase64(update), deleted_at: null },
	clocks: {}
});

/** A document typed on another device: its updates. */
function typed(text: string, client: number): Uint8Array[] {
	const d = new Y.Doc();
	d.clientID = client;
	const out: Uint8Array[] = [];
	d.on('update', (u: Uint8Array) => out.push(u));
	const p = new Y.XmlElement('paragraph');
	d.getXmlFragment('default').insert(0, [p]);
	p.insert(0, [new Y.XmlText(text)]);
	return out;
}

const text = (doc: LiveDoc) => doc.fragment.toString();

describe('a live document', () => {
	it('applies rows once and sends only its own edits, gathered', async () => {
		const sent: { id: string; data: string }[] = [];
		const doc = new LiveDoc(
			'w',
			{ kind: 'task', id: 't1' },
			async (id, data) => void sent.push({ id, data })
		);
		const rows = typed('From the laptop', 7).map((u, i) => row(`r${i}`, u));
		doc.sync(rows);
		doc.sync(rows);
		expect(text(doc)).toBe('<paragraph>From the laptop</paragraph>');
		expect(sent).toEqual([]);

		// Two edits here go out as one row, which is not applied again.
		const p = doc.fragment.get(0) as Y.XmlElement;
		(p.get(0) as Y.XmlText).insert(0, 'Hi. ');
		(p.get(0) as Y.XmlText).insert(0, 'Oh. ');
		await doc.flush();
		expect(sent).toHaveLength(1);
		doc.sync([...rows, row(sent[0].id, fromBase64(sent[0].data))]);
		expect(text(doc)).toBe('<paragraph>Oh. Hi. From the laptop</paragraph>');

		// The row holds both edits: another device gets them from it.
		const other = new LiveDoc('w', { kind: 'task', id: 't1' }, async () => {});
		other.sync([...rows, row(sent[0].id, fromBase64(sent[0].data))]);
		expect(text(other)).toBe(text(doc));

		// Closing sends what is left; nothing goes out after.
		(p.get(0) as Y.XmlText).insert(0, '!');
		await doc.close();
		expect(sent).toHaveLength(2);
	});

	it('keeps notes from before live documents read-only until their document arrives', () => {
		expect(editable('', [])).toBe(true);
		expect(editable('Old notes', [])).toBe(false);
		expect(editable('Old notes', [row('r1', typed('x', 3)[0])])).toBe(true);
	});

	it('reads base64 back as it was written', () => {
		const bytes = new Uint8Array(70_000).map((_, i) => i % 251);
		expect(fromBase64(toBase64(bytes))).toEqual(bytes);
	});
});
