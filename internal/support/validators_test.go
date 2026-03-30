package support

import "testing"

func TestIsAllowedPriority(t *testing.T) {
	t.Parallel()

	// All valid priority constants must be accepted.
	valid := []string{
		TicketPriorityLow,
		TicketPriorityNormal,
		TicketPriorityHigh,
		TicketPriorityUrgent,
	}
	for _, p := range valid {
		if !isAllowedPriority(p) {
			t.Errorf("isAllowedPriority(%q) = false, want true", p)
		}
	}

	// Invalid inputs — typos, case variants, empty.
	invalid := []string{"", "Low", "URGENT", "critical", "medium", "p1"}
	for _, p := range invalid {
		if isAllowedPriority(p) {
			t.Errorf("isAllowedPriority(%q) = true, want false", p)
		}
	}
}

func TestIsAllowedStatus(t *testing.T) {
	t.Parallel()

	valid := []string{
		TicketStatusOpen,
		TicketStatusInProgress,
		TicketStatusWaitingOnCustomer,
		TicketStatusResolved,
		TicketStatusClosed,
	}
	for _, s := range valid {
		if !isAllowedStatus(s) {
			t.Errorf("isAllowedStatus(%q) = false, want true", s)
		}
	}

	invalid := []string{"", "Open", "CLOSED", "pending", "cancelled", "wontfix"}
	for _, s := range invalid {
		if isAllowedStatus(s) {
			t.Errorf("isAllowedStatus(%q) = true, want false", s)
		}
	}
}

func TestIsAllowedSenderType(t *testing.T) {
	t.Parallel()

	valid := []string{SenderCustomer, SenderSupport, SenderSystem}
	for _, s := range valid {
		if !isAllowedSenderType(s) {
			t.Errorf("isAllowedSenderType(%q) = false, want true", s)
		}
	}

	invalid := []string{"", "Customer", "SYSTEM", "admin", "bot", "agent"}
	for _, s := range invalid {
		if isAllowedSenderType(s) {
			t.Errorf("isAllowedSenderType(%q) = true, want false", s)
		}
	}
}

func TestTicketConstants(t *testing.T) {
	t.Parallel()

	// Guard against accidental renames — these values are stored in DB.
	priorities := map[string]string{
		TicketPriorityLow:    "low",
		TicketPriorityNormal: "normal",
		TicketPriorityHigh:   "high",
		TicketPriorityUrgent: "urgent",
	}
	for got, want := range priorities {
		if got != want {
			t.Errorf("priority constant = %q, want %q", got, want)
		}
	}

	statuses := map[string]string{
		TicketStatusOpen:              "open",
		TicketStatusInProgress:        "in_progress",
		TicketStatusWaitingOnCustomer: "waiting_on_customer",
		TicketStatusResolved:          "resolved",
		TicketStatusClosed:            "closed",
	}
	for got, want := range statuses {
		if got != want {
			t.Errorf("status constant = %q, want %q", got, want)
		}
	}

	senders := map[string]string{
		SenderCustomer: "customer",
		SenderSupport:  "support",
		SenderSystem:   "system",
	}
	for got, want := range senders {
		if got != want {
			t.Errorf("sender constant = %q, want %q", got, want)
		}
	}
}

func TestExtractTicketIDFromSubject_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		subject string
		wantID  string
		wantOK  bool
	}{
		{
			name:    "standard reply format",
			subject: "Re: [Ticket #abcdef01-2345-6789-abcd-ef0123456789]",
			wantID:  "abcdef01-2345-6789-abcd-ef0123456789",
			wantOK:  true,
		},
		{
			name:    "uppercase UUID normalised to lowercase",
			subject: "[Ticket #ABCDEF01-2345-6789-ABCD-EF0123456789]",
			wantID:  "abcdef01-2345-6789-abcd-ef0123456789",
			wantOK:  true,
		},
		{
			name:    "ticket id embedded in longer subject",
			subject: "Fwd: Re: Your issue [Ticket #11111111-2222-3333-4444-555555555555] update",
			wantID:  "11111111-2222-3333-4444-555555555555",
			wantOK:  true,
		},
		{
			name:    "no ticket marker",
			subject: "Hello support team",
			wantID:  "",
			wantOK:  false,
		},
		{
			name:    "partial UUID rejected",
			subject: "[Ticket #abcdef01-2345]",
			wantID:  "",
			wantOK:  false,
		},
		{
			name:    "empty subject",
			subject: "",
			wantID:  "",
			wantOK:  false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			id, ok := ExtractTicketIDFromSubject(tc.subject)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if tc.wantOK && id != tc.wantID {
				t.Fatalf("id = %q, want %q", id, tc.wantID)
			}
		})
	}
}

func TestNormalizeEmailAddress_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{
			name: "plain lowercase",
			raw:  "user@example.com",
			want: "user@example.com",
		},
		{
			name: "mixed case normalised",
			raw:  "User@Example.COM",
			want: "user@example.com",
		},
		{
			name: "display name stripped",
			raw:  "John Doe <john@example.com>",
			want: "john@example.com",
		},
		{
			name: "leading/trailing whitespace trimmed",
			raw:  "  user@example.com  ",
			want: "user@example.com",
		},
		{
			name:    "empty string rejected",
			raw:     "",
			wantErr: true,
		},
		{
			name:    "garbage rejected",
			raw:     "not-an-email",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeEmailAddress(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSentinelErrors(t *testing.T) {
	t.Parallel()

	// All sentinel errors must be distinct — prevents copy-paste mix-ups.
	sentinels := []error{
		ErrEmptySubject,
		ErrEmptyBody,
		ErrInvalidPriority,
		ErrInvalidSender,
		ErrInvalidStatus,
		ErrNoTicketUpdates,
		ErrStoreUnavailable,
	}
	seen := make(map[string]bool, len(sentinels))
	for _, err := range sentinels {
		msg := err.Error()
		if seen[msg] {
			t.Errorf("duplicate sentinel error message: %q", msg)
		}
		seen[msg] = true
	}
}
