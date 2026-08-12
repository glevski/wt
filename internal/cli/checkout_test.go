package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/gittest"
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
	if got := out.String(); got != authPath+"\n" {
		t.Errorf("stdout = %q, want %q", got, authPath+"\n")
	}
}

func TestCheckoutUniquePrefix(t *testing.T) {
	repo, _, fixPath := checkoutFixture(t)
	out, _ := setupOutputs(t)

	if err := checkout(repo, []string{"feature-f"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != fixPath+"\n" {
		t.Errorf("stdout = %q, want %q", got, fixPath+"\n")
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
	if got := out.String(); got != path+"\n" {
		t.Errorf("stdout = %q, want %q (must cd like git checkout -b)", got, path+"\n")
	}
	if got := gittest.Git(t, path, "symbolic-ref", "--short", "HEAD"); got != "hotfix" {
		t.Errorf("branch = %q, want hotfix", got)
	}
	if got := gittest.Git(t, path, "ls-files", "--others", "--exclude-standard"); got != "wip.txt" {
		t.Errorf("untracked in fork = %q, want wip.txt", got)
	}
}

func TestCheckoutDashBExistingBranch(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	setupOutputs(t)
	gittest.Git(t, repo, "branch", "taken")

	err := checkout(repo, []string{"-b", "taken"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckoutMainWorktreeByRepoName(t *testing.T) {
	repo, _, _ := checkoutFixture(t)
	out, _ := setupOutputs(t)

	if err := checkout(repo, []string{filepath.Base(repo)}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != repo+"\n" {
		t.Errorf("stdout = %q, want %q", got, repo+"\n")
	}
}
