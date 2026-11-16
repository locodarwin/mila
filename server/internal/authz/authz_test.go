package authz

import "testing"

func TestCan(t *testing.T) {
	// Each row: the action, then whether owner, admin, member, guest may.
	table := []struct {
		action Action
		want   [4]bool
	}{
		{ViewWorkspace, [4]bool{true, true, true, true}},
		{RenameWorkspace, [4]bool{true, true, false, false}},
		{DeleteWorkspace, [4]bool{true, false, false, false}},
		{ViewMembers, [4]bool{true, true, true, false}},
		{InviteMembers, [4]bool{true, true, false, false}},
		{ManageMembers, [4]bool{true, true, false, false}},
		{EditContent, [4]bool{true, true, true, false}},
		{SubscribeCalendar, [4]bool{true, true, true, true}},
		{ExportWorkspace, [4]bool{true, true, true, true}},
		{ShareProjects, [4]bool{true, true, false, false}},
		{Action("unknown"), [4]bool{false, false, false, false}},
	}
	for _, row := range table {
		for i, role := range Roles {
			if got := Can(role, row.action); got != row.want[i] {
				t.Errorf("Can(%s, %s) = %v, want %v", role, row.action, got, row.want[i])
			}
		}
	}
	if Can(Role("superuser"), ViewWorkspace) {
		t.Error("an unknown role must be refused")
	}
}

func TestCanInviteAs(t *testing.T) {
	cases := []struct {
		actor, role Role
		want        bool
	}{
		{Owner, Owner, true},
		{Owner, Admin, true},
		{Admin, Owner, false},
		{Admin, Admin, true},
		{Admin, Guest, true},
		{Member, Member, false},
		{Guest, Guest, false},
		{Owner, Role("boss"), false},
	}
	for _, c := range cases {
		if got := CanInviteAs(c.actor, c.role); got != c.want {
			t.Errorf("CanInviteAs(%s, %s) = %v, want %v", c.actor, c.role, got, c.want)
		}
	}
}

func TestCanChangeRole(t *testing.T) {
	cases := []struct {
		actor, from, to Role
		want            bool
	}{
		{Owner, Admin, Owner, true},
		{Owner, Owner, Member, true},
		{Admin, Member, Admin, true}, // same as inviting an admin
		{Admin, Member, Guest, true},
		{Admin, Guest, Member, true},
		{Admin, Admin, Member, false},
		{Admin, Owner, Member, false},
		{Admin, Member, Owner, false},
		{Member, Guest, Member, false},
		{Owner, Member, Role("x"), false},
	}
	for _, c := range cases {
		if got := CanChangeRole(c.actor, c.from, c.to); got != c.want {
			t.Errorf("CanChangeRole(%s, %s -> %s) = %v, want %v", c.actor, c.from, c.to, got, c.want)
		}
	}
}

func TestCanRemove(t *testing.T) {
	cases := []struct {
		actor, target Role
		want          bool
	}{
		{Owner, Owner, true},
		{Owner, Admin, true},
		{Admin, Owner, false},
		{Admin, Admin, false},
		{Admin, Member, true},
		{Admin, Guest, true},
		{Member, Guest, false},
	}
	for _, c := range cases {
		if got := CanRemove(c.actor, c.target); got != c.want {
			t.Errorf("CanRemove(%s, %s) = %v, want %v", c.actor, c.target, got, c.want)
		}
	}
}

func TestCanInstance(t *testing.T) {
	// Each row: the action, then whether an instance admin and anyone
	// else may.
	table := []struct {
		action InstanceAction
		admin  bool
		other  bool
	}{
		{ViewInstance, true, false},
		{ManageUsers, true, false},
		{ManageInvites, true, false},
		{BackUpInstance, true, false},
		{InstanceAction("instance.unknown"), false, false},
		{InstanceAction(ViewWorkspace), false, false},
	}
	for _, row := range table {
		if got := CanInstance(true, row.action); got != row.admin {
			t.Errorf("CanInstance(admin, %s) = %v, want %v", row.action, got, row.admin)
		}
		if got := CanInstance(false, row.action); got != row.other {
			t.Errorf("CanInstance(non-admin, %s) = %v, want %v", row.action, got, row.other)
		}
	}
}
