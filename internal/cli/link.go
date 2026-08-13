package cli

import (
	"errors"
	"flag"
	"io"
	"path/filepath"

	"wt/internal/config"
	"wt/internal/git"
)

const linkUsage = "usage: wt link [-r|--register] [name]"

// link records the project name in `git config wt.name`. create and fork
// require it — the name decides where a repo's worktrees live. With
// -r/--register the project is also added to the global registry that
// wt global works from; a bare `wt link -r` registers an already linked
// repo. Registration is always explicit, but renaming a registered project
// keeps it registered under the new name.
func link(dir string, args []string) error {
	fs := flag.NewFlagSet("wt link", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	register := fs.Bool("r", false, "also register for wt global")
	fs.BoolVar(register, "register", false, "also register for wt global")
	if err := fs.Parse(args); err != nil || fs.NArg() > 1 {
		return errors.New(linkUsage)
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	if fs.NArg() == 0 {
		if *register {
			if !ws.linked {
				return ws.requireLink()
			}
			return registerProject(ws, ws.name)
		}
		return linkStatus(ws)
	}

	name := fs.Arg(0)
	if name != dirName(name) {
		return hintf("wt link "+dirName(name), "'%s' is not filesystem-safe", name)
	}
	if _, err := git.Run(ws.dir(), "config", "wt.name", name); err != nil {
		return err
	}
	wasRegistered := false
	if ws.linked && ws.name != name {
		if path, ok := config.ProjectPath(ws.dir(), ws.name); ok && path == ws.repo.Worktrees[0].Path {
			config.UnregisterProject(ws.dir(), ws.name)
			wasRegistered = true
		}
	}

	switch {
	case ws.linked && ws.name == name:
		logf("already linked to '%s'", name)
	case ws.linked:
		logf("relinked from '%s' to '%s' — existing worktrees keep working", ws.name, name)
	default:
		logf("linked to '%s'", name)
	}
	if *register || wasRegistered {
		if err := registerProject(ws, name); err != nil {
			return err
		}
	}
	base, err := config.Root(ws.dir())
	if err != nil {
		return err
	}
	logf("worktrees go to %s", filepath.Join(base, name))
	return nil
}

func registerProject(ws *workspace, name string) error {
	if err := config.RegisterProject(ws.dir(), name, ws.repo.Worktrees[0].Path); err != nil {
		return err
	}
	logf("registered '%s' for wt global", name)
	return nil
}

func linkStatus(ws *workspace) error {
	if !ws.linked {
		return ws.requireLink()
	}
	logf("linked to '%s' (worktrees go to %s)", ws.name, ws.root)
	if path, ok := config.ProjectPath(ws.dir(), ws.name); ok && path == ws.repo.Worktrees[0].Path {
		logf("registered for wt global")
	} else {
		logf("not registered — wt link -r makes wt global see it")
	}
	return nil
}
