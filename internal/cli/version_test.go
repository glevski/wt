package cli

import (
	"strings"
	"testing"
)

func TestVersionString(t *testing.T) {
	origV, origC := version, commit
	t.Cleanup(func() { version, commit = origV, origC })

	version, commit = "0.0.7", "abc1234"
	if got := versionString(); got != "wt 0.0.7 (abc1234)" {
		t.Errorf("versionString() = %q", got)
	}
	version, commit = "", "abc1234"
	if got := versionString(); got != "wt dev (abc1234)" {
		t.Errorf("versionString() = %q", got)
	}
}

func TestRunVersion(t *testing.T) {
	for _, arg := range []string{"--version", "version"} {
		out, _ := setupOutputs(t)
		if code := Run([]string{arg}); code != 0 {
			t.Fatalf("wt %s exit code = %d", arg, code)
		}
		if !strings.HasPrefix(out.String(), "wt ") {
			t.Errorf("wt %s output = %q", arg, out.String())
		}
	}
}
