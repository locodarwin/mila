// Adding or removing a passkey, changing the email address and deleting
// the account need a sign-in from the last 10 minutes (the server's
// sign_in_again).
// A failure that asks for one offers to sign in again and come back.

import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { ApiError, messageOf } from './api/client';
import { toasts } from './toasts.svelte';

/** Shows a failure as a toast, with Sign in again when that is the way through. */
export function showFailure(err: unknown) {
	if (err instanceof ApiError && err.code === 'sign_in_again') {
		const back = encodeURIComponent(location.pathname + location.search);
		toasts.show(err.message, {
			tone: 'error',
			action: {
				label: 'Sign in again',
				// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() plus a query
				run: () => void goto(`${resolve('/(auth)/login')}?again=1&next=${back}`)
			}
		});
		return;
	}
	toasts.error(messageOf(err));
}
