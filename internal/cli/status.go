package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

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
	isMain := current.Path == ws.repo.Worktrees[0].Path
	name := filepath.Base(current.Path)
	if colorEnabled() {
		managed := strings.HasPrefix(current.Path, ws.root+"/")
		name = worktreeColor(isMain, managed) + name + ansiReset
	}
	if isMain {
		name += " (main)"
	}
	fmt.Fprintf(stdout, "project   %s\n", project)
	fmt.Fprintf(stdout, "worktree  %s\n", name)
	fmt.Fprintf(stdout, "path      %s\n\n", current.Path)

	// git inherits our stdout/stderr so it does its own TTY detection —
	// colors and status config behave exactly like a hand-typed git status.
	return git.RunPassthrough(ws.dir(), stdout, stderr, "status")
}
