package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/git"
	"wt/internal/gittest"
)

func removeFixture(t *testing.T) (repo, side string) {
	t.Helper()
	repo = gittest.NewRepo(t)
	wtRoot(t)
	side = filepath.Join(t.TempDir(), "side")
	gittest.Git(t, repo, "worktree", "add", "-b", "side", side)
	return repo, side
}

func TestRemoveKeepsBranchByDefault(t *testing.T) {
	repo, side := removeFixture(t)
	_, errOut := setupOutputs(t)

	if err := remove(repo, []string{"side"}); err != nil {
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

func TestRemoveDeletesBranchWithFlag(t *testing.T) {
	repo, _ := removeFixture(t)
	setupOutputs(t)

	if err := remove(repo, []string{"-b", "side"}); err != nil {
		t.Fatal(err)
	}
	if refs, _ := git.LookupBranch(repo, "side"); refs.Local {
		t.Error("branch still exists despite -b")
	}
}

func TestRemoveUnmergedBranchIsKept(t *testing.T) {
	repo, side := removeFixture(t)
	setupOutputs(t)
	gittest.WriteFile(t, side, "extra.txt", "x")
	gittest.Commit(t, side, "unmerged work")

	err := remove(repo, []string{"-b", "side"})
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

func TestRemoveDirtyWorktree(t *testing.T) {
	repo, side := removeFixture(t)
	setupOutputs(t)
	gittest.WriteFile(t, side, "dirty.txt", "x")

	if err := remove(repo, []string{"side"}); err == nil {
		t.Fatal("expected an error for a dirty worktree")
	}
	if _, err := os.Stat(side); err != nil {
		t.Fatal("dirty worktree was removed without -f")
	}
	if err := remove(repo, []string{"-f", "side"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(side); !os.IsNotExist(err) {
		t.Error("worktree survived remove -f")
	}
}

func TestRemoveMainWorktree(t *testing.T) {
	repo, _ := removeFixture(t)
	setupOutputs(t)

	err := remove(repo, []string{filepath.Base(repo)})
	if err == nil || !strings.Contains(err.Error(), "main worktree") {
		t.Fatalf("err = %v", err)
	}
}

func TestRemoveFromInsideTheWorktree(t *testing.T) {
	_, side := removeFixture(t)
	setupOutputs(t)

	err := remove(side, []string{"side"})
	if err == nil || !strings.Contains(err.Error(), "you are inside") {
		t.Fatalf("err = %v", err)
	}
}

// wildcardFixture: linked repo with managed worktrees main-2/main-3, a base
// 'staging', and an external side worktree outside the workspace root.
func wildcardFixture(t *testing.T) (repo string, root string, external string) {
	t.Helper()
	repo = linkedRepo(t)
	root = wtRoot(t)
	setupOutputs(t)
	if err := create(repo, nil); err != nil { // main-2
		t.Fatal(err)
	}
	if err := create(repo, nil); err != nil { // main-3
		t.Fatal(err)
	}
	gittest.Git(t, repo, "branch", "staging")
	if err := base(repo, []string{"add", "staging"}); err != nil {
		t.Fatal(err)
	}
	external = filepath.Join(t.TempDir(), "main-external")
	gittest.Git(t, repo, "worktree", "add", "-b", "external", external)
	return repo, root, external
}

func TestRemoveWildcard(t *testing.T) {
	repo, root, _ := wildcardFixture(t)
	_, errOut := setupOutputs(t)
	stdinInput(t, "y\n")

	if err := remove(repo, []string{"main-*"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"main-2", "main-3"} {
		if _, err := os.Stat(worktreePath(root, "proj", name)); !os.IsNotExist(err) {
			t.Errorf("worktree %s still exists", name)
		}
	}
	if refs, _ := git.LookupBranch(repo, "main-2"); !refs.Local {
		t.Error("branch main-2 deleted without -b")
	}
	log := errOut.String()
	if !strings.Contains(log, "remove 2 worktree(s)? [y/N]") || !strings.Contains(log, "main-2") {
		t.Errorf("prompt/listing missing:\n%s", log)
	}
}

func TestRemoveWildcardStarProtectsBasesAndExternal(t *testing.T) {
	repo, root, external := wildcardFixture(t)
	_, errOut := setupOutputs(t)
	stdinInput(t, "y\n")

	if err := remove(repo, []string{"*"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(worktreePath(root, "proj", "staging")); err != nil {
		t.Error("base worktree was removed by '*'")
	}
	if _, err := os.Stat(external); err != nil {
		t.Error("external worktree was removed by '*'")
	}
	if _, err := os.Stat(worktreePath(root, "proj", "main-2")); !os.IsNotExist(err) {
		t.Error("managed worktree survived '*'")
	}
	if !strings.Contains(errOut.String(), "skipping base worktree 'staging'") {
		t.Errorf("missing base skip note:\n%s", errOut.String())
	}
}

func TestRemoveWildcardRequiresExactYes(t *testing.T) {
	repo, root, _ := wildcardFixture(t)
	for _, answer := range []string{"n\n", "yes\n", "\n"} {
		setupOutputs(t)
		stdinInput(t, answer)
		err := remove(repo, []string{"main-*"})
		if err == nil || !strings.Contains(err.Error(), "aborted") {
			t.Fatalf("answer %q: err = %v, want aborted", answer, err)
		}
	}
	if _, err := os.Stat(worktreePath(root, "proj", "main-2")); err != nil {
		t.Error("worktree removed despite aborted confirmation")
	}
}

func TestRemoveWildcardWithFlags(t *testing.T) {
	repo, root, _ := wildcardFixture(t)
	setupOutputs(t)
	stdinInput(t, "y\n")
	gittest.WriteFile(t, worktreePath(root, "proj", "main-2"), "dirty.txt", "x")

	if err := remove(repo, []string{"-f", "-b", "main-*"}); err != nil {
		t.Fatal(err)
	}
	if refs, _ := git.LookupBranch(repo, "main-2"); refs.Local {
		t.Error("branch main-2 survived -b")
	}
	if refs, _ := git.LookupBranch(repo, "main-3"); refs.Local {
		t.Error("branch main-3 survived -b")
	}
}

func TestRemoveWildcardNoMatches(t *testing.T) {
	repo, _, _ := wildcardFixture(t)
	setupOutputs(t)

	err := remove(repo, []string{"zzz-*"})
	if err == nil || !strings.Contains(err.Error(), "no removable worktrees match") {
		t.Fatalf("err = %v", err)
	}
}
