// Package config resolves where worktree directories are created.
package config

import (
	"os"
	"path/filepath"
	"strings"

	"wt/internal/git"
)

// CopyIgnored reports whether create/fork should copy git-ignored files
// (.env, node_modules, …) into new worktrees so they are runnable
// immediately. Default true; disable with `git config wt.copyignored false`.
func CopyIgnored(repoDir string) bool {
	v, _ := git.Run(repoDir, "config", "--get", "--type=bool", "wt.copyignored")
	return v != "false"
}

// Name returns the project name from `git config wt.name`, ok=false when unset.
func Name(repoDir string) (string, bool) {
	name, err := git.Run(repoDir, "config", "--get", "wt.name")
	if err != nil || name == "" {
		return "", false
	}
	return name, true
}

// Root returns the base directory for worktrees: $WT_ROOT, then
// `git config wt.root`, then ~/worktrees. "~" and relative paths resolve
// against the home directory.
func Root(repoDir string) (string, error) {
	root := os.Getenv("WT_ROOT")
	if root == "" {
		root, _ = git.Run(repoDir, "config", "--get", "wt.root") // exit 1 just means unset
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch {
	case root == "":
		return filepath.Join(home, "worktrees"), nil
	case root == "~":
		return home, nil
	case strings.HasPrefix(root, "~/"):
		return filepath.Join(home, root[2:]), nil
	case !filepath.IsAbs(root):
		return filepath.Join(home, root), nil
	default:
		return root, nil
	}
}
