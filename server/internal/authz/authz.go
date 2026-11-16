// Package authz decides who may do what. Every path that changes or reads
// workspace data, whether REST, sync or a background job, asks here, so no
// path can skip a check. There are two layers: instance admins run the
// instance (its users, invitations and backups), and workspace roles
// decide what a member may do in a workspace. Guests see only the
// projects shared with them; sync applies that rule row by row.
package authz

// Role is a member's role in a workspace.
type Role string

const (
	Owner  Role = "owner"
	Admin  Role = "admin"
	Member Role = "member"
	Guest  Role = "guest"
)

// Roles lists every role, highest first.
var Roles = []Role{Owner, Admin, Member, Guest}

// Valid reports whether r is a known role.
func (r Role) Valid() bool {
	for _, known := range Roles {
		if r == known {
			return true
		}
	}
	return false
}

// rank orders roles so "at least admin" is a comparison.
func (r Role) rank() int {
	switch r {
	case Owner:
		return 4
	case Admin:
		return 3
	case Member:
		return 2
	case Guest:
		return 1
	}
	return 0
}

// Action is something a member may try to do to a workspace.
type Action string

const (
	ViewWorkspace   Action = "workspace.view"
	RenameWorkspace Action = "workspace.rename"
	DeleteWorkspace Action = "workspace.delete"
	ViewMembers     Action = "members.view"
	// ViewPeople is knowing who else is in a workspace by name and picture,
	// which comments and mentions need. Unlike ViewMembers it shows no email
	// addresses, and guests may.
	ViewPeople    Action = "people.view"
	InviteMembers Action = "members.invite"
	ManageMembers Action = "members.manage"
	// ManageWebhooks is adding, changing and reading the log of a workspace's webhooks.
	ManageWebhooks Action = "webhooks.manage"
	// EditContent covers projects, statuses and tasks. Guests are handled
	// apart: they may edit tasks only in projects shared with them.
	EditContent Action = "content.edit"
	// ReceiveEmail is having addresses that turn email into tasks.
	ReceiveEmail Action = "email.receive"
	// SubscribeCalendar is making a calendar feed of what the member can
	// see. Guests may too; their feeds show only the projects shared with
	// them.
	SubscribeCalendar Action = "calendar.subscribe"
	// ExportWorkspace is downloading a copy of what the member can see.
	// Everyone may: a guest's export holds only the projects shared with
	// them, as their sync does.
	ExportWorkspace Action = "workspace.export"
	// ShareProjects is giving a guest a project, or taking it away.
	ShareProjects Action = "projects.share"
)

// minimum is the lowest role allowed each action.
var minimum = map[Action]Role{
	ViewWorkspace:   Guest,
	RenameWorkspace: Admin,
	DeleteWorkspace: Owner,
	ViewMembers:     Member,
	ViewPeople:      Guest,
	InviteMembers:   Admin,
	ManageMembers:   Admin,
	ManageWebhooks:  Admin,
	EditContent:     Member,
	ReceiveEmail:    Member,

	SubscribeCalendar: Guest,
	ExportWorkspace:   Guest,
	ShareProjects:     Admin,
}

// Can reports whether a member with role actor may perform action.
// Unknown roles and actions are always refused.
func Can(actor Role, action Action) bool {
	min, ok := minimum[action]
	return ok && actor.Valid() && actor.rank() >= min.rank()
}

// CanInviteAs reports whether actor may invite someone with role. Only an
// owner can bring in another owner.
func CanInviteAs(actor, role Role) bool {
	return Can(actor, InviteMembers) && role.Valid() && (role != Owner || actor == Owner)
}

// CanChangeRole reports whether actor may move a member from one role to
// another. Admins manage members and guests; only owners touch owners.
// Keeping at least one owner is the caller's check, since it needs a count.
func CanChangeRole(actor, from, to Role) bool {
	if !Can(actor, ManageMembers) || !from.Valid() || !to.Valid() {
		return false
	}
	if actor == Owner {
		return true
	}
	return from.rank() < Admin.rank() && to.rank() < Owner.rank()
}

// CanRemove reports whether actor may remove a member with role target
// from the workspace. Anyone may leave; that case is checked by the caller,
// which knows whether actor and target are the same person.
func CanRemove(actor, target Role) bool {
	if !Can(actor, ManageMembers) || !target.Valid() {
		return false
	}
	return actor == Owner || target.rank() < Admin.rank()
}

// InstanceAction is something done to the instance as a whole. Only its
// admins may, whatever their roles in workspaces.
type InstanceAction string

const (
	// ViewInstance is reading the admin page: version, database, backups,
	// every account and every open invitation.
	ViewInstance InstanceAction = "instance.view"
	// ManageUsers is making someone an admin or taking it away, and
	// signing someone out everywhere.
	ManageUsers InstanceAction = "instance.users"
	// ManageInvites is canceling any open invitation.
	ManageInvites InstanceAction = "instance.invites"
	// BackUpInstance is asking for a backup now.
	BackUpInstance InstanceAction = "instance.backup"
	// ConnectMailbox is signing Mila in to, or out of, a Microsoft mailbox
	// for email to task.
	ConnectMailbox InstanceAction = "instance.mailbox"
)

var instanceActions = map[InstanceAction]bool{
	ViewInstance: true, ManageUsers: true, ManageInvites: true, BackUpInstance: true, ConnectMailbox: true,
}

// CanInstance reports whether someone may perform an instance action:
// only an instance admin, and only a known action.
func CanInstance(isAdmin bool, action InstanceAction) bool {
	return isAdmin && instanceActions[action]
}
