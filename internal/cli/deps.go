package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"wt/internal/config"
	"wt/internal/git"
)

const depsUsage = "usage: wt deps [list | add <path> | rm <path> | sync [--copy] [name] | eject [--no-copy] [name]]"

// deps manages the project's declared dependency paths (node_modules and
// friends) that create/fork bring into new worktrees in the background —
// hardlinked to the source by default, so they cost almost no space.
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
	case "eject":
		return depsEject(dir, rest)
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

// depsSync re-syncs deps into a worktree in the foreground — recovery after
// a failed background transfer, and onboarding for worktrees created before
// deps were declared. Source is the worktree you run it from (the root when
// you target the one you are standing in). Hardlinks by default like
// create/fork; --copy forces real copies.
func depsSync(dir string, args []string) error {
	fs := flag.NewFlagSet("wt deps sync", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	copyMode := fs.Bool("copy", false, "copy instead of hardlinking")
	if err := fs.Parse(args); err != nil || fs.NArg() > 1 {
		return errors.New("usage: wt deps sync [--copy] [worktree-name]")
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
	if fs.NArg() == 1 {
		target, err = matchWorktree(ws.repo.Worktrees, fs.Arg(0))
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
	link := !*copyMode && !config.DepsCopy(ws.dir())
	if link && !sameDevice(source.Path, target.Path) {
		link = false
		logf("'%s' and '%s' are on different filesystems — copying, not hardlinking",
			filepath.Base(source.Path), filepath.Base(target.Path))
	}
	linked, err := transferDepsInto(source.Path, target.Path, present, link)
	if err != nil {
		git.WriteDepsState(target.Path, "failed: "+err.Error())
		return err
	}
	git.WriteDepsState(target.Path, doneState(linked))
	verb := "copied"
	if linked {
		verb = "linked"
	}
	logf("synced %d dep(s) from '%s' into '%s' (%s)", len(present), filepath.Base(source.Path), filepath.Base(target.Path), verb)
	return nil
}

// depsEject makes a worktree own its deps: the (possibly hardlinked) trees
// are rebuilt in place as private copies, read through the existing files —
// no surviving source worktree needed, and the .wt-partial staging keeps it
// atomic. --no-copy just deletes the deps for a manual reinstall.
func depsEject(dir string, args []string) error {
	fs := flag.NewFlagSet("wt deps eject", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	noCopy := fs.Bool("no-copy", false, "delete the deps instead of copying them")
	if err := fs.Parse(args); err != nil || fs.NArg() > 1 {
		return errors.New("usage: wt deps eject [--no-copy] [name]")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	target, err := ws.currentWorktree()
	if err != nil {
		return err
	}
	if fs.NArg() == 1 {
		target, err = matchWorktree(ws.repo.Worktrees, fs.Arg(0))
		if err != nil {
			return err
		}
	}
	name := filepath.Base(target.Path)
	if state, ok := git.DepsState(target.Path); ok && strings.HasPrefix(state, "copying") {
		return fmt.Errorf("'%s' is still syncing deps — wait for it first", name)
	}
	var present []string
	for _, dep := range config.Deps(ws.dir()) {
		dep = strings.TrimSuffix(dep, "/")
		if _, err := os.Lstat(filepath.Join(target.Path, dep)); err == nil {
			present = append(present, dep)
		}
	}
	if len(present) == 0 {
		return fmt.Errorf("no declared deps present in '%s'", name)
	}

	if *noCopy {
		for _, dep := range present {
			if err := os.RemoveAll(filepath.Join(target.Path, dep)); err != nil {
				return err
			}
			logf("removed '%s'", dep)
		}
		git.WriteDepsState(target.Path, "ejected")
		logf("ejected '%s' without copying — reinstall deps when you need them", name)
		return nil
	}
	if _, err := transferDepsInto(target.Path, target.Path, present, false); err != nil {
		git.WriteDepsState(target.Path, "failed: "+err.Error())
		return err
	}
	git.WriteDepsState(target.Path, "done")
	logf("ejected %d dep(s) — '%s' now owns private copies", len(present), name)
	return nil
}
