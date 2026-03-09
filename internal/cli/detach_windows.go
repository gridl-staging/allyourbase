//go:build windows

package cli

import "os/exec"

func setDetachAttrs(cmd *exec.Cmd) {}

func detachSupported() bool { return false }
