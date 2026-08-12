package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/git"
	"wt/internal/gittest"
	"wt/internal/tui"
)

func checkoutFixture(t *testing.T) (repo, authPath, fixPath string) {
	t.Helper()
	repo = gittest.NewRepo(t)
	wtRoot(t)
	base := t.TempDir()
	authPath = filepath.Join(base, "feature-auth")
	fixPath = filepath.Join(base, "feature-fix")
	gittest.Git(t, repo, "worktree", "add", "-b", "feature/auth", authPath)
	gittest.Git(t, repo, "worktree", "add", "-b", "feature/fix", fixPath)
	return repo, authPath, fixPath
}

func TestCheckoutExactMatch(t *testing.T) {
	repo, authPath, _ := checkoutFixture(t)
	out, _ := setupOutputs(t)

	if err := checkout(repo, []string{"feature-auth"}); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), jumpScript(authPath, repo); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestCheckoutUniquePrefix(t *testing.T) {
	repo, _, fixPath := checkoutFixture(t)
	out, _ := setupOutputs(t)

	if err := checkout(repo, []string{"feature-f"}); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), jumpScript(fixPath, repo); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestCheckoutAmbiguousPrefix(t *testing.T) {
	repo, _, _ := checkoutFixture(t)
	out, _ := setupOutputs(t)

	err := checkout(repo, []string{"feature"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("err = %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout not empty on error: %q", out.String())
	}
}

func TestCheckoutUnknown(t *testing.T) {
	repo, _, _ := checkoutFixture(t)
	out, _ := setupOutputs(t)

	err := checkout(repo, []string{"zzz"})
	if err == nil || !strings.Contains(err.Error(), "no worktree named") {
		t.Fatalf("err = %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout not empty on error: %q", out.String())
	}
}

func TestCheckoutDashB(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	out, _ := setupOutputs(t)
	gittest.WriteFile(t, repo, "wip.txt", "x")

	if err := checkout(repo, []string{"-b", "hotfix"}); err != nil {
		t.Fatal(err)
	}
	path := worktreePath(root, "proj", "hotfix")
	if got, want := out.String(), jumpScript(path, repo); got != want {
		t.Errorf("stdout = %q, want %q (must cd like git checkout -b)", got, want)
	}
	if got := gittest.Git(t, path, "symbolic-ref", "--short", "HEAD"); got != "hotfix" {
		t.Errorf("branch = %q, want hotfix", got)
	}
	if got := gittest.Git(t, path, "ls-files", "--others", "--exclude-standard"); got != "wip.txt" {
		t.Errorf("untracked in fork = %q, want wip.txt", got)
	}
}

func TestCheckoutDashBExistingBranchBasesOffIt(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	out, _ := setupOutputs(t)
	gittest.Git(t, repo, "branch", "taken")

	if err := checkout(repo, []string{"-b", "taken"}); err != nil {
		t.Fatal(err)
	}
	path := worktreePath(root, "proj", "taken-2")
	if got, want := out.String(), jumpScript(path, repo); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got := gittest.Git(t, path, "symbolic-ref", "--short", "HEAD"); got != "taken-2" {
		t.Errorf("branch = %q, want auto-named taken-2", got)
	}
}

func TestCheckoutMainWorktreeByRepoName(t *testing.T) {
	repo, _, _ := checkoutFixture(t)
	out, _ := setupOutputs(t)

	if err := checkout(repo, []string{filepath.Base(repo)}); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), jumpScript(repo, repo); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

// swapPick replaces the interactive picker for one test.
func swapPick(t *testing.T, fake func(title string, items []string) (int, error)) {
	t.Helper()
	orig := pick
	pick = fake
	t.Cleanup(func() { pick = orig })
}

func TestCheckoutInteractive(t *testing.T) {
	repo, authPath, fixPath := checkoutFixture(t)
	git.TouchCheckoutStamp(fixPath) // stamped → most recent, ahead of created-only auth
	out, _ := setupOutputs(t)

	var seen []string
	swapPick(t, func(title string, items []string) (int, error) {
		seen = items
		return 1, nil
	})

	if err := checkout(repo, nil); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 {
		t.Fatalf("picker got %d items, want 2 (current worktree excluded): %v", len(seen), seen)
	}
	if !strings.HasPrefix(seen[0], "feature-fix") || !strings.Contains(seen[0], "feature/fix") {
		t.Errorf("first item = %q, want stamped feature-fix with its branch", seen[0])
	}
	if got, want := out.String(), jumpScript(authPath, repo); got != want {
		t.Errorf("stdout = %q, want %q (picked index 1)", got, want)
	}
}

func TestCheckoutInteractiveCancel(t *testing.T) {
	repo, _, _ := checkoutFixture(t)
	out, _ := setupOutputs(t)
	swapPick(t, func(string, []string) (int, error) { return 0, tui.ErrCanceled })

	if err := checkout(repo, nil); err == nil {
		t.Fatal("expected an error on cancel")
	}
	if out.Len() != 0 {
		t.Errorf("stdout not empty on cancel: %q", out.String())
	}
}

func TestCheckoutInteractiveNoTTY(t *testing.T) {
	repo, _, _ := checkoutFixture(t)
	out, _ := setupOutputs(t)
	swapPick(t, func(string, []string) (int, error) { return 0, tui.ErrNoTTY })

	err := checkout(repo, nil)
	if err == nil || !strings.Contains(err.Error(), "needs a terminal") {
		t.Fatalf("err = %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout not empty: %q", out.String())
	}
}

func TestCheckoutInteractiveNoOtherWorktree(t *testing.T) {
	repo := gittest.NewRepo(t)
	wtRoot(t)
	setupOutputs(t)
	swapPick(t, func(string, []string) (int, error) {
		t.Fatal("picker must not open with nothing to pick")
		return 0, nil
	})

	err := checkout(repo, nil)
	if err == nil || !strings.Contains(err.Error(), "no other worktree") {
		t.Fatalf("err = %v", err)
	}
}
