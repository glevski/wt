package cli

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"wt/internal/config"
	"wt/internal/gittest"
)

// globalFixture: two linked projects — alpha (one worktree "dev") and
// alpine (no worktrees) — plus a directory outside any repo to run from.
func globalFixture(t *testing.T) (alpha, alpine, outside string) {
	t.Helper()
	isolatedGlobalConfig(t)
	root := wtRoot(t)
	setupOutputs(t)
	alpha = gittest.NewRepo(t)
	if err := link(alpha, []string{"-r", "alpha"}); err != nil {
		t.Fatal(err)
	}
	gittest.Git(t, alpha, "worktree", "add", "-b", "dev", worktreePath(root, "alpha", "dev"))
	alpine = gittest.NewRepo(t)
	if err := link(alpine, []string{"--register", "alpine"}); err != nil {
		t.Fatal(err)
	}
	return alpha, alpine, t.TempDir()
}

func TestLinkRegisterIsExplicit(t *testing.T) {
	isolatedGlobalConfig(t)
	wtRoot(t)
	repo := gittest.NewRepo(t)
	_, errOut := setupOutputs(t)

	// plain link never registers
	if err := link(repo, []string{"myproj"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := config.ProjectPath(repo, "myproj"); ok {
		t.Error("plain wt link registered the project")
	}

	// bare -r registers an already linked repo
	if err := link(repo, []string{"-r"}); err != nil {
		t.Fatal(err)
	}
	if path, ok := config.ProjectPath(repo, "myproj"); !ok || path != repo {
		t.Errorf("registry entry = %q, %v; want %q", path, ok, repo)
	}
	if !strings.Contains(errOut.String(), "registered 'myproj'") {
		t.Errorf("log missing:\n%s", errOut.String())
	}

	// renaming a registered project keeps it registered, under the new name
	if err := link(repo, []string{"renamed"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := config.ProjectPath(repo, "myproj"); ok {
		t.Error("old registry entry survived the rename")
	}
	if path, ok := config.ProjectPath(repo, "renamed"); !ok || path != repo {
		t.Errorf("moved registry entry = %q, %v", path, ok)
	}
}

func TestLinkRegisterNeedsLink(t *testing.T) {
	isolatedGlobalConfig(t)
	wtRoot(t)
	repo := gittest.NewRepo(t)
	setupOutputs(t)

	err := link(repo, []string{"-r"})
	if err == nil || !strings.Contains(err.Error(), "not linked") {
		t.Fatalf("err = %v", err)
	}
}

func TestLinkStatusShowsRegistration(t *testing.T) {
	isolatedGlobalConfig(t)
	wtRoot(t)
	repo := gittest.NewRepo(t)
	setupOutputs(t)
	if err := link(repo, []string{"myproj"}); err != nil {
		t.Fatal(err)
	}

	_, errOut := setupOutputs(t)
	if err := link(repo, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "not registered") {
		t.Errorf("status missing the unregistered note:\n%s", errOut.String())
	}

	setupOutputs(t)
	if err := link(repo, []string{"-r"}); err != nil {
		t.Fatal(err)
	}
	_, errOut = setupOutputs(t)
	if err := link(repo, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "registered for wt global") {
		t.Errorf("status missing the registered note:\n%s", errOut.String())
	}
}

func TestGlobalList(t *testing.T) {
	alpha, alpine, outside := globalFixture(t)
	out, _ := setupOutputs(t)

	if err := global(outside, []string{"list"}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"NAME", "WORKTREES", "PATH", "alpha", "alpine", alpha, alpine} {
		if !strings.Contains(got, want) {
			t.Errorf("global list missing %q:\n%s", want, got)
		}
	}
	// alpha has one worktree, alpine none
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "alpha ") && !strings.Contains(line, " 1 ") {
			t.Errorf("alpha row count wrong: %q", line)
		}
		if strings.HasPrefix(line, "alpine") && !strings.Contains(line, " 0 ") {
			t.Errorf("alpine row count wrong: %q", line)
		}
	}
}

func TestGlobalListPrunesStale(t *testing.T) {
	_, _, outside := globalFixture(t)
	gone := filepath.Join(t.TempDir(), "gone")
	if err := config.RegisterProject(outside, "ghost", gone); err != nil {
		t.Fatal(err)
	}
	out, errOut := setupOutputs(t)

	if err := global(outside, []string{"ls"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "ghost") {
		t.Errorf("stale project still listed:\n%s", out.String())
	}
	if !strings.Contains(errOut.String(), "pruned 'ghost'") {
		t.Errorf("prune log missing:\n%s", errOut.String())
	}
	if _, ok := config.ProjectPath(outside, "ghost"); ok {
		t.Error("stale registry entry survived")
	}
}

func TestGlobalOnlySeesRegistered(t *testing.T) {
	_, _, outside := globalFixture(t)
	gamma := gittest.NewRepo(t)
	gittest.Git(t, gamma, "config", "wt.name", "gamma") // linked, never registered
	out, _ := setupOutputs(t)

	if err := global(outside, []string{"ls"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "gamma") {
		t.Errorf("unregistered project listed:\n%s", out.String())
	}
}

func TestGlobalListProject(t *testing.T) {
	_, _, outside := globalFixture(t)
	out, _ := setupOutputs(t)

	if err := global(outside, []string{"ls", "alpha"}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "dev") || !strings.Contains(got, "main") || !strings.Contains(got, "BRANCH") {
		t.Errorf("project worktree table wrong:\n%s", got)
	}
}

func TestGlobalStatus(t *testing.T) {
	_, _, outside := globalFixture(t)
	out, _ := setupOutputs(t)

	if err := global(outside, []string{"status", "alpha/dev"}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "project   alpha") || !strings.Contains(got, "worktree  dev") {
		t.Errorf("global status wrong:\n%s", got)
	}

	out, _ = setupOutputs(t)
	if err := global(outside, []string{"status", "-g", "alpha/dev"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "On branch dev") {
		t.Errorf("-g not passed through:\n%s", out.String())
	}
}

func TestGlobalGitLog(t *testing.T) {
	alpha, _, outside := globalFixture(t)
	root := filepath.Join(wtRootOf(t, alpha), "alpha", "dev")
	gittest.WriteFile(t, root, "f.txt", "x")
	gittest.Commit(t, root, "dev work")
	out, _ := setupOutputs(t)

	if err := global(outside, []string{"git-log", "alpha/dev", "--oneline", "-1"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "dev work") {
		t.Errorf("global git-log wrong:\n%s", out.String())
	}

	// bare project logs the root repo, prefixes resolve
	out, _ = setupOutputs(t)
	if err := global(outside, []string{"git-log", "alpha", "--oneline"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "initial") || strings.Contains(out.String(), "dev work") {
		t.Errorf("bare project git-log wrong:\n%s", out.String())
	}
}

func TestGlobalCheckout(t *testing.T) {
	alpha, _, outside := globalFixture(t)
	dev := filepath.Join(wtRootOf(t, alpha), "alpha", "dev")

	out, _ := setupOutputs(t)
	if err := global(outside, []string{"ch", "alpha/dev"}); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), jumpScript(dev, alpha); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}

	out, _ = setupOutputs(t)
	if err := global(outside, []string{"checkout", "alpha"}); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), jumpScript(alpha, alpha); got != want {
		t.Errorf("bare project jump = %q, want %q", got, want)
	}
}

func TestGlobalResolveErrors(t *testing.T) {
	_, _, outside := globalFixture(t)
	setupOutputs(t)

	err := global(outside, []string{"ch", "nosuch"})
	if err == nil || !strings.Contains(err.Error(), "no registered project named 'nosuch'") {
		t.Fatalf("err = %v", err)
	}
	err = global(outside, []string{"ch", "alp"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("prefix 'alp': err = %v", err)
	}
	// an exact name wins even when it prefixes another project
	if err := global(outside, []string{"status", "alpha"}); err != nil {
		t.Fatalf("exact name treated as ambiguous: %v", err)
	}
}

func TestGlobalCompletion(t *testing.T) {
	_, _, outside := globalFixture(t)
	setupOutputs(t)

	menu := completionCandidates(outside, []string{"global"})
	if !slices.Contains(menu, "checkout:jump to a project or its worktree") {
		t.Errorf("global submenu wrong: %v", menu)
	}
	if got := completionCandidates(outside, []string{"global", "ch"}); !slices.Contains(got, "alpha") {
		t.Errorf("project completion = %v", got)
	}
}

// wtRootOf resolves the WT_ROOT the fixture set (stable across the test).
func wtRootOf(t *testing.T, repoDir string) string {
	t.Helper()
	base, err := config.Root(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	return base
}
