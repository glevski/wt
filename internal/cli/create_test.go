package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/gittest"
)

func TestCreateFromLocalBranch(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	out, errOut := setupOutputs(t)
	gittest.Git(t, repo, "branch", "feature/auth")

	if err := create(repo, []string{"feature/auth"}); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout should stay empty without -c, got %q", out.String())
	}

	path := worktreePath(root, "proj", "feature-auth")
	if got := gittest.Git(t, path, "symbolic-ref", "--short", "HEAD"); got != "feature/auth" {
		t.Errorf("worktree branch = %q, want feature/auth", got)
	}
	log := errOut.String()
	if !strings.Contains(log, "found locally") || !strings.Contains(log, "(no upstream)") {
		t.Errorf("log missing source/upstream info:\n%s", log)
	}
	if !strings.Contains(log, path) {
		t.Errorf("log does not mention worktree path %s:\n%s", path, log)
	}
}

func TestCreateLogsDivergence(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	_, errOut := setupOutputs(t)
	remote := gittest.AddRemote(t, repo)

	gittest.Git(t, repo, "checkout", "-b", "feature")
	gittest.WriteFile(t, repo, "f.txt", "1")
	gittest.Commit(t, repo, "c1")
	gittest.Git(t, repo, "push", "-u", "origin", "feature")

	gittest.WriteFile(t, repo, "f.txt", "2")
	gittest.Commit(t, repo, "ahead")

	clone := filepath.Join(t.TempDir(), "clone")
	gittest.Git(t, filepath.Dir(clone), "clone", remote, clone)
	gittest.Git(t, clone, "checkout", "feature")
	gittest.WriteFile(t, clone, "g.txt", "3")
	gittest.Commit(t, clone, "behind")
	gittest.Git(t, clone, "push", "origin", "feature")

	gittest.Git(t, repo, "fetch", "origin")
	gittest.Git(t, repo, "checkout", "main")

	if err := create(repo, []string{"feature"}); err != nil {
		t.Fatal(err)
	}
	if log := errOut.String(); !strings.Contains(log, "ahead 1, behind 1") {
		t.Errorf("log missing divergence:\n%s", log)
	}
	if _, err := os.Stat(worktreePath(root, "proj", "feature")); err != nil {
		t.Error("worktree directory missing")
	}
}

func TestCreateFromRemoteOnlyBranch(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	_, errOut := setupOutputs(t)
	gittest.AddRemote(t, repo)
	gittest.Git(t, repo, "push", "origin", "main:hotfix")

	if err := create(repo, []string{"hotfix"}); err != nil {
		t.Fatal(err)
	}

	path := worktreePath(root, "proj", "hotfix")
	if got := gittest.Git(t, path, "rev-parse", "--abbrev-ref", "hotfix@{upstream}"); got != "origin/hotfix" {
		t.Errorf("upstream = %q, want origin/hotfix", got)
	}
	if log := errOut.String(); !strings.Contains(log, "created from origin/hotfix") {
		t.Errorf("log missing remote source:\n%s", log)
	}
}

func TestCreateUnknownBranch(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	setupOutputs(t)

	err := create(repo, []string{"ghost"})
	if err == nil || !strings.Contains(err.Error(), "not found locally or on any remote") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(worktreePath(root, "proj", "ghost")); statErr == nil {
		t.Error("worktree directory created despite the error")
	}
}

func TestCreateAlreadyCheckedOut(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	setupOutputs(t)

	err := create(repo, []string{"main"})
	if err == nil || !strings.Contains(err.Error(), "already checked out") {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateNoArgs(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	setupOutputs(t)
	gittest.WriteFile(t, repo, "README.md", "dirty")
	gittest.WriteFile(t, repo, "staged.txt", "s")
	gittest.Git(t, repo, "add", "staged.txt")

	if err := create(repo, nil); err != nil {
		t.Fatal(err)
	}
	path := worktreePath(root, "proj", "main-2")
	if got := gittest.Git(t, path, "symbolic-ref", "--short", "HEAD"); got != "main-2" {
		t.Errorf("branch = %q, want main-2", got)
	}
	if status := gittest.Git(t, path, "status", "--porcelain"); status != "" {
		t.Errorf("new worktree is not clean:\n%s", status)
	}
	if status := gittest.Git(t, repo, "status", "--porcelain"); status == "" {
		t.Error("source worktree lost its local changes")
	}

	if err := create(repo, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(worktreePath(root, "proj", "main-3")); err != nil {
		t.Error("second run did not pick main-3")
	}
}

func TestCreateNoArgsDetachedHead(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	setupOutputs(t)
	gittest.Git(t, repo, "checkout", "--detach")

	err := create(repo, nil)
	if err == nil || !strings.Contains(err.Error(), "detached HEAD") {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateCheckoutFlag(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	out, errOut := setupOutputs(t)
	gittest.Git(t, repo, "branch", "feature")

	if err := create(repo, []string{"-c", "feature"}); err != nil {
		t.Fatal(err)
	}
	want := worktreePath(root, "proj", "feature") + "\n"
	if got := out.String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if strings.Contains(errOut.String(), "switch with") {
		t.Error("switch hint should be dropped when -c cd's there anyway")
	}

	if err := create(repo, []string{"--checkout"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out.String(), worktreePath(root, "proj", "main-2")+"\n") {
		t.Errorf("--checkout (no branch) stdout = %q", out.String())
	}
}

func TestCreateUnlinkedRepo(t *testing.T) {
	repo := gittest.NewRepo(t)
	root := wtRoot(t)
	setupOutputs(t)

	err := create(repo, nil)
	if err == nil || !strings.Contains(err.Error(), "not linked") {
		t.Fatalf("err = %v", err)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Error("something was created despite the unlinked error")
	}
}
