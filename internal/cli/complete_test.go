package cli

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"wt/internal/gittest"
)

func completeLines(t *testing.T, dir string, words ...string) []string {
	t.Helper()
	out, _ := setupOutputs(t)
	if err := complete(dir, words); err != nil {
		t.Fatalf("complete(%v): %v", words, err)
	}
	if out.Len() == 0 {
		return nil
	}
	return strings.Split(strings.TrimSpace(out.String()), "\n")
}

func TestCompleteCommands(t *testing.T) {
	lines := completeLines(t, t.TempDir()) // works outside a repo too
	for _, want := range []string{"checkout:", "ch:", "base:", "root:", "rm:"} {
		if !slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, want) }) {
			t.Errorf("command menu missing %q entry:\n%v", want, lines)
		}
	}
}

func TestCompleteWorktreeNames(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	setupOutputs(t)
	side := filepath.Join(t.TempDir(), "side")
	gittest.Git(t, repo, "worktree", "add", "-b", "side-branch", side)
	gittest.Git(t, repo, "branch", "staging")
	if err := base(repo, []string{"add", "staging"}); err != nil {
		t.Fatal(err)
	}

	forCh := completeLines(t, repo, "ch")
	for _, want := range []string{filepath.Base(repo), "side", "staging"} {
		if !slices.Contains(forCh, want) {
			t.Errorf("ch candidates missing %q: %v", want, forCh)
		}
	}

	// rm mirrors remove's guards: no main checkout, no bases, no current
	// worktree (the invocation dir is the main checkout here anyway).
	if forRm := completeLines(t, repo, "rm"); !reflect.DeepEqual(forRm, []string{"side"}) {
		t.Errorf("rm candidates = %v, want [side]", forRm)
	}

	if forBaseRm := completeLines(t, repo, "base", "rm"); !reflect.DeepEqual(forBaseRm, []string{"staging"}) {
		t.Errorf("base rm candidates = %v, want [staging]", forBaseRm)
	}
}

func TestCompleteBranches(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	setupOutputs(t)
	gittest.AddRemote(t, repo)
	gittest.Git(t, repo, "branch", "free")
	gittest.Git(t, repo, "push", "origin", "main:remote-only")

	// create accepts every local branch (checked-out ones base off their tip)
	// plus remote-only branches.
	if got := completeLines(t, repo, "create"); !reflect.DeepEqual(got, []string{"free", "main", "remote-only"}) {
		t.Errorf("create candidates = %v, want [free main remote-only]", got)
	}

	// fork and reset take existing local branches only.
	if got := completeLines(t, repo, "fork"); !reflect.DeepEqual(got, []string{"free", "main"}) {
		t.Errorf("fork candidates = %v, want [free main]", got)
	}

	// after -b a brand-new branch name follows — nothing to offer.
	if got := completeLines(t, repo, "ch", "-b"); got != nil {
		t.Errorf("ch -b candidates = %v, want none", got)
	}
}

func TestCompleteRoot(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	setupOutputs(t)

	menu := completeLines(t, repo, "root")
	if !slices.ContainsFunc(menu, func(l string) bool { return strings.HasPrefix(l, "fork:") }) {
		t.Errorf("root menu missing fork: %v", menu)
	}
	if got := completeLines(t, repo, "root", "checkout"); !slices.Contains(got, filepath.Base(repo)) {
		t.Errorf("root checkout candidates = %v, want the main worktree name", got)
	}
}

func TestCompleteSilentOutsideRepo(t *testing.T) {
	if got := completeLines(t, t.TempDir(), "ch"); got != nil {
		t.Errorf("candidates outside a repo = %v, want none", got)
	}
	if got := completeLines(t, t.TempDir(), "nonsense"); got != nil {
		t.Errorf("candidates for unknown command = %v, want none", got)
	}
}
