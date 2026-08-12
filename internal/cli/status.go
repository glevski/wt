package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"wt/internal/git"
)

// status shows where you are — project link, current worktree, its path —
// followed by plain `git status` output.
func status(dir string, args []string) error {
	if len(args) != 0 {
		return errors.New("usage: wt status")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	current, err := ws.currentWorktree()
	if err != nil {
		return err
	}

	project := ws.name
	if !ws.linked {
		project = "(not linked — run: wt link <name>)"
	}
	name := filepath.Base(current.Path)
	if current.Path == ws.repo.Worktrees[0].Path {
		name += " (main)"
	}
	fmt.Fprintf(stdout, "project   %s\n", project)
	fmt.Fprintf(stdout, "worktree  %s\n", name)
	fmt.Fprintf(stdout, "path      %s\n\n", current.Path)

	gitStatus, err := git.Run(ws.dir(), "status")
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, gitStatus)
	return nil
}
