package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"wt/internal/git"
)

const resetUsage = "usage: wt reset [--hard] [base-branch]"

// reset moves the worktree's branch back to its base branch's tip, staying on
// the same branch. The base is recorded when wt creates the worktree; an
// explicit argument overrides it (and is the only way for worktrees created
// without one).
func reset(dir string, args []string) error {
	fs := flag.NewFlagSet("wt reset", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	hard := fs.Bool("hard", false, "discard tracked changes")
	if err := fs.Parse(args); err != nil || fs.NArg() > 1 {
		return errors.New(resetUsage)
	}

	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	current, err := ws.currentWorktree()
	if err != nil {
		return err
	}
	if current.Path == ws.repo.Worktrees[0].Path {
		return errors.New("reset only works in a worktree, not the main checkout")
	}
	if current.Branch == "" {
		return errors.New("detached HEAD — there is no branch to move")
	}

	base := fs.Arg(0)
	if base == "" {
		recorded, ok := git.BaseBranch(current.Path)
		if !ok {
			return hintf("wt reset <branch> names it explicitly",
				"no base branch recorded for this worktree")
		}
		base = recorded
	}
	tip, err := git.Run(ws.dir(), "rev-parse", "--verify", "--quiet", base)
	if err != nil || tip == "" {
		return fmt.Errorf("base '%s' does not resolve to a commit", base)
	}

	if !*hard {
		status, err := git.Run(current.Path, "status", "--porcelain")
		if err != nil {
			return err
		}
		if status != "" {
			return hintf("wt reset --hard discards tracked changes (untracked files are kept)",
				"worktree has local changes")
		}
	}

	if _, err := git.Run(current.Path, "reset", "--hard", tip); err != nil {
		return err
	}
	logf("reset '%s' to '%s' (%s, was %s)", current.Branch, base, shortSHA(tip), shortSHA(current.Head))
	return nil
}
