package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/git"
	"wt/internal/gittest"
)

// rootFixture: linked repo plus a side worktree; commands run FROM side to
// prove the root context wins.
func rootFixture(t *testing.T) (repo, side, wtRootDir string) {
	t.Helper()
	repo = linkedRepo(t)
	wtRootDir = wtRoot(t)
	side = filepath.Join(t.TempDir(), "side")
	gittest.Git(t, repo, "worktree", "add", "-b", "side", side)
	return repo, side, wtRootDir
}

func TestRootStatus(t *testing.T) {
	repo, side, _ := rootFixture(t)
	gittest.WriteFile(t, repo, "root-only.txt", "dirty in the root")
	out, _ := setupOutputs(t)

	if err := root(side, []string{"status", "-g"}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "worktree  "+filepath.Base(repo)+" (home)") {
		t.Errorf("status not about the root checkout:\n%s", got)
	}
	if !strings.Contains(got, "root-only.txt") {
		t.Errorf("git status part not run in the root:\n%s", got)
	}
}

func TestRootCheckoutJumpsHome(t *testing.T) {
	repo, side, _ := rootFixture(t)
	out, _ := setupOutputs(t)

	if err := root(side, []string{"checkout"}); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), jumpScript(repo, repo); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRootCheckoutStillResolvesNames(t *testing.T) {
	repo, side, _ := rootFixture(t)
	out, _ := setupOutputs(t)

	if err := root(side, []string{"checkout", "side"}); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), jumpScript(side, repo); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRootCreateUsesRootBranch(t *testing.T) {
	_, side, wtRootDir := rootFixture(t)
	setupOutputs(t)

	// invoked from the side worktree (branch "side"), yet based on root's main
	if err := root(side, []string{"create"}); err != nil {
		t.Fatal(err)
	}
	path := worktreePath(wtRootDir, "proj", "main-2")
	if got := gittest.Git(t, path, "symbolic-ref", "--short", "HEAD"); got != "main-2" {
		t.Errorf("branch = %q, want main-2 (off the root's branch, not side)", got)
	}
	if base, ok := git.BaseBranch(path); !ok || base != "main" {
		t.Errorf("recorded base = %q, %v; want main", base, ok)
	}
}

func TestRootForkCarriesRootState(t *testing.T) {
	repo, side, wtRootDir := rootFixture(t)
	setupOutputs(t)
	gittest.WriteFile(t, repo, "root-wip.txt", "root's untracked work")

	if err := root(side, []string{"fork"}); err != nil {
		t.Fatal(err)
	}
	path := worktreePath(wtRootDir, "proj", "main-2")
	if _, err := os.Stat(filepath.Join(path, "root-wip.txt")); err != nil {
		t.Error("fork did not carry the root's untracked file")
	}
	if got := gittest.Git(t, path, "symbolic-ref", "--short", "HEAD"); got != "main-2" {
		t.Errorf("branch = %q, want main-2", got)
	}
}

func TestRootUsageErrors(t *testing.T) {
	repo, _, _ := rootFixture(t)
	setupOutputs(t)

	if err := root(repo, nil); err == nil || !strings.Contains(err.Error(), "usage: wt root") {
		t.Fatalf("no-arg err = %v", err)
	}
	if err := root(repo, []string{"list"}); err == nil || !strings.Contains(err.Error(), "unknown root command") {
		t.Fatalf("unknown subcommand err = %v", err)
	}
}
