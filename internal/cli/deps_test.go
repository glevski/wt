package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"wt/internal/git"
	"wt/internal/gittest"
)

// depsFixture: ignoredFixture plus node_modules declared as a dep.
func depsFixture(t *testing.T) (repo, root string) {
	t.Helper()
	repo, root = ignoredFixture(t)
	gittest.Git(t, repo, "config", "--add", "wt.deps", "node_modules")
	return repo, root
}

func TestDepsAddRmList(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	out, errOut := setupOutputs(t)
	gittest.WriteFile(t, repo, ".gitignore", "node_modules/\n")
	gittest.Commit(t, repo, "gitignore")

	if err := deps(repo, []string{"add", "node_modules"}); err != nil {
		t.Fatal(err)
	}
	if err := deps(repo, []string{"add", "src"}); err != nil { // tracked-ish path
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "not git-ignored") {
		t.Errorf("missing not-ignored warning:\n%s", errOut.String())
	}

	out.Reset()
	if err := deps(repo, nil); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "node_modules\nsrc\n" {
		t.Errorf("deps list = %q", got)
	}

	if err := deps(repo, []string{"rm", "src"}); err != nil {
		t.Fatal(err)
	}
	if err := deps(repo, []string{"rm", "src"}); err == nil {
		t.Error("removing a non-dep should error")
	}
	out.Reset()
	if err := deps(repo, nil); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "node_modules\n" {
		t.Errorf("deps list after rm = %q", got)
	}
}

func TestCreateCopiesDepsWithWait(t *testing.T) {
	repo, root := depsFixture(t)

	if err := create(repo, []string{"-w"}); err != nil {
		t.Fatal(err)
	}
	dest := worktreePath(root, "proj", "main-2")
	if _, err := os.Stat(filepath.Join(dest, "node_modules/dep/index.js")); err != nil {
		t.Error("dep not copied with -w")
	}
	if _, err := os.Stat(filepath.Join(dest, ".env")); err != nil {
		t.Error("non-dep ignored file not copied")
	}
	if state, ok := git.DepsState(dest); !ok || state != "done" {
		t.Errorf("deps state = %q, %v; want done", state, ok)
	}
	if _, err := os.Stat(filepath.Join(dest, "node_modules.wt-partial")); !os.IsNotExist(err) {
		t.Error("partial temp dir left behind")
	}
}

func TestCreateNoDepsFlag(t *testing.T) {
	repo, root := depsFixture(t)

	if err := create(repo, []string{"--no-deps"}); err != nil {
		t.Fatal(err)
	}
	dest := worktreePath(root, "proj", "main-2")
	if _, err := os.Stat(filepath.Join(dest, "node_modules")); !os.IsNotExist(err) {
		t.Error("deps copied despite --no-deps")
	}
	if _, err := os.Stat(filepath.Join(dest, ".env")); err != nil {
		t.Error("--no-deps must still copy the other ignored files")
	}
}

func TestCreateNoIgnoredFlag(t *testing.T) {
	repo, root := depsFixture(t)

	if err := create(repo, []string{"--no-ignored"}); err != nil {
		t.Fatal(err)
	}
	dest := worktreePath(root, "proj", "main-2")
	for _, f := range []string{".env", "node_modules"} {
		if _, err := os.Stat(filepath.Join(dest, f)); !os.IsNotExist(err) {
			t.Errorf("%s copied despite --no-ignored", f)
		}
	}
}

