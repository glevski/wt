package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"wt/internal/git"
)

func create(dir string, args []string) error {
	if len(args) > 1 {
		return errors.New("usage: wt create [branch]")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	if !ws.linked {
		return ws.requireLink()
	}
	if len(args) == 0 {
		return createFromHead(ws)
	}
	return createForBranch(ws, args[0])
}

// createForBranch adds a worktree for an existing branch: the local one when
// it exists, otherwise a new tracking branch from a remote.
func createForBranch(ws *workspace, branch string) error {
	refs, err := git.LookupBranch(ws.dir(), branch)
	if err != nil {
		return err
	}
	switch {
	case refs.Local:
		return createFromLocal(ws, branch)
	case len(refs.Remotes) > 0:
		return createFromRemote(ws, branch, refs.Remotes)
	default:
		return hintf(
			fmt.Sprintf("fetch first (git fetch), or start a new branch: wt fork %s", branch),
			"branch '%s' not found locally or on any remote", branch)
	}
}

func createFromLocal(ws *workspace, branch string) error {
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
	reportCreated(path)
	return nil
}

func createFromRemote(ws *workspace, branch string, remotes []string) error {
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
	reportCreated(path)
	return nil
}

// createFromHead adds a clean worktree at the current commit on a new
// auto-named branch; local changes stay behind (use fork to carry them).
func createFromHead(ws *workspace) error {
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
	reportCreated(path)
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

func reportCreated(path string) {
	name := filepath.Base(path)
	logf("created worktree '%s' at %s", name, path)
	logf("switch with: wt ch %s", name)
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
