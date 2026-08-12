// Package gittest builds throwaway git repositories for tests.
package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Git runs git -C dir with a fixed test identity, failing the test on error.
func Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	base := []string{"-C", dir,
		"-c", "user.name=wt-test", "-c", "user.email=wt@test",
		"-c", "commit.gpgsign=false",
	}
	out, err := exec.Command("git", append(base, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// NewRepo creates a repo with one commit on main and returns its path.
func NewRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	Git(t, dir, "init", "-b", "main")
	WriteFile(t, dir, "README.md", "hello\n")
	Git(t, dir, "add", ".")
	Git(t, dir, "commit", "-m", "initial")
	return dir
}

// AddRemote wires repo to a fresh bare "origin" and pushes main to it.
func AddRemote(t *testing.T, repo string) string {
	t.Helper()
	remote := t.TempDir()
	Git(t, remote, "init", "--bare", "-b", "main")
	Git(t, repo, "remote", "add", "origin", remote)
	Git(t, repo, "push", "-u", "origin", "main")
	return remote
}

// WriteFile writes content to name (relative to dir), creating parent dirs.
func WriteFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Commit stages everything and commits.
func Commit(t *testing.T, dir, msg string) {
	t.Helper()
	Git(t, dir, "add", "-A")
	Git(t, dir, "commit", "-m", msg)
}
