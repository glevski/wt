package cli

import (
	"fmt"
	"runtime/debug"
)

// version and commit are stamped by the Makefile via -ldflags -X, so the
// reported version matches the release tag (git describe). A plain
// `go build` leaves them empty; the commit then falls back to the VCS info
// Go embeds on its own.
var (
	version = ""
	commit  = ""
)

func versionString() string {
	v := version
	if v == "" {
		v = "dev"
	}
	c := commit
	if c == "" {
		c = vcsRevision()
	}
	if c != "" {
		return fmt.Sprintf("wt %s (%s)", v, c)
	}
	return "wt " + v
}

func vcsRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 7 {
			return s.Value[:7]
		}
	}
	return ""
}
