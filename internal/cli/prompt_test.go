package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/git"
	"wt/internal/gittest"
)

// promptFor runs the prompt command from dir and returns the emitted segment.
func promptFor(t *testing.T, dir string) string {
	t.Helper()
	out, _ := setupOutputs(t)
	if err := prompt(dir, []string{"zsh"}); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSuffix(out.String(), "\n")
}

func TestPromptRootRepo(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)

	want := "%B%F{cyan}proj%f%b (%B%F{cyan}main%f%b)"
	if got := promptFor(t, repo); got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
}

func TestPromptSubdirectory(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	sub := filepath.Join(repo, "src", "app")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := promptFor(t, sub); !strings.Contains(got, "proj/src/app") {
		t.Errorf("prompt = %q, want the project-anchored subpath", got)
	}
}

func TestPromptManagedWorktree(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	path := worktreePath(root, "proj", "dev")
	gittest.Git(t, repo, "worktree", "add", "-b", "dev", path)

	want := "%B%F{cyan}proj%f%b (%B%F{green}dev%f%b)"
	if got := promptFor(t, path); got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
}

func TestPromptBaseAndDrift(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	path := worktreePath(root, "proj", "staging")
	gittest.Git(t, repo, "worktree", "add", "-b", "staging", path)
	git.WriteBaseMark(path, "staging")

	if got := promptFor(t, path); !strings.Contains(got, "(%B%F{208}staging%f%b)") {
		t.Errorf("base prompt = %q, want an orange branch", got)
	}

	gittest.Git(t, path, "checkout", "-b", "hotfix")
	if got := promptFor(t, path); !strings.Contains(got, "(%B%F{red}hotfix%f%b)") {
		t.Errorf("drifted base prompt = %q, want a red branch", got)
	}
}

func TestPromptExternalWorktree(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	path := filepath.Join(t.TempDir(), "elsewhere")
	gittest.Git(t, repo, "worktree", "add", "-b", "feature", path)

	got := promptFor(t, path)
	if !strings.Contains(got, "(%B%F{magenta}feature%f%b)") {
		t.Errorf("prompt = %q, want a magenta branch", got)
	}
	if !strings.Contains(got, "%B%F{cyan}proj%f%b") {
		t.Errorf("prompt = %q, want the project name, not the directory name", got)
	}
}

func TestPromptUnlinkedRepoUsesRepoName(t *testing.T) {
	repo := gittest.NewRepo(t)
	wtRoot(t)

	if got := promptFor(t, repo); !strings.Contains(got, filepath.Base(repo)) {
		t.Errorf("prompt = %q, want the repo name %q", got, filepath.Base(repo))
	}
}

func TestPromptOutsideRepo(t *testing.T) {
	if got := promptFor(t, t.TempDir()); got != zshPlainPath {
		t.Errorf("prompt = %q, want the plain-path fallback %q", got, zshPlainPath)
	}
}

func TestPromptDetachedShowsSHA(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	gittest.Git(t, repo, "checkout", "--detach")
	sha := gittest.Git(t, repo, "rev-parse", "--short=7", "HEAD")

	if got := promptFor(t, repo); !strings.Contains(got, sha) {
		t.Errorf("prompt = %q, want the short sha %q", got, sha)
	}
}

func TestPromptInsidePeek(t *testing.T) {
	repo, root := peekFixture(t)
	setupOutputs(t)
	if err := peek(repo, []string{"--no-ignored", "HEAD~1"}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "proj", "peek-HEAD~1")

	want := "%B%F{cyan}proj%f%b (%B%F{red}peek:HEAD~1%f%b)"
	if got := promptFor(t, dest); got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
}

func TestPromptEscapesPercent(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	sub := filepath.Join(repo, "50%off")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := promptFor(t, sub); !strings.Contains(got, "50%%off") {
		t.Errorf("prompt = %q, want literal %% doubled", got)
	}
}

func TestPromptUsage(t *testing.T) {
	setupOutputs(t)
	if err := prompt(t.TempDir(), nil); err == nil {
		t.Fatal("expected a usage error without a shell argument")
	}
	if err := prompt(t.TempDir(), []string{"fish"}); err == nil {
		t.Fatal("expected a usage error for an unsupported shell")
	}
}
