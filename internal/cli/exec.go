package cli

import (
	"bytes"
	"os/exec"
)

// newExecCmd returns an *exec.Cmd with output suppressed.
func newExecCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	return cmd
}
