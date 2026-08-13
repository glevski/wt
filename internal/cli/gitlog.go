package cli

import (
	"strings"

	"wt/internal/git"
)

// gitLog runs plain `git log` in a worktree picked by name (default: the
// one you stand in), unique prefixes included. Passthrough, so colors and
// the pager behave exactly like a hand-typed git log; any extra arguments
// go to git log verbatim.
func gitLog(dir string, args []string) error {
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	var target *git.Worktree
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		target, err = matchWorktree(ws.repo.Worktrees, args[0])
		args = args[1:]
	} else {
		target, err = ws.currentWorktree()
	}
	if err != nil {
		return err
	}
	return git.RunPassthrough(target.Path, stdout, stderr, append([]string{"log"}, args...)...)
}
