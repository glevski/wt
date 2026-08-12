package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"wt/internal/git"
)

// checkout prints the chosen worktree's absolute path on stdout; the wt()
// shell function turns that into a cd. Nothing to check out in git terms —
// every worktree has its branch permanently checked out.
func checkout(dir string, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: wt checkout <worktree-name>")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	wt, err := matchWorktree(ws.repo.Worktrees, args[0])
	if err != nil {
		return err
	}
	git.TouchCheckoutStamp(wt.Path)
	logf("→ %s/%s", ws.name, filepath.Base(wt.Path))
	fmt.Fprintln(stdout, wt.Path)
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
