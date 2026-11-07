package authz

import "testing"

// Every role × action combination, spelled out. Changing a permission
// means changing this table on purpose.
func TestCan(t *testing.T) {
	tests := []struct {
		role  Role
		admin bool
		want  map[Action]bool
	}{
		{Customer, false, map[Action]bool{ViewMenu: true, ManageDishes: false, WriteReview: true}},
		{Staff, false, map[Action]bool{ViewMenu: true, ManageDishes: true, WriteReview: false}},
		{Owner, false, map[Action]bool{ViewMenu: true, ManageDishes: true, WriteReview: false}},
		{Customer, true, map[Action]bool{ViewMenu: true, ManageDishes: true, WriteReview: true}},
		{"unknown-role", false, map[Action]bool{ViewMenu: false, ManageDishes: false, WriteReview: false}},
	}
	for _, tt := range tests {
		for action, want := range tt.want {
			if got := Can(tt.role, tt.admin, action); got != want {
				t.Errorf("Can(%s, admin=%v, %s) = %v, want %v", tt.role, tt.admin, action, got, want)
			}
		}
	}
	if len(Policy) != 3 {
		t.Errorf("Policy has %d actions; add the new one to this test", len(Policy))
	}
}
