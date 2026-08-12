package cli

import (
	"path/filepath"
	"testing"

	"wt/internal/gittest"
)

func TestHome(t *testing.T) {
	repo := gittest.NewRepo(t)
	wtRoot(t)
	side := filepath.Join(t.TempDir(), "side")
	gittest.Git(t, repo, "worktree", "add", "-b", "side", side)

	out, _ := setupOutputs(t)
	if err := home(side, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), jumpScript(repo, repo); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}

	if err := home(repo, []string{"extra"}); err == nil {
		t.Error("expected usage error with arguments")
	}
}
