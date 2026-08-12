package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/gittest"
)

func TestStatusInMainWorktree(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	out, _ := setupOutputs(t)

	if err := status(repo, nil); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"project   proj",
		"worktree  " + filepath.Base(repo) + " (main)",
		"path      " + repo,
		"On branch main",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("status output missing %q:\n%s", want, got)
		}
	}
}

func TestStatusInLinkedWorktree(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	side := filepath.Join(t.TempDir(), "side")
	gittest.Git(t, repo, "worktree", "add", "-b", "side", side)
	gittest.WriteFile(t, side, "dirty.txt", "x")
	out, _ := setupOutputs(t)

	if err := status(side, nil); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "worktree  side\n") {
		t.Errorf("status output missing worktree name (or wrongly marked main):\n%s", got)
	}
	if !strings.Contains(got, "dirty.txt") {
		t.Errorf("git status part missing the dirty file:\n%s", got)
	}
}

func TestStatusUnlinkedRepo(t *testing.T) {
	repo := gittest.NewRepo(t)
	wtRoot(t)
	out, _ := setupOutputs(t)

	if err := status(repo, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "not linked") {
		t.Errorf("status output should flag the missing link:\n%s", out.String())
	}
}
