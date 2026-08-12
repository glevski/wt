package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"wt/internal/config"
	"wt/internal/git"
)

// workspace is the per-invocation context: the loaded repo, its project
// identity, and the directory its worktrees are created in (<root>/<name>).
type workspace struct {
	repo   *git.Repo
	name   string // wt.name when linked, else the repo dir basename (display fallback)
	linked bool
	root   string
}

func loadWorkspace(dir string) (*workspace, error) {
	repo, err := git.Load(dir)
	if err != nil {
		return nil, err
	}
	base, err := config.Root(dir)
	if err != nil {
		return nil, err
	}
	name, linked := config.Name(dir)
	if !linked {
		name = repo.Name
	}
	return &workspace{repo: repo, name: name, linked: linked, root: filepath.Join(base, name)}, nil
}

// requireLink guards the commands that create worktrees under <root>/<name>.
func (ws *workspace) requireLink() error {
	return hintf(fmt.Sprintf("run: wt link <name>  (e.g. wt link %s)", dirName(ws.repo.Name)),
		"this repository is not linked to a project name")
}

func (ws *workspace) dir() string { return ws.repo.Dir }

// currentWorktree is the worktree the command was invoked from.
func (ws *workspace) currentWorktree() (*git.Worktree, error) {
	wt := ws.repo.Current()
	if wt == nil {
		return nil, errors.New("cannot determine the current worktree")
	}
	return wt, nil
}

// targetPath is where a worktree directory called name would live, erroring
// when it is already taken.
func (ws *workspace) targetPath(name string) (string, error) {
	path := filepath.Join(ws.root, name)
	for _, wt := range ws.repo.Worktrees {
		if wt.Path == path {
			return "", hintf("wt ch "+name,
				"worktree '%s' already exists (branch '%s')", name, wt.Branch)
		}
	}
	if _, err := os.Stat(path); err == nil {
		return "", hintf("remove it, or run: git worktree prune",
			"%s exists but is not a registered worktree", path)
	}
	return path, nil
}

// freeName picks the first <base>-N (N ≥ 2) that is free both as a local
// branch and as a directory under the workspace root.
func (ws *workspace) freeName(base string) (branch, path string, err error) {
	existing, err := git.LocalBranches(ws.dir(), base+"-*")
	if err != nil {
		return "", "", err
	}
	taken := make(map[string]bool, len(existing))
	for _, b := range existing {
		taken[b] = true
	}
	for n := 2; n < 100; n++ {
		branch = fmt.Sprintf("%s-%d", base, n)
		path = filepath.Join(ws.root, dirName(branch))
		if taken[branch] {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			continue
		}
		return branch, path, nil
	}
	return "", "", fmt.Errorf("no free name between %s-2 and %s-99", base, base)
}

// freeBranch picks the first <base>-N (N ≥ 2) not taken by a local branch,
// for when the directory name is chosen explicitly.
func (ws *workspace) freeBranch(base string) (string, error) {
	existing, err := git.LocalBranches(ws.dir(), base+"-*")
	if err != nil {
		return "", err
	}
	taken := make(map[string]bool, len(existing))
	for _, b := range existing {
		taken[b] = true
	}
	for n := 2; n < 100; n++ {
		candidate := fmt.Sprintf("%s-%d", base, n)
		if !taken[candidate] {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no free branch between %s-2 and %s-99", base, base)
}

// namedNew pairs an auto-suffixed branch off base with an explicitly named
// worktree directory.
func (ws *workspace) namedNew(base, nameOverride string) (branch, path string, err error) {
	branch, err = ws.freeBranch(base)
	if err != nil {
		return "", "", err
	}
	name, err := worktreeName(base, nameOverride)
	if err != nil {
		return "", "", err
	}
	path, err = ws.targetPath(name)
	if err != nil {
		return "", "", err
	}
	return branch, path, nil
}
