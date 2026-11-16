// Fractional sort keys: strings that sort in list order, so moving an item
// means giving it one new key between its neighbors instead of renumbering
// the list. This is the well-known scheme with an integer part, so that
// appending increments a counter and keys grow only logarithmically:
// a key is an integer part, whose first character also says how long it
// is ('a' is two characters, 'b' three, 'Z' two below zero...), then an
// optional base-62 fraction with no trailing zero.

const DIGITS = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz';
const SMALLEST_INTEGER = 'A' + '0'.repeat(26);

function midpoint(a: string, b: string | null): string {
	if (b !== null) {
		let n = 0;
		while ((a[n] ?? '0') === b[n]) n++;
		if (n > 0) return b.slice(0, n) + midpoint(a.slice(n), b.slice(n));
	}
	const da = a ? DIGITS.indexOf(a[0]) : 0;
	const db = b !== null ? DIGITS.indexOf(b[0]) : DIGITS.length;
	if (db - da > 1) return DIGITS[Math.round((da + db) / 2)];
	if (b !== null && b.length > 1) return b.slice(0, 1);
	return DIGITS[da] + midpoint(a.slice(1), null);
}

function integerLength(head: string): number {
	if (head >= 'a' && head <= 'z') return head.charCodeAt(0) - 97 + 2;
	if (head >= 'A' && head <= 'Z') return 90 - head.charCodeAt(0) + 2;
	throw new Error(`invalid sort key head ${head}`);
}

function integerPart(key: string): string {
	const n = integerLength(key[0]);
	if (n > key.length) throw new Error(`invalid sort key ${key}`);
	return key.slice(0, n);
}

function validate(key: string) {
	if (key === SMALLEST_INTEGER) throw new Error(`invalid sort key ${key}`);
	const fraction = key.slice(integerPart(key).length);
	if (fraction.endsWith('0')) throw new Error(`invalid sort key ${key}`);
}

function increment(x: string): string | null {
	const [head, ...digits] = x.split('');
	let carry = true;
	for (let i = digits.length - 1; carry && i >= 0; i--) {
		const d = DIGITS.indexOf(digits[i]) + 1;
		if (d === DIGITS.length) digits[i] = '0';
		else {
			digits[i] = DIGITS[d];
			carry = false;
		}
	}
	if (!carry) return head + digits.join('');
	if (head === 'Z') return 'a0';
	if (head === 'z') return null;
	const next = String.fromCharCode(head.charCodeAt(0) + 1);
	if (next > 'a') digits.push('0');
	else digits.pop();
	return next + digits.join('');
}

function decrement(x: string): string | null {
	const [head, ...digits] = x.split('');
	let borrow = true;
	for (let i = digits.length - 1; borrow && i >= 0; i--) {
		const d = DIGITS.indexOf(digits[i]) - 1;
		if (d === -1) digits[i] = DIGITS[DIGITS.length - 1];
		else {
			digits[i] = DIGITS[d];
			borrow = false;
		}
	}
	if (!borrow) return head + digits.join('');
	if (head === 'a') return 'Z' + DIGITS[DIGITS.length - 1];
	if (head === 'A') return null;
	const next = String.fromCharCode(head.charCodeAt(0) - 1);
	if (next < 'Z') digits.push(DIGITS[DIGITS.length - 1]);
	else digits.pop();
	return next + digits.join('');
}

/**
 * A key that sorts after `before` and before `after`. Either may be null for
 * the start or end of the list.
 */
export function keyBetween(before: string | null, after: string | null): string {
	if (before !== null) validate(before);
	if (after !== null) validate(after);
	if (before !== null && after !== null && before >= after) {
		throw new Error(`keyBetween: ${before} is not before ${after}`);
	}
	if (before === null) {
		if (after === null) return 'a0';
		const ib = integerPart(after);
		const fb = after.slice(ib.length);
		if (ib === SMALLEST_INTEGER) return ib + midpoint('', fb);
		if (ib < after) return ib;
		const lower = decrement(ib);
		if (lower === null) throw new Error('keyBetween: no key before the smallest');
		return lower;
	}
	const ia = integerPart(before);
	const fa = before.slice(ia.length);
	if (after === null) {
		const next = increment(ia);
		return next === null ? ia + midpoint(fa, null) : next;
	}
	const ib = integerPart(after);
	const fb = after.slice(ib.length);
	if (ia === ib) return ia + midpoint(fa, fb);
	const next = increment(ia);
	if (next === null) throw new Error('keyBetween: out of keys');
	return next < after ? next : ia + midpoint(fa, null);
}

/** Orders by key, then by ID so two devices' equal keys still sort the same. */
export function bySortKey<T extends { sortKey: string; id: string }>(a: T, b: T): number {
	if (a.sortKey !== b.sortKey) return a.sortKey < b.sortKey ? -1 : 1;
	return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
}
