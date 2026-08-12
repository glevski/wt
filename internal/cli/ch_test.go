package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/gittest"
)

func chFixture(t *testing.T) (repo, authPath, fixPath string) {
	t.Helper()
	repo = gittest.NewRepo(t)
	wtRoot(t)
	base := t.TempDir()
	authPath = filepath.Join(base, "feature-auth")
	fixPath = filepath.Join(base, "feature-fix")
	gittest.Git(t, repo, "worktree", "add", "-b", "feature/auth", authPath)
	gittest.Git(t, repo, "worktree", "add", "-b", "feature/fix", fixPath)
	return repo, authPath, fixPath
}

func TestChExactMatch(t *testing.T) {
	repo, authPath, _ := chFixture(t)
	out, _ := setupOutputs(t)

	if err := ch(repo, []string{"feature-auth"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != authPath+"\n" {
		t.Errorf("stdout = %q, want %q", got, authPath+"\n")
	}
}

func TestChUniquePrefix(t *testing.T) {
	repo, _, fixPath := chFixture(t)
	out, _ := setupOutputs(t)

	if err := ch(repo, []string{"feature-f"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != fixPath+"\n" {
		t.Errorf("stdout = %q, want %q", got, fixPath+"\n")
	}
}

func TestChAmbiguousPrefix(t *testing.T) {
	repo, _, _ := chFixture(t)
	out, _ := setupOutputs(t)

	err := ch(repo, []string{"feature"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("err = %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout not empty on error: %q", out.String())
	}
}

func TestChUnknown(t *testing.T) {
	repo, _, _ := chFixture(t)
	out, _ := setupOutputs(t)

	err := ch(repo, []string{"zzz"})
	if err == nil || !strings.Contains(err.Error(), "no worktree named") {
		t.Fatalf("err = %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout not empty on error: %q", out.String())
	}
}

func TestChMainWorktreeByRepoName(t *testing.T) {
	repo, _, _ := chFixture(t)
	out, _ := setupOutputs(t)

	if err := ch(repo, []string{filepath.Base(repo)}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != repo+"\n" {
		t.Errorf("stdout = %q, want %q", got, repo+"\n")
	}
}
