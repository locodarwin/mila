// The typed API client. Types come from shared/openapi.yaml through
// `npm run generate`; never edit schema.d.ts by hand.
import createClient from 'openapi-fetch';
import type { components, paths } from './schema';

export type Schemas = components['schemas'];
export type User = Schemas['User'];
export type Workspace = Schemas['Workspace'];
export type Role = Schemas['Role'];

export const api = createClient<paths>({ baseUrl: '/api/v1' });

/** A failed request, with a message that is safe to show. */
export class ApiError extends Error {
	constructor(
		readonly code: string,
		message: string,
		readonly status: number,
		/** Seconds the server asked to wait (Retry-After), when it said. */
		readonly retryAfter?: number
	) {
		super(message);
	}
}

/**
 * Awaits an openapi-fetch call and returns its data, or throws an ApiError
 * carrying the server's message.
 */
export async function call<T>(
	request: Promise<{ data?: T; error?: unknown; response: Response }>
): Promise<T> {
	let result: { data?: T; error?: unknown; response: Response };
	try {
		result = await request;
	} catch {
		throw new ApiError('network', 'Mila could not reach the server. Check your connection.', 0);
	}
	const { data, error, response } = result;
	if (!response.ok) {
		const e = (error ?? {}) as Partial<Schemas['Error']>;
		const wait = Number(response.headers.get('Retry-After') ?? NaN);
		throw new ApiError(
			e.error ?? 'http_' + response.status,
			e.message ?? `The server answered ${response.status}.`,
			response.status,
			Number.isFinite(wait) ? wait : undefined
		);
	}
	return data as T;
}

/** The message to show for anything thrown by a call. */
export function messageOf(err: unknown): string {
	if (err instanceof ApiError) return err.message;
	if (err instanceof Error) return err.message;
	return 'Something went wrong.';
}
