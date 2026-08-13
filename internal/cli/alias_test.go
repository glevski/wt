package cli

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"wt/internal/config"
	"wt/internal/gittest"
)

// isolatedGlobalConfig points git's global config at a scratch file so
// alias add/rm never touch the developer's real ~/.gitconfig.
func isolatedGlobalConfig(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
}

func TestAliasAddListRemove(t *testing.T) {
	isolatedGlobalConfig(t)
	repo := gittest.NewRepo(t)
	out, errOut := setupOutputs(t)

	if err := alias(repo, []string{"add", "cr", "create", "-c"}); err != nil {
		t.Fatal(err)
	}
	if got := config.Alias(repo, "cr"); strings.Join(got, " ") != "create -c" {
		t.Errorf("stored alias = %v", got)
	}

	if err := alias(repo, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "cr = create -c") {
		t.Errorf("alias list missing the entry:\n%s", out.String())
	}

	if err := alias(repo, []string{"rm", "cr"}); err != nil {
		t.Fatal(err)
	}
	if config.Alias(repo, "cr") != nil {
		t.Error("alias still resolves after rm")
	}
	err := alias(repo, []string{"rm", "cr"})
	if err == nil || !strings.Contains(err.Error(), "no alias named") {
		t.Errorf("err = %v", err)
	}
	_ = errOut
}

func TestAliasQuotedExpansion(t *testing.T) {
	isolatedGlobalConfig(t)
	repo := gittest.NewRepo(t)
	setupOutputs(t)

	// `wt alias add cr "create -c"` arrives as one argument
	if err := alias(repo, []string{"add", "cr", "create -c"}); err != nil {
		t.Fatal(err)
	}
	if got := config.Alias(repo, "cr"); strings.Join(got, " ") != "create -c" {
		t.Errorf("stored alias = %v", got)
	}
}

func TestAliasAddRejections(t *testing.T) {
	isolatedGlobalConfig(t)
	repo := gittest.NewRepo(t)
	setupOutputs(t)

	for _, c := range []struct{ args, wantErr string }{
		{"ch checkout", "is a wt command"},
		{"we?rd status", "invalid alias name"},
		{"x frobnicate", "must expand to a wt command"},
		{"x", "usage"},
	} {
		err := alias(repo, append([]string{"add"}, strings.Fields(c.args)...))
		if err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("alias add %s: err = %v, want %q", c.args, err, c.wantErr)
		}
	}
}

func TestAliasDispatch(t *testing.T) {
	isolatedGlobalConfig(t)
	repo := linkedRepo(t)
	wtRoot(t)
	gittest.Git(t, repo, "config", "wt.alias.st", "status")
	t.Chdir(repo)
	out, _ := setupOutputs(t)

	if err := dispatch("st", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "project   proj") {
		t.Errorf("aliased status output wrong:\n%s", out.String())
	}
}

func TestAliasDispatchMergesArgs(t *testing.T) {
	isolatedGlobalConfig(t)
	repo := linkedRepo(t)
	wtRoot(t)
	side := filepath.Join(t.TempDir(), "side")
	gittest.Git(t, repo, "worktree", "add", "-b", "side", side)
	gittest.Git(t, repo, "config", "wt.alias.stg", "status -g")
	t.Chdir(repo)
	out, _ := setupOutputs(t)

	// alias args come first, typed args are appended — like git
	if err := dispatch("stg", []string{"side"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "path      "+side) || !strings.Contains(out.String(), "On branch side") {
		t.Errorf("aliased status -g side output wrong:\n%s", out.String())
	}
}

func TestAliasBuiltinWins(t *testing.T) {
	isolatedGlobalConfig(t)
	repo := linkedRepo(t)
	wtRoot(t)
	gittest.Git(t, repo, "config", "wt.alias.status", "list")
	t.Chdir(repo)
	out, _ := setupOutputs(t)

	if err := dispatch("status", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "project   proj") {
		t.Errorf("builtin status was shadowed by the alias:\n%s", out.String())
	}
}

func TestAliasToUnknownCommand(t *testing.T) {
	isolatedGlobalConfig(t)
	repo := gittest.NewRepo(t)
	gittest.Git(t, repo, "config", "wt.alias.bad", "nope")
	t.Chdir(repo)
	setupOutputs(t)

	err := dispatch("bad", nil)
	if err == nil || !strings.Contains(err.Error(), "alias 'bad' expands to unknown command") {
		t.Fatalf("err = %v", err)
	}
}

func TestJumpAliasProbe(t *testing.T) {
	isolatedGlobalConfig(t)
	repo := gittest.NewRepo(t)
	gittest.Git(t, repo, "config", "wt.alias.cr", "create -c")
	gittest.Git(t, repo, "config", "wt.alias.st", "status")
	gittest.Git(t, repo, "config", "wt.alias.r", "root")
	gittest.Git(t, repo, "config", "wt.alias.git-log", "checkout")

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"cr", ""}, "1\n"},
		{[]string{"st", ""}, ""},
		{[]string{"r", "checkout"}, "1\n"},
		{[]string{"r", "status"}, ""},
		{[]string{"undefined", ""}, ""},
		// a hand-set alias shadowing a builtin never wins, so it must not
		// make the wrapper capture stdout either
		{[]string{"git-log", ""}, ""},
		// jump builtins missing from an older wrapper's static case list
		// are answered too — new commands jump without a re-source
		{[]string{"finish", "-d"}, "1\n"},
	} {
		out, _ := setupOutputs(t)
		if err := jumpAlias(repo, c.args); err != nil {
			t.Fatal(err)
		}
		if out.String() != c.want {
			t.Errorf("__jump-alias %v = %q, want %q", c.args, out.String(), c.want)
		}
	}
}

func TestAliasCompletion(t *testing.T) {
	isolatedGlobalConfig(t)
	repo := gittest.NewRepo(t)
	setupOutputs(t)
	gittest.Git(t, repo, "config", "wt.alias.cr", "create -c")

	menu := completionCandidates(repo, nil)
	if !slices.Contains(menu, "cr:alias for 'create -c'") {
		t.Errorf("command menu missing the alias: %v", menu)
	}
	// the alias completes like its expansion: create offers branches
	if got := completionCandidates(repo, []string{"cr"}); !slices.Contains(got, "main") {
		t.Errorf("alias argument completion = %v, want branch candidates", got)
	}
	if got := completionCandidates(repo, []string{"alias", "rm"}); !slices.Contains(got, "cr") {
		t.Errorf("alias rm completion = %v", got)
	}
}
