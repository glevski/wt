package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"wt/internal/config"
	"wt/internal/git"
)

const depsUsage = "usage: wt deps [list | add <path> | rm <path> | sync [name]]"

// deps manages the project's declared dependency paths (node_modules and
// friends) that create/fork copy into new worktrees in the background.
func deps(dir string, args []string) error {
	if len(args) == 0 {
		return depsList(dir)
	}
	switch sub, rest := args[0], args[1:]; sub {
	case "list":
		if len(rest) != 0 {
			return errors.New(depsUsage)
		}
		return depsList(dir)
	case "add":
		return depsAdd(dir, rest)
	case "rm":
		return depsRemove(dir, rest)
	case "sync":
		return depsSync(dir, rest)
	default:
		return fmt.Errorf("unknown deps command %q — %s", sub, depsUsage)
	}
}

func depsList(dir string) error {
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	entries := config.Deps(ws.dir())
	if len(entries) == 0 {
		logf("no deps declared — add one: wt deps add node_modules")
		return nil
	}
	for _, d := range entries {
		fmt.Fprintln(stdout, d)
	}
	if cur := ws.repo.Current(); cur != nil {
		if state, ok := git.DepsState(cur.Path); ok {
			logf("deps state in '%s': %s", filepath.Base(cur.Path), state)
		}
	}
	return nil
}

func depsAdd(dir string, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: wt deps add <path>")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	dep := strings.TrimSuffix(args[0], "/")
	for _, existing := range config.Deps(ws.dir()) {
		if existing == dep {
			logf("'%s' is already a dep", dep)
			return nil
		}
	}
	if _, err := git.Run(ws.dir(), "check-ignore", "-q", dep); err != nil {
		logf("warning: '%s' is not git-ignored — deps are meant for ignored paths like node_modules", dep)
	}
	if _, err := git.Run(ws.dir(), "config", "--add", "wt.deps", dep); err != nil {
		return err
	}
	logf("added dep '%s'", dep)
	return nil
}

func depsRemove(dir string, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: wt deps rm <path>")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	dep := strings.TrimSuffix(args[0], "/")
	if _, err := git.Run(ws.dir(), "config", "--unset", "--fixed-value", "wt.deps", dep); err != nil {
		return hintf("wt deps lists them", "'%s' is not a declared dep", dep)
	}
	logf("removed dep '%s'", dep)
	return nil
}

// depsSync re-copies deps into a worktree in the foreground — recovery after
// a failed background copy, and onboarding for worktrees created before deps
// were declared. Source is the worktree you run it from (the root when you
// target the one you are standing in).
func depsSync(dir string, args []string) error {
	if len(args) > 1 {
		return errors.New("usage: wt deps sync [worktree-name]")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	entries := config.Deps(ws.dir())
	if len(entries) == 0 {
		return hintf("wt deps add <path> declares one", "no deps declared")
	}
	target, err := ws.currentWorktree()
	if err != nil {
		return err
	}
	if len(args) == 1 {
		target, err = matchWorktree(ws.repo.Worktrees, args[0])
		if err != nil {
			return err
		}
	}
	source := ws.repo.Current()
	if source == nil || source.Path == target.Path {
		source = &ws.repo.Worktrees[0]
	}
	if source.Path == target.Path {
		return errors.New("nothing to sync from — run it from a worktree that has the deps")
	}

	ignored := map[string]bool{}
	paths, err := git.IgnoredPaths(source.Path)
	if err != nil {
		return err
	}
	for _, rel := range paths {
		ignored[strings.TrimSuffix(rel, "/")] = true
	}
	present := presentDeps(source.Path, entries, ignored)
	if len(present) == 0 {
		return fmt.Errorf("none of the declared deps exist in '%s'", filepath.Base(source.Path))
	}
	if err := copyDepsInto(source.Path, target.Path, present); err != nil {
		git.WriteDepsState(target.Path, "failed: "+err.Error())
		return err
	}
	git.WriteDepsState(target.Path, "done")
	logf("synced %d dep(s) from '%s' into '%s'", len(present), filepath.Base(source.Path), filepath.Base(target.Path))
	return nil
}
