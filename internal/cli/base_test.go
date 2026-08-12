package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/git"
	"wt/internal/gittest"
)

// baseFixture: linked repo (on main) with a free "staging" branch added as a
// base worktree.
func baseFixture(t *testing.T) (repo, basePath string) {
	t.Helper()
	repo = linkedRepo(t)
	root := wtRoot(t)
	setupOutputs(t)
	gittest.Git(t, repo, "branch", "staging")
	if err := base(repo, []string{"add", "staging"}); err != nil {
		t.Fatal(err)
	}
	return repo, worktreePath(root, "proj", "staging")
}

func TestBaseAdd(t *testing.T) {
	_, basePath := baseFixture(t)

	if got := gittest.Git(t, basePath, "symbolic-ref", "--short", "HEAD"); got != "staging" {
		t.Errorf("base branch = %q, want staging", got)
	}
	if branch, ok := git.ReadBaseMark(basePath); !ok || branch != "staging" {
		t.Errorf("base mark = %q, %v; want staging", branch, ok)
	}
}

func TestBaseAddBusyBranch(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	setupOutputs(t)

	err := base(repo, []string{"add", "main"}) // checked out in the root
	if err == nil || !strings.Contains(err.Error(), "is checked out at") {
		t.Fatalf("err = %v", err)
	}
}

func TestBaseAddUnknownBranch(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	setupOutputs(t)

	err := base(repo, []string{"add", "ghost"})
	if err == nil || !strings.Contains(err.Error(), "not found locally or on any remote") {
		t.Fatalf("err = %v", err)
	}
}

func TestBaseAddRemoteOnly(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	setupOutputs(t)
	gittest.AddRemote(t, repo)
	gittest.Git(t, repo, "push", "origin", "main:releases")

	if err := base(repo, []string{"add", "releases"}); err != nil {
		t.Fatal(err)
	}
	path := worktreePath(root, "proj", "releases")
	if got := gittest.Git(t, path, "rev-parse", "--abbrev-ref", "releases@{upstream}"); got != "origin/releases" {
		t.Errorf("upstream = %q, want origin/releases", got)
	}
	if branch, ok := git.ReadBaseMark(path); !ok || branch != "releases" {
		t.Errorf("base mark = %q, %v", branch, ok)
	}
}

func TestBaseAddDuplicate(t *testing.T) {
	repo, _ := baseFixture(t)

	err := base(repo, []string{"add", "staging"})
	if err == nil || !strings.Contains(err.Error(), "already a base") {
		t.Fatalf("err = %v", err)
	}
}

func TestWtRmRefusesBases(t *testing.T) {
	repo, _ := baseFixture(t)

	for _, args := range [][]string{{"staging"}, {"-f", "staging"}} {
		err := remove(repo, args)
		if err == nil || !strings.Contains(err.Error(), "base worktree") {
			t.Fatalf("remove(%v) = %v, want base-worktree refusal", args, err)
		}
	}
}

func TestBaseRm(t *testing.T) {
	repo, basePath := baseFixture(t)

	gittest.WriteFile(t, basePath, "dirty.txt", "x")
	err := base(repo, []string{"rm", "staging"})
	if err == nil {
		t.Fatal("dirty base removed without -f")
	}
	if err := base(repo, []string{"rm", "-f", "staging"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(basePath); !os.IsNotExist(err) {
		t.Error("base worktree still exists")
	}
	if refs, _ := git.LookupBranch(repo, "staging"); !refs.Local {
		t.Error("branch was deleted — base rm must never touch it")
	}
}

func TestBaseRmFromInside(t *testing.T) {
	_, basePath := baseFixture(t)

	err := base(basePath, []string{"rm", "staging"})
	if err == nil || !strings.Contains(err.Error(), "you are inside") {
		t.Fatalf("err = %v", err)
	}
}

func TestResetRefusesBase(t *testing.T) {
	_, basePath := baseFixture(t)

	err := reset(basePath, nil)
	if err == nil || !strings.Contains(err.Error(), "base worktree") {
		t.Fatalf("err = %v", err)
	}
}

func TestListShowsDrift(t *testing.T) {
	repo, basePath := baseFixture(t)
	gittest.Git(t, basePath, "checkout", "-b", "sneaky")
	out, _ := setupOutputs(t)

	if err := list(repo, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "sneaky!") {
		t.Errorf("drifted base not flagged:\n%s", out.String())
	}
}

func TestStatusBaseLabels(t *testing.T) {
	repo, basePath := baseFixture(t)
	out, _ := setupOutputs(t)

	if err := status(basePath, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "staging (base)") {
		t.Errorf("status missing (base) label:\n%s", out.String())
	}

	gittest.Git(t, basePath, "checkout", "-b", "sneaky")
	out.Reset()
	if err := status(repo, []string{"staging"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "(base: staging, drifted)") {
		t.Errorf("status missing drift label:\n%s", out.String())
	}
}

func TestBaseListFiltersToBases(t *testing.T) {
	repo, _ := baseFixture(t)
	side := filepath.Join(t.TempDir(), "plain-side")
	gittest.Git(t, repo, "worktree", "add", "-b", "plainbranch", side)
	out, errOut := setupOutputs(t)

	if err := base(repo, []string{"list"}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "staging") {
		t.Errorf("base list missing the base:\n%s", got)
	}
	if strings.Contains(got, "plain-side") || strings.Contains(got, filepath.Base(repo)) {
		t.Errorf("base list shows non-bases:\n%s", got)
	}

	// with no bases at all: a hint, not an empty table
	repo2 := linkedRepo(t)
	out.Reset()
	errOut.Reset()
	if err := base(repo2, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "no base branches") {
		t.Errorf("missing empty-state hint:\n%s", errOut.String())
	}
}

func TestBaseUpdateFastForwards(t *testing.T) {
	repo, basePath := baseFixture(t)
	_, errOut := setupOutputs(t)
	remote := gittest.AddRemote(t, repo)
	gittest.Git(t, repo, "push", "-u", "origin", "staging")

	clone := filepath.Join(t.TempDir(), "clone")
	gittest.Git(t, filepath.Dir(clone), "clone", remote, clone)
	gittest.Git(t, clone, "checkout", "staging")
	gittest.WriteFile(t, clone, "hot.txt", "x")
	gittest.Commit(t, clone, "remote work")
	gittest.Git(t, clone, "push", "origin", "staging")
	remoteTip := gittest.Git(t, clone, "rev-parse", "HEAD")

	if err := base(repo, []string{"update"}); err != nil {
		t.Fatal(err)
	}
	if got := gittest.Git(t, basePath, "rev-parse", "HEAD"); got != remoteTip {
		t.Errorf("base HEAD = %s, want remote tip %s", got, remoteTip)
	}
	if !strings.Contains(errOut.String(), "staging: updated to") {
		t.Errorf("log missing update line:\n%s", errOut.String())
	}
}

func TestBaseUpdateSkips(t *testing.T) {
	repo, basePath := baseFixture(t)
	_, errOut := setupOutputs(t)

	// no upstream configured → noted, nothing else happens
	if err := base(repo, []string{"update"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "staging: skipped (no upstream)") {
		t.Errorf("log missing no-upstream skip:\n%s", errOut.String())
	}

	// dirty base → skipped even with an upstream
	gittest.AddRemote(t, repo)
	gittest.Git(t, repo, "push", "-u", "origin", "staging")
	gittest.WriteFile(t, basePath, "wip.txt", "x")
	errOut.Reset()
	if err := base(repo, []string{"update"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "staging: skipped (dirty)") {
		t.Errorf("log missing dirty skip:\n%s", errOut.String())
	}
}
