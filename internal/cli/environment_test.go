package cli

import (
	"os"
	"path/filepath"
	"testing"

	"wt/internal/gittest"
)

// ignoredFixture: linked repo whose .gitignore covers .env and node_modules/,
// with both present in the working tree.
func ignoredFixture(t *testing.T) (repo, root string) {
	t.Helper()
	repo = linkedRepo(t)
	root = wtRoot(t)
	setupOutputs(t)
	gittest.WriteFile(t, repo, ".gitignore", ".env\nnode_modules/\n")
	gittest.Commit(t, repo, "add gitignore")
	gittest.WriteFile(t, repo, ".env", "SECRET=1")
	gittest.WriteFile(t, repo, "node_modules/dep/index.js", "js")
	return repo, root
}

func TestCreateCopiesIgnored(t *testing.T) {
	repo, root := ignoredFixture(t)

	if err := create(repo, nil); err != nil {
		t.Fatal(err)
	}
	dest := worktreePath(root, "proj", "main-2")
	for _, f := range []string{".env", "node_modules/dep/index.js"} {
		if _, err := os.Stat(filepath.Join(dest, f)); err != nil {
			t.Errorf("%s missing in the new worktree", f)
		}
	}
	if status := gittest.Git(t, dest, "status", "--porcelain"); status != "" {
		t.Errorf("ignored copies made the worktree dirty:\n%s", status)
	}
}

func TestForkCopiesIgnored(t *testing.T) {
	repo, root := ignoredFixture(t)

	if err := fork(repo, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(worktreePath(root, "proj", "main-2"), "node_modules/dep/index.js")); err != nil {
		t.Error("node_modules not copied by fork")
	}
}

func TestCopyIgnoredDisabled(t *testing.T) {
	repo, root := ignoredFixture(t)
	gittest.Git(t, repo, "config", "wt.copyignored", "false")

	if err := create(repo, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(worktreePath(root, "proj", "main-2"), ".env")); !os.IsNotExist(err) {
		t.Error(".env copied despite wt.copyignored=false")
	}
}
