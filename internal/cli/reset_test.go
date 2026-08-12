package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/gittest"
)

// resetFixture builds a linked repo with a wt-created worktree main-2, whose
// recorded base is main.
func resetFixture(t *testing.T) (repo, wtPath string) {
	t.Helper()
	repo = linkedRepo(t)
	root := wtRoot(t)
	setupOutputs(t)
	if err := create(repo, nil); err != nil {
		t.Fatal(err)
	}
	return repo, worktreePath(root, "proj", "main-2")
}

func TestResetToRecordedBase(t *testing.T) {
	repo, wtPath := resetFixture(t)
	gittest.WriteFile(t, repo, "advance.txt", "x")
	gittest.Commit(t, repo, "advance main")
	mainSHA := gittest.Git(t, repo, "rev-parse", "HEAD")

	if err := reset(wtPath, nil); err != nil {
		t.Fatal(err)
	}
	if got := gittest.Git(t, wtPath, "rev-parse", "HEAD"); got != mainSHA {
		t.Errorf("worktree HEAD = %s, want main's tip %s", got, mainSHA)
	}
	if got := gittest.Git(t, wtPath, "symbolic-ref", "--short", "HEAD"); got != "main-2" {
		t.Errorf("branch = %q, want to stay on main-2", got)
	}
}

func TestResetRefusesMainCheckout(t *testing.T) {
	repo, _ := resetFixture(t)

	err := reset(repo, nil)
	if err == nil || !strings.Contains(err.Error(), "main checkout") {
		t.Fatalf("err = %v", err)
	}
}

func TestResetBlocksOnChanges(t *testing.T) {
	_, wtPath := resetFixture(t)
	gittest.WriteFile(t, wtPath, "untracked.txt", "u")

	err := reset(wtPath, nil)
	if err == nil || !strings.Contains(err.Error(), "local changes") {
		t.Fatalf("untracked-only err = %v", err)
	}

	gittest.WriteFile(t, wtPath, "README.md", "tracked change")
	if err := reset(wtPath, []string{"--hard"}); err != nil {
		t.Fatal(err)
	}
	status := gittest.Git(t, wtPath, "status", "--porcelain")
	if !strings.Contains(status, "untracked.txt") || strings.Contains(status, "README.md") {
		t.Errorf("after --hard: tracked change gone, untracked kept; got:\n%s", status)
	}
}

func TestResetWithoutRecordedBase(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	setupOutputs(t)
	raw := filepath.Join(t.TempDir(), "raw")
	gittest.Git(t, repo, "worktree", "add", "-b", "rawbranch", raw)

	err := reset(raw, nil)
	if err == nil || !strings.Contains(err.Error(), "no base branch recorded") {
		t.Fatalf("err = %v", err)
	}

	if err := reset(raw, []string{"main"}); err != nil {
		t.Fatal(err)
	}
	mainSHA := gittest.Git(t, repo, "rev-parse", "main")
	if got := gittest.Git(t, raw, "rev-parse", "HEAD"); got != mainSHA {
		t.Errorf("explicit-base reset HEAD = %s, want %s", got, mainSHA)
	}
}

func TestResetUnknownBase(t *testing.T) {
	_, wtPath := resetFixture(t)

	err := reset(wtPath, []string{"ghost"})
	if err == nil || !strings.Contains(err.Error(), "does not resolve") {
		t.Fatalf("err = %v", err)
	}
}
