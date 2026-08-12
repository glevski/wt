package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"wt/internal/git"
)

func fork(dir string, args []string) error {
	fs := flag.NewFlagSet("wt fork", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	checkout := fs.Bool("c", false, "cd into the new worktree")
	fs.BoolVar(checkout, "checkout", false, "cd into the new worktree")
	name := fs.String("n", "", "worktree directory name")
	fs.StringVar(name, "name", "", "worktree directory name")
	if err := fs.Parse(args); err != nil || fs.NArg() > 1 {
		return errors.New("usage: wt fork [-c|--checkout] [-n|--name <name>] [new-branch]")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	if !ws.linked {
		return ws.requireLink()
	}
	source, err := ws.currentWorktree()
	if err != nil {
		return err
	}
	branch, path, err := forkName(ws, source, fs.Args(), *name)
	if err != nil {
		return err
	}

	// Snapshot the source state first. stash create builds a stash commit
	// without touching the working tree (and returns nothing when there are
	// no tracked changes); untracked files are never part of a stash.
	stashSHA, err := git.Run(source.Path, "stash", "create", "wt fork")
	if err != nil {
		return err
	}
	staged, err := git.StagedFiles(source.Path)
	if err != nil {
		return err
	}
	unstaged, err := git.UnstagedFiles(source.Path)
	if err != nil {
		return err
	}
	untracked, err := git.UntrackedFiles(source.Path)
	if err != nil {
		return err
	}

	if _, err := git.Run(ws.dir(), "worktree", "add", "-b", branch, path, "HEAD"); err != nil {
		return err
	}
	logf("created new branch '%s' from HEAD (%s)", branch, shortSHA(source.Head))
	logf("created worktree '%s' at %s", filepath.Base(path), path)

	if stashSHA != "" {
		// Worktrees share the object database, so the unreferenced stash
		// commit applies directly; --index restores the staged/unstaged
		// split exactly as it was.
		if _, err := git.Run(path, "stash", "apply", "--index", stashSHA); err != nil {
			return fmt.Errorf("worktree created, but applying your changes to it failed: %w", err)
		}
	}
	for _, rel := range untracked {
		if err := copyPath(filepath.Join(source.Path, rel), filepath.Join(path, rel)); err != nil {
			return fmt.Errorf("worktree created, but copying untracked '%s' failed: %w", rel, err)
		}
	}

	logf("carried over: %d staged, %d unstaged, %d untracked file(s)", len(staged), len(unstaged), len(untracked))
	reportSwitch(path, *checkout)
	return nil
}

// forkName resolves the new branch and worktree path: the explicit argument
// (which must not clash with an existing branch), or <current-branch>-N.
func forkName(ws *workspace, source *git.Worktree, args []string, nameOverride string) (string, string, error) {
	if len(args) == 1 {
		branch := args[0]
		refs, err := git.LookupBranch(ws.dir(), branch)
		if err != nil {
			return "", "", err
		}
		if refs.Local {
			return "", "", hintf("wt create "+branch+" adds a worktree for the existing branch",
				"branch '%s' already exists", branch)
		}
		name, err := worktreeName(branch, nameOverride)
		if err != nil {
			return "", "", err
		}
		path, err := ws.targetPath(name)
		if err != nil {
			return "", "", err
		}
		return branch, path, nil
	}
	if source.Branch == "" {
		return "", "", hintf("wt fork <name> names the new branch explicitly",
			"detached HEAD — cannot derive a branch name")
	}
	if nameOverride == "" {
		return ws.freeName(source.Branch)
	}
	return ws.namedNew(source.Branch, nameOverride)
}

// copyPath copies one untracked file into the new worktree, preserving
// symlinks and permission bits.
func copyPath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
