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
	if got := out.String(); got != side+"\n" {
		t.Errorf("stdout = %q, want %q", got, side+"\n")
	}
	if !strings.Contains(errOut.String(), "→") {
		t.Errorf("log missing jump line:\n%s", errOut.String())
	}
	if _, ok := git.CheckoutStamp(side); !ok {
		t.Error("switch did not stamp the destination worktree")
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
