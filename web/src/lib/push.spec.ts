import { describe, expect, it } from 'vitest';
import { keyBytes, pushSupport, sameKey, type PushEnv } from './push';

describe('pushSupport', () => {
	const ok: PushEnv = {
		secure: true,
		serviceWorker: true,
		pushManager: true,
		notifications: true,
		permission: 'default'
	};
	it('is available in a secure context with the APIs and no refusal', () => {
		expect(pushSupport(ok)).toBe('available');
		expect(pushSupport({ ...ok, permission: 'granted' })).toBe('available');
	});
	it('names the connection first, since it also hides the APIs', () => {
		expect(pushSupport({ ...ok, secure: false, serviceWorker: false, pushManager: false })).toBe(
			'insecure'
		);
	});
	it('says when the browser lacks push', () => {
		expect(pushSupport({ ...ok, pushManager: false })).toBe('unsupported');
		expect(pushSupport({ ...ok, serviceWorker: false })).toBe('unsupported');
		expect(pushSupport({ ...ok, notifications: false, permission: 'denied' })).toBe('unsupported');
	});
	it('says when notifications are blocked', () => {
		expect(pushSupport({ ...ok, permission: 'denied' })).toBe('denied');
	});
});

describe('keys', () => {
	// The RFC 8291 example's application server key.
	const key =
		'BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8';
	it('decodes base64url into the 65-octet point', () => {
		const bytes = keyBytes(key);
		expect(bytes.length).toBe(65);
		expect(bytes[0]).toBe(4);
		expect(bytes[1]).toBe(0xfe);
		expect(bytes[64]).toBe(0x0f);
	});
	it('tells whether a subscription was made with the key', () => {
		const bytes = keyBytes(key);
		expect(sameKey(bytes.slice().buffer, bytes)).toBe(true);
		const other = bytes.slice();
		other[10] ^= 1;
		expect(sameKey(other.buffer, bytes)).toBe(false);
		expect(sameKey(bytes.slice(1).buffer, bytes)).toBe(false);
		expect(sameKey(null, bytes)).toBe(false);
	});
});
