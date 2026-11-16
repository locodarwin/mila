import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { WorkspaceTheme } from '../sync/entities';
import { blank } from './document';
import {
	MAX_ANSWERS,
	suggestion,
	withAnswer,
	workspaceDoc,
	workspaceThemes
} from './workspaceThemes.svelte';

const ID = '0b5c4f6e-8a1d-4c2e-9f3a-1d2e3f4a5b6c';

function row(over: Partial<WorkspaceTheme> = {}): WorkspaceTheme {
	return {
		id: ID,
		name: 'Harbor',
		body: { ...blank('calm', 'My harbor'), id: '1d2e3f4a-5b6c-4f6e-8a1d-0b5c4f6e9f3a' },
		sourceId: null,
		suggestedAt: null,
		publishedBy: null,
		publishedAt: null,
		sortKey: 'a0',
		...over
	};
}

describe('workspace themes', () => {
	it('read a row as a document under its own id and name', () => {
		const doc = workspaceDoc(row());
		expect(doc?.id).toBe(ID);
		expect(doc?.name).toBe('Harbor');
		expect(doc?.base).toBe('calm');
		expect(workspaceDoc(row({ body: 'a theme' }))).toBeNull();
		expect(workspaceDoc(row({ body: [] }))).toBeNull();
		expect(workspaceDoc(row({ body: { mila: 'template' } }))).toBeNull();
	});

	it('read the same body as the same document, until its name changes', () => {
		const r = row();
		expect(workspaceDoc(r)).toBe(workspaceDoc({ ...r }));
		expect(workspaceDoc({ ...r, name: 'Pier' })?.name).toBe('Pier');
	});

	it('tell the picker only when what is offered changes', () => {
		const list = [row()];
		workspaceThemes.offer({ ws: 'ws1', wsName: 'Acme Studio', list });
		const first = workspaceThemes.offered;
		workspaceThemes.offer({ ws: 'ws1', wsName: 'Acme Studio', list: list.map((t) => ({ ...t })) });
		expect(workspaceThemes.offered).toBe(first);
		workspaceThemes.offer({
			ws: 'ws1',
			wsName: 'Acme Studio',
			list: [{ ...list[0], name: 'Pier' }]
		});
		expect(workspaceThemes.offered).not.toBe(first);
		workspaceThemes.offer(null);
		expect(workspaceThemes.offered).toBeNull();
	});

	it('suggest the one suggested last', () => {
		const a = row({ id: 'a', suggestedAt: '2026-11-01T10:00:00Z' });
		const b = row({ id: 'b', suggestedAt: '2026-11-01T11:00:00Z' });
		const c = row({ id: 'c' });
		expect(suggestion([a, b, c])?.id).toBe('b');
		expect(suggestion([c])).toBeUndefined();
	});

	it('keep each answer once, the newest last, up to the limit', () => {
		expect(withAnswer(['a', 'b'], 'a')).toEqual(['b', 'a']);
		const many = Array.from({ length: MAX_ANSWERS }, (_, i) => `t${i}`);
		const next = withAnswer(many, 'new');
		expect(next).toHaveLength(MAX_ANSWERS);
		expect(next[0]).toBe('t1');
		expect(next.at(-1)).toBe('new');
	});
});

describe('the workspace theme in use', () => {
	const stored = new Map<string, string>();
	beforeEach(() => {
		stored.clear();
		vi.stubGlobal('localStorage', {
			getItem: (k: string) => stored.get(k) ?? null,
			setItem: (k: string, v: string) => void stored.set(k, v),
			removeItem: (k: string) => void stored.delete(k)
		});
		workspaceThemes.forget();
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('is kept on the device, and marked gone when no longer offered', () => {
		const doc = workspaceDoc(row())!;
		workspaceThemes.keep('ws1', 'Acme Studio', doc, 'u1');
		expect(workspaceThemes.get(ID)?.name).toBe('Harbor');
		expect(workspaceThemes.get('another')).toBeUndefined();
		const kept = JSON.parse(localStorage.getItem('mila.workspaceTheme')!);
		expect(kept).toMatchObject({ ws: 'ws1', wsName: 'Acme Studio', gone: false, user: 'u1' });
		workspaceThemes.lose();
		expect(workspaceThemes.kept?.gone).toBe(true);
		// Still drawn while gone, until another is chosen.
		expect(workspaceThemes.get(ID)).toBeDefined();
		// Offered again (an unpublish undone): no longer gone.
		workspaceThemes.keep('ws1', 'Acme Studio', doc, 'u1');
		expect(workspaceThemes.kept?.gone).toBe(false);
	});

	it('goes when someone else signs in here', () => {
		workspaceThemes.keep('ws1', 'Acme Studio', workspaceDoc(row())!, 'u1');
		workspaceThemes.connect('u1');
		expect(workspaceThemes.kept).not.toBeNull();
		workspaceThemes.connect('u2');
		expect(workspaceThemes.kept).toBeNull();
		expect(localStorage.getItem('mila.workspaceTheme')).toBeNull();
	});
});
