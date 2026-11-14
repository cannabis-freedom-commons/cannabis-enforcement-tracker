//go:build !unix

package proc

import "os/exec"

func own(*exec.Cmd) {}
