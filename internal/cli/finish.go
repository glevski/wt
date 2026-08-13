package cli

import (
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"

	"wt/internal/git"
)

const finishUsage = "usage: wt finish [-d|--delete [-b|--branch]] [-f|--force]"

// finish wraps up work in the current worktree: jump back to the last
// location this shell jumped from (home when there is none), optionally
// deleting the worktree (-d) and its branch (-d -b) on the way out.
func finish(dir string, args []string) error {
	fs := flag.NewFlagSet("wt finish", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	del := fs.Bool("d", false, "also delete the worktree")
	fs.BoolVar(del, "delete", false, "also delete the worktree")
	branch := fs.Bool("b", false, "also delete the branch")
	fs.BoolVar(branch, "branch", false, "also delete the branch")
	force := fs.Bool("f", false, "discard local changes")
	fs.BoolVar(force, "force", false, "discard local changes")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return errors.New(finishUsage)
	}
	if *branch && !*del {
		return hintf("wt finish -d -b deletes both", "-b/--branch needs -d/--delete")
	}
	if _, ok := findPeekRoot(dir); ok {
		return hintf("wt unpeek exits and deletes it", "a peek is not finished — it is discarded")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	cur, err := ws.currentWorktree()
	if err != nil {
		return err
	}
	name := filepath.Base(cur.Path)
	if cur.Path == ws.repo.Worktrees[0].Path {
		return hintf("it only makes sense inside a worktree — wt ch jumps into one",
			"the main checkout is never finished")
	}
	if _, isBase := git.ReadBaseMark(cur.Path); isBase && *del {
		return hintf("wt base rm "+name+" removes it for real", "'%s' is a base worktree", name)
	}
	if state, ok := git.DepsState(cur.Path); ok && strings.HasPrefix(state, "copying") && *del && !*force {
		return hintf("wait for it, or discard with: wt finish -d -f", "'%s' is still syncing deps", name)
	}

	dest := finishDestination(ws, cur)
	if *del {
		// leave before deleting: neither git nor this process may run the
		// removal from inside the directory it removes
		if err := os.Chdir(dest); err != nil {
			return err
		}
		ws.repo.Dir = ws.repo.Worktrees[0].Path
		if err := removeWorktree(ws, cur, *force, *branch, "wt finish -d -f discards them"); err != nil {
			return err
		}
	}

	home, note := "", dest
	if there, err := loadWorkspace(dest); err == nil {
		home = there.repo.Worktrees[0].Path
		if wt := there.repo.Current(); wt != nil {
			git.TouchCheckoutStamp(wt.Path)
			note = there.name + "/" + filepath.Base(wt.Path)
		}
	}
	logf("finished '%s' → %s", name, note)
	emitJump(dest, home)
	return nil
}

// finishDestination is where finish lands: the last location this shell
// jumped from, unless it is gone or inside the worktree being finished —
// then the main checkout.
func finishDestination(ws *workspace, cur *git.Worktree) string {
	main := ws.repo.Worktrees[0].Path
	prev := os.Getenv("WT_PREV")
	if prev == "" || prev == cur.Path || strings.HasPrefix(prev, cur.Path+string(filepath.Separator)) {
		return main
	}
	if info, err := os.Stat(prev); err != nil || !info.IsDir() {
		return main
	}
	return prev
}
