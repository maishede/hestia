//go:build !windows

package server

import "os/exec"

func hideChildWindow(cmd *exec.Cmd) {}
