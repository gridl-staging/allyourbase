//go:build windows

package cli

import (
	"fmt"
	"os"
)

// notifyUSR1 returns a channel that never receives (SIGUSR1 is not available on Windows).
func notifyUSR1() <-chan os.Signal {
	return make(chan os.Signal)
}

// sendUSR1 returns an error because SIGUSR1 is not available on Windows.
func sendUSR1(proc *os.Process) error {
	return fmt.Errorf("password reset via signal is not supported on Windows")
}
