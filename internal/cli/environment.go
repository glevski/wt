package cli

import (
	"os"
	"path/filepath"
	"strings"

	"wt/internal/config"
	"wt/internal/git"
)

// copyEnvironment mirrors the source worktree's git-ignored files (.env,
// node_modules, …) into a freshly created worktree so it is runnable without
// reinstalling everything. Failures only cost a warning — the worktree itself
// is fine. Disable with `git config wt.copyignored false`.
func copyEnvironment(ws *workspace, dest string) {
	if !config.CopyIgnored(ws.dir()) {
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
	copied := 0
	for _, rel := range paths {
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
