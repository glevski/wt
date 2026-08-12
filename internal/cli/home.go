package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"wt/internal/git"
)

// home prints the main worktree's path on stdout — the wt() shell function
// cd's there. It's checkout's "go back to the root repo" sibling.
func home(dir string, args []string) error {
	if len(args) != 0 {
		return errors.New("usage: wt home")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	root := ws.repo.Worktrees[0]
	git.TouchCheckoutStamp(root.Path)
	logf("→ %s/%s", ws.name, filepath.Base(root.Path))
	fmt.Fprintln(stdout, root.Path)
	return nil
}
