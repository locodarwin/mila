// Settings' pages in their three sections (docs/PLAN.md section 3): about
// you, about a workspace (the one the picker in the Workspace heading
// chooses; its pages in two groups, for everyone in it and just yours), and
// about this Mila. Each page has an icon, so a page never reads as a
// heading. The rail draws this; roles and the desktop app's local mode leave
// out what they cannot use, as the tabs did.
import { resolve } from '$app/paths';
import type { RailIconName } from './railIcons';
import type { Schemas } from './api/client';
import { workspaceSettings, type WorkspacePage } from './settingsAddress';
export { workspaceSettings, type WorkspacePage };
import {
	canAutomate,
	canManageWebhooks,
	canOrganize,
	canReceiveEmail,
	canUseTemplates,
	canViewMembers
} from './roles';

export interface NavItem {
	label: string;
	href: string;
	/** Its icon in the rail (railIcons.ts). */
	icon: RailIconName;
	/** A workspace's page, for keeping to the same page when the picker changes. */
	page?: WorkspacePage;
}

export interface NavGroup {
	id: 'you' | 'everyone' | 'yours' | 'mila';
	/** What the rail shows: a section's name, or a caption inside Workspace. */
	label: string;
	/** What a screen reader hears for the list, when the label leans on the heading. */
	listLabel?: string;
	items: NavItem[];
}

export interface NavFor {
	/** The workspace the picker chooses, if any. */
	workspace: Schemas['Workspace'] | null;
	/** The desktop app's mode on this computer: no sign-in, nobody else, nothing leaving it. */
	local: boolean;
	instanceAdmin: boolean;
}

/** Settings' groups and their pages, for this person, workspace and mode. */
export function settingsNav({ workspace: w, local, instanceAdmin }: NavFor): NavGroup[] {
	const you: NavItem[] = [
		{ label: 'Profile', icon: 'profile', href: resolve('/(app)/settings/profile') },
		{ label: 'Appearance', icon: 'appearance', href: resolve('/(app)/settings/appearance') },
		{ label: 'Preferences', icon: 'preferences', href: resolve('/(app)/settings/preferences') },
		{ label: 'Keyboard', icon: 'keyboard', href: resolve('/(app)/settings/keyboard') },
		{
			label: 'Notifications',
			icon: 'notifications',
			href: resolve('/(app)/settings/notifications')
		},
		...(local
			? []
			: [
					{
						label: 'Security',
						icon: 'security' as const,
						href: resolve('/(app)/settings/security')
					}
				]),
		...(local
			? []
			: [{ label: 'API tokens', icon: 'tokens' as const, href: resolve('/(app)/settings/tokens') }])
	];
	const groups: NavGroup[] = [{ id: 'you', label: 'You', items: you }];
	if (w) {
		const at = (label: string, page: WorkspacePage): NavItem => ({
			label,
			page,
			icon: page,
			href: workspaceSettings(w.id, page)
		});
		const shared = !w.isPersonal;
		groups.push(
			{
				id: 'everyone',
				label: 'For everyone in it',
				listLabel: `For everyone in ${w.name}`,
				items: [
					at('General', 'general'),
					...(shared && canViewMembers(w.role) ? [at('People', 'people')] : []),
					// A guest's labels and fields are the workspace's to change.
					...(canOrganize(w.role) ? [at('Labels and fields', 'labels')] : []),
					// Templates and rules are the members'; guests never see them.
					...(canUseTemplates(w.role) ? [at('Templates', 'templates')] : []),
					...(canAutomate(w.role) ? [at('Automations', 'automations')] : []),
					...(shared ? [at('Workspace themes', 'themes')] : []),
					...(!local && canManageWebhooks(w.role) ? [at('Webhooks', 'webhooks')] : [])
				]
			},
			{
				id: 'yours',
				label: 'Just yours',
				listLabel: `Just yours in ${w.name}`,
				items: [
					...(local ? [] : [at('Calendar feeds', 'calendar')]),
					// Guests make no tasks by email.
					...(!local && canReceiveEmail(w.role) ? [at('Email to task', 'email')] : []),
					at('Import and export', 'data')
				]
			}
		);
	}
	if (instanceAdmin && !local)
		groups.push({
			id: 'mila',
			label: 'This Mila',
			items: [{ label: 'Admin', icon: 'admin', href: resolve('/(app)/settings/admin') }]
		});
	return groups;
}
