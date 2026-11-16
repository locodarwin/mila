// What changed between two versions of a text, word by word, for a page's
// history: runs of text kept, added and removed, in reading order.

export interface Change {
	kind: 'same' | 'added' | 'removed';
	text: string;
}

/** Words, punctuation and spaces, so joining them gives the text back. */
const tokens = (s: string) => s.match(/\s+|[\p{L}\p{N}_'’-]+|[^\s\p{L}\p{N}_'’-]/gu) ?? [];

/** The most words the middle of a diff compares one by one before it gives up and replaces it whole. */
const MAX_CELLS = 4_000_000;

/** The changes that turn before into after. */
export function wordDiff(before: string, after: string): Change[] {
	const a = tokens(before);
	const b = tokens(after);
	let start = 0;
	while (start < a.length && start < b.length && a[start] === b[start]) start++;
	let end = 0;
	while (
		end < a.length - start &&
		end < b.length - start &&
		a[a.length - 1 - end] === b[b.length - 1 - end]
	)
		end++;
	const midA = a.slice(start, a.length - end);
	const midB = b.slice(start, b.length - end);
	const out: Change[] = [];
	const push = (kind: Change['kind'], text: string) => {
		if (!text) return;
		const last = out.at(-1);
		if (last?.kind === kind) last.text += text;
		else out.push({ kind, text });
	};
	push('same', a.slice(0, start).join(''));
	if (midA.length * midB.length > MAX_CELLS) {
		push('removed', midA.join(''));
		push('added', midB.join(''));
	} else {
		// Longest common subsequence, from the end, then read forward.
		const n = midA.length;
		const m = midB.length;
		const lcs = Array.from({ length: n + 1 }, () => new Uint32Array(m + 1));
		for (let i = n - 1; i >= 0; i--)
			for (let j = m - 1; j >= 0; j--)
				lcs[i][j] =
					midA[i] === midB[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1]);
		let i = 0;
		let j = 0;
		while (i < n && j < m) {
			if (midA[i] === midB[j]) {
				push('same', midA[i]);
				i++;
				j++;
			} else if (lcs[i + 1][j] >= lcs[i][j + 1]) push('removed', midA[i++]);
			else push('added', midB[j++]);
		}
		while (i < n) push('removed', midA[i++]);
		while (j < m) push('added', midB[j++]);
	}
	push('same', a.slice(a.length - end).join(''));
	return out;
}
