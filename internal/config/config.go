// Package config resolves where worktree directories are created.
package config

import (
	"os"
	"path/filepath"
	"strings"

	"wt/internal/git"
)

// Deps returns the declared dependency paths (`git config --get-all wt.deps`)
// that create/fork copy asynchronously into new worktrees.
func Deps(repoDir string) []string {
	out, err := git.Run(repoDir, "config", "--get-all", "wt.deps")
	if err != nil || out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// Alias returns the words a user-defined alias (`git config wt.alias.<name>`)
// expands to, nil when undefined. Works outside repos too: git then reads
// only the global config.
func Alias(dir, name string) []string {
	out, err := git.Run(dir, "config", "--get", "wt.alias."+name)
	if err != nil || out == "" {
		return nil
	}
	return strings.Fields(out)
}

// Aliases returns all defined aliases as {name, expansion} pairs, keeping
// git's order; for a name set in both global and local config the local
// value wins, matching what Alias resolves.
func Aliases(dir string) [][2]string {
	out, err := git.Run(dir, "config", "--get-regexp", `^wt\.alias\.`)
	if err != nil || out == "" {
		return nil
	}
	index := make(map[string]int)
	var pairs [][2]string
	for _, line := range strings.Split(out, "\n") {
		key, value, _ := strings.Cut(line, " ")
		name := strings.TrimPrefix(key, "wt.alias.")
		if i, seen := index[name]; seen {
			pairs[i][1] = value
			continue
		}
		index[name] = len(pairs)
		pairs = append(pairs, [2]string{name, value})
	}
	return pairs
}

// Projects returns the globally registered {name, root repo path} pairs
// (`git config --global wt.project.<name>`), the registry behind wt global.
// Works outside any repo.
func Projects(dir string) [][2]string {
	out, err := git.Run(dir, "config", "--get-regexp", `^wt\.project\.`)
	if err != nil || out == "" {
		return nil
	}
	index := make(map[string]int)
	var pairs [][2]string
	for _, line := range strings.Split(out, "\n") {
		key, value, _ := strings.Cut(line, " ")
		name := strings.TrimPrefix(key, "wt.project.")
		if i, seen := index[name]; seen {
			pairs[i][1] = value
			continue
		}
		index[name] = len(pairs)
		pairs = append(pairs, [2]string{name, value})
	}
	return pairs
}

// ProjectPath returns the registered root repo path of a project.
func ProjectPath(dir, name string) (string, bool) {
	path, err := git.Run(dir, "config", "--get", "wt.project."+name)
	if err != nil || path == "" {
		return "", false
	}
	return path, true
}

// RegisterProject records a project's root repo path in the global registry.
func RegisterProject(dir, name, path string) error {
	_, err := git.Run(dir, "config", "--global", "wt.project."+name, path)
	return err
}

// UnregisterProject drops a project from the global registry.
func UnregisterProject(dir, name string) {
	_, _ = git.Run(dir, "config", "--global", "--unset", "wt.project."+name)
}

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
