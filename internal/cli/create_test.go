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

func TestCreateCheckedOutBranchForksFromItsTip(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	_, errOut := setupOutputs(t)
	mainSHA := gittest.Git(t, repo, "rev-parse", "HEAD")

	// invoke from a linked worktree whose HEAD has moved past main
	other := filepath.Join(t.TempDir(), "other")
	gittest.Git(t, repo, "worktree", "add", "-b", "other", other)
	gittest.WriteFile(t, other, "o.txt", "x")
	gittest.Commit(t, other, "diverge")

	if err := create(other, []string{"main"}); err != nil {
		t.Fatal(err)
	}
	path := worktreePath(root, "proj", "main-2")
	if got := gittest.Git(t, path, "symbolic-ref", "--short", "HEAD"); got != "main-2" {
		t.Errorf("branch = %q, want main-2", got)
	}
	if got := gittest.Git(t, path, "rev-parse", "HEAD"); got != mainSHA {
		t.Errorf("new branch starts at %s, want main's tip %s", got, mainSHA)
	}
	log := errOut.String()
	if !strings.Contains(log, "already checked out") || !strings.Contains(log, "created new branch 'main-2' from 'main'") {
		t.Errorf("log missing fallback explanation:\n%s", log)
	}

	// -n still applies in the fallback
	if err := create(other, []string{"-n", "-hot", "main"}); err != nil {
		t.Fatal(err)
	}
	if got := gittest.Git(t, worktreePath(root, "proj", "main-hot"), "symbolic-ref", "--short", "HEAD"); got != "main-3" {
		t.Errorf("named fallback branch = %q, want main-3", got)
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
	want := jumpScript(worktreePath(root, "proj", "feature"), repo)
	if got := out.String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if strings.Contains(errOut.String(), "switch with") {
		t.Error("switch hint should be dropped when -c cd's there anyway")
	}

	if err := create(repo, []string{"--checkout"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out.String(), jumpScript(worktreePath(root, "proj", "main-2"), repo)) {
		t.Errorf("--checkout (no branch) stdout = %q", out.String())
	}
}

func TestCreateNamedWorktree(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	setupOutputs(t)
	gittest.Git(t, repo, "branch", "feature/auth")

	if err := create(repo, []string{"-n", "authwork", "feature/auth"}); err != nil {
		t.Fatal(err)
	}
	path := worktreePath(root, "proj", "authwork")
	if got := gittest.Git(t, path, "symbolic-ref", "--short", "HEAD"); got != "feature/auth" {
		t.Errorf("branch = %q, want feature/auth", got)
	}
}

func TestCreateNameDashShorthand(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	setupOutputs(t)
	gittest.Git(t, repo, "branch", "feature/auth")

	// -n -fix on branch feature/auth → directory feature-auth-fix
	if err := create(repo, []string{"-n", "-fix", "feature/auth"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(worktreePath(root, "proj", "feature-auth-fix")); err != nil {
		t.Error("dash shorthand did not expand to <branch>-fix")
	}

	// no positional: expands against the current branch, branch auto-named
	if err := create(repo, []string{"-n", "-exp"}); err != nil {
		t.Fatal(err)
	}
	path := worktreePath(root, "proj", "main-exp")
	if got := gittest.Git(t, path, "symbolic-ref", "--short", "HEAD"); got != "main-2" {
		t.Errorf("branch = %q, want auto-named main-2", got)
	}
}

func TestCreateNameUnsafe(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	setupOutputs(t)
	gittest.Git(t, repo, "branch", "feature")

	err := create(repo, []string{"-n", "bad name", "feature"})
	if err == nil || !strings.Contains(err.Error(), "not filesystem-safe") {
		t.Fatalf("err = %v", err)
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
