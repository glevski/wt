package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeJump = "cd -- '/x' && export WT_HOME='/y'"

// fakeUI writes a stand-in wt-ui into dir and returns its path: a script that
// reports how it was called on stderr, then runs body.
func fakeUI(t *testing.T, dir, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "wt-ui")
	script := `#!/bin/sh
echo "args: $*" >&2
echo "wrapper: [$WT_UI_WRAPPER]" >&2
echo "jump: [$WT_JUMP]" >&2
` + body + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// uiEnv hides every wt-ui the machine may have and stands in a known
// directory, so each test opts into one way of finding the binary. Returns
// the working directory wt-ui should be pointed at.
func uiEnv(t *testing.T) string {
	t.Helper()
	t.Setenv("WT_UI_BIN", "")
	t.Setenv("PATH", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("WT_WRAPPER_VERSION", "")
	t.Setenv("WT_UI_WRAPPER", "")
	t.Setenv("WT_JUMP", "")
	t.Chdir(t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return cwd
}

func TestUIJump(t *testing.T) {
	cwd := uiEnv(t)
	t.Setenv("PATH", filepath.Dir(fakeUI(t, t.TempDir(), `echo "`+fakeJump+`"`)))
	out, errOut := setupOutputs(t)

	if code := Run([]string{"ui"}); code != 0 {
		t.Fatalf("wt ui exited %d:\n%s", code, errOut.String())
	}
	if got, want := out.String(), fakeJump+"\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if want := "args: -C " + cwd + "\n"; !strings.Contains(errOut.String(), want) {
		t.Errorf("wt-ui was not run as %q:\n%s", want, errOut.String())
	}
	if strings.Contains(errOut.String(), "wt:") {
		t.Errorf("wt added narration to a successful run:\n%s", errOut.String())
	}
}

func TestUIPassesArgsAndJumpFormat(t *testing.T) {
	cwd := uiEnv(t)
	t.Setenv("WT_UI_BIN", fakeUI(t, t.TempDir(), ""))
	t.Setenv("WT_JUMP", "json")
	_, errOut := setupOutputs(t)

	if code := Run([]string{"ui", "--some", "flag"}); code != 0 {
		t.Fatalf("wt ui exited %d:\n%s", code, errOut.String())
	}
	for _, want := range []string{"args: -C " + cwd + " --some flag\n", "jump: [json]\n"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("wt-ui did not see %q:\n%s", want, errOut.String())
		}
	}
}

func TestUIExitStatus(t *testing.T) {
	for _, c := range []struct {
		name, body string
		want       int
	}{
		{"ctrl-c", "exit 130", 130},
		{"failure", "exit 1", 1},
		{"killed", "kill -TERM $$", 143},
	} {
		t.Run(c.name, func(t *testing.T) {
			uiEnv(t)
			t.Setenv("PATH", filepath.Dir(fakeUI(t, t.TempDir(), c.body)))
			out, errOut := setupOutputs(t)

			if code := Run([]string{"ui"}); code != c.want {
				t.Errorf("wt ui exited %d, want %d", code, c.want)
			}
			if out.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", out.String())
			}
			if strings.Contains(errOut.String(), "wt:") {
				t.Errorf("wt commented on wt-ui's exit status:\n%s", errOut.String())
			}
		})
	}
}

func TestUINotInstalled(t *testing.T) {
	uiEnv(t)
	out, errOut := setupOutputs(t)

	if code := Run([]string{"ui"}); code != 1 {
		t.Errorf("wt ui exited %d, want 1", code)
	}
	want := "wt: wt-ui is not installed\nhint: " + uiInstallHint + "\n"
	if errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", out.String())
	}
}

func TestUIWrapperEnv(t *testing.T) {
	uiEnv(t)
	t.Setenv("WT_UI_BIN", fakeUI(t, t.TempDir(), ""))
	_, errOut := setupOutputs(t)

	// the bare binary: wt-ui must be left to print its own reminder
	if code := Run([]string{"ui"}); code != 0 {
		t.Fatalf("wt ui exited %d:\n%s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "wrapper: []\n") {
		t.Errorf("WT_UI_WRAPPER set without the wt() function:\n%s", errOut.String())
	}

	errOut.Reset()
	t.Setenv("WT_WRAPPER_VERSION", wrapperVersion)
	if code := Run([]string{"ui"}); code != 0 {
		t.Fatalf("wt ui exited %d:\n%s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "wrapper: [1]\n") {
		t.Errorf("WT_UI_WRAPPER not set under the wt() function:\n%s", errOut.String())
	}
}

func TestUIFindsBinary(t *testing.T) {
	uiEnv(t)
	home, _ := os.UserHomeDir()
	fallback := fakeUI(t, filepath.Join(home, ".local", "bin"), "echo fallback")
	out, errOut := setupOutputs(t)
	run := func(want string) {
		t.Helper()
		out.Reset()
		if code := Run([]string{"ui"}); code != 0 {
			t.Fatalf("wt ui exited %d:\n%s", code, errOut.String())
		}
		if got := strings.TrimSpace(out.String()); got != want {
			t.Errorf("ran the %q wt-ui, want %q", got, want)
		}
	}

	// off PATH, the installer's directory still counts
	run("fallback")

	t.Setenv("PATH", filepath.Dir(fakeUI(t, t.TempDir(), "echo path")))
	run("path")

	t.Setenv("WT_UI_BIN", fakeUI(t, t.TempDir(), "echo override"))
	run("override")

	// a non-executable file in ~/.local/bin is not an installation
	t.Setenv("WT_UI_BIN", "")
	t.Setenv("PATH", "")
	if err := os.Chmod(fallback, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"ui"}); code != 1 || !strings.Contains(errOut.String(), "wt-ui is not installed") {
		t.Errorf("exit %d, stderr:\n%s", code, errOut.String())
	}
}

func TestUIBrokenOverride(t *testing.T) {
	uiEnv(t)
	t.Setenv("WT_UI_BIN", filepath.Join(t.TempDir(), "gone"))
	out, errOut := setupOutputs(t)

	if code := Run([]string{"ui"}); code != 1 {
		t.Errorf("wt ui exited %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "wt: cannot run wt-ui") {
		t.Errorf("stderr does not say what failed:\n%s", errOut.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", out.String())
	}
}
