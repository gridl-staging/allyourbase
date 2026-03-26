package tenant

import "testing"

// validTransitionCases maps each valid transition to true.
var validTransitionCases = []struct {
	from TenantState
	to   TenantState
}{
	{TenantStateProvisioning, TenantStateActive},
	{TenantStateActive, TenantStateSuspended},
	{TenantStateSuspended, TenantStateActive},
	{TenantStateActive, TenantStateDeleting},
	{TenantStateSuspended, TenantStateDeleting},
	{TenantStateDeleting, TenantStateDeleted},
}

// invalidTransitionCases holds transitions that must be rejected.
var invalidTransitionCases = []struct {
	from TenantState
	to   TenantState
}{
	{TenantStateProvisioning, TenantStateDeleted},
	{TenantStateProvisioning, TenantStateSuspended},
	{TenantStateProvisioning, TenantStateDeleting},
	{TenantStateProvisioning, TenantStateProvisioning},
	{TenantStateActive, TenantStateProvisioning},
	{TenantStateActive, TenantStateDeleted},
	{TenantStateActive, TenantStateActive},
	{TenantStateSuspended, TenantStateProvisioning},
	{TenantStateSuspended, TenantStateDeleted},
	{TenantStateSuspended, TenantStateSuspended},
	{TenantStateDeleting, TenantStateProvisioning},
	{TenantStateDeleting, TenantStateActive},
	{TenantStateDeleting, TenantStateSuspended},
	{TenantStateDeleting, TenantStateDeleting},
	{TenantStateDeleted, TenantStateProvisioning},
	{TenantStateDeleted, TenantStateActive},
	{TenantStateDeleted, TenantStateSuspended},
	{TenantStateDeleted, TenantStateDeleting},
	{TenantStateDeleted, TenantStateDeleted},
}

func TestIsValidTransition_ValidCases(t *testing.T) {
	for _, tc := range validTransitionCases {
		if !IsValidTransition(tc.from, tc.to) {
			t.Errorf("expected valid transition %s -> %s to be accepted, but it was rejected", tc.from, tc.to)
		}
	}
}

func TestIsValidTransition_InvalidCases(t *testing.T) {
	for _, tc := range invalidTransitionCases {
		if IsValidTransition(tc.from, tc.to) {
			t.Errorf("expected invalid transition %s -> %s to be rejected, but it was accepted", tc.from, tc.to)
		}
	}
}

func TestTenantStateConstants(t *testing.T) {
	// Verify each constant matches the DB/JSON representation expected by SQL CHECK constraints.
	cases := map[TenantState]string{
		TenantStateProvisioning: "provisioning",
		TenantStateActive:       "active",
		TenantStateSuspended:    "suspended",
		TenantStateDeleting:     "deleting",
		TenantStateDeleted:      "deleted",
	}
	for state, want := range cases {
		if string(state) != want {
			t.Errorf("TenantState constant: got %q, want %q", string(state), want)
		}
	}
}

func TestNormalizeIsolationMode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "legacy_database_maps_to_shared", in: "database", want: "shared"},
		{name: "shared_stays_shared", in: "shared", want: "shared"},
		{name: "schema_stays_schema", in: "schema", want: "schema"},
		{name: "empty_defaults_to_shared", in: "", want: "shared"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeIsolationMode(tt.in); got != tt.want {
				t.Errorf("NormalizeIsolationMode(%q): got %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
