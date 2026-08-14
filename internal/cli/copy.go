package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const copyUsage = "usage: wt copy [-f] [--from <worktree>] <file> [dst-file]"

// copyCmd copies one file into the current worktree — from the root repo by
// default, or from the worktree named with --from. Made for the files git
// doesn't carry across worktrees (.env and friends); paths are relative to
// each worktree's root, and dst defaults to the same relative path.
func copyCmd(dir string, args []string) error {
	fs := flag.NewFlagSet("wt copy", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	force := fs.Bool("f", false, "overwrite an existing destination")
	from := fs.String("from", "", "source worktree (default: the root repo)")
	if err := fs.Parse(args); err != nil || fs.NArg() < 1 || fs.NArg() > 2 {
		return errors.New(copyUsage)
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	target, err := ws.currentWorktree()
	if err != nil {
		return err
	}
	source := &ws.repo.Worktrees[0]
	if *from != "" {
		source, err = matchWorktree(ws.repo.Worktrees, *from)
		if err != nil {
			return err
		}
	}
	if source.Path == target.Path {
		return hintf("--from <worktree> names a different source", "source and destination are both '%s'", filepath.Base(target.Path))
	}

	srcRel := fs.Arg(0)
	dstRel := srcRel
	if fs.NArg() == 2 {
		dstRel = fs.Arg(1)
	}
	src := filepath.Join(source.Path, srcRel)
	dst := filepath.Join(target.Path, dstRel)

	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("'%s' not found in '%s'", srcRel, filepath.Base(source.Path))
	}
	if info.IsDir() {
		return fmt.Errorf("'%s' is a directory — wt copy handles single files", srcRel)
	}
	if _, err := os.Lstat(dst); err == nil {
		if !*force {
			return hintf("wt copy -f overwrites it", "'%s' already exists in '%s'", dstRel, filepath.Base(target.Path))
		}
		if err := os.Remove(dst); err != nil {
			return err
		}
	}
	if err := copyPath(src, dst); err != nil {
		return err
	}
	logf("copied '%s' from '%s' → '%s/%s'", srcRel, filepath.Base(source.Path), filepath.Base(target.Path), dstRel)
	return nil
}
