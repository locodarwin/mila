// Web push on this device: whether the browser can do it, turning it on
// and off, and keeping the server's record of this browser current. The
// service worker shows the pushes (src/service-worker).
import { api, call } from './api/client.js';

/** What stands between this device and push, if anything. */
export type PushSupport = 'insecure' | 'unsupported' | 'denied' | 'available';

/** The parts of the browser that decide PushSupport. */
export interface PushEnv {
	secure: boolean;
	serviceWorker: boolean;
	pushManager: boolean;
	notifications: boolean;
	permission: string;
}

export function pushSupport(env: PushEnv): PushSupport {
	// Without a secure context the other APIs are missing too, and the
	// reason worth telling is the connection.
	if (!env.secure) return 'insecure';
	if (!env.serviceWorker || !env.pushManager || !env.notifications) return 'unsupported';
	if (env.permission === 'denied') return 'denied';
	return 'available';
}

function env(): PushEnv {
	const notifications = typeof Notification !== 'undefined';
	return {
		secure: globalThis.isSecureContext === true,
		serviceWorker: typeof navigator !== 'undefined' && 'serviceWorker' in navigator,
		pushManager: typeof PushManager !== 'undefined',
		notifications,
		permission: notifications ? Notification.permission : 'default'
	};
}

/** Whether this browser can get pushes now. */
export function currentSupport(): PushSupport {
	return pushSupport(env());
}

// Who turned push on in this browser. A browser keeps its subscription
// across accounts, so the next person to sign in here must not inherit it.
const OWNER_KEY = 'mila.push-user';

function owner(): string | null {
	try {
		return localStorage.getItem(OWNER_KEY);
	} catch {
		return null;
	}
}

function setOwner(userId: string | null) {
	try {
		if (userId) localStorage.setItem(OWNER_KEY, userId);
		else localStorage.removeItem(OWNER_KEY);
	} catch {
		// Without storage the next start turns push off here, the safe side.
	}
}

/** A failure with a message to show. */
export class PushError extends Error {}

/** Decodes the server's base64url key into what pushManager.subscribe takes. */
export function keyBytes(key: string): Uint8Array<ArrayBuffer> {
	const b64 = key.replace(/-/g, '+').replace(/_/g, '/');
	const bin = atob(b64 + '='.repeat((4 - (b64.length % 4)) % 4));
	const out = new Uint8Array(bin.length);
	for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
	return out;
}

/** Whether a subscription was made with this key. */
export function sameKey(had: ArrayBuffer | null | undefined, key: Uint8Array): boolean {
	if (!had || had.byteLength !== key.length) return false;
	const a = new Uint8Array(had);
	return a.every((b, i) => b === key[i]);
}

/** The service worker, or a PushError if it has not started in time. */
async function registration(): Promise<ServiceWorkerRegistration> {
	let timer: ReturnType<typeof setTimeout> | undefined;
	const late = new Promise<never>((_, reject) => {
		timer = setTimeout(
			() =>
				reject(
					new PushError(
						'The part of Mila that receives pushes has not started. Reload the page and try again.'
					)
				),
			10_000
		);
	});
	try {
		return await Promise.race([navigator.serviceWorker.ready, late]);
	} finally {
		clearTimeout(timer);
	}
}

/**
 * This browser's subscription, if push is on here. It asks the service
 * worker registered now, without waiting for one to start.
 */
export async function deviceSubscription(): Promise<PushSubscription | null> {
	if (currentSupport() === 'insecure' || currentSupport() === 'unsupported') return null;
	const reg = await navigator.serviceWorker.getRegistration();
	return (await reg?.pushManager.getSubscription()) ?? null;
}

async function register(sub: PushSubscription) {
	const json = sub.toJSON();
	await call(
		api.POST('/me/push/subscribe', {
			body: {
				endpoint: sub.endpoint,
				expirationTime: json.expirationTime ?? null,
				keys: { p256dh: json.keys?.p256dh ?? '', auth: json.keys?.auth ?? '' }
			}
		})
	);
}

/**
 * The subscription for the server's key: the existing one, or a new one
 * when there is none or it was made with another key.
 */
async function subscribe(reg: ServiceWorkerRegistration): Promise<PushSubscription> {
	const { publicKey } = await call(api.GET('/me/push'));
	const key = keyBytes(publicKey);
	let sub = await reg.pushManager.getSubscription();
	if (sub && !sameKey(sub.options.applicationServerKey, key)) {
		await sub.unsubscribe();
		sub = null;
	}
	if (sub) return sub;
	try {
		return await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key });
	} catch {
		throw new PushError(
			'This browser could not reach its push service. Check the connection and try again.'
		);
	}
}

/**
 * Turns push on for this browser, for the signed-in user. The permission
 * prompt comes from here, so only call it from a click.
 */
export async function enablePush(userId: string): Promise<void> {
	if (currentSupport() === 'unsupported' || currentSupport() === 'insecure')
		throw new PushError('This browser cannot receive push notifications.');
	const permission = await Notification.requestPermission();
	if (permission === 'denied')
		throw new PushError(
			'Notifications are blocked for Mila in this browser. Allow them in the site settings, then try again.'
		);
	if (permission !== 'granted')
		throw new PushError('Push stays off until you allow notifications for Mila.');
	const sub = await subscribe(await registration());
	await register(sub);
	setOwner(userId);
}

/** Turns push off for this browser, here first so it stops even offline. */
export async function disablePush(): Promise<void> {
	setOwner(null);
	const sub = await deviceSubscription();
	if (!sub) return;
	const endpoint = sub.endpoint;
	await sub.unsubscribe();
	await call(api.POST('/me/push/unsubscribe', { body: { endpoint } })).catch(() => {
		// The server forgets it anyway once its push service says it is gone.
	});
}

/**
 * Re-sends this browser's subscription at app start, so the server has it
 * for the current session and with current keys. Never prompts. Push that
 * someone else turned on here is turned off instead.
 */
export async function refreshPush(userId: string): Promise<void> {
	try {
		if (currentSupport() !== 'available' || Notification.permission !== 'granted') return;
		if (!(await deviceSubscription())) return;
		if (owner() !== userId) {
			await forgetPush();
			return;
		}
		await register(await subscribe(await registration()));
	} catch {
		// Offline or not set up: the next start tries again.
	}
}

/** Stops push on this browser when signing out; the server already has. */
export async function forgetPush(): Promise<void> {
	setOwner(null);
	try {
		await (await deviceSubscription())?.unsubscribe();
	} catch {
		// Nothing to forget.
	}
}
