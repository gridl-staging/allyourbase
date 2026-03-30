package status

import "testing"

func TestIncidentStatus_IsValid(t *testing.T) {
	// Valid statuses — all four lifecycle states must be accepted.
	valid := []IncidentStatus{
		IncidentInvestigating,
		IncidentIdentified,
		IncidentMonitoring,
		IncidentResolved,
	}
	for _, s := range valid {
		if !s.IsValid() {
			t.Errorf("IsValid(%q) = false, want true", s)
		}
	}

	// Invalid statuses — typos, empty, mixed-case originals, arbitrary strings.
	invalid := []IncidentStatus{
		"",
		"open",
		"Investigating", // uppercase I — raw constants are lowercase
		"closed",
		"unknown",
		"RESOLVED",
	}
	for _, s := range invalid {
		if s.IsValid() {
			t.Errorf("IsValid(%q) = true, want false", s)
		}
	}
}

func TestParseIncidentStatus(t *testing.T) {
	tests := []struct {
		raw       string
		wantValid bool
		want      IncidentStatus
	}{
		// Exact lowercase — pass through unchanged.
		{"investigating", true, IncidentInvestigating},
		{"identified", true, IncidentIdentified},
		{"monitoring", true, IncidentMonitoring},
		{"resolved", true, IncidentResolved},

		// Normalisation: uppercase and mixed-case are lowered.
		{"Investigating", true, IncidentInvestigating},
		{"RESOLVED", true, IncidentResolved},
		{"Monitoring", true, IncidentMonitoring},

		// Normalisation: leading/trailing whitespace is trimmed.
		{"  investigating  ", true, IncidentInvestigating},
		{"\tresolved\n", true, IncidentResolved},

		// Invalid inputs.
		{"", false, ""},
		{"open", false, "open"},
		{"closed", false, "closed"},
		{"inv estigating", false, "inv estigating"}, // embedded space
	}
	for _, tc := range tests {
		got, ok := ParseIncidentStatus(tc.raw)
		if ok != tc.wantValid {
			t.Errorf("ParseIncidentStatus(%q) valid = %v, want %v", tc.raw, ok, tc.wantValid)
		}
		if tc.wantValid && got != tc.want {
			t.Errorf("ParseIncidentStatus(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestServiceStatusConstants(t *testing.T) {
	// Guard against accidental renaming — these values are stored in JSON/DB.
	consts := map[ServiceStatus]string{
		Operational:   "operational",
		Degraded:      "degraded",
		PartialOutage: "partial_outage",
		MajorOutage:   "major_outage",
	}
	for c, want := range consts {
		if string(c) != want {
			t.Errorf("ServiceStatus constant = %q, want %q", c, want)
		}
	}
}

func TestServiceNameConstants(t *testing.T) {
	// Guard against accidental renaming — these are used in API responses.
	names := map[ServiceName]string{
		Database:  "database",
		Storage:   "storage",
		Auth:      "auth",
		Realtime:  "realtime",
		Functions: "functions",
	}
	for n, want := range names {
		if string(n) != want {
			t.Errorf("ServiceName constant = %q, want %q", n, want)
		}
	}
}

func TestIncidentStatusConstants(t *testing.T) {
	// Guard against accidental renaming — stored in DB and API responses.
	statuses := map[IncidentStatus]string{
		IncidentInvestigating: "investigating",
		IncidentIdentified:    "identified",
		IncidentMonitoring:    "monitoring",
		IncidentResolved:      "resolved",
	}
	for s, want := range statuses {
		if string(s) != want {
			t.Errorf("IncidentStatus constant = %q, want %q", s, want)
		}
	}
}
