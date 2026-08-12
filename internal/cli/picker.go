package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"wt/internal/git"
	"wt/internal/tui"
)

// pick is swapped out in tests to avoid needing a real terminal.
var pick = tui.Pick

const pickerLimit = 10

// pickWorktree shows an arrow-key picker of the most recently used worktrees
// and returns the choice. Recency is the last wt checkout stamp, falling back
// to the worktree's creation time.
func pickWorktree(ws *workspace) (*git.Worktree, error) {
	entries := recentWorktrees(ws, pickerLimit)
	if len(entries) == 0 {
		return nil, hintf("wt create or wt fork makes one", "no other worktree to switch to")
	}
	width := 0
	for _, e := range entries {
		if n := len(filepath.Base(e.wt.Path)); n > width {
			width = n
		}
	}
	labels := make([]string, len(entries))
	for i, e := range entries {
		labels[i] = fmt.Sprintf("%-*s  %s · %s",
			width, filepath.Base(e.wt.Path), branchLabel(e.wt), when(e.lastUsed, !e.lastUsed.IsZero()))
	}
	idx, err := pick("switch to:", labels)
	switch {
	case errors.Is(err, tui.ErrNoTTY):
		return nil, hintf("wt ch <name> works anywhere", "interactive picking needs a terminal")
	case err != nil:
		return nil, err
	}
	return entries[idx].wt, nil
}

type recentEntry struct {
	wt       *git.Worktree
	lastUsed time.Time
}

// recentWorktrees is every worktree except the current one (and bare repo
// entries), most recently used first.
func recentWorktrees(ws *workspace, limit int) []recentEntry {
	current := ws.repo.Current()
	var entries []recentEntry
	for i := range ws.repo.Worktrees {
		wt := &ws.repo.Worktrees[i]
		if wt.Bare || (current != nil && wt.Path == current.Path) {
			continue
		}
		var last time.Time
		if t, ok := git.CheckoutStamp(wt.Path); ok {
			last = t
		} else if t, ok := git.CreatedAt(wt.Path); ok {
			last = t
		}
		entries = append(entries, recentEntry{wt, last})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if !entries[i].lastUsed.Equal(entries[j].lastUsed) {
			return entries[i].lastUsed.After(entries[j].lastUsed)
		}
		return filepath.Base(entries[i].wt.Path) < filepath.Base(entries[j].wt.Path)
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries
}

func branchLabel(wt *git.Worktree) string {
	if wt.Detached {
		return "(detached " + shortSHA(wt.Head) + ")"
	}
	return wt.Branch
}
