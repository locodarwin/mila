import { describe, expect, it } from 'vitest';
import { deviceName } from './device';

describe('deviceName', () => {
	it.each([
		['Mozilla/5.0 (X11; Linux x86_64; rv:135.0) Gecko/20100101 Firefox/135.0', 'Firefox on Linux'],
		[
			'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36 Edg/131.0',
			'Edge on Windows'
		],
		[
			'Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1',
			'Safari on iPhone'
		],
		[
			'Mozilla/5.0 (Linux; Android 15; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Mobile Safari/537.36',
			'Chrome on Android'
		],
		['curl/8.0', 'Unknown device']
	])('names %s', (ua, want) => {
		expect(deviceName(ua)).toBe(want);
	});
});
