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
		"worktree  " + filepath.Base(repo) + " (home)",
		"path      " + repo,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("status output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "On branch") {
		t.Errorf("git status shown without -g:\n%s", got)
	}

	out.Reset()
	if err := status(repo, []string{"-g"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "On branch main") {
		t.Errorf("-g did not append git status:\n%s", out.String())
	}
}

func TestStatusInLinkedWorktree(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	side := filepath.Join(t.TempDir(), "side")
	gittest.Git(t, repo, "worktree", "add", "-b", "side", side)
	gittest.WriteFile(t, side, "dirty.txt", "x")
	out, _ := setupOutputs(t)

	if err := status(side, []string{"--git"}); err != nil {
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

func TestStatusNamedWorktree(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	side := filepath.Join(t.TempDir(), "side")
	gittest.Git(t, repo, "worktree", "add", "-b", "side", side)
	gittest.WriteFile(t, side, "dirty.txt", "x")
	out, _ := setupOutputs(t)

	// invoked from the main checkout, but showing the named worktree
	if err := status(repo, []string{"-g", "side"}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "worktree  side\n") || !strings.Contains(got, "path      "+side) {
		t.Errorf("status output not about 'side':\n%s", got)
	}
	if !strings.Contains(got, "dirty.txt") {
		t.Errorf("git status part not run in the named worktree:\n%s", got)
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
