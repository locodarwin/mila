import { describe, expect, it } from 'vitest';
import {
	activeQuery,
	insert,
	matching,
	mentioned,
	segments,
	toDisplay,
	token,
	toStored,
	activeSlash,
	commentParts,
	headingsIn,
	referencesToDisplay,
	referencesToStored,
	slugOf
} from './mentions';

const dana = { userId: '0192-dana', name: 'Dana' };
const danaLee = { userId: '0192-dana-lee', name: 'Dana Lee' };
const lee = { userId: '0192-lee', name: 'Lee Park' };

describe('stored mentions', () => {
	it('splits a body into text and mentions', () => {
		expect(segments('Ready, @[Dana](user:0192-dana)? Ask @[Lee Park](user:0192-lee).')).toEqual([
			{ text: 'Ready, ' },
			{ name: 'Dana', userId: '0192-dana' },
			{ text: '? Ask ' },
			{ name: 'Lee Park', userId: '0192-lee' },
			{ text: '.' }
		]);
		expect(segments('no one')).toEqual([{ text: 'no one' }]);
		expect(segments('')).toEqual([]);
	});

	it('ignores things that only look like mentions', () => {
		expect(segments('@[Dana](mailto:x) and @[](user:1) and @Dana')).toEqual([
			{ text: '@[Dana](mailto:x) and @[](user:1) and @Dana' }
		]);
	});

	it('lists each mentioned user once', () => {
		expect(mentioned(`${token(dana)} ${token(lee)} ${token(dana)}`)).toEqual([
			'0192-dana',
			'0192-lee'
		]);
	});

	it('keeps names a mention can hold', () => {
		expect(token({ userId: 'u1', name: 'A [team]\nlead' })).toBe('@[A  team  lead](user:u1)');
	});
});

describe('the comment box', () => {
	it('shows stored mentions as @Name and stores them again', () => {
		const body = `Hi @[Dana](user:0192-dana), see @[Lee Park](user:0192-lee).`;
		const { text, picked } = toDisplay(body);
		expect(text).toBe('Hi @Dana, see @Lee Park.');
		expect(picked).toEqual([dana, lee]);
		expect(toStored(text, picked)).toBe(body);
	});

	it('prefers the longer name', () => {
		expect(toStored('@Dana Lee and @Dana', [dana, danaLee])).toBe(
			`${token(danaLee)} and ${token(dana)}`
		);
	});

	it('needs a word boundary on both sides', () => {
		expect(toStored('mail@Dana and @Danaish', [dana])).toBe('mail@Dana and @Danaish');
		expect(toStored('(@Dana)', [dana])).toBe(`(${token(dana)})`);
	});

	it('resolves typed names only when one member has them', () => {
		expect(toStored('@Lee Park, @Dana', [], [lee, dana])).toBe(`${token(lee)}, ${token(dana)}`);
		const twin = { userId: 'other', name: 'Dana' };
		expect(toStored('@Dana', [], [dana, twin])).toBe('@Dana');
		expect(toStored('@Dana', [twin], [dana, twin])).toBe(token(twin));
	});

	it('finds the mention being typed', () => {
		expect(activeQuery('@', 1)).toEqual({ start: 0, query: '' });
		expect(activeQuery('Hi @Da', 6)).toEqual({ start: 3, query: 'Da' });
		expect(activeQuery('Hi @Dana L', 10)).toEqual({ start: 3, query: 'Dana L' });
		expect(activeQuery('one\n(@le', 8)).toEqual({ start: 5, query: 'le' });
		expect(activeQuery('Hi @Da and more', 6)).toEqual({ start: 3, query: 'Da' });
		expect(activeQuery('mail@Da', 7)).toBeNull();
		expect(activeQuery('@Da\nx', 5)).toBeNull();
		expect(activeQuery('@ Da', 4)).toBeNull();
		expect(activeQuery('@Dana  x', 8)).toBeNull();
		expect(activeQuery('no mention', 10)).toBeNull();
	});

	it('matches names, then words in names', () => {
		const people = [lee, dana, danaLee];
		expect(matching(people, 'da').map((p) => p.name)).toEqual(['Dana', 'Dana Lee']);
		expect(matching(people, 'le').map((p) => p.name)).toEqual(['Lee Park', 'Dana Lee']);
		expect(matching(people, '').length).toBe(3);
		expect(matching(people, 'x')).toEqual([]);
	});

	it('puts the pick in place of the query', () => {
		expect(insert('Hi @Da, there', { start: 3, query: 'Da' }, 6, dana)).toEqual({
			text: 'Hi @Dana , there',
			caret: 9
		});
		expect(insert('Hi @Da more', { start: 3, query: 'Da' }, 6, dana)).toEqual({
			text: 'Hi @Dana more',
			caret: 9
		});
	});
});

describe('references in comments', () => {
	const id = '01a19d6b-69c3-75c1-b0d9-20f017a4ef5a';
	const href = `/?task=${id}`;

	it('shows a stored reference as a title and stores it back', () => {
		const stored = `See [Paint the shed](${href}) today`;
		const { text, refs } = referencesToDisplay(stored);
		expect(text).toBe('See [[Paint the shed]] today');
		expect(refs).toEqual([{ label: 'Paint the shed', href }]);
		expect(referencesToStored(text, refs)).toBe(stored);
	});

	it('leaves links that are not to the app as typed', () => {
		const stored = 'See [the docs](/not/ours) and [more](https://example.com)';
		expect(referencesToDisplay(stored)).toEqual({ text: stored, refs: [] });
	});

	it('finds a slash command only where one can start', () => {
		expect(activeSlash('/', 1)).toEqual({ start: 0, word: '', query: null });
		expect(activeSlash('hi /ta', 6)).toEqual({ start: 3, word: 'ta', query: null });
		expect(activeSlash('hi /task paint the', 18)).toEqual({
			start: 3,
			word: 'task',
			query: 'paint the'
		});
		expect(activeSlash('and/or', 6)).toBeNull();
		expect(activeSlash('https://example.com/task', 24)).toBeNull();
		expect(activeSlash('/usr/local', 10)).toBeNull();
		expect(activeSlash('/other words', 12)).toBeNull();
	});

	it('reads headings, leaving code blocks out', () => {
		expect(
			headingsIn('# One\n\ntext\n\n## **Two** ##\n\n```\n# not this\n```\n### [Three](/x)')
		).toEqual(['One', 'Two', 'Three']);
		expect(headingsIn(null)).toEqual([]);
		expect(slugOf('When to water?')).toBe('when-to-water');
		expect(slugOf('Déjà vu 2')).toBe('déjà-vu-2');
	});

	it('splits a comment into what it shows', () => {
		expect(
			commentParts(`Ask @[Ana](user:u1) about [Beds](${href}) at https://example.com/a.`)
		).toEqual([
			{ text: 'Ask ' },
			{ userId: 'u1', name: 'Ana' },
			{ text: ' about ' },
			{ label: 'Beds', href, target: { kind: 'task', id } },
			{ text: ' at ' },
			{ url: 'https://example.com/a' },
			{ text: '.' }
		]);
	});

	it('keeps sentence punctuation out of a web address, and balanced brackets in', () => {
		const urls = (s: string) => commentParts(s).flatMap((p) => ('url' in p ? [p.url] : []));
		expect(urls('(see http://a.example/x).')).toEqual(['http://a.example/x']);
		expect(urls('http://a.example/wiki/Foo_(bar)')).toEqual(['http://a.example/wiki/Foo_(bar)']);
		expect(urls('ftp://a.example javascript:alert(1)')).toEqual([]);
	});
});
