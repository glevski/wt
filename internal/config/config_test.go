package config

import (
	"os"
	"path/filepath"
	"testing"

	"wt/internal/gittest"
)

func TestDeps(t *testing.T) {
	repo := gittest.NewRepo(t)

	if got := Deps(repo); got != nil {
		t.Errorf("Deps on fresh repo = %v, want nil", got)
	}
	gittest.Git(t, repo, "config", "--add", "wt.deps", "node_modules")
	gittest.Git(t, repo, "config", "--add", "wt.deps", ".venv")
	if got := Deps(repo); len(got) != 2 || got[0] != "node_modules" || got[1] != ".venv" {
		t.Errorf("Deps = %v", got)
	}
}

func TestCopyIgnored(t *testing.T) {
	repo := gittest.NewRepo(t)

	if !CopyIgnored(repo) {
		t.Error("default should be true")
	}
	gittest.Git(t, repo, "config", "wt.copyignored", "false")
	if CopyIgnored(repo) {
		t.Error("wt.copyignored=false should disable copying")
	}
	gittest.Git(t, repo, "config", "wt.copyignored", "true")
	if !CopyIgnored(repo) {
		t.Error("wt.copyignored=true should enable copying")
	}
}

func TestName(t *testing.T) {
	repo := gittest.NewRepo(t)

	if _, ok := Name(repo); ok {
		t.Error("Name ok=true on a repo without wt.name")
	}
	gittest.Git(t, repo, "config", "wt.name", "myproj")
	if got, ok := Name(repo); !ok || got != "myproj" {
		t.Errorf("Name = %q, %v; want myproj, true", got, ok)
	}
}

func TestRootPrecedence(t *testing.T) {
	repo := gittest.NewRepo(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("WT_ROOT", "")
	if got, _ := Root(repo); got != filepath.Join(home, "worktrees") {
		t.Errorf("default root = %q", got)
	}

	gittest.Git(t, repo, "config", "wt.root", "~/custom")
	if got, _ := Root(repo); got != filepath.Join(home, "custom") {
		t.Errorf("wt.root root = %q", got)
	}

	t.Setenv("WT_ROOT", "/abs/override")
	if got, _ := Root(repo); got != "/abs/override" {
		t.Errorf("WT_ROOT root = %q", got)
	}

	t.Setenv("WT_ROOT", "relative/dir")
	if got, _ := Root(repo); got != filepath.Join(home, "relative/dir") {
		t.Errorf("relative WT_ROOT root = %q", got)
	}
}
