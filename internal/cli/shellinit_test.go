package cli

import (
	"strings"
	"testing"
)

func TestShellInit(t *testing.T) {
	out, _ := setupOutputs(t)
	if err := shellInit([]string{"zsh"}); err != nil {
		t.Fatal(err)
	}
	fn := out.String()
	for _, want := range []string{"wt() {", "checkout|ch|create|fork|home|switch) _wt_jump=1 ;;",
		`root) case "$2" in checkout|ch|create|fork) _wt_jump=1 ;; esac ;;`,
		`eval "$_wt_script"`, `_wt_prev="$PWD"`, `WT_PREV="${_wt_prev:-}"`,
		"WT_WRAPPER_VERSION=" + wrapperVersion + " "} {
		if !strings.Contains(fn, want) {
			t.Errorf("emitted function missing %q:\n%s", want, fn)
		}
	}
}

func TestShellInitRejectsUnknownShell(t *testing.T) {
	setupOutputs(t)
	if err := shellInit([]string{"fish"}); err == nil {
		t.Fatal("expected an error for unsupported shell")
	}
	if err := shellInit(nil); err == nil {
		t.Fatal("expected an error for missing shell")
	}
}
