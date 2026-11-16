// The server's permission rules (internal/authz), mirrored so the UI can
// hide controls a role cannot use. The server still enforces every one.
import type { Role } from './api/client';

const rank: Record<Role, number> = { owner: 4, admin: 3, member: 2, guest: 1 };

export const canRename = (r: Role) => rank[r] >= rank.admin;
export const canDelete = (r: Role) => r === 'owner';
export const canViewMembers = (r: Role) => rank[r] >= rank.member;
export const canInvite = (r: Role) => rank[r] >= rank.admin;
/** Changing saved views; a guest's changes to a shared view stay on their screen. */
export const canEditViews = (r: Role) => rank[r] >= rank.member;
/** Saving and using templates, which guests never see. */
export const canUseTemplates = (r: Role) => rank[r] >= rank.member;
/** Changing or deleting time someone else logged. */
export const canEditOthersTime = (r: Role) => rank[r] >= rank.admin;
/** Publishing, suggesting and unpublishing the workspace's themes. */
export const canPublishThemes = (r: Role) => rank[r] >= rank.admin;
/**
 * Making and changing projects, their statuses, labels and fields, and using
 * the inbox. A guest works on what is in the projects shared with them.
 */
export const canOrganize = (r: Role) => rank[r] >= rank.member;
/** Deleting a comment someone else wrote. */
export const canDeleteOthersComments = (r: Role) => rank[r] >= rank.admin;
/** Adding, changing and reading the log of the workspace's webhooks. */
export const canReceiveEmail = (r: Role) => rank[r] >= rank.member;
export const canManageWebhooks = (r: Role) => rank[r] >= rank.admin;
/** A rule for a project is anyone's who organizes; one across the workspace, an admin's. */
export const canAutomate = (r: Role) => rank[r] >= rank.member;
export const canAutomateWorkspace = (r: Role) => rank[r] >= rank.admin;
/** Giving guests projects, and taking them away. */
export const canShare = (r: Role) => rank[r] >= rank.admin;

/** Roles the actor may hand out in an invite. */
export function invitableRoles(actor: Role): Role[] {
	if (!canInvite(actor)) return [];
	return actor === 'owner' ? ['owner', 'admin', 'member', 'guest'] : ['admin', 'member', 'guest'];
}

/** Roles the actor may move a member with role `from` to; empty if none. */
export function assignableRoles(actor: Role, from: Role): Role[] {
	if (actor === 'owner') return ['owner', 'admin', 'member', 'guest'];
	if (actor === 'admin' && rank[from] < rank.admin) return ['admin', 'member', 'guest'];
	return [];
}

export function canRemove(actor: Role, target: Role): boolean {
	if (actor === 'owner') return true;
	return actor === 'admin' && rank[target] < rank.admin;
}
