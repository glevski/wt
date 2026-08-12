package cli

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"

	"wt/internal/config"
	"wt/internal/git"
)

// complete is the plumbing behind tab completion: the generated shell
// functions hand over the words typed before the cursor (after "wt" itself)
// and get candidates back on stdout, one per line — "name:description" for
// menus, plain names otherwise. Everything fails silent: a broken completion
// must never spam the prompt.
func complete(dir string, words []string) error {
	for _, c := range completionCandidates(dir, words) {
		fmt.Fprintln(stdout, c)
	}
	return nil
}

var commandMenu = []string{
	"create:add a worktree for a branch",
	"fork:fork current state into a new worktree",
	"checkout:jump to a worktree",
	"ch:jump to a worktree (alias)",
	"home:jump to the main checkout",
	"switch:toggle current and previous location",
	"root:run a command in the root repo's context",
	"base:manage base branches",
	"list:list worktrees",
	"ls:list worktrees (alias)",
	"status:show a worktree",
	"remove:remove a worktree",
	"rm:remove a worktree (alias)",
	"reset:move the branch back to its base",
	"link:link the repo to a project name",
	"deps:manage dependency paths copied to new worktrees",
	"init:print shell integration",
}

var depsMenu = []string{
	"list:show declared deps",
	"add:declare a dependency path",
	"rm:remove a declared dep",
	"sync:re-copy deps into a worktree",
}

var baseMenu = []string{
	"add:add a base branch",
	"list:list base worktrees",
	"rm:remove a base worktree",
	"update:fast-forward bases to their upstreams",
	"reset:hard-sync a base to its upstream",
}

var rootMenu = []string{
	"status:the root repo's status",
	"checkout:jump, resolved at the root",
	"create:create off the root's branch",
	"fork:fork the root's current state",
}

func completionCandidates(dir string, words []string) []string {
	if len(words) == 0 {
		return commandMenu
	}
	cmd, rest := words[0], words[1:]
	switch cmd {
	case "checkout", "ch":
		if slices.Contains(rest, "-b") {
			return nil // -b names a brand-new branch
		}
		return worktreeNames(dir, nil)
	case "status":
		return worktreeNames(dir, nil)
	case "remove", "rm":
		return worktreeNames(dir, keepRemovable)
	case "create":
		return branchCandidates(dir, true)
	case "fork", "reset":
		return branchCandidates(dir, false)
	case "base":
		if len(rest) == 0 {
			return baseMenu
		}
		switch rest[0] {
		case "add":
			return branchCandidates(dir, true)
		case "rm", "update", "reset":
			return worktreeNames(dir, keepBases)
		}
		return nil
	case "deps":
		if len(rest) == 0 {
			return depsMenu
		}
		switch rest[0] {
		case "rm":
			return config.Deps(dir)
		case "sync":
			return worktreeNames(dir, nil)
		}
		return nil
	case "root":
		if len(rest) == 0 {
			return rootMenu
		}
		return completionCandidates(dir, rest)
	case "init":
		return []string{"zsh", "bash"}
	}
	return nil
}

// worktreeNames lists worktree directory basenames, optionally filtered.
func worktreeNames(dir string, keep func(*workspace, *git.Worktree) bool) []string {
	ws, err := loadWorkspace(dir)
	if err != nil {
		return nil
	}
	var names []string
	for i := range ws.repo.Worktrees {
		wt := &ws.repo.Worktrees[i]
		if wt.Bare || (keep != nil && !keep(ws, wt)) {
			continue
		}
		names = append(names, filepath.Base(wt.Path))
	}
	return names
}

// keepRemovable mirrors remove's guards: not the main checkout, not a base,
// not the worktree you are standing in.
func keepRemovable(ws *workspace, wt *git.Worktree) bool {
	if wt.Path == ws.repo.Worktrees[0].Path {
		return false
	}
	if _, isBase := git.ReadBaseMark(wt.Path); isBase {
		return false
	}
	cur := ws.repo.Current()
	return cur == nil || cur.Path != wt.Path
}

func keepBases(ws *workspace, wt *git.Worktree) bool {
	_, ok := git.ReadBaseMark(wt.Path)
	return ok
}

// branchCandidates are the branches a command would accept: every local
// branch, plus (for create and base add) remote branches with no local
// counterpart.
func branchCandidates(dir string, includeRemote bool) []string {
	locals, remotes, err := git.Branches(dir)
	if err != nil {
		return nil
	}
	isLocal := make(map[string]bool, len(locals))
	for _, b := range locals {
		isLocal[b] = true
	}
	out := slices.Clone(locals)
	if includeRemote {
		for _, b := range remotes {
			if !isLocal[b] {
				out = append(out, b)
			}
		}
	}
	sort.Strings(out)
	return out
}
