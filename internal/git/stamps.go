package git

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// adminDir is the metadata directory git keeps per worktree: .git itself for
// the main worktree, .git/worktrees/<id> for linked ones (resolved from the
// "gitdir:" line in the worktree's .git file).
func adminDir(worktreePath string) (string, error) {
	dotGit := filepath.Join(worktreePath, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return dotGit, nil
	}
	raw, err := os.ReadFile(dotGit)
	if err != nil {
		return "", err
	}
	dir := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(raw)), "gitdir:"))
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(worktreePath, dir)
	}
	return dir, nil
}

// CreatedAt reports when a linked worktree was created, taken from the mtime
// of its admin "gitdir" file (written once at creation). ok=false for the
// main worktree, which has no such file.
func CreatedAt(worktreePath string) (time.Time, bool) {
	dir, err := adminDir(worktreePath)
	if err != nil {
		return time.Time{}, false
	}
	info, err := os.Stat(filepath.Join(dir, "gitdir"))
	if err != nil {
		return time.Time{}, false
	}
	return info.ModTime(), true
}

const checkoutStamp = "wt-checkout"

// TouchCheckoutStamp records "wt jumped into this worktree now" as a marker
// file in the admin dir; it is removed together with the worktree. Best
// effort — a failure only costs a table cell.
func TouchCheckoutStamp(worktreePath string) {
	dir, err := adminDir(worktreePath)
	if err != nil {
		return
	}
	path := filepath.Join(dir, checkoutStamp)
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		_ = os.WriteFile(path, nil, 0o644)
	}
}

const baseFile = "wt-base"

// WriteBase records the branch a worktree was created from. Best effort —
// a reset without it just needs an explicit branch argument.
func WriteBase(worktreePath, base string) {
	dir, err := adminDir(worktreePath)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, baseFile), []byte(base+"\n"), 0o644)
}

// BaseBranch returns the recorded base branch, ok=false when none was recorded.
func BaseBranch(worktreePath string) (string, bool) {
	dir, err := adminDir(worktreePath)
	if err != nil {
		return "", false
	}
	raw, err := os.ReadFile(filepath.Join(dir, baseFile))
	if err != nil {
		return "", false
	}
	base := strings.TrimSpace(string(raw))
	return base, base != ""
}

// CheckoutStamp reports when wt last jumped into the worktree.
func CheckoutStamp(worktreePath string) (time.Time, bool) {
	dir, err := adminDir(worktreePath)
	if err != nil {
		return time.Time{}, false
	}
	info, err := os.Stat(filepath.Join(dir, checkoutStamp))
	if err != nil {
		return time.Time{}, false
	}
	return info.ModTime(), true
}
