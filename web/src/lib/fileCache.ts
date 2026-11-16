// Files kept on the device once they have been fetched, so they still open
// offline. The service worker fills this cache; signing out empties it, along
// with everything else but the app (signout.ts).

export const FILES_CACHE = 'mila-files';

/** Beyond this, the files held longest go first. */
export const FILES_LIMIT = 500 * 1024 * 1024;

const FILE_PATH = /^\/api\/v1\/workspaces\/([^/]+)\/attachments\/[^/]+\/file$/;

export const isFilePath = (path: string) => FILE_PATH.test(path);

/** What to let go of, given what is held, oldest first, to come within the limit. */
export function overLimit<T extends { size: number }>(held: T[], limit = FILES_LIMIT): T[] {
	let total = held.reduce((sum, f) => sum + f.size, 0);
	const out: T[] = [];
	for (const file of held) {
		if (total <= limit) break;
		out.push(file);
		total -= file.size;
	}
	return out;
}

/** Whether this device holds a file, by the address it is served at. */
export async function hasFile(url: string): Promise<boolean> {
	try {
		const cache = await caches.open(FILES_CACHE);
		return Boolean(await cache.match(url, { ignoreVary: true }));
	} catch {
		return false;
	}
}

/** Lets go of a workspace's files, when this device stops holding that workspace. */
export async function forgetFiles(ws: string): Promise<void> {
	try {
		if (typeof caches === 'undefined' || !(await caches.has(FILES_CACHE))) return;
		const cache = await caches.open(FILES_CACHE);
		for (const key of await cache.keys()) {
			if (FILE_PATH.exec(new URL(key.url).pathname)?.[1] === ws) await cache.delete(key);
		}
	} catch {
		// Storage blocked: there is nothing held to let go of.
	}
}
