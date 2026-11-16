// A short, human name for a browser, such as "Firefox on Linux", from its
// user agent string. Used to label passkeys and signed-in devices.

const browsers: [RegExp, string][] = [
	[/Edg\//, 'Edge'],
	[/OPR\//, 'Opera'],
	[/Firefox\//, 'Firefox'],
	[/Chrome\//, 'Chrome'],
	[/Safari\//, 'Safari']
];

const systems: [RegExp, string][] = [
	[/iPhone/, 'iPhone'],
	[/iPad/, 'iPad'],
	[/Android/, 'Android'],
	[/CrOS/, 'ChromeOS'],
	[/Mac OS X|Macintosh/, 'macOS'],
	[/Windows/, 'Windows'],
	[/Linux/, 'Linux']
];

export function deviceName(userAgent: string): string {
	const browser = browsers.find(([re]) => re.test(userAgent))?.[1];
	const system = systems.find(([re]) => re.test(userAgent))?.[1];
	if (browser && system) return `${browser} on ${system}`;
	return browser ?? system ?? 'Unknown device';
}

/**
 * Whether this is an Apple device, where Mod is shown as ⌘. iPadOS says
 * "MacIntel", which is right for an iPad with a keyboard.
 */
export function isApple(): boolean {
	if (typeof navigator === 'undefined') return false;
	const n = navigator as Navigator & { userAgentData?: { platform: string } };
	return /mac|iphone|ipad|ipod/i.test(n.userAgentData?.platform || n.platform || '');
}
