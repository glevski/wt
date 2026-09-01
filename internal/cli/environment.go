package cli

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"wt/internal/config"
	"wt/internal/git"
)

// envOptions tunes what create/fork carry into a new worktree.
type envOptions struct {
	noIgnored bool // copy nothing: no ignored files, no deps
	noDeps    bool // copy ignored files but skip declared deps
	wait      bool // bring deps over synchronously instead of in the background
	copyDeps  bool // force real copies instead of the default hardlinks
}

// copyEnvironment mirrors the source worktree's git-ignored files (.env,
// node_modules, …) into a freshly created worktree so it is runnable without
// reinstalling everything. Declared deps (wt.deps) get their own pipeline:
// copied by a detached background worker by default so the jump is instant.
// Failures only cost a warning — the worktree itself is fine.
func copyEnvironment(ws *workspace, dest string, opts envOptions) {
	if opts.noIgnored || !config.CopyIgnored(ws.dir()) {
		return
	}
	source := ws.repo.Current()
	if source == nil || source.Path == dest {
		return
	}
	paths, err := git.IgnoredPaths(source.Path)
	if err != nil {
		logf("warning: listing git-ignored files failed: %v", err)
		return
	}
	deps := config.Deps(ws.dir())
	isDep := make(map[string]bool, len(deps))
	for _, d := range deps {
		isDep[strings.TrimSuffix(d, "/")] = true
	}

	copied := 0
	ignored := make(map[string]bool, len(paths))
	for _, rel := range paths {
		ignored[strings.TrimSuffix(rel, "/")] = true
		if isDep[strings.TrimSuffix(rel, "/")] {
			continue // deps have their own pipeline below
		}
		from, to := filepath.Join(source.Path, rel), filepath.Join(dest, rel)
		if strings.HasSuffix(rel, "/") {
			err = copyTree(from, to)
		} else {
			err = copyPath(from, to)
		}
		if err != nil {
			logf("warning: copying git-ignored '%s' failed: %v", rel, err)
			continue
		}
		copied++
	}
	if copied > 0 {
		logf("copied %d git-ignored path(s) from '%s'", copied, filepath.Base(source.Path))
	}

	if opts.noDeps || len(deps) == 0 {
		return
	}
	present := presentDeps(source.Path, deps, ignored)
	if len(present) == 0 {
		return
	}
	link := !opts.copyDeps && !config.DepsCopy(ws.dir())
	if link && !sameDevice(source.Path, dest) {
		link = false
		logf("'%s' and the new worktree are on different filesystems — deps will be copied, not hardlinked", filepath.Base(source.Path))
	}
	verb := "linking"
	if !link {
		verb = "copying"
	}
	if opts.wait {
		linked, err := transferDepsInto(source.Path, dest, present, link)
		if err != nil {
			git.WriteDepsState(dest, "failed: "+err.Error())
			logf("warning: bringing deps over failed: %v", err)
			return
		}
		git.WriteDepsState(dest, doneState(linked))
		if linked {
			logf("linked %d dep(s) to '%s' — wt deps eject makes them private copies", len(present), filepath.Base(source.Path))
		} else {
			logf("copied %d dep(s) from '%s'", len(present), filepath.Base(source.Path))
		}
		return
	}
	git.WriteDepsState(dest, "copying")
	if err := startDepsWorker(source.Path, dest, present, link); err != nil {
		git.WriteDepsState(dest, "failed: "+err.Error())
		logf("warning: starting the deps %s failed: %v", verb, err)
		return
	}
	logf("%s %d dep(s) in the background — wt status shows progress, -w waits", verb, len(present))
}

// presentDeps filters declared deps down to the ones that exist in the source
// worktree and are actually git-ignored there (tracked paths are git's
// business, not the environment copy's).
func presentDeps(src string, deps []string, ignored map[string]bool) []string {
	var present []string
	for _, dep := range deps {
		dep = strings.TrimSuffix(dep, "/")
		if _, err := os.Lstat(filepath.Join(src, dep)); err != nil {
			continue
		}
		if !ignored[dep] {
			logf("skipping dep '%s' — not git-ignored in '%s'", dep, filepath.Base(src))
			continue
		}
		present = append(present, dep)
	}
	return present
}

// copyTree copies a directory recursively, preserving symlinks and modes.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		return copyPath(p, filepath.Join(dst, rel))
	})
}

// sameDevice reports whether two paths live on the same filesystem — the
// precondition for hardlinks. A stat failure counts as different, so callers
// fall back to copying instead of promising links they cannot make.
func sameDevice(a, b string) bool {
	var sa, sb syscall.Stat_t
	if err := syscall.Stat(a, &sa); err != nil {
		return false
	}
	if err := syscall.Stat(b, &sb); err != nil {
		return false
	}
	return sa.Dev == sb.Dev
}

// linkTree recreates a directory with hardlinked files: real directories,
// but every regular file shares its inode with the source. Errors wrap EXDEV
// when the destination is on another filesystem.
func linkTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		return linkPath(p, filepath.Join(dst, rel))
	})
}

// linkPath is copyPath's hardlinking sibling. Symlinks are still recreated
// as symlinks — link(2) on a symlink is unportable — so only regular files
// share inodes.
func linkPath(src, dst string) error {
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
	return os.Link(src, dst)
}
