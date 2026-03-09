//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

func setDetachAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func detachSupported() bool { return true }
