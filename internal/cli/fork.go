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
	return forkRun(dir, fs.Args(), *name, *checkout)
}

// forkRun is the fork engine; `wt checkout -b` delegates here too.
func forkRun(dir string, args []string, nameOverride string, checkout bool) error {
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
	branch, path, base, err := forkName(ws, source, args, nameOverride)
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

	if _, err := git.Run(ws.dir(), "worktree", "add", "-b", branch, path, base); err != nil {
		return err
	}
	if base == "HEAD" {
		logf("created new branch '%s' from HEAD (%s)", branch, shortSHA(source.Head))
	} else {
		logf("branch '%s' already exists; created new branch '%s' from its tip %s",
			base, branch, upstreamNote(ws.dir(), base))
	}
	logf("created worktree '%s' at %s", filepath.Base(path), path)

	if stashSHA != "" {
		// Worktrees share the object database, so the unreferenced stash
		// commit applies directly; --index restores the staged/unstaged
		// split exactly as it was. Onto a different base this can conflict.
		if _, err := git.Run(path, "stash", "apply", "--index", stashSHA); err != nil {
			return hintf(fmt.Sprintf("resolve there manually: git -C %s stash apply %s", path, stashSHA),
				"worktree created, but applying your changes to it failed: %v", err)
		}
	}
	for _, rel := range untracked {
		if err := copyPath(filepath.Join(source.Path, rel), filepath.Join(path, rel)); err != nil {
			return fmt.Errorf("worktree created, but copying untracked '%s' failed: %w", rel, err)
		}
	}

	logf("carried over: %d staged, %d unstaged, %d untracked file(s)", len(staged), len(unstaged), len(untracked))
	reportSwitch(ws, path, checkout)
	return nil
}

// forkName resolves the new branch, its worktree path, and the commit-ish the
// branch starts from: HEAD normally, or an existing branch's tip when that
// branch is passed as the argument (then the new branch is auto-named off it).
func forkName(ws *workspace, source *git.Worktree, args []string, nameOverride string) (branch, path, base string, err error) {
	if len(args) == 1 {
		requested := args[0]
		refs, err := git.LookupBranch(ws.dir(), requested)
		if err != nil {
			return "", "", "", err
		}
		if refs.Local {
			branch, path, err = ws.newFrom(requested, nameOverride)
			return branch, path, requested, err
		}
		name, err := worktreeName(requested, nameOverride)
		if err != nil {
			return "", "", "", err
		}
		path, err = ws.targetPath(name)
		if err != nil {
			return "", "", "", err
		}
		return requested, path, "HEAD", nil
	}
	if source.Branch == "" {
		return "", "", "", hintf("wt fork <name> names the new branch explicitly",
			"detached HEAD — cannot derive a branch name")
	}
	branch, path, err = ws.newFrom(source.Branch, nameOverride)
	return branch, path, "HEAD", err
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
