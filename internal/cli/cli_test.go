package cli

import (
	"strings"
	"testing"
)

// The alias tests go through Run, which resolves the repo from the process
// working directory — the wt repo itself, which is fine for routing checks.
func TestAliases(t *testing.T) {
	out, errOut := setupOutputs(t)

	if code := Run([]string{"ls"}); code != 0 {
		t.Errorf("wt ls exited %d:\n%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "BRANCH") {
		t.Errorf("wt ls did not produce the list table:\n%s", out.String())
	}

	if code := Run([]string{"rm"}); code != 1 {
		t.Errorf("wt rm without args exited %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "usage: wt remove") {
		t.Errorf("wt rm did not route to remove:\n%s", errOut.String())
	}

	if code := Run([]string{"ch"}); code != 1 {
		t.Errorf("wt ch without args exited %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "usage: wt checkout") {
		t.Errorf("wt ch did not route to checkout:\n%s", errOut.String())
	}
}

func TestOutdatedWrapperWarning(t *testing.T) {
	_, errOut := setupOutputs(t)

	t.Setenv("WT_WRAPPER_VERSION", "0")
	Run([]string{"ls"})
	if !strings.Contains(errOut.String(), "outdated") {
		t.Errorf("no staleness warning with an old wrapper version:\n%s", errOut.String())
	}

	errOut.Reset()
	t.Setenv("WT_WRAPPER_VERSION", wrapperVersion)
	Run([]string{"ls"})
	if strings.Contains(errOut.String(), "outdated") {
		t.Error("staleness warning fired for the current wrapper version")
	}
}
