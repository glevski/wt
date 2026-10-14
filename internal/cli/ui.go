package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

const uiInstallHint = "curl -fsSL https://github.com/glevski/wt-tui/raw/main/install.sh | sh"

// ui launches wt-ui, the interactive worktree browser — a separate program
// (github.com/glevski/wt-tui) that wt neither bundles nor downloads, so this
// works only where it is installed. wt-ui draws on /dev/tty and prints
// nothing on stdout but the jump script, in wt's own shape (WT_JUMP=json
// included), which is what makes `wt ui` a jump command like `wt ch`: the
// wt() function captures and evals whatever the child printed.
func ui(dir string, args []string) error {
	bin, ok := findUI()
	if !ok {
		return hintf(uiInstallHint, "wt-ui is not installed")
	}
	cmd := exec.Command(bin, append([]string{"-C", dir}, args...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	if os.Getenv("WT_WRAPPER_VERSION") != "" {
		// Under the wt() function the script gets eval'd. Without it wt-ui
		// is left to print its own "this needs a shell function" reminder,
		// like a bare `worktree ch` would.
		cmd.Env = append(os.Environ(), "WT_UI_WRAPPER=1")
	}
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		return &exitError{code: exitStatus(exit)}
	case err != nil:
		return fmt.Errorf("cannot run wt-ui: %w", err)
	}
	return nil
}

// findUI locates the wt-ui binary: $WT_UI_BIN (a development build, say),
// then PATH, then ~/.local/bin — where its installer puts it, which may be
// off PATH in a fresh shell.
func findUI() (string, bool) {
	if bin := os.Getenv("WT_UI_BIN"); bin != "" {
		return bin, true
	}
	if bin, err := exec.LookPath("wt-ui"); err == nil {
		return bin, true
	}
	if home, err := os.UserHomeDir(); err == nil {
		bin := filepath.Join(home, ".local", "bin", "wt-ui")
		if info, err := os.Stat(bin); err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			return bin, true
		}
	}
	return "", false
}

// exitStatus is the status a shell would report for the child: its exit
// code, or 128+signal when a signal killed it.
func exitStatus(exit *exec.ExitError) int {
	if ws, ok := exit.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return exit.ExitCode()
}
