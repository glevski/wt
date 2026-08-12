package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"wt/internal/git"
)

const removeUsage = "usage: wt remove [-f] [-b] <worktree-name>"

// remove deletes a worktree; the branch stays unless -b asks for a safe delete.
func remove(dir string, args []string) error {
	fs := flag.NewFlagSet("wt remove", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	force := fs.Bool("f", false, "remove even when the worktree is dirty")
	deleteBranch := fs.Bool("b", false, "also delete the branch (git branch -d)")
	if err := fs.Parse(args); err != nil || len(fs.Args()) != 1 {
		return errors.New(removeUsage)
	}

	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	wt, err := matchWorktree(ws.repo.Worktrees, fs.Args()[0])
	if err != nil {
		return err
	}
	name := filepath.Base(wt.Path)
	if wt.Path == ws.repo.Worktrees[0].Path {
		return fmt.Errorf("'%s' is the main worktree and cannot be removed", name)
	}
	if cur := ws.repo.Current(); cur != nil && cur.Path == wt.Path {
		return hintf("wt ch "+ws.repo.Name+" first", "you are inside '%s'", name)
	}

	removeArgs := []string{"worktree", "remove"}
	if *force {
		removeArgs = append(removeArgs, "--force")
	}
	if _, err := git.Run(ws.dir(), append(removeArgs, wt.Path)...); err != nil {
		if !*force {
			return hintf(fmt.Sprintf("wt rm -f %s discards them", name), "%v", err)
		}
		return err
	}
	logf("removed worktree '%s'", name)

	switch {
	case wt.Branch == "":
	case *deleteBranch:
		if _, err := git.Run(ws.dir(), "branch", "-d", wt.Branch); err != nil {
			return hintf(fmt.Sprintf("git branch -D %s discards it", wt.Branch),
				"worktree removed, but the branch is kept: %v", err)
		}
		logf("deleted branch '%s'", wt.Branch)
	default:
		logf("branch '%s' kept (delete: git branch -d %s)", wt.Branch, wt.Branch)
	}
	return nil
}
