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

// baseAddEnvFixture: a repo with an ignored .env and a declared node_modules
// dep, plus a "staging" branch ready to become a base.
func baseAddEnvFixture(t *testing.T) (repo, root string) {
	t.Helper()
	repo, root = depsFixture(t)
	gittest.Git(t, repo, "branch", "staging")
	return repo, root
}

func TestBaseAddCopiesIgnoredButNotDeps(t *testing.T) {
	repo, root := baseAddEnvFixture(t)
	setupOutputs(t)

	if err := base(repo, []string{"add", "staging"}); err != nil {
		t.Fatal(err)
	}
	path := worktreePath(root, "proj", "staging")
	if got, err := os.ReadFile(filepath.Join(path, ".env")); err != nil || string(got) != "SECRET=1" {
		t.Errorf(".env not copied into the base: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(path, "node_modules")); !os.IsNotExist(err) {
		t.Error("declared dep copied into the base by default")
	}
	if state, ok := git.DepsState(path); ok && strings.HasPrefix(state, "copying") {
		t.Errorf("deps worker started for a base: state = %q", state)
	}
}

func TestBaseAddNoIgnored(t *testing.T) {
	repo, root := baseAddEnvFixture(t)
	setupOutputs(t)

	if err := base(repo, []string{"add", "--no-ignored", "staging"}); err != nil {
		t.Fatal(err)
	}
	path := worktreePath(root, "proj", "staging")
	if _, err := os.Stat(filepath.Join(path, ".env")); !os.IsNotExist(err) {
		t.Error(".env copied despite --no-ignored")
	}
}

func TestBaseAddWithDeps(t *testing.T) {
	repo, root := baseAddEnvFixture(t)
	setupOutputs(t)

	if err := base(repo, []string{"add", "--deps", "-w", "staging"}); err != nil {
		t.Fatal(err)
	}
	path := worktreePath(root, "proj", "staging")
	if _, err := os.Stat(filepath.Join(path, "node_modules/dep/index.js")); err != nil {
		t.Error("--deps -w did not copy the dep")
	}
	if state, _ := git.DepsState(path); state != "done" {
		t.Errorf("deps state = %q, want done", state)
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

func TestBaseResetToUpstream(t *testing.T) {
	repo, basePath := baseFixture(t)
	_, errOut := setupOutputs(t)
	gittest.AddRemote(t, repo)
	gittest.Git(t, repo, "push", "-u", "origin", "staging")
	upstreamTip := gittest.Git(t, repo, "rev-parse", "origin/staging")

	// the base moved ahead locally (an accidental commit); tree stays clean
	gittest.WriteFile(t, basePath, "oops.txt", "committed by accident")
	gittest.Commit(t, basePath, "accidental commit")

	if err := base(repo, []string{"reset", "staging"}); err != nil {
		t.Fatal(err)
	}
	if got := gittest.Git(t, basePath, "rev-parse", "HEAD"); got != upstreamTip {
		t.Errorf("base HEAD = %s, want upstream tip %s", got, upstreamTip)
	}
	if !strings.Contains(errOut.String(), "reset base 'staging' to origin/staging") {
		t.Errorf("log missing reset line:\n%s", errOut.String())
	}
}

func TestBaseResetBlocksOnChanges(t *testing.T) {
	repo, basePath := baseFixture(t)
	setupOutputs(t)
	gittest.AddRemote(t, repo)
	gittest.Git(t, repo, "push", "-u", "origin", "staging")
	gittest.WriteFile(t, basePath, "untracked.txt", "u")

	err := base(repo, []string{"reset", "staging"})
	if err == nil || !strings.Contains(err.Error(), "local changes") {
		t.Fatalf("err = %v", err)
	}

	// --hard, invoked from inside the base with no name argument
	gittest.WriteFile(t, basePath, "README.md", "tracked change")
	if err := base(basePath, []string{"reset", "--hard"}); err != nil {
		t.Fatal(err)
	}
	status := gittest.Git(t, basePath, "status", "--porcelain")
	if !strings.Contains(status, "untracked.txt") || strings.Contains(status, "README.md") {
		t.Errorf("after --hard: tracked change gone, untracked kept; got:\n%s", status)
	}
}

func TestBaseResetGuards(t *testing.T) {
	repo, _ := baseFixture(t)
	setupOutputs(t)

	if err := base(repo, []string{"reset", "staging"}); err == nil || !strings.Contains(err.Error(), "no upstream") {
		t.Fatalf("no-upstream err = %v", err)
	}
	if err := base(repo, []string{"reset"}); err == nil || !strings.Contains(err.Error(), "not inside a base") {
		t.Fatalf("outside-base err = %v", err)
	}
}

func TestBaseResetRecoversDrift(t *testing.T) {
	repo, basePath := baseFixture(t)
	_, errOut := setupOutputs(t)
	gittest.AddRemote(t, repo)
	gittest.Git(t, repo, "push", "-u", "origin", "staging")
	upstreamTip := gittest.Git(t, repo, "rev-parse", "origin/staging")
	gittest.Git(t, basePath, "checkout", "-b", "sneaky")

	if err := base(repo, []string{"reset", "staging"}); err != nil {
		t.Fatal(err)
	}
	if got := gittest.Git(t, basePath, "symbolic-ref", "--short", "HEAD"); got != "staging" {
		t.Errorf("base branch = %q, want staging back", got)
	}
	if got := gittest.Git(t, basePath, "rev-parse", "HEAD"); got != upstreamTip {
		t.Errorf("base HEAD = %s, want upstream tip %s", got, upstreamTip)
	}
	if refs, _ := git.LookupBranch(repo, "sneaky"); !refs.Local {
		t.Error("the drifted-to branch should survive as a branch")
	}
	if !strings.Contains(errOut.String(), "had drifted to branch 'sneaky'") {
		t.Errorf("log missing drift recovery:\n%s", errOut.String())
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
