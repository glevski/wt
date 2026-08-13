package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/git"
	"wt/internal/gittest"
)

// finishFixture: linked repo plus a managed "dev" worktree to finish.
func finishFixture(t *testing.T) (repo, dev string) {
	t.Helper()
	repo = linkedRepo(t)
	root := wtRoot(t)
	dev = worktreePath(root, "proj", "dev")
	gittest.Git(t, repo, "worktree", "add", "-b", "dev", dev)
	t.Setenv("WT_PREV", "")
	return repo, dev
}

func TestFinishJumpsHome(t *testing.T) {
	repo, dev := finishFixture(t)
	out, errOut := setupOutputs(t)

	if err := finish(dev, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), jumpScript(repo, repo); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if _, err := os.Stat(dev); err != nil {
		t.Error("worktree deleted without -d")
	}
	if !strings.Contains(errOut.String(), "finished 'dev'") {
		t.Errorf("log missing:\n%s", errOut.String())
	}
}

func TestFinishJumpsToPrev(t *testing.T) {
	_, dev := finishFixture(t)
	prev := t.TempDir()
	t.Setenv("WT_PREV", prev)
	out, _ := setupOutputs(t)

	if err := finish(dev, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), fmt.Sprintf("cd '%s'\n", prev); got != want {
		t.Errorf("stdout = %q, want %q (no WT_HOME outside a repo)", got, want)
	}
}

func TestFinishDeleteFallsBackHomeWhenPrevInside(t *testing.T) {
	repo, dev := finishFixture(t)
	sub := filepath.Join(dev, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WT_PREV", sub)
	out, errOut := setupOutputs(t)

	if err := finish(dev, []string{"-d"}); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), jumpScript(repo, repo); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if _, err := os.Stat(dev); !os.IsNotExist(err) {
		t.Error("worktree still exists after -d")
	}
	if _, err := git.Run(repo, "rev-parse", "--verify", "--quiet", "dev"); err != nil {
		t.Error("branch deleted without -b")
	}
	if !strings.Contains(errOut.String(), "branch 'dev' kept") {
		t.Errorf("log missing the kept-branch note:\n%s", errOut.String())
	}
}

func TestFinishDeleteBranch(t *testing.T) {
	repo, dev := finishFixture(t)
	setupOutputs(t)

	if err := finish(dev, []string{"--delete", "--branch"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dev); !os.IsNotExist(err) {
		t.Error("worktree still exists")
	}
	if out, _ := git.Run(repo, "rev-parse", "--verify", "--quiet", "dev"); out != "" {
		t.Error("branch still exists after -d -b")
	}
}

func TestFinishDeleteDirtyNeedsForce(t *testing.T) {
	_, dev := finishFixture(t)
	gittest.WriteFile(t, dev, "wip.txt", "x")
	out, _ := setupOutputs(t)

	err := finish(dev, []string{"-d"})
	if err == nil || !strings.Contains(errHint(err), "wt finish -d -f") {
		t.Fatalf("err = %v, want the finish-flavored force hint", err)
	}
	if out.Len() != 0 {
		t.Errorf("no jump on failure, stdout = %q", out.String())
	}
	if _, err := os.Stat(dev); err != nil {
		t.Error("worktree removed despite the error")
	}

	setupOutputs(t)
	if err := finish(dev, []string{"-d", "-f"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dev); !os.IsNotExist(err) {
		t.Error("worktree still exists after -d -f")
	}
}

func TestFinishBranchNeedsDelete(t *testing.T) {
	_, dev := finishFixture(t)
	setupOutputs(t)

	err := finish(dev, []string{"-b"})
	if err == nil || !strings.Contains(err.Error(), "needs -d") {
		t.Fatalf("err = %v", err)
	}
}

func TestFinishRefusedInMain(t *testing.T) {
	repo, _ := finishFixture(t)
	setupOutputs(t)

	err := finish(repo, nil)
	if err == nil || !strings.Contains(err.Error(), "main checkout") {
		t.Fatalf("err = %v", err)
	}
}

func TestFinishBase(t *testing.T) {
	repo, dev := finishFixture(t)
	git.WriteBaseMark(dev, "dev")
	out, _ := setupOutputs(t)

	err := finish(dev, []string{"-d"})
	if err == nil || !strings.Contains(err.Error(), "base worktree") {
		t.Fatalf("-d on a base: err = %v", err)
	}

	// without -d it is just a jump away — allowed
	if err := finish(dev, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), jumpScript(repo, repo); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

// errHint digs the hint out of a hintError for assertions.
func errHint(err error) string {
	if withHint, ok := err.(*hintError); ok {
		return withHint.hint
	}
	return ""
}
