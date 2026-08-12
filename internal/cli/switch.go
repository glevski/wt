package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"wt/internal/git"
)

// switchTo jumps back to where the last wt jump left from — like `cd -`,
// toggling on repeat. The state lives in the wt() shell function's _wt_prev
// variable (per shell, handed to us as WT_PREV), because a child process
// cannot see or keep per-shell state itself.
func switchTo(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: wt switch")
	}
	prev := os.Getenv("WT_PREV")
	if prev == "" {
		return hintf("jump somewhere first: wt ch <name>",
			"no previous location in this shell")
	}
	if info, err := os.Stat(prev); err != nil || !info.IsDir() {
		return fmt.Errorf("previous location %s no longer exists", prev)
	}
	// Best-effort niceties when the previous location is inside a worktree.
	home := ""
	if ws, err := loadWorkspace(prev); err == nil {
		home = ws.repo.Worktrees[0].Path
		if wt := ws.repo.Current(); wt != nil {
			git.TouchCheckoutStamp(wt.Path)
			logf("→ %s/%s", ws.name, filepath.Base(wt.Path))
		}
	}
	emitJump(prev, home)
	return nil
}
