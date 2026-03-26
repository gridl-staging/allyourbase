package backup

import "testing"

func TestParseWALFileNameValid(t *testing.T) {
	got, err := ParseWALFileName("000000020000000A000000FE")
	if err != nil {
		t.Fatalf("ParseWALFileName: %v", err)
	}
	if got.Timeline != 2 {
		t.Errorf("Timeline = %d; want 2", got.Timeline)
	}
	if got.SegmentHigh != 0x0000000A {
		t.Errorf("SegmentHigh = %#x; want %#x", got.SegmentHigh, uint32(0x0000000A))
	}
	if got.SegmentLow != 0x000000FE {
		t.Errorf("SegmentLow = %#x; want %#x", got.SegmentLow, uint32(0x000000FE))
	}
	if got.OriginalName != "000000020000000A000000FE" {
		t.Errorf("OriginalName = %q; want exact input", got.OriginalName)
	}
}

func TestWALFileNameLSNBoundaries(t *testing.T) {
	wal, err := ParseWALFileName("000000010000000000000002")
	if err != nil {
		t.Fatalf("ParseWALFileName: %v", err)
	}
	if got := wal.StartLSN(); got != "0/2000000" {
		t.Errorf("StartLSN() = %q; want %q", got, "0/2000000")
	}
	if got := wal.EndLSN(); got != "0/3000000" {
		t.Errorf("EndLSN() = %q; want %q", got, "0/3000000")
	}

	carry, err := ParseWALFileName("0000000100000001000000FF")
	if err != nil {
		t.Fatalf("ParseWALFileName carry case: %v", err)
	}
	if got := carry.StartLSN(); got != "1/FF000000" {
		t.Errorf("StartLSN() carry = %q; want %q", got, "1/FF000000")
	}
	if got := carry.EndLSN(); got != "2/0" {
		t.Errorf("EndLSN() carry = %q; want %q", got, "2/0")
	}
}

func TestParseWALFileNameRejectsInvalid(t *testing.T) {
	cases := []struct {
		name    string
		wantErr string
	}{
		{name: "", wantErr: "empty"},
		{name: "0000000100000000000000", wantErr: "24"},
		{name: "00000001000000000000000Z", wantErr: "hex"},
		{name: "000000010000000000000001.partial", wantErr: ".partial"},
		{name: "000000010000000000000001.backup", wantErr: ".backup"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseWALFileName(tc.name)
			if err == nil {
				t.Fatalf("expected error for %q", tc.name)
			}
			if tc.wantErr != "" && !containsFold(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func containsFold(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if equalFoldASCII(s[i:i+len(sub)], sub) {
			return true
		}
	}
	return false
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca := a[i]
		cb := b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
