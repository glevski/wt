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
	// Inside a peek there is no repo to load — show the peek instead.
	if root, ok := findPeekRoot(dir); ok && fs.NArg() == 0 {
		info, _ := readPeekInfo(root)
		fmt.Fprintf(stdout, "peek      %s\n", filepath.Base(root))
		fmt.Fprintf(stdout, "rev       %s\n", info.Rev)
		fmt.Fprintf(stdout, "commit    %s\n", shortSHA(info.SHA))
		fmt.Fprintf(stdout, "source    %s\n", info.Source)
		fmt.Fprintf(stdout, "age       %s\n", ago(info.Created))
		if state, sok := git.DepsState(root); sok && state != "done" {
			if strings.HasPrefix(state, "copying") {
				state = "copying…"
			}
			fmt.Fprintf(stdout, "deps      %s\n", state)
		}
		return nil
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
	commit := shortSHA(current.Head)
	if info, err := git.Run(ws.dir(), "log", "-1", "--format=%an, %ar", current.Head); err == nil && info != "" {
		commit += "  " + info
	}
	fmt.Fprintf(stdout, "project   %s\n", project)
	fmt.Fprintf(stdout, "worktree  %s\n", name)
	fmt.Fprintf(stdout, "branch    %s\n", branch)
	fmt.Fprintf(stdout, "commit    %s\n", commit)
	fmt.Fprintf(stdout, "path      %s\n", current.Path)
	if state, ok := git.DepsState(current.Path); ok && state != "done" {
		if strings.HasPrefix(state, "copying") {
			state = "copying… (log: " + git.DepsLogPath(current.Path) + ")"
		}
		fmt.Fprintf(stdout, "deps      %s\n", state)
	}
	if !*withGit {
		return nil
	}
	fmt.Fprintln(stdout)
	// git inherits our stdout/stderr so it does its own TTY detection —
	// colors and status config behave exactly like a hand-typed git status.
	return git.RunPassthrough(statusDir, stdout, stderr, "status")
}
