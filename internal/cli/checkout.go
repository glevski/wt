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

const checkoutUsage = "usage: wt checkout [worktree-name] | wt checkout -b <new-branch>"

// checkout prints the chosen worktree's absolute path on stdout; the wt()
// shell function turns that into a cd. Nothing to check out in git terms —
// every worktree has its branch permanently checked out. With -b it behaves
// like `git checkout -b`: fork the current state into a new branch and jump.
// With no argument it opens an interactive picker of the most recently used
// worktrees.
func checkout(dir string, args []string) error {
	fs := flag.NewFlagSet("wt checkout", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	newBranch := fs.String("b", "", "fork into a new branch and jump there")
	if err := fs.Parse(args); err != nil {
		return errors.New(checkoutUsage)
	}
	if *newBranch != "" {
		if fs.NArg() != 0 {
			return errors.New(checkoutUsage)
		}
		return forkRun(dir, []string{*newBranch}, "", true, envOptions{})
	}
	if fs.NArg() > 1 {
		return errors.New(checkoutUsage)
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	var wt *git.Worktree
	if fs.NArg() == 1 {
		wt, err = matchWorktree(ws.repo.Worktrees, fs.Arg(0))
	} else {
		wt, err = pickWorktree(ws)
	}
	if err != nil {
		return err
	}
	git.TouchCheckoutStamp(wt.Path)
	logf("→ %s/%s", ws.name, filepath.Base(wt.Path))
	emitJump(wt.Path, ws.repo.Worktrees[0].Path)
	return nil
}

// matchWorktree resolves name against worktree directory basenames: exact
// match first, then a unique prefix.
func matchWorktree(worktrees []git.Worktree, name string) (*git.Worktree, error) {
	var prefixed []*git.Worktree
	for i := range worktrees {
		base := filepath.Base(worktrees[i].Path)
		if base == name {
			return &worktrees[i], nil
		}
		if strings.HasPrefix(base, name) {
			prefixed = append(prefixed, &worktrees[i])
		}
	}
	switch len(prefixed) {
	case 1:
		return prefixed[0], nil
	case 0:
		return nil, hintf("wt list shows what exists", "no worktree named '%s'", name)
	default:
		names := make([]string, len(prefixed))
		for i, wt := range prefixed {
			names[i] = filepath.Base(wt.Path)
		}
		return nil, fmt.Errorf("'%s' is ambiguous: %s", name, strings.Join(names, ", "))
	}
}
