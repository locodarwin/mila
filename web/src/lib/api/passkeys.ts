// Passkey ceremonies: the server sends options, the browser asks the
// authenticator, the server checks the answer.
import {
	browserSupportsWebAuthn,
	startAuthentication,
	startRegistration,
	type PublicKeyCredentialCreationOptionsJSON,
	type PublicKeyCredentialRequestOptionsJSON
} from '@simplewebauthn/browser';
import { api, call, type User } from './client';

export { browserSupportsWebAuthn };

type Credential = Record<string, unknown>;

/** Turns the browser's refusals into something a person can act on. */
function explain(err: unknown): never {
	const name = err instanceof Error ? err.name : '';
	if (name === 'NotAllowedError' || name === 'AbortError') {
		throw new Error('The passkey request was canceled or timed out.');
	}
	if (name === 'InvalidStateError') {
		throw new Error('This device already has a passkey for your account.');
	}
	throw err;
}

/** Adds a passkey for the signed-in user on this device. */
export async function registerPasskey(name: string) {
	const ceremony = await call(api.POST('/me/passkeys/begin'));
	const options = ceremony.options.publicKey as PublicKeyCredentialCreationOptionsJSON;
	const credential = await startRegistration({ optionsJSON: options }).catch(explain);
	return call(
		api.POST('/me/passkeys/finish', {
			body: {
				ceremonyId: ceremony.ceremonyId,
				credential: credential as unknown as Credential,
				name
			}
		})
	);
}

/** Signs in with whichever of this site's passkeys the browser offers. */
export async function signInWithPasskey(): Promise<User> {
	const ceremony = await call(api.POST('/auth/passkey/begin'));
	const options = ceremony.options.publicKey as PublicKeyCredentialRequestOptionsJSON;
	const credential = await startAuthentication({ optionsJSON: options }).catch(explain);
	return call(
		api.POST('/auth/passkey/finish', {
			body: { ceremonyId: ceremony.ceremonyId, credential: credential as unknown as Credential }
		})
	);
}
