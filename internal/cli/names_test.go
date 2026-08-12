package cli

import "testing"

func TestDirName(t *testing.T) {
	cases := map[string]string{
		"main":          "main",
		"feature/foo":   "feature-foo",
		"fix/a/b":       "fix-a-b",
		"with space":    "with-space",
		"win\\path:x":   "win-path-x",
		"..hidden":      "hidden",
		"-leading-dash": "leading-dash",
		"///":           "wt",
		"тест/юникод":   "тест-юникод",
	}
	for in, want := range cases {
		if got := dirName(in); got != want {
			t.Errorf("dirName(%q) = %q, want %q", in, got, want)
		}
	}
}
