package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/git"
	"wt/internal/gittest"
)

func TestSwitch(t *testing.T) {
	repo := gittest.NewRepo(t)
	wtRoot(t)
	side := filepath.Join(t.TempDir(), "side")
	gittest.Git(t, repo, "worktree", "add", "-b", "side", side)
	out, errOut := setupOutputs(t)

	t.Setenv("WT_PREV", side)
	if err := switchTo(nil); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), jumpScript(side, repo); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if !strings.Contains(errOut.String(), "→") {
		t.Errorf("log missing jump line:\n%s", errOut.String())
	}
	if _, ok := git.CheckoutStamp(side); !ok {
		t.Error("switch did not stamp the destination worktree")
	}
}

func TestSwitchOutsideAnyRepo(t *testing.T) {
	out, _ := setupOutputs(t)
	prev := t.TempDir()
	t.Setenv("WT_PREV", prev)

	if err := switchTo(nil); err != nil {
		t.Fatal(err)
	}
	// no repo context → cd only, WT_HOME untouched
	if got, want := out.String(), "cd '"+prev+"'\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestSwitchWithoutHistory(t *testing.T) {
	setupOutputs(t)
	t.Setenv("WT_PREV", "")

	err := switchTo(nil)
	if err == nil || !strings.Contains(err.Error(), "no previous location") {
		t.Fatalf("err = %v", err)
	}
}

func TestSwitchToVanishedLocation(t *testing.T) {
	setupOutputs(t)
	t.Setenv("WT_PREV", filepath.Join(t.TempDir(), "gone"))

	err := switchTo(nil)
	if err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("err = %v", err)
	}
}
