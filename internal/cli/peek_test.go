package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/git"
	"wt/internal/gittest"
)

// peekFixture: linked repo with two commits (README v1 → v2) and a dirty
// working tree — peeks must work regardless.
func peekFixture(t *testing.T) (repo, root string) {
	t.Helper()
	repo = linkedRepo(t)
	root = wtRoot(t)
	setupOutputs(t)
	gittest.WriteFile(t, repo, "README.md", "version two")
	gittest.Commit(t, repo, "v2")
	gittest.WriteFile(t, repo, "README.md", "uncommitted local change")
	return repo, root
}

func TestPeekExportsRevision(t *testing.T) {
	repo, root := peekFixture(t)
	out, errOut := setupOutputs(t)

	if err := peek(repo, []string{"HEAD~1"}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "proj", "peek-HEAD~1")
	if got, want := out.String(), jumpScript(dest, repo); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	content, err := os.ReadFile(filepath.Join(dest, "README.md"))
	if err != nil || string(content) != "hello\n" {
		t.Errorf("peeked README = %q, %v; want the v1 content", content, err)
	}
	if _, ok := readPeekInfo(dest); !ok {
		t.Error("peek marker missing")
	}
	// the source worktree is untouched, dirt included
	if got, _ := os.ReadFile(filepath.Join(repo, "README.md")); string(got) != "uncommitted local change" {
		t.Errorf("source README changed: %q", got)
	}
	if !strings.Contains(errOut.String(), "throwaway") {
		t.Errorf("log missing the throwaway warning:\n%s", errOut.String())
	}
}

func TestPeekCopiesEnvironment(t *testing.T) {
	repo, root := depsFixture(t)
	setupOutputs(t)

	var workerDst string
	origStart := startDepsWorker
	startDepsWorker = func(src, dst string, deps []string) error {
		workerDst = dst
		return nil
	}
	t.Cleanup(func() { startDepsWorker = origStart })

	if err := peek(repo, []string{"HEAD"}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "proj", "peek-HEAD")
	if _, err := os.Stat(filepath.Join(dest, ".env")); err != nil {
		t.Error(".env not copied into the peek")
	}
	if workerDst != dest {
		t.Errorf("deps worker dst = %q, want %q", workerDst, dest)
	}
	if state, ok := git.DepsState(dest); !ok || !strings.HasPrefix(state, "copying") {
		t.Errorf("in-dir deps state = %q, %v; want copying", state, ok)
	}
}

func TestPeekWaitCopiesDepsInline(t *testing.T) {
	repo, root := depsFixture(t)
	setupOutputs(t)

	if err := peek(repo, []string{"-w", "HEAD"}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "proj", "peek-HEAD")
	if _, err := os.Stat(filepath.Join(dest, "node_modules/dep/index.js")); err != nil {
		t.Error("deps not copied with -w")
	}
	if state, _ := git.DepsState(dest); state != "done" {
		t.Errorf("deps state = %q, want done", state)
	}
}

func TestPeekExistingJumpsWithoutReexport(t *testing.T) {
	repo, root := peekFixture(t)
	setupOutputs(t)
	if err := peek(repo, []string{"--no-ignored", "HEAD~1"}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "proj", "peek-HEAD~1")
	first, _ := readPeekInfo(dest)

	out, errOut := setupOutputs(t)
	if err := peek(repo, []string{"--no-ignored", "HEAD~1"}); err != nil {
		t.Fatal(err)
	}
	second, _ := readPeekInfo(dest)
	if !first.Created.Equal(second.Created) {
		t.Error("existing peek was re-exported")
	}
	if got, want := out.String(), jumpScript(dest, repo); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if !strings.Contains(errOut.String(), "already peeking") {
		t.Errorf("log missing already-peeking note:\n%s", errOut.String())
	}
}

func TestPeekOffFromInside(t *testing.T) {
	repo, root := peekFixture(t)
	setupOutputs(t)
	if err := peek(repo, []string{"--no-ignored", "HEAD~1"}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "proj", "peek-HEAD~1")

	out, _ := setupOutputs(t)
	if err := peek(dest, []string{"off"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("peek dir still exists after off")
	}
	if got, want := out.String(), jumpScript(repo, repo); got != want {
		t.Errorf("stdout = %q, want %q (jump back to the source)", got, want)
	}
}

func TestPeekOffNamed(t *testing.T) {
	repo, root := peekFixture(t)
	setupOutputs(t)
	if err := peek(repo, []string{"--no-ignored", "HEAD~1"}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "proj", "peek-HEAD~1")

	out, _ := setupOutputs(t)
	if err := peek(repo, []string{"off", "peek-HEAD~1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("peek dir still exists")
	}
	if out.Len() != 0 {
		t.Errorf("named off must not jump, stdout = %q", out.String())
	}
}

func TestPeekOffOutsideAnyPeek(t *testing.T) {
	repo, _ := peekFixture(t)
	setupOutputs(t)

	err := peek(repo, []string{"off"})
	if err == nil || !strings.Contains(err.Error(), "not inside a peek") {
		t.Fatalf("err = %v", err)
	}
}

func TestPeekInsidePeekRefused(t *testing.T) {
	repo, root := peekFixture(t)
	setupOutputs(t)
	if err := peek(repo, []string{"--no-ignored", "HEAD~1"}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "proj", "peek-HEAD~1")

	err := peek(dest, []string{"HEAD"})
	if err == nil || !strings.Contains(err.Error(), "already inside a peek") {
		t.Fatalf("err = %v", err)
	}
}

func TestPeekUnknownRev(t *testing.T) {
	repo, _ := peekFixture(t)
	setupOutputs(t)

	err := peek(repo, []string{"no-such-rev"})
	if err == nil || !strings.Contains(err.Error(), "does not resolve") {
		t.Fatalf("err = %v", err)
	}
}

func TestPeekListAndVisibility(t *testing.T) {
	repo, root := peekFixture(t)
	setupOutputs(t)

	_, errOut := setupOutputs(t)
	if err := peek(repo, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "no peeks") {
		t.Errorf("empty state missing hint:\n%s", errOut.String())
	}

	if err := peek(repo, []string{"--no-ignored", "HEAD~1"}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "proj", "peek-HEAD~1")

	_, errOut = setupOutputs(t)
	if err := peek(repo, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "peek-HEAD~1") || !strings.Contains(errOut.String(), "'HEAD~1'") {
		t.Errorf("peek listing wrong:\n%s", errOut.String())
	}

	out, _ := setupOutputs(t)
	if err := list(repo, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "(peek: HEAD~1)") || !strings.Contains(out.String(), "peek") {
		t.Errorf("wt list missing the peek row:\n%s", out.String())
	}

	out, _ = setupOutputs(t)
	if err := status(dest, nil); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "peek      peek-HEAD~1") || !strings.Contains(got, "rev       HEAD~1") {
		t.Errorf("status inside peek wrong:\n%s", got)
	}
}

func TestUnpeekAlias(t *testing.T) {
	repo, root := peekFixture(t)
	setupOutputs(t)
	if err := peek(repo, []string{"--no-ignored", "HEAD~1"}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "proj", "peek-HEAD~1")
	out, errOut := setupOutputs(t)

	if err := status(dest, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "wt peek off (or wt unpeek)") {
		t.Errorf("status missing the exit hint:\n%s", errOut.String())
	}

	// dispatch-level: unpeek == peek off
	if err := peek(dest, []string{"off"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("peek dir still exists")
	}
	_ = out
}
