import { describe, expect, it } from 'vitest';
import { engine } from './engine.svelte';

// A sample workspace keeps edits in memory, so the engine runs without a
// database or a server.
function workspace() {
	const ws = `undo-${Math.random()}`;
	const replica = engine.sample(ws, []);
	return { ws, title: (id: string) => replica.get('task', id)?.values.title };
}

describe('undo', () => {
	it('takes back the last edit, or group of edits', async () => {
		const { ws, title } = workspace();
		await engine.mutate(ws, 'task', 'a', { title: 'A', sort_key: 'a0' });
		await engine.mutate(ws, 'task', 'a', { title: 'A2' });
		await engine.group(async () => {
			await engine.mutate(ws, 'task', 'a', { title: 'A3' });
			await engine.mutate(ws, 'task', 'a', { title: 'A4' });
		});
		expect(await engine.undo()).toBe(true);
		expect(title('a')).toBe('A2');
		engine.dropSample(ws);
	});

	it("a tracked edit's undo reverses that edit alone, whatever came after", async () => {
		const { ws, title } = workspace();
		await engine.mutate(ws, 'task', 'a', { title: 'A', sort_key: 'a0' });
		await engine.mutate(ws, 'task', 'b', { title: 'B', sort_key: 'a1' });
		const first = engine.track(async () => {
			await engine.mutate(ws, 'task', 'a', { title: 'A done' });
			await engine.mutate(ws, 'task', 'next', { title: 'Next', sort_key: 'a2' });
		});
		await first.done;
		const second = engine.track(() => engine.mutate(ws, 'task', 'b', { title: 'B done' }));
		await second.done;
		expect(await first.undo()).toBe(true);
		expect(title('a')).toBe('A');
		expect(engine.replica(ws)?.task('next')).toBeUndefined();
		expect(title('b')).toBe('B done');
		// Once is all: it is no longer on the undo stack either.
		expect(await first.undo()).toBe(false);
		expect(await engine.undo()).toBe(true);
		expect(title('b')).toBe('B');
		engine.dropSample(ws);
	});

	it('an undo pressed before the edit lands waits for it', async () => {
		const { ws, title } = workspace();
		await engine.mutate(ws, 'task', 'a', { title: 'A', sort_key: 'a0' });
		const { undo } = engine.track(() => engine.mutate(ws, 'task', 'a', { title: 'A2' }));
		expect(await undo()).toBe(true);
		expect(title('a')).toBe('A');
		engine.dropSample(ws);
	});
});
