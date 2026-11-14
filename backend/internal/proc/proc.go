// Package proc starts child processes apart from the terminal's process group.
package proc

import "os/exec"

// Own puts cmd in a process group of its own, so a Ctrl-C in the terminal reaches only this
// process, which decides when its children stop (through the context they were started with):
// the worker's first Ctrl-C lets the running jobs finish.
func Own(cmd *exec.Cmd) *exec.Cmd {
	own(cmd)
	return cmd
}
