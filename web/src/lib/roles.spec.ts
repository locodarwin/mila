import { describe, expect, it } from 'vitest';
import {
	assignableRoles,
	canEditViews,
	canInvite,
	canPublishThemes,
	canRemove,
	canShare,
	invitableRoles
} from './roles';

// Mirrors server/internal/authz/authz_test.go.
describe('roles', () => {
	it('lets admins and owners invite', () => {
		expect([
			canInvite('owner'),
			canInvite('admin'),
			canInvite('member'),
			canInvite('guest')
		]).toEqual([true, true, false, false]);
	});

	it('lets admins and owners publish the workspace themes', () => {
		expect(['owner', 'admin', 'member', 'guest'].map((r) => canPublishThemes(r as never))).toEqual([
			true,
			true,
			false,
			false
		]);
	});

	it('lets admins and owners share projects with guests', () => {
		expect([canShare('owner'), canShare('admin'), canShare('member'), canShare('guest')]).toEqual([
			true,
			true,
			false,
			false
		]);
	});

	it('lets members and above change saved views', () => {
		expect([
			canEditViews('owner'),
			canEditViews('admin'),
			canEditViews('member'),
			canEditViews('guest')
		]).toEqual([true, true, true, false]);
	});

	it('keeps owner invites to owners', () => {
		expect(invitableRoles('owner')).toContain('owner');
		expect(invitableRoles('admin')).not.toContain('owner');
		expect(invitableRoles('member')).toEqual([]);
	});

	it('lets admins manage members and guests only', () => {
		expect(assignableRoles('admin', 'member')).toEqual(['admin', 'member', 'guest']);
		expect(assignableRoles('admin', 'admin')).toEqual([]);
		expect(assignableRoles('admin', 'owner')).toEqual([]);
		expect(assignableRoles('member', 'guest')).toEqual([]);
		expect(canRemove('admin', 'guest')).toBe(true);
		expect(canRemove('admin', 'admin')).toBe(false);
		expect(canRemove('owner', 'owner')).toBe(true);
	});
});
