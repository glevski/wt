// Package git wraps the git binary for the worktree CLI. Every helper takes
// an explicit directory and runs `git -C <dir> …`, so nothing depends on the
// process working directory.
package git

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Run executes git -C dir with args and returns stdout with surrounding
// whitespace trimmed. On failure the error carries git's stderr.
func Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// RunPassthrough executes git -C dir with args writing straight to the given
// writers, nothing captured or translated. When stdout is the process's own
// terminal the child inherits its fd, so git sees a TTY and enables colors.
func RunPassthrough(dir string, stdout, stderr io.Writer, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}
