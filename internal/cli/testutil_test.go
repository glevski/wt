package cli

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"wt/internal/gittest"
)

// setupOutputs redirects the package's stdout/stderr for one test.
func setupOutputs(t *testing.T) (out, errOut *bytes.Buffer) {
	t.Helper()
	out, errOut = new(bytes.Buffer), new(bytes.Buffer)
	origOut, origErr := stdout, stderr
	stdout, stderr = out, errOut
	t.Cleanup(func() { stdout, stderr = origOut, origErr })
	return out, errOut
}

// wtRoot points WT_ROOT at a fresh temp dir and returns it.
func wtRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("WT_ROOT", root)
	return root
}

// linkedRepo is a fresh repo linked to the project name "proj".
func linkedRepo(t *testing.T) string {
	t.Helper()
	repo := gittest.NewRepo(t)
	gittest.Git(t, repo, "config", "wt.name", "proj")
	return repo
}

// worktreePath is where a worktree named name lands for a project under root.
func worktreePath(root, project, name string) string {
	return filepath.Join(root, project, name)
}

// jumpScript is what a jump command emits on stdout for the wrapper to eval.
func jumpScript(dest, home string) string {
	return fmt.Sprintf("cd '%s'\nexport WT_HOME='%s'\n", dest, home)
}
