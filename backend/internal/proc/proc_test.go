//go:build unix

package proc

import (
	"os/exec"
	"syscall"
	"testing"
)

// A Ctrl-C reaches the terminal's whole process group; a child in a group of its own is not in it.
func TestOwnStartsAChildInItsOwnGroup(t *testing.T) {
	cmd := Own(exec.Command("sleep", "5"))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	group, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if group == syscall.Getpgrp() || group != cmd.Process.Pid {
		t.Fatalf("child group %d, own group %d", group, syscall.Getpgrp())
	}
}
