package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"wt/internal/gittest"
)

func TestStamps(t *testing.T) {
	repo := gittest.NewRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	gittest.Git(t, repo, "worktree", "add", "-b", "side", linked)

	if created, ok := CreatedAt(linked); !ok || time.Since(created) > time.Minute {
		t.Errorf("CreatedAt(linked) = %v, %v; want a recent time", created, ok)
	}
	if _, ok := CreatedAt(repo); ok {
		t.Error("CreatedAt(main worktree) ok=true, want false")
	}

	if _, ok := CheckoutStamp(linked); ok {
		t.Error("CheckoutStamp before any touch ok=true, want false")
	}
	TouchCheckoutStamp(linked)
	TouchCheckoutStamp(linked) // second touch exercises the Chtimes path
	if stamp, ok := CheckoutStamp(linked); !ok || time.Since(stamp) > time.Minute {
		t.Errorf("CheckoutStamp after touch = %v, %v; want a recent time", stamp, ok)
	}
	TouchCheckoutStamp(repo)
	if _, ok := CheckoutStamp(repo); !ok {
		t.Error("CheckoutStamp for the main worktree should work too")
	}
}

func TestBaseRecord(t *testing.T) {
	repo := gittest.NewRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	gittest.Git(t, repo, "worktree", "add", "-b", "side", linked)

	if _, ok := BaseBranch(linked); ok {
		t.Error("BaseBranch before any write ok=true, want false")
	}
	WriteBase(linked, "main")
	if base, ok := BaseBranch(linked); !ok || base != "main" {
		t.Errorf("BaseBranch = %q, %v; want main, true", base, ok)
	}

	if _, ok := ReadBaseMark(linked); ok {
		t.Error("ReadBaseMark before any write ok=true, want false")
	}
	WriteBaseMark(linked, "side")
	if branch, ok := ReadBaseMark(linked); !ok || branch != "side" {
		t.Errorf("ReadBaseMark = %q, %v; want side, true", branch, ok)
	}

	if _, ok := DepsState(linked); ok {
		t.Error("DepsState before any write ok=true, want false")
	}
	WriteDepsState(linked, "copying")
	if state, ok := DepsState(linked); !ok || state != "copying" {
		t.Errorf("DepsState = %q, %v; want copying, true", state, ok)
	}
	WriteDepsState(linked, "done")
	if state, _ := DepsState(linked); state != "done" {
		t.Errorf("DepsState after overwrite = %q, want done", state)
	}
}

func TestDepsStateFallbackForPlainDirs(t *testing.T) {
	dir := t.TempDir() // no .git anywhere — like a peek directory

	if _, ok := DepsState(dir); ok {
		t.Error("DepsState on an empty plain dir ok=true")
	}
	WriteDepsState(dir, "copying 123")
	if state, ok := DepsState(dir); !ok || state != "copying 123" {
		t.Errorf("DepsState = %q, %v", state, ok)
	}
	if _, err := os.Stat(filepath.Join(dir, ".wt-deps-state")); err != nil {
		t.Error("fallback dotfile not written inside the dir")
	}
	if got := DepsLogPath(dir); got != filepath.Join(dir, ".wt-deps.log") {
		t.Errorf("DepsLogPath = %q", got)
	}
}
