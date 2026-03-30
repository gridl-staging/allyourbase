package tenant

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// computeRemaining
// ---------------------------------------------------------------------------

func TestComputeRemaining(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		limit   int64
		current int
		want    int
	}{
		// Normal cases — remaining = limit - current, clamped to 0.
		{"under limit", 100, 30, 70},
		{"at limit", 100, 100, 0},
		{"over limit", 100, 150, 0}, // clamped to 0, not negative
		{"zero current", 50, 0, 50},

		// Edge cases with limit <= 0.
		{"zero limit", 0, 5, 0},
		{"negative limit", -1, 0, 0},

		// Large values.
		{"large limit", 1000000, 999999, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := computeRemaining(tc.limit, tc.current)
			if got != tc.want {
				t.Errorf("computeRemaining(%d, %d) = %d, want %d",
					tc.limit, tc.current, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// normalizeRPSLimit
// ---------------------------------------------------------------------------

func TestNormalizeRPSLimit(t *testing.T) {
	t.Parallel()

	t.Run("nil returns (0, false)", func(t *testing.T) {
		t.Parallel()
		val, ok := normalizeRPSLimit(nil)
		if ok || val != 0 {
			t.Errorf("normalizeRPSLimit(nil) = (%d, %v), want (0, false)", val, ok)
		}
	})

	t.Run("zero returns (0, false)", func(t *testing.T) {
		t.Parallel()
		zero := 0
		val, ok := normalizeRPSLimit(&zero)
		if ok || val != 0 {
			t.Errorf("normalizeRPSLimit(0) = (%d, %v), want (0, false)", val, ok)
		}
	})

	t.Run("negative returns (0, false)", func(t *testing.T) {
		t.Parallel()
		neg := -5
		val, ok := normalizeRPSLimit(&neg)
		if ok || val != 0 {
			t.Errorf("normalizeRPSLimit(-5) = (%d, %v), want (0, false)", val, ok)
		}
	})

	t.Run("positive RPS converts to per-minute", func(t *testing.T) {
		t.Parallel()
		// 10 RPS → 10 * 60 = 600 requests per minute.
		rps := 10
		val, ok := normalizeRPSLimit(&rps)
		if !ok {
			t.Fatal("normalizeRPSLimit(10) ok = false, want true")
		}
		want := int64(10) * int64(time.Minute.Seconds())
		if val != want {
			t.Errorf("normalizeRPSLimit(10) = %d, want %d", val, want)
		}
	})

	t.Run("1 RPS", func(t *testing.T) {
		t.Parallel()
		one := 1
		val, ok := normalizeRPSLimit(&one)
		if !ok {
			t.Fatal("ok = false")
		}
		if val != 60 { // 1 * 60
			t.Errorf("val = %d, want 60", val)
		}
	})
}

// ---------------------------------------------------------------------------
// pruneTimestamps
// ---------------------------------------------------------------------------

func TestPruneTimestamps(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	cutoff := base

	t.Run("removes timestamps at or before cutoff", func(t *testing.T) {
		t.Parallel()
		timestamps := []time.Time{
			base.Add(-2 * time.Second), // before cutoff
			base,                       // at cutoff (not After, so pruned)
			base.Add(1 * time.Second),  // after cutoff (kept)
			base.Add(5 * time.Second),  // after cutoff (kept)
		}
		// Make a copy to avoid shared backing array issues.
		input := make([]time.Time, len(timestamps))
		copy(input, timestamps)

		got := pruneTimestamps(input, cutoff)
		if len(got) != 2 {
			t.Fatalf("pruneTimestamps len = %d, want 2", len(got))
		}
		if !got[0].Equal(base.Add(1 * time.Second)) {
			t.Errorf("got[0] = %v, want base+1s", got[0])
		}
		if !got[1].Equal(base.Add(5 * time.Second)) {
			t.Errorf("got[1] = %v, want base+5s", got[1])
		}
	})

	t.Run("all timestamps before cutoff", func(t *testing.T) {
		t.Parallel()
		timestamps := []time.Time{
			base.Add(-3 * time.Second),
			base.Add(-1 * time.Second),
		}
		input := make([]time.Time, len(timestamps))
		copy(input, timestamps)

		got := pruneTimestamps(input, cutoff)
		if len(got) != 0 {
			t.Fatalf("expected empty, got %d", len(got))
		}
	})

	t.Run("all timestamps after cutoff", func(t *testing.T) {
		t.Parallel()
		timestamps := []time.Time{
			base.Add(1 * time.Second),
			base.Add(2 * time.Second),
		}
		input := make([]time.Time, len(timestamps))
		copy(input, timestamps)

		got := pruneTimestamps(input, cutoff)
		if len(got) != 2 {
			t.Fatalf("expected 2, got %d", len(got))
		}
	})

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()
		got := pruneTimestamps([]time.Time{}, cutoff)
		if len(got) != 0 {
			t.Fatalf("expected empty, got %d", len(got))
		}
	})
}

// ---------------------------------------------------------------------------
// intToInt64Ptr
// ---------------------------------------------------------------------------

func TestIntToInt64Ptr(t *testing.T) {
	t.Parallel()

	t.Run("nil input returns nil", func(t *testing.T) {
		t.Parallel()
		got := intToInt64Ptr(nil)
		if got != nil {
			t.Fatalf("intToInt64Ptr(nil) = %v, want nil", *got)
		}
	})

	t.Run("positive value", func(t *testing.T) {
		t.Parallel()
		v := 42
		got := intToInt64Ptr(&v)
		if got == nil || *got != 42 {
			t.Fatalf("intToInt64Ptr(42) = %v, want *42", got)
		}
	})

	t.Run("zero value", func(t *testing.T) {
		t.Parallel()
		v := 0
		got := intToInt64Ptr(&v)
		if got == nil || *got != 0 {
			t.Fatalf("intToInt64Ptr(0) = %v, want *0", got)
		}
	})

	t.Run("negative value", func(t *testing.T) {
		t.Parallel()
		v := -7
		got := intToInt64Ptr(&v)
		if got == nil || *got != -7 {
			t.Fatalf("intToInt64Ptr(-7) = %v, want *-7", got)
		}
	})

	t.Run("mutation isolation", func(t *testing.T) {
		t.Parallel()
		// Changing the original after conversion must not affect the result.
		v := 100
		got := intToInt64Ptr(&v)
		v = 999
		if *got != 100 {
			t.Fatalf("mutating original affected result: got %d, want 100", *got)
		}
	})
}

// ---------------------------------------------------------------------------
// mapOrgRoleToEffectiveRole
// ---------------------------------------------------------------------------

func TestMapOrgRoleToEffectiveRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		role   string
		want   string
		wantOK bool
	}{
		// Owner and admin both map to admin effective role.
		{OrgRoleOwner, MemberRoleAdmin, true},
		{OrgRoleAdmin, MemberRoleAdmin, true},
		// Member and viewer map to viewer effective role.
		{OrgRoleMember, MemberRoleViewer, true},
		{OrgRoleViewer, MemberRoleViewer, true},
		// Unknown roles are rejected.
		{"", "", false},
		{"superadmin", "", false},
		{"guest", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.role, func(t *testing.T) {
			t.Parallel()
			got, ok := mapOrgRoleToEffectiveRole(tc.role)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if tc.wantOK && got != tc.want {
				t.Errorf("mapOrgRoleToEffectiveRole(%q) = %q, want %q", tc.role, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// mapTeamRoleToEffectiveRole
// ---------------------------------------------------------------------------

func TestMapTeamRoleToEffectiveRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		role   string
		want   string
		wantOK bool
	}{
		// Lead maps to member effective role.
		{TeamRoleLead, MemberRoleMember, true},
		// Member maps to viewer effective role.
		{TeamRoleMember, MemberRoleViewer, true},
		// Unknown roles are rejected.
		{"", "", false},
		{"admin", "", false},
		{"owner", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.role, func(t *testing.T) {
			t.Parallel()
			got, ok := mapTeamRoleToEffectiveRole(tc.role)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if tc.wantOK && got != tc.want {
				t.Errorf("mapTeamRoleToEffectiveRole(%q) = %q, want %q", tc.role, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// migrationIdempotencyKey
// ---------------------------------------------------------------------------

func TestMigrationIdempotencyKey(t *testing.T) {
	t.Parallel()

	t.Run("format is prefix + user ID", func(t *testing.T) {
		t.Parallel()
		got := migrationIdempotencyKey("user-123")
		want := "miglegacy:user-123"
		if got != want {
			t.Errorf("migrationIdempotencyKey(%q) = %q, want %q", "user-123", got, want)
		}
	})

	t.Run("different users produce different keys", func(t *testing.T) {
		t.Parallel()
		k1 := migrationIdempotencyKey("alice")
		k2 := migrationIdempotencyKey("bob")
		if k1 == k2 {
			t.Error("different user IDs should produce different keys")
		}
	})

	t.Run("empty user ID", func(t *testing.T) {
		t.Parallel()
		got := migrationIdempotencyKey("")
		if got != "miglegacy:" {
			t.Errorf("migrationIdempotencyKey('') = %q, want 'miglegacy:'", got)
		}
	})
}

// ---------------------------------------------------------------------------
// normalizeIntLimit (dependency of normalizeRPSLimit)
// ---------------------------------------------------------------------------

func TestNormalizeIntLimit(t *testing.T) {
	t.Parallel()

	t.Run("nil returns (0, false)", func(t *testing.T) {
		t.Parallel()
		val, ok := normalizeIntLimit(nil)
		if ok || val != 0 {
			t.Errorf("got (%d, %v), want (0, false)", val, ok)
		}
	})

	t.Run("zero returns (0, false)", func(t *testing.T) {
		t.Parallel()
		v := 0
		val, ok := normalizeIntLimit(&v)
		if ok || val != 0 {
			t.Errorf("got (%d, %v), want (0, false)", val, ok)
		}
	})

	t.Run("negative returns (0, false)", func(t *testing.T) {
		t.Parallel()
		v := -3
		val, ok := normalizeIntLimit(&v)
		if ok || val != 0 {
			t.Errorf("got (%d, %v), want (0, false)", val, ok)
		}
	})

	t.Run("positive returns (int64(v), true)", func(t *testing.T) {
		t.Parallel()
		v := 42
		val, ok := normalizeIntLimit(&v)
		if !ok || val != 42 {
			t.Errorf("got (%d, %v), want (42, true)", val, ok)
		}
	})
}
