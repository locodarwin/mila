// The address of each workspace page in Settings, apart from the rail
// (settingsNav.ts), so links to them from anywhere in the app stay small.
import { resolve } from '$app/paths';

/** A workspace's pages in Settings, by the last part of their address. */
export type WorkspacePage =
	| 'general'
	| 'people'
	| 'labels'
	| 'templates'
	| 'automations'
	| 'themes'
	| 'webhooks'
	| 'calendar'
	| 'email'
	| 'data';

/** The address of a workspace's page in Settings. */
export function workspaceSettings(ws: string, p: WorkspacePage): string {
	switch (p) {
		case 'general':
			return resolve('/(app)/settings/w/[ws]/general', { ws });
		case 'people':
			return resolve('/(app)/settings/w/[ws]/people', { ws });
		case 'labels':
			return resolve('/(app)/settings/w/[ws]/labels', { ws });
		case 'templates':
			return resolve('/(app)/settings/w/[ws]/templates', { ws });
		case 'automations':
			return resolve('/(app)/settings/w/[ws]/automations', { ws });
		case 'themes':
			return resolve('/(app)/settings/w/[ws]/themes', { ws });
		case 'webhooks':
			return resolve('/(app)/settings/w/[ws]/webhooks', { ws });
		case 'calendar':
			return resolve('/(app)/settings/w/[ws]/calendar', { ws });
		case 'email':
			return resolve('/(app)/settings/w/[ws]/email', { ws });
		case 'data':
			return resolve('/(app)/settings/w/[ws]/data', { ws });
	}
}
