// Package git wraps the git binary for the worktree CLI. Every helper takes
// an explicit directory and runs `git -C <dir> …`, so nothing depends on the
// process working directory.
package git

import (
	"bytes"
	"fmt"
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
