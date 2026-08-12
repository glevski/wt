package cli

import "strings"

// dirName maps a branch name to a filesystem-safe worktree directory name,
// e.g. "feature/foo" → "feature-foo".
func dirName(branch string) string {
	safe := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', ' ':
			return '-'
		}
		if r < 0x20 {
			return '-'
		}
		return r
	}, branch)
	safe = strings.TrimLeft(safe, "-.")
	if safe == "" {
		return "wt"
	}
	return safe
}
