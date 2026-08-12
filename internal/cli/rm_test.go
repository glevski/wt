package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/git"
	"wt/internal/gittest"
)

func rmFixture(t *testing.T) (repo, side string) {
	t.Helper()
	repo = gittest.NewRepo(t)
	wtRoot(t)
	side = filepath.Join(t.TempDir(), "side")
	gittest.Git(t, repo, "worktree", "add", "-b", "side", side)
	return repo, side
}

func TestRmKeepsBranchByDefault(t *testing.T) {
	repo, side := rmFixture(t)
	_, errOut := setupOutputs(t)

	if err := rm(repo, []string{"side"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(side); !os.IsNotExist(err) {
		t.Error("worktree directory still exists")
	}
	if refs, _ := git.LookupBranch(repo, "side"); !refs.Local {
		t.Error("branch was deleted without -b")
	}
	if !strings.Contains(errOut.String(), "branch 'side' kept") {
		t.Errorf("log missing branch-kept note:\n%s", errOut.String())
	}
}

func TestRmDeletesBranchWithFlag(t *testing.T) {
	repo, _ := rmFixture(t)
	setupOutputs(t)

	if err := rm(repo, []string{"-b", "side"}); err != nil {
		t.Fatal(err)
	}
	if refs, _ := git.LookupBranch(repo, "side"); refs.Local {
		t.Error("branch still exists despite -b")
	}
}

func TestRmUnmergedBranchIsKept(t *testing.T) {
	repo, side := rmFixture(t)
	setupOutputs(t)
	gittest.WriteFile(t, side, "extra.txt", "x")
	gittest.Commit(t, side, "unmerged work")

	err := rm(repo, []string{"-b", "side"})
	if err == nil || !strings.Contains(err.Error(), "branch is kept") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(side); !os.IsNotExist(statErr) {
		t.Error("worktree should be removed even when branch deletion fails")
	}
	if refs, _ := git.LookupBranch(repo, "side"); !refs.Local {
		t.Error("unmerged branch was deleted")
	}
}

func TestRmDirtyWorktree(t *testing.T) {
	repo, side := rmFixture(t)
	setupOutputs(t)
	gittest.WriteFile(t, side, "dirty.txt", "x")

	if err := rm(repo, []string{"side"}); err == nil {
		t.Fatal("expected an error for a dirty worktree")
	}
	if _, err := os.Stat(side); err != nil {
		t.Fatal("dirty worktree was removed without -f")
	}
	if err := rm(repo, []string{"-f", "side"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(side); !os.IsNotExist(err) {
		t.Error("worktree survived rm -f")
	}
}

func TestRmMainWorktree(t *testing.T) {
	repo, _ := rmFixture(t)
	setupOutputs(t)

	err := rm(repo, []string{filepath.Base(repo)})
	if err == nil || !strings.Contains(err.Error(), "main worktree") {
		t.Fatalf("err = %v", err)
	}
}

func TestRmFromInsideTheWorktree(t *testing.T) {
	repo, side := rmFixture(t)
	setupOutputs(t)

	err := rm(side, []string{"side"})
	if err == nil || !strings.Contains(err.Error(), "you are inside") {
		t.Fatalf("err = %v", err)
	}
	_ = repo
}
