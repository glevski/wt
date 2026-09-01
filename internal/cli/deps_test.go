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

func TestCreateLinksDepsWithWait(t *testing.T) {
	repo, root := depsFixture(t)

	if err := create(repo, []string{"-w"}); err != nil {
		t.Fatal(err)
	}
	dest := worktreePath(root, "proj", "main-2")
	if !sameInode(t, filepath.Join(repo, "node_modules/dep/index.js"), filepath.Join(dest, "node_modules/dep/index.js")) {
		t.Error("dep file not hardlinked to the source")
	}
	if _, err := os.Stat(filepath.Join(dest, ".env")); err != nil {
		t.Error("non-dep ignored file not copied")
	}
	if state, ok := git.DepsState(dest); !ok || state != "linked" {
		t.Errorf("deps state = %q, %v; want linked", state, ok)
	}
	if _, err := os.Stat(filepath.Join(dest, "node_modules.wt-partial")); !os.IsNotExist(err) {
		t.Error("partial temp dir left behind")
	}
}

func TestCreateCopyDepsFlag(t *testing.T) {
	repo, root := depsFixture(t)

	if err := create(repo, []string{"-w", "--copy-deps"}); err != nil {
		t.Fatal(err)
	}
	dest := worktreePath(root, "proj", "main-2")
	if sameInode(t, filepath.Join(repo, "node_modules/dep/index.js"), filepath.Join(dest, "node_modules/dep/index.js")) {
		t.Error("--copy-deps still hardlinked the dep")
	}
	if state, ok := git.DepsState(dest); !ok || state != "done" {
		t.Errorf("deps state = %q, %v; want done", state, ok)
	}
}

// sameInode reports whether two paths are the same file (hardlinked).
func sameInode(t *testing.T, a, b string) bool {
	t.Helper()
	ai, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	bi, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(ai, bi)
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
	var gotLink bool
	origStart := startDepsWorker
	startDepsWorker = func(src, dst string, deps []string, link bool) error {
		gotSrc, gotDst, gotDeps, gotLink = src, dst, deps, link
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
	if !gotLink {
		t.Error("worker not asked to link by default")
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
	if sameInode(t, filepath.Join(repo, "node_modules/dep/index.js"), filepath.Join(dest, "node_modules/dep/index.js")) {
		t.Error("plain worker mode must copy, not link")
	}
	if state, ok := git.DepsState(dest); !ok || state != "done" {
		t.Errorf("deps state = %q, %v; want done", state, ok)
	}
}

func TestDepsWorkerLinkMode(t *testing.T) {
	repo, root := depsFixture(t)
	setupOutputs(t)
	if err := create(repo, []string{"--no-deps"}); err != nil {
		t.Fatal(err)
	}
	dest := worktreePath(root, "proj", "main-2")

	if err := depsWorker([]string{"--link", repo, dest, "node_modules"}); err != nil {
		t.Fatal(err)
	}
	if !sameInode(t, filepath.Join(repo, "node_modules/dep/index.js"), filepath.Join(dest, "node_modules/dep/index.js")) {
		t.Error("--link worker did not hardlink")
	}
	if state, ok := git.DepsState(dest); !ok || state != "linked" {
		t.Errorf("deps state = %q, %v; want linked", state, ok)
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
	if !sameInode(t, filepath.Join(repo, "node_modules/dep/index.js"), filepath.Join(dest, "node_modules/dep/index.js")) {
		t.Error("sync did not hardlink the dep by default")
	}
	if state, ok := git.DepsState(dest); !ok || state != "linked" {
		t.Errorf("deps state = %q, %v; want linked", state, ok)
	}

	// --copy forces owned copies
	if err := deps(repo, []string{"sync", "--copy", "main-2"}); err != nil {
		t.Fatal(err)
	}
	if sameInode(t, filepath.Join(repo, "node_modules/dep/index.js"), filepath.Join(dest, "node_modules/dep/index.js")) {
		t.Error("sync --copy still hardlinked")
	}
	if state, _ := git.DepsState(dest); state != "done" {
		t.Errorf("deps state after --copy = %q, want done", state)
	}
}

func TestDepsEject(t *testing.T) {
	repo, root := depsFixture(t)
	setupOutputs(t)
	if err := create(repo, []string{"-w"}); err != nil {
		t.Fatal(err)
	}
	dest := worktreePath(root, "proj", "main-2")
	srcFile := filepath.Join(repo, "node_modules/dep/index.js")
	dstFile := filepath.Join(dest, "node_modules/dep/index.js")
	if !sameInode(t, srcFile, dstFile) {
		t.Fatal("fixture expects a linked worktree")
	}

	if err := deps(repo, []string{"eject", "main-2"}); err != nil {
		t.Fatal(err)
	}
	if sameInode(t, srcFile, dstFile) {
		t.Error("eject left the dep hardlinked")
	}
	want, _ := os.ReadFile(srcFile)
	got, err := os.ReadFile(dstFile)
	if err != nil || string(got) != string(want) {
		t.Errorf("ejected content differs: %q vs %q (%v)", got, want, err)
	}
	if state, _ := git.DepsState(dest); state != "done" {
		t.Errorf("deps state = %q, want done", state)
	}
}

func TestDepsEjectNoCopy(t *testing.T) {
	repo, root := depsFixture(t)
	setupOutputs(t)
	if err := create(repo, []string{"-w"}); err != nil {
		t.Fatal(err)
	}
	dest := worktreePath(root, "proj", "main-2")

	if err := deps(repo, []string{"eject", "--no-copy", "main-2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "node_modules")); !os.IsNotExist(err) {
		t.Error("--no-copy left the dep in place")
	}
	if state, _ := git.DepsState(dest); state != "ejected" {
		t.Errorf("deps state = %q, want ejected", state)
	}
	if _, err := os.Stat(filepath.Join(repo, "node_modules/dep/index.js")); err != nil {
		t.Error("source worktree lost its dep — shared inode must survive removal")
	}
}

func TestDepsEjectRefusesWhileSyncing(t *testing.T) {
	repo, root := depsFixture(t)
	setupOutputs(t)
	if err := create(repo, []string{"-w"}); err != nil {
		t.Fatal(err)
	}
	dest := worktreePath(root, "proj", "main-2")
	git.WriteDepsState(dest, "copying")

	err := deps(repo, []string{"eject", "main-2"})
	if err == nil || !strings.Contains(err.Error(), "still syncing") {
		t.Fatalf("err = %v", err)
	}
}

func TestLinkTreePreservesSymlinks(t *testing.T) {
	src, dst := t.TempDir(), filepath.Join(t.TempDir(), "out")
	gittest.WriteFile(t, src, "pkg/real.js", "content")
	if err := os.Symlink("real.js", filepath.Join(src, "pkg/alias.js")); err != nil {
		t.Fatal(err)
	}

	if err := linkTree(src, dst); err != nil {
		t.Fatal(err)
	}
	if !sameInode(t, filepath.Join(src, "pkg/real.js"), filepath.Join(dst, "pkg/real.js")) {
		t.Error("regular file not hardlinked")
	}
	info, err := os.Lstat(filepath.Join(dst, "pkg/alias.js"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlink not recreated as a symlink: %v, %v", info, err)
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
