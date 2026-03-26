package backup

import "testing"

func TestBuildBaseBackupArgs(t *testing.T) {
	dbURL := "postgresql://postgres:postgres@127.0.0.1:5432/app"
	args := buildBaseBackupArgs(dbURL)

	want := []string{
		"--dbname=" + dbURL,
		"--format=tar",
		"--checkpoint=fast",
		"--wal-method=none",
		"-D",
		"-",
	}

	if len(args) != len(want) {
		t.Fatalf("arg length = %d; want %d (%v)", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("arg[%d] = %q; want %q", i, args[i], want[i])
		}
	}
}
