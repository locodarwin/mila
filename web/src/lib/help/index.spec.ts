import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { HELP, HELP_GROUPS, neighbors, topic, topicFiles } from './index';
import shots from './shots.json';

const here = import.meta.dirname;
const TOPICS = join(here, 'topics');
const SRC = join(here, '..', '..');
const text = (slug: string) => readFileSync(join(TOPICS, `${slug}.svelte`), 'utf8');

/** Every .svelte file under a folder. */
function svelteFiles(dir: string): string[] {
	return readdirSync(dir).flatMap((name) => {
		const path = join(dir, name);
		if (statSync(path).isDirectory()) return svelteFiles(path);
		return name.endsWith('.svelte') ? [path] : [];
	});
}

describe('help topics', () => {
	it('have unique kebab-case slugs', () => {
		const slugs = HELP.map((t) => t.slug);
		expect(new Set(slugs).size).toBe(slugs.length);
		for (const s of slugs) expect(s).toMatch(/^[a-z]+(-[a-z]+)*$/);
	});

	it('each have one file, and every file is a topic', () => {
		const files = readdirSync(TOPICS)
			.filter((f) => f.endsWith('.svelte'))
			.map((f) => f.replace(/\.svelte$/, ''));
		const slugs = HELP.map((t) => t.slug);
		expect(files.toSorted()).toEqual(slugs.toSorted());
		expect(topicFiles().toSorted()).toEqual(slugs.toSorted());
	});

	it('list the headings their files have, in order', () => {
		for (const t of HELP) {
			const ids = [...text(t.slug).matchAll(/<h2 id="([^"]+)"/g)].map((m) => m[1]);
			expect(ids, t.slug).toEqual(t.sections.map((s) => s.id));
		}
	});

	it('are each in one group, and read from the first to the last', () => {
		const grouped = HELP_GROUPS.flatMap((g) => g.topics);
		expect(grouped.toSorted()).toEqual(HELP.map((t) => t.slug).toSorted());
		expect(neighbors(grouped[0]).previous).toBeUndefined();
		expect(neighbors(grouped[0]).next?.slug).toBe(grouped[1]);
		expect(neighbors(grouped[1]).previous?.slug).toBe(grouped[0]);
		expect(neighbors(grouped.at(-1)!).next).toBeUndefined();
		expect(neighbors('no-such-topic')).toEqual({});
	});

	it('show pictures that exist, in both schemes, and say what each one shows', () => {
		const used = HELP.flatMap((t) =>
			[...text(t.slug).matchAll(/<Shot\s+name="([^"]+)"\s+alt="([^"]+)"/g)].map((m) => {
				expect(m[2].length, m[1]).toBeGreaterThan(20);
				return m[1];
			})
		);
		const pictures = join(SRC, '..', 'static', 'help-images');
		// Every Shot has its name and words first, as the pattern above finds them.
		const tags = HELP.flatMap((t) => [...text(t.slug).matchAll(/<Shot\s/g)]);
		expect(used).toHaveLength(tags.length);
		expect(used.toSorted()).toEqual(Object.keys(shots).toSorted());
		for (const name of used)
			for (const scheme of ['light', 'dark'])
				expect(existsSync(join(pictures, `${name}-${scheme}.webp`)), `${name} ${scheme}`).toBe(
					true
				);
	});

	it('relate only to topics that exist', () => {
		for (const t of HELP) for (const r of t.related ?? []) expect(topic(r), r).toBeDefined();
	});

	it('are linked to only where a topic and section exist', () => {
		const links = svelteFiles(SRC).flatMap((file) =>
			[
				...readFileSync(file, 'utf8').matchAll(
					/<HelpLink\s+topic="([^"]+)"(?:\s+section="([^"]+)")?/g
				)
			].map((m) => ({ file, topic: m[1], section: m[2] }))
		);
		expect(links.length).toBeGreaterThan(4);
		for (const l of links) {
			const t = topic(l.topic);
			expect(t, `${l.file}: ${l.topic}`).toBeDefined();
			if (l.section)
				expect(
					t!.sections.map((s) => s.id),
					`${l.file}: ${l.topic}#${l.section}`
				).toContain(l.section);
		}
	});

	it('are written plainly: no em dashes, British spellings or keys typed by hand', () => {
		const british = /\b(colour|behaviour|cancelled|organise|favourite|centre|recognise)\b/i;
		for (const t of HELP) {
			const body = text(t.slug);
			expect(body, t.slug).not.toContain('—');
			expect(body, t.slug).not.toMatch(british);
			expect(body, t.slug).not.toMatch(/<kbd/i);
		}
	});
});
