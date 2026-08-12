package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"wt/internal/git"
)

// status shows where you are — project link, current worktree, its path —
// and with -g/--git appends plain `git status` output.
func status(dir string, args []string) error {
	fs := flag.NewFlagSet("wt status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	withGit := fs.Bool("g", false, "append git status output")
	fs.BoolVar(withGit, "git", false, "append git status output")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return errors.New("usage: wt status [-g|--git]")
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
		name += " (home)"
	}
	fmt.Fprintf(stdout, "project   %s\n", project)
	fmt.Fprintf(stdout, "worktree  %s\n", name)
	fmt.Fprintf(stdout, "path      %s\n", current.Path)
	if !*withGit {
		return nil
	}
	fmt.Fprintln(stdout)
	// git inherits our stdout/stderr so it does its own TTY detection —
	// colors and status config behave exactly like a hand-typed git status.
	return git.RunPassthrough(ws.dir(), stdout, stderr, "status")
}