func TestCreateSpawnsDepsWorker(t *testing.T) {
	repo, root := depsFixture(t)
	_, errOut := setupOutputs(t)

	var gotSrc, gotDst string
	var gotDeps []string
	origStart := startDepsWorker
	startDepsWorker = func(src, dst string, deps []string) error {
		gotSrc, gotDst, gotDeps = src, dst, deps
		return nil
	}
	t.Cleanup(func() { startDepsWorker = origStart })

	if err := create(repo, nil); err != nil {
		t.Fatal(err)
	}
	dest := worktreePath(root, "proj", "main-2")
	if gotSrc != repo || gotDst != dest || len(gotDeps) != 1 || gotDeps[0] != "node_modules" {
		t.Errorf("worker args = %q %q %v", gotSrc, gotDst, gotDeps)
	}
	if state, ok := git.DepsState(dest); !ok || state != "copying" {
		t.Errorf("deps state = %q, %v; want copying", state, ok)
	}
	if !strings.Contains(errOut.String(), "in the background") {
		t.Errorf("log missing background note:\n%s", errOut.String())
	}
}

func TestDepsWorkerCopiesAndMarksDone(t *testing.T) {
	repo, root := depsFixture(t)
	setupOutputs(t)
	if err := create(repo, []string{"--no-deps"}); err != nil {
		t.Fatal(err)
	}
	dest := worktreePath(root, "proj", "main-2")

	if err := depsWorker([]string{repo, dest, "node_modules"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "node_modules/dep/index.js")); err != nil {
		t.Error("worker did not copy the dep")
	}
	if state, ok := git.DepsState(dest); !ok || state != "done" {
		t.Errorf("deps state = %q, %v; want done", state, ok)
	}
}

func TestDepsSync(t *testing.T) {
	repo, root := depsFixture(t)
	setupOutputs(t)
	if err := create(repo, []string{"--no-deps"}); err != nil {
		t.Fatal(err)
	}
	dest := worktreePath(root, "proj", "main-2")

	// run from the root: source = current (root repo), target = main-2
	if err := deps(repo, []string{"sync", "main-2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "node_modules/dep/index.js")); err != nil {
		t.Error("sync did not copy the dep")
	}
	if state, ok := git.DepsState(dest); !ok || state != "done" {
		t.Errorf("deps state = %q, %v; want done", state, ok)
	}
}

func TestRemoveRefusesWhileSyncing(t *testing.T) {
	repo, root := depsFixture(t)
	setupOutputs(t)
	if err := create(repo, []string{"--no-deps"}); err != nil {
		t.Fatal(err)
	}
	dest := worktreePath(root, "proj", "main-2")
	git.WriteDepsState(dest, "copying")

	err := remove(repo, []string{"main-2"})
	if err == nil || !strings.Contains(err.Error(), "still syncing deps") {
		t.Fatalf("err = %v", err)
	}
	if err := remove(repo, []string{"-f", "main-2"}); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveForceKillsWorker(t *testing.T) {
	repo, root := depsFixture(t)
	setupOutputs(t)
	if err := create(repo, []string{"--no-deps"}); err != nil {
		t.Fatal(err)
	}
	dest := worktreePath(root, "proj", "main-2")

	// stand in for a live worker: a detached sleep whose pid is recorded
	worker := exec.Command("sleep", "60")
	worker.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := worker.Start(); err != nil {
		t.Fatal(err)
	}
	git.WriteDepsState(dest, fmt.Sprintf("copying %d", worker.Process.Pid))

	if err := remove(repo, []string{"-f", "main-2"}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- worker.Wait() }()
	select {
	case <-done: // terminated by the removal — what we want
	case <-time.After(3 * time.Second):
		_ = worker.Process.Kill()
		t.Fatal("worker still alive after forced removal")
	}
}

func TestListShowsSyncing(t *testing.T) {
	repo, root := depsFixture(t)
	setupOutputs(t)
	if err := create(repo, []string{"--no-deps"}); err != nil {
		t.Fatal(err)
	}
	dest := worktreePath(root, "proj", "main-2")
	git.WriteDepsState(dest, "copying")
	out, _ := setupOutputs(t)

	if err := list(repo, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "syncing") {
		t.Errorf("list missing syncing state:\n%s", out.String())
	}

	out.Reset()
	if err := status(repo, []string{"main-2"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "deps      copying…") {
		t.Errorf("status missing deps line:\n%s", out.String())
	}
}
