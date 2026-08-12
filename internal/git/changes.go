package git

import "strings"

// StagedFiles returns paths with changes staged in the index.
func StagedFiles(worktreeRoot string) ([]string, error) {
	return pathList(worktreeRoot, "diff", "--cached", "--name-only", "-z")
}

// UnstagedFiles returns tracked paths with changes not yet staged.
func UnstagedFiles(worktreeRoot string) ([]string, error) {
	return pathList(worktreeRoot, "diff", "--name-only", "-z")
}

// UntrackedFiles returns non-ignored untracked paths, relative to worktreeRoot.
func UntrackedFiles(worktreeRoot string) ([]string, error) {
	return pathList(worktreeRoot, "ls-files", "--others", "--exclude-standard", "-z")
}

func pathList(dir string, args ...string) ([]string, error) {
	out, err := Run(dir, args...)
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(strings.TrimRight(out, "\x00"), "\x00"), nil
}
