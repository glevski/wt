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

func create(dir string, args []string) error {
	fs := flag.NewFlagSet("wt create", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	checkout := fs.Bool("c", false, "cd into the new worktree")
	fs.BoolVar(checkout, "checkout", false, "cd into the new worktree")
	if err := fs.Parse(args); err != nil || fs.NArg() > 1 {
		return errors.New("usage: wt create [-c|--checkout] [branch]")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	if !ws.linked {
		return ws.requireLink()
	}
	if fs.NArg() == 0 {
		return createFromHead(ws, *checkout)
	}
	return createForBranch(ws, fs.Arg(0), *checkout)
}

// createForBranch adds a worktree for an existing branch: the local one when
// it exists, otherwise a new tracking branch from a remote.
func createForBranch(ws *workspace, branch string, checkout bool) error {
	refs, err := git.LookupBranch(ws.dir(), branch)
	if err != nil {
		return err
	}
	switch {
	case refs.Local:
		return createFromLocal(ws, branch, checkout)
	case len(refs.Remotes) > 0:
		return createFromRemote(ws, branch, refs.Remotes, checkout)
	default:
		return hintf(
			fmt.Sprintf("fetch first (git fetch), or start a new branch: wt fork %s", branch),
			"branch '%s' not found locally or on any remote", branch)
	}
}

func createFromLocal(ws *workspace, branch string, checkout bool) error {
	if wt := ws.repo.CheckedOut(branch); wt != nil {
		return hintf("wt ch "+filepath.Base(wt.Path),
			"branch '%s' is already checked out at %s", branch, wt.Path)
	}
	path, err := ws.targetPath(branch)
	if err != nil {
		return err
	}
	if _, err := git.Run(ws.dir(), "worktree", "add", path, branch); err != nil {
		return err
	}
	logf("branch '%s' found locally %s", branch, upstreamNote(ws.dir(), branch))
	reportCreated(path, checkout)
	return nil
}

func createFromRemote(ws *workspace, branch string, remotes []string, checkout bool) error {
	remote, err := pickRemote(branch, remotes)
	if err != nil {
		return err
	}
	path, err := ws.targetPath(branch)
	if err != nil {
		return err
	}
	if _, err := git.Run(ws.dir(), "worktree", "add", "--track", "-b", branch, path, remote+"/"+branch); err != nil {
		return err
	}
	logf("branch '%s' not found locally; created from %s/%s (tracking it)", branch, remote, branch)
	reportCreated(path, checkout)
	return nil
}

// createFromHead adds a clean worktree at the current commit on a new
// auto-named branch; local changes stay behind (use fork to carry them).
func createFromHead(ws *workspace, checkout bool) error {
	source, err := ws.currentWorktree()
	if err != nil {
		return err
	}
	if source.Branch == "" {
		return hintf("wt fork <name> creates a named branch here",
			"detached HEAD — cannot derive a branch name")
	}
	branch, path, err := ws.freeName(source.Branch)
	if err != nil {
		return err
	}
	if _, err := git.Run(ws.dir(), "worktree", "add", "-b", branch, path, "HEAD"); err != nil {
		return err
	}
	logf("created new branch '%s' from HEAD (%s), without your local changes", branch, shortSHA(source.Head))
	reportCreated(path, checkout)
	return nil
}

func pickRemote(branch string, remotes []string) (string, error) {
	if len(remotes) == 1 {
		return remotes[0], nil
	}
	for _, r := range remotes {
		if r == "origin" {
			return r, nil
		}
	}
	return "", hintf(
		fmt.Sprintf("pick one: git branch --track %s <remote>/%s, then wt create %s", branch, branch, branch),
		"branch '%s' exists on multiple remotes (%s) and none is origin", branch, strings.Join(remotes, ", "))
}

func upstreamNote(dir, branch string) string {
	up, ok := git.UpstreamOf(dir, branch)
	switch {
	case !ok:
		return "(no upstream)"
	case up.Track == "":
		return fmt.Sprintf("(in sync with %s)", up.Name)
	default:
		return fmt.Sprintf("(%s: %s)", up.Name, up.Track)
	}
}

func reportCreated(path string, checkout bool) {
	logf("created worktree '%s' at %s", filepath.Base(path), path)
	reportSwitch(path, checkout)
}

// reportSwitch either emits the path for the wt() shell function to cd into
// (that's all -c/--checkout is: a path on stdout), or says how to get there.
func reportSwitch(path string, checkout bool) {
	if checkout {
		fmt.Fprintln(stdout, path)
		return
	}
	logf("switch with: wt ch %s", filepath.Base(path))
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
