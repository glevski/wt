package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"wt/internal/git"
)

const removeUsage = "usage: wt remove [-f] [-b] <worktree-name | 'pattern'>"

// remove deletes a worktree; the branch stays unless -b asks for a safe
// delete. A glob pattern ('dev-*', '*') bulk-removes matching wt-managed
// worktrees — always listing them and asking for confirmation first.
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
	name := fs.Args()[0]
	if strings.ContainsAny(name, "*?[") {
		return removeMatching(ws, name, *force, *deleteBranch)
	}

	wt, err := matchWorktree(ws.repo.Worktrees, name)
	if err != nil {
		return err
	}
	name = filepath.Base(wt.Path)
	if wt.Path == ws.repo.Worktrees[0].Path {
		return fmt.Errorf("'%s' is the main worktree and cannot be removed", name)
	}
	if _, isBase := git.ReadBaseMark(wt.Path); isBase {
		return hintf("wt base rm "+name, "'%s' is a base worktree", name)
	}
	if cur := ws.repo.Current(); cur != nil && cur.Path == wt.Path {
		return hintf("wt ch "+ws.repo.Name+" first", "you are inside '%s'", name)
	}
	if state, ok := git.DepsState(wt.Path); ok && state == "copying" && !*force {
		return hintf("wait for it, or discard with: wt rm -f "+name, "'%s' is still syncing deps", name)
	}
	return removeWorktree(ws, wt, *force, *deleteBranch)
}

// removeMatching is the wildcard path: collect removable wt-managed matches,
// show them, confirm, then remove each with the same flags.
func removeMatching(ws *workspace, pattern string, force, deleteBranch bool) error {
	current := ws.repo.Current()
	var candidates []*git.Worktree
	for i := range ws.repo.Worktrees {
		wt := &ws.repo.Worktrees[i]
		if wt.Path == ws.repo.Worktrees[0].Path {
			continue
		}
		if !strings.HasPrefix(wt.Path, ws.root+"/") {
			continue // wildcards only touch wt-managed worktrees
		}
		name := filepath.Base(wt.Path)
		matched, err := path.Match(pattern, name)
		if err != nil {
			return fmt.Errorf("bad pattern '%s': %w", pattern, err)
		}
		if !matched {
			continue
		}
		if _, isBase := git.ReadBaseMark(wt.Path); isBase {
			logf("skipping base worktree '%s' (wt base rm removes bases)", name)
			continue
		}
		if current != nil && wt.Path == current.Path {
			logf("skipping '%s' — you are inside it", name)
			continue
		}
		if state, ok := git.DepsState(wt.Path); ok && state == "copying" && !force {
			logf("skipping '%s' — deps still syncing", name)
			continue
		}
		candidates = append(candidates, wt)
	}
	if len(candidates) == 0 {
		return fmt.Errorf("no removable worktrees match '%s'", pattern)
	}

	logf("matching worktrees:")
	for _, wt := range candidates {
		branch := wt.Branch
		if branch == "" {
			branch = "(detached)"
		}
		fmt.Fprintf(stderr, "  %s  (branch %s)\n", filepath.Base(wt.Path), branch)
	}
	if !confirm(fmt.Sprintf("remove %d worktree(s)?", len(candidates))) {
		return errors.New("aborted")
	}

	failed := 0
	for _, wt := range candidates {
		if err := removeWorktree(ws, wt, force, deleteBranch); err != nil {
			logf("%s: %v", filepath.Base(wt.Path), err)
			failed++
		}
	}
	if failed == 0 {
		return nil
	}
	if !force {
		return hintf(fmt.Sprintf("wt rm -f '%s' retries discarding local changes", pattern),
			"%d of %d removals failed", failed, len(candidates))
	}
	return fmt.Errorf("%d of %d removals failed", failed, len(candidates))
}

// removeWorktree removes one worktree (guards already done by the caller).
func removeWorktree(ws *workspace, wt *git.Worktree, force, deleteBranch bool) error {
	name := filepath.Base(wt.Path)
	removeArgs := []string{"worktree", "remove"}
	if force {
		removeArgs = append(removeArgs, "--force")
	}
	if _, err := git.Run(ws.dir(), append(removeArgs, wt.Path)...); err != nil {
		if !force {
			return hintf(fmt.Sprintf("wt rm -f %s discards them", name), "%v", err)
		}
		return err
	}
	logf("removed worktree '%s'", name)

	switch {
	case wt.Branch == "":
	case deleteBranch:
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
