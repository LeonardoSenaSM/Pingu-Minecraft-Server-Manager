//go:build !windows

package manager

import "os/exec"

func configureChildProcess(cmd *exec.Cmd) {}
