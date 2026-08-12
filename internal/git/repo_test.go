package git

import (
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/gittest"
)

func TestLoad(t *testing.T) {
	repo := gittest.NewRepo(t)

	r, err := Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != filepath.Base(repo) {
		t.Errorf("Name = %q, want %q", r.Name, filepath.Base(repo))
	}
	if len(r.Worktrees) != 1 {
		t.Fatalf("got %d worktrees, want 1", len(r.Worktrees))
	}
	if r.Worktrees[0].Branch != "main" {
		t.Errorf("main worktree branch = %q, want main", r.Worktrees[0].Branch)
	}
}

func TestLoadNotARepo(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "not inside a git repository") {
		t.Fatalf("err = %v, want 'not inside a git repository'", err)
	}
}

func TestCurrentFromSubdirAndLinkedWorktree(t *testing.T) {
	repo := gittest.NewRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	gittest.Git(t, repo, "worktree", "add", "-b", "side", linked)

	sub := filepath.Join(repo, "a", "b")
	gittest.WriteFile(t, repo, "a/b/f.txt", "x")

	r, err := Load(sub)
	if err != nil {
		t.Fatal(err)
	}
	if cur := r.Current(); cur == nil || cur.Branch != "main" {
		t.Errorf("Current() from subdir = %+v, want main worktree", cur)
	}
	if r.Name != filepath.Base(repo) {
		t.Errorf("Name = %q, want %q", r.Name, filepath.Base(repo))
	}

	rl, err := Load(linked)
	if err != nil {
		t.Fatal(err)
	}
	if cur := rl.Current(); cur == nil || cur.Branch != "side" {
		t.Errorf("Current() from linked worktree = %+v, want side", cur)
	}
	if rl.Name != filepath.Base(repo) {
		t.Errorf("Name from linked worktree = %q, want %q (main worktree basename)", rl.Name, filepath.Base(repo))
	}
}

func TestCheckedOut(t *testing.T) {
	repo := gittest.NewRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	gittest.Git(t, repo, "worktree", "add", "-b", "side", linked)

	r, err := Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if wt := r.CheckedOut("side"); wt == nil {
		t.Error("CheckedOut(side) = nil, want the linked worktree")
	}
	if wt := r.CheckedOut("nope"); wt != nil {
		t.Errorf("CheckedOut(nope) = %+v, want nil", wt)
	}
	if wt := r.CheckedOut(""); wt != nil {
		t.Errorf("CheckedOut(\"\") = %+v, want nil", wt)
	}
}
