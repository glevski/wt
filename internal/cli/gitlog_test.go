package cli

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"wt/internal/gittest"
)

// gitLogFixture: linked repo plus a "side" worktree with one extra commit.
func gitLogFixture(t *testing.T) (repo, side string) {
	t.Helper()
	repo = linkedRepo(t)
	wtRoot(t)
	side = filepath.Join(t.TempDir(), "side")
	gittest.Git(t, repo, "worktree", "add", "-b", "side", side)
	gittest.WriteFile(t, side, "f.txt", "x")
	gittest.Commit(t, side, "side-only change")
	return repo, side
}

func TestGitLogNamedWorktree(t *testing.T) {
	repo, _ := gitLogFixture(t)
	out, _ := setupOutputs(t)

	if err := gitLog(repo, []string{"side"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "side-only change") {
		t.Errorf("git-log side missing the worktree's commit:\n%s", out.String())
	}
}

func TestGitLogPrefixAndCurrent(t *testing.T) {
	repo, side := gitLogFixture(t)

	out, _ := setupOutputs(t)
	if err := gitLog(repo, []string{"si"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "side-only change") {
		t.Errorf("prefix match failed:\n%s", out.String())
	}

	out, _ = setupOutputs(t)
	if err := gitLog(side, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "side-only change") {
		t.Errorf("no-arg git-log not run in the current worktree:\n%s", out.String())
	}
}

func TestGitLogPassesArgsThrough(t *testing.T) {
	repo, _ := gitLogFixture(t)
	out, _ := setupOutputs(t)

	if err := gitLog(repo, []string{"side", "--oneline", "-1"}); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(out.String())
	if strings.Contains(got, "Author:") || !strings.Contains(got, "side-only change") || strings.Count(got, "\n") != 0 {
		t.Errorf("--oneline -1 not passed through:\n%s", got)
	}

	// flag-first form: everything goes to git log, run where you stand
	out, _ = setupOutputs(t)
	if err := gitLog(repo, []string{"--oneline", "-1"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "side-only change") {
		t.Errorf("flag-first form should log the current worktree, not side:\n%s", out.String())
	}
}

func TestGitLogUnknownWorktree(t *testing.T) {
	repo, _ := gitLogFixture(t)
	setupOutputs(t)

	err := gitLog(repo, []string{"nope"})
	if err == nil || !strings.Contains(err.Error(), "no worktree named") {
		t.Fatalf("err = %v", err)
	}
}

func TestGitLogCompletion(t *testing.T) {
	repo, _ := gitLogFixture(t)
	setupOutputs(t)

	if got := completionCandidates(repo, []string{"git-log"}); !slices.Contains(got, "side") {
		t.Errorf("git-log completion = %v, want worktree names", got)
	}
}
