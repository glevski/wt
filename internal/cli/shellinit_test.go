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
	for _, want := range []string{"wt() {", "checkout|ch|create|fork|home|switch|peek|unpeek) _wt_jump=1 ;;",
		`root) case "$2" in checkout|ch|create|fork) _wt_jump=1 ;; esac ;;`,
		`"$_wt_bin" __jump-alias "$1" "$2"`,
		`eval "$_wt_script"`, `_wt_prev="$PWD"`, `WT_PREV="${_wt_prev:-}"`,
		"WT_WRAPPER_VERSION=" + wrapperVersion + " ",
		"_describe", "autoload -Uz compinit", "compdef _wt wt worktree"} {
		if !strings.Contains(fn, want) {
			t.Errorf("emitted zsh setup missing %q:\n%s", want, fn)
		}
	}
}

func TestShellInitBashCompletion(t *testing.T) {
	out, _ := setupOutputs(t)
	if err := shellInit([]string{"bash"}); err != nil {
		t.Fatal(err)
	}
	fn := out.String()
	for _, want := range []string{"wt() {", "_wt_complete()", "COMPREPLY",
		"complete -F _wt_complete wt worktree"} {
		if !strings.Contains(fn, want) {
			t.Errorf("emitted bash setup missing %q:\n%s", want, fn)
		}
	}
	if strings.Contains(fn, "compdef") {
		t.Error("bash setup must not contain zsh compdef")
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
