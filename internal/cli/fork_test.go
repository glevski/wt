package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/gittest"
)

func TestForkCarriesFullState(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	_, errOut := setupOutputs(t)

	gittest.WriteFile(t, repo, "staged.txt", "base")
	gittest.WriteFile(t, repo, "unstaged.txt", "base")
	gittest.Commit(t, repo, "baseline")

	gittest.WriteFile(t, repo, "staged.txt", "changed")
	gittest.Git(t, repo, "add", "staged.txt")
	gittest.WriteFile(t, repo, "unstaged.txt", "changed")
	gittest.WriteFile(t, repo, "newdir/untracked.txt", "new")
	gittest.WriteFile(t, repo, "script.sh", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(repo, "script.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("README.md", filepath.Join(repo, "link")); err != nil {
		t.Fatal(err)
	}
	statusBefore := gittest.Git(t, repo, "status", "--porcelain")

	if err := fork(repo, nil); err != nil {
		t.Fatal(err)
	}

	path := worktreePath(root, "proj", "main-2")
	if got := gittest.Git(t, path, "diff", "--cached", "--name-only"); got != "staged.txt" {
		t.Errorf("staged in fork = %q, want staged.txt", got)
	}
	if got := gittest.Git(t, path, "diff", "--name-only"); got != "unstaged.txt" {
		t.Errorf("unstaged in fork = %q, want unstaged.txt", got)
	}
	content, err := os.ReadFile(filepath.Join(path, "newdir/untracked.txt"))
	if err != nil || string(content) != "new" {
		t.Errorf("untracked file in fork: %v, content %q", err, content)
	}
	if info, err := os.Stat(filepath.Join(path, "script.sh")); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Errorf("script.sh in fork lost exec bit: %v, %v", info, err)
	}
	if target, err := os.Readlink(filepath.Join(path, "link")); err != nil || target != "README.md" {
		t.Errorf("symlink in fork = %q, %v", target, err)
	}
	if statusAfter := gittest.Git(t, repo, "status", "--porcelain"); statusAfter != statusBefore {
		t.Errorf("source worktree changed:\nbefore:\n%s\nafter:\n%s", statusBefore, statusAfter)
	}
	if log := errOut.String(); !strings.Contains(log, "carried over: 1 staged, 1 unstaged, 3 untracked") {
		t.Errorf("log missing carry summary:\n%s", log)
	}
}

func TestForkCleanTree(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	_, errOut := setupOutputs(t)

	if err := fork(repo, nil); err != nil {
		t.Fatal(err)
	}
	path := worktreePath(root, "proj", "main-2")
	if status := gittest.Git(t, path, "status", "--porcelain"); status != "" {
		t.Errorf("fork of clean tree is dirty:\n%s", status)
	}
	if log := errOut.String(); !strings.Contains(log, "carried over: 0 staged, 0 unstaged, 0 untracked") {
		t.Errorf("log missing carry summary:\n%s", log)
	}
}

func TestForkUntrackedOnly(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	setupOutputs(t)
	gittest.WriteFile(t, repo, "only.txt", "u")

	if err := fork(repo, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(worktreePath(root, "proj", "main-2"), "only.txt")); err != nil {
		t.Error("untracked-only file missing in fork")
	}
}

func TestForkExplicitName(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	setupOutputs(t)

	if err := fork(repo, []string{"experiment"}); err != nil {
		t.Fatal(err)
	}
	path := worktreePath(root, "proj", "experiment")
	if got := gittest.Git(t, path, "symbolic-ref", "--short", "HEAD"); got != "experiment" {
		t.Errorf("branch = %q, want experiment", got)
	}
}

func TestForkExistingBranchBasesOffItsTip(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	_, errOut := setupOutputs(t)
	mainSHA := gittest.Git(t, repo, "rev-parse", "HEAD")

	// move to a diverged branch, with local changes to carry
	gittest.Git(t, repo, "checkout", "-b", "dev")
	gittest.WriteFile(t, repo, "devwork.txt", "x")
	gittest.Commit(t, repo, "dev work")
	gittest.WriteFile(t, repo, "README.md", "unstaged change")
	gittest.WriteFile(t, repo, "wip.txt", "untracked")

	if err := fork(repo, []string{"main"}); err != nil {
		t.Fatal(err)
	}

	path := worktreePath(root, "proj", "main-2")
	if got := gittest.Git(t, path, "symbolic-ref", "--short", "HEAD"); got != "main-2" {
		t.Errorf("branch = %q, want auto-named main-2", got)
	}
	if got := gittest.Git(t, path, "rev-parse", "HEAD"); got != mainSHA {
		t.Errorf("fork based on %s, want main's tip %s (not dev's HEAD)", got, mainSHA)
	}
	if got := gittest.Git(t, path, "diff", "--name-only"); got != "README.md" {
		t.Errorf("unstaged changes in fork = %q, want README.md", got)
	}
	if got := gittest.Git(t, path, "ls-files", "--others", "--exclude-standard"); got != "wip.txt" {
		t.Errorf("untracked in fork = %q, want wip.txt", got)
	}
	if !strings.Contains(errOut.String(), "branch 'main' already exists; created new branch 'main-2'") {
		t.Errorf("log missing base explanation:\n%s", errOut.String())
	}
}

func TestForkCheckoutFlag(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	out, _ := setupOutputs(t)

	if err := fork(repo, []string{"-c", "experiment"}); err != nil {
		t.Fatal(err)
	}
	want := jumpScript(worktreePath(root, "proj", "experiment"), repo)
	if got := out.String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestForkNamedWorktree(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	setupOutputs(t)
	gittest.WriteFile(t, repo, "wip.txt", "x")

	if err := fork(repo, []string{"-n", "-wip"}); err != nil {
		t.Fatal(err)
	}
	path := worktreePath(root, "proj", "main-wip")
	if got := gittest.Git(t, path, "symbolic-ref", "--short", "HEAD"); got != "main-2" {
		t.Errorf("branch = %q, want auto-named main-2", got)
	}
	if _, err := os.Stat(filepath.Join(path, "wip.txt")); err != nil {
		t.Error("untracked file missing in named fork")
	}

	// explicit branch + explicit name
	if err := fork(repo, []string{"-n", "playground", "experiment"}); err != nil {
		t.Fatal(err)
	}
	path = worktreePath(root, "proj", "playground")
	if got := gittest.Git(t, path, "symbolic-ref", "--short", "HEAD"); got != "experiment" {
		t.Errorf("branch = %q, want experiment", got)
	}
}

func TestForkUnlinkedRepo(t *testing.T) {
	repo := gittest.NewRepo(t)
	root := wtRoot(t)
	setupOutputs(t)

	err := fork(repo, nil)
	if err == nil || !strings.Contains(err.Error(), "not linked") {
		t.Fatalf("err = %v", err)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Error("something was created despite the unlinked error")
	}
}
