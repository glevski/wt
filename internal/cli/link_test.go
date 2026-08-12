package cli

import (
	"errors"
	"strings"
	"testing"

	"wt/internal/gittest"
)

func TestLinkSetsName(t *testing.T) {
	repo := gittest.NewRepo(t)
	wtRoot(t)
	_, errOut := setupOutputs(t)

	if err := link(repo, []string{"myproj"}); err != nil {
		t.Fatal(err)
	}
	if got := gittest.Git(t, repo, "config", "--get", "wt.name"); got != "myproj" {
		t.Errorf("wt.name = %q, want myproj", got)
	}
	if !strings.Contains(errOut.String(), "linked to 'myproj'") {
		t.Errorf("log:\n%s", errOut.String())
	}
}

func TestLinkRejectsUnsafeName(t *testing.T) {
	repo := gittest.NewRepo(t)
	wtRoot(t)
	setupOutputs(t)

	err := link(repo, []string{"feature/x"})
	if err == nil || !strings.Contains(err.Error(), "not filesystem-safe") {
		t.Fatalf("err = %v", err)
	}
	var hint *hintError
	if !errors.As(err, &hint) || !strings.Contains(hint.hint, "feature-x") {
		t.Errorf("hint should suggest the sanitized name, got %v", err)
	}
}

func TestLinkRelink(t *testing.T) {
	repo := gittest.NewRepo(t)
	wtRoot(t)
	_, errOut := setupOutputs(t)
	gittest.Git(t, repo, "config", "wt.name", "old")

	if err := link(repo, []string{"new"}); err != nil {
		t.Fatal(err)
	}
	if got := gittest.Git(t, repo, "config", "--get", "wt.name"); got != "new" {
		t.Errorf("wt.name = %q, want new", got)
	}
	if !strings.Contains(errOut.String(), "relinked from 'old' to 'new'") {
		t.Errorf("log:\n%s", errOut.String())
	}
}

func TestLinkStatus(t *testing.T) {
	repo := gittest.NewRepo(t)
	wtRoot(t)
	_, errOut := setupOutputs(t)

	if err := link(repo, nil); err == nil || !strings.Contains(err.Error(), "not linked") {
		t.Fatalf("unlinked status err = %v", err)
	}

	gittest.Git(t, repo, "config", "wt.name", "proj")
	if err := link(repo, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "linked to 'proj'") {
		t.Errorf("log:\n%s", errOut.String())
	}
}
