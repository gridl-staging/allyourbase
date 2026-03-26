package tenant

import "testing"

func TestRoleRank(t *testing.T) {
	tests := []struct {
		name string
		role string
		want int
	}{
		{name: "owner", role: MemberRoleOwner, want: 4},
		{name: "admin", role: MemberRoleAdmin, want: 3},
		{name: "member", role: MemberRoleMember, want: 2},
		{name: "viewer", role: MemberRoleViewer, want: 1},
		{name: "unknown", role: "unknown", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := roleRank(tt.role); got != tt.want {
				t.Fatalf("roleRank(%q) = %d, want %d", tt.role, got, tt.want)
			}
		})
	}
}
