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

// status shows a worktree — the current one, or any named one — as project
// link, worktree name and path; -g/--git appends plain `git status` output
// as if run there.
func status(dir string, args []string) error {
	fs := flag.NewFlagSet("wt status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	withGit := fs.Bool("g", false, "append git status output")
	fs.BoolVar(withGit, "git", false, "append git status output")
	if err := fs.Parse(args); err != nil || fs.NArg() > 1 {
		return errors.New("usage: wt status [-g|--git] [worktree-name]")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	current, statusDir := (*git.Worktree)(nil), ""
	if fs.NArg() == 1 {
		current, err = matchWorktree(ws.repo.Worktrees, fs.Arg(0))
		if err != nil {
			return err
		}
		statusDir = current.Path
	} else {
		current, err = ws.currentWorktree()
		if err != nil {
			return err
		}
		statusDir = ws.dir() // keep git status paths relative to where you stand
	}

	project := ws.name
	if !ws.linked {
		project = "(not linked — run: wt link <name>)"
	}
	isMain := current.Path == ws.repo.Worktrees[0].Path
	baseBranch, isBase := git.ReadBaseMark(current.Path)
	drifted := isBase && current.Branch != baseBranch
	paint := colorEnabled()
	name := filepath.Base(current.Path)
	if paint {
		managed := strings.HasPrefix(current.Path, ws.root+"/")
		name = worktreeColor(isMain, managed, isBase) + name + ansiReset
	}
	switch {
	case isMain:
		name += " (home)"
	case drifted:
		label := fmt.Sprintf(" (base: %s, drifted)", baseBranch)
		if paint {
			label = ansiRed + label + ansiReset
		}
		name += label
	case isBase:
		name += " (base)"
	}
	branch := current.Branch
	if current.Detached {
		branch = "(detached)"
	}
	if drifted && paint {
		branch = ansiRed + branch + ansiReset
	}
	fmt.Fprintf(stdout, "project   %s\n", project)
	fmt.Fprintf(stdout, "worktree  %s\n", name)
	fmt.Fprintf(stdout, "branch    %s\n", branch)
	fmt.Fprintf(stdout, "commit    %s\n", shortSHA(current.Head))
	fmt.Fprintf(stdout, "path      %s\n", current.Path)
	if !*withGit {
		return nil
	}
	fmt.Fprintln(stdout)
	// git inherits our stdout/stderr so it does its own TTY detection —
	// colors and status config behave exactly like a hand-typed git status.
	return git.RunPassthrough(statusDir, stdout, stderr, "status")
}
