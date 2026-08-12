package cli

import "strings"

// worktreeName resolves an explicit -n/--name override against the branch the
// worktree is for: empty derives the name from the branch, and a leading dash
// appends to it — on branch main, "-fix" becomes "main-fix".
func worktreeName(branch, override string) (string, error) {
	if override == "" {
		return dirName(branch), nil
	}
	name := override
	if strings.HasPrefix(override, "-") {
		name = dirName(branch) + override
	}
	if name != dirName(name) {
		return "", hintf("use: -n "+dirName(name), "worktree name '%s' is not filesystem-safe", name)
	}
	return name, nil
}

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
