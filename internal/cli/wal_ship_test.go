package cli

import "testing"

func TestWalShipCommandRegistered(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "wal-ship" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected wal-ship command to be registered")
	}
}

func TestWalShipRequiresArgs(t *testing.T) {
	rootCmd.SetArgs([]string{"wal-ship"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected args error")
	}
}

func TestWalShipMissingConfig(t *testing.T) {
	rootCmd.SetArgs([]string{"wal-ship", "/tmp/seg", "000000010000000000000001", "--config", "/nonexistent/ayb.toml"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected config load error")
	}
}
