package cli

import (
	"errors"
	"path/filepath"

	"wt/internal/config"
	"wt/internal/git"
)

// link records the project name in `git config wt.name`. create and fork
// require it — the name decides where a repo's worktrees live.
func link(dir string, args []string) error {
	if len(args) > 1 {
		return errors.New("usage: wt link [name]")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return linkStatus(ws)
	}

	name := args[0]
	if name != dirName(name) {
		return hintf("wt link "+dirName(name), "'%s' is not filesystem-safe", name)
	}
	if _, err := git.Run(ws.dir(), "config", "wt.name", name); err != nil {
		return err
	}

	switch {
	case ws.linked && ws.name == name:
		logf("already linked to '%s'", name)
	case ws.linked:
		logf("relinked from '%s' to '%s' — existing worktrees keep working", ws.name, name)
	default:
		logf("linked to '%s'", name)
	}
	base, err := config.Root(ws.dir())
	if err != nil {
		return err
	}
	logf("worktrees go to %s", filepath.Join(base, name))
	return nil
}

func linkStatus(ws *workspace) error {
	if !ws.linked {
		return ws.requireLink()
	}
	logf("linked to '%s' (worktrees go to %s)", ws.name, ws.root)
	return nil
}
