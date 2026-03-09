//go:build !windows

package cli

import (
	"os"
	"os/signal"
	"syscall"
)

// notifyUSR1 returns a channel that receives SIGUSR1 signals.
func notifyUSR1() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGUSR1)
	return ch
}

// sendUSR1 sends SIGUSR1 to the given process.
func sendUSR1(proc *os.Process) error {
	return proc.Signal(syscall.SIGUSR1)
}
