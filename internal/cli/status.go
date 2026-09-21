package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"wt/internal/config"
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
		logf("exit with: wt peek off (or wt unpeek)")
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
	// \x1f keeps author/date and subject splittable whatever they contain
	if info, err := git.Run(ws.dir(), "log", "-1", "--format=%an, %ar\x1f%s", current.Head); err == nil && info != "" {
		meta, subject, _ := strings.Cut(info, "\x1f")
		commit += "  " + meta
		if subject != "" {
			commit += ": " + truncate(subject, 40)
		}
	}
	fmt.Fprintf(stdout, "project   %s\n", project)
	fmt.Fprintf(stdout, "worktree  %s\n", name)
	fmt.Fprintf(stdout, "branch    %s\n", branch)
	fmt.Fprintf(stdout, "commit    %s\n", commit)
	fmt.Fprintf(stdout, "path      %s\n", current.Path)
	if state, ok := git.DepsState(current.Path); ok {
		switch {
		case strings.HasPrefix(state, "copying"):
			state = "copying… (log: " + git.DepsLogPath(current.Path) + ")"
		case state == "done":
			state = "copied"
		case state == "linked":
			state = "linked (shared hardlinks — wt deps eject to own them)"
		}
		fmt.Fprintf(stdout, "deps      %s\n", state)
	} else if declared := config.Deps(ws.dir()); len(declared) > 0 {
		// no transfer ever happened here (the root repo, or a pre-wt
		// worktree) — still say what this project declares
		fmt.Fprintf(stdout, "deps      declared: %s\n", strings.Join(declared, ", "))
	}
	if series := loadSnapshots(current.Path)[current.Head]; len(series) > 0 {
		last := series[len(series)-1]
		fmt.Fprintf(stdout, "snapshots %d (latest %q, %s)\n", len(series), last.Message, agoPhrase(last.When))
	}
	if !*withGit {
		return nil
	}
	fmt.Fprintln(stdout)
	// git inherits our stdout/stderr so it does its own TTY detection —
	// colors and status config behave exactly like a hand-typed git status.
	return git.RunPassthrough(statusDir, stdout, stderr, "status")
}

// truncate caps s at max runes, marking the cut with an ellipsis.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
