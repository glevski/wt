package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/gittest"
)

// copyFixture: linked repo with an untracked .env in the root and a
// wt-created worktree main-2.
func copyFixture(t *testing.T) (repo, wtPath string) {
	t.Helper()
	repo = linkedRepo(t)
	root := wtRoot(t)
	setupOutputs(t)
	gittest.WriteFile(t, repo, ".env", "SECRET=root\n")
	if err := create(repo, nil); err != nil {
		t.Fatal(err)
	}
	return repo, worktreePath(root, "proj", "main-2")
}

func TestCopyFromRoot(t *testing.T) {
	_, wtPath := copyFixture(t)
	setupOutputs(t)

	if err := copyCmd(wtPath, []string{".env"}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(wtPath, ".env"))
	if err != nil || string(content) != "SECRET=root\n" {
		t.Errorf("copied content = %q, %v", content, err)
	}
}

func TestCopyRenamesDestination(t *testing.T) {
	_, wtPath := copyFixture(t)
	setupOutputs(t)

	if err := copyCmd(wtPath, []string{".env", "config/.env.local"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(wtPath, "config/.env.local")); err != nil {
		t.Error("renamed destination missing (parent dirs should be created)")
	}
}

func TestCopyFromNamedWorktree(t *testing.T) {
	repo, wtPath := copyFixture(t)
	setupOutputs(t)
	side := filepath.Join(t.TempDir(), "side")
	gittest.Git(t, repo, "worktree", "add", "-b", "side", side)
	gittest.WriteFile(t, side, "notes.txt", "from side")

	if err := copyCmd(wtPath, []string{"--from", "side", "notes.txt"}); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(filepath.Join(wtPath, "notes.txt"))
	if string(content) != "from side" {
		t.Errorf("copied content = %q", content)
	}
}

func TestCopyRefusesOverwrite(t *testing.T) {
	_, wtPath := copyFixture(t)
	setupOutputs(t)
	gittest.WriteFile(t, wtPath, ".env", "SECRET=local\n")

	err := copyCmd(wtPath, []string{".env"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v", err)
	}

	if err := copyCmd(wtPath, []string{"-f", ".env"}); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(filepath.Join(wtPath, ".env"))
	if string(content) != "SECRET=root\n" {
		t.Errorf("-f did not overwrite: %q", content)
	}
}

func TestCopyGuards(t *testing.T) {
	repo, wtPath := copyFixture(t)
	setupOutputs(t)

	if err := copyCmd(wtPath, []string{"missing.txt"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing-source err = %v", err)
	}
	if err := copyCmd(repo, []string{".env"}); err == nil || !strings.Contains(err.Error(), "source and destination") {
		t.Fatalf("same-worktree err = %v", err)
	}
	gittest.WriteFile(t, repo, "dir/file.txt", "x")
	if err := copyCmd(wtPath, []string{"dir"}); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("directory err = %v", err)
	}
}
