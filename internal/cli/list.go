package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"wt/internal/git"
)

const (
	ansiCyan    = "\x1b[36m"
	ansiMagenta = "\x1b[35m"
	ansiGreen   = "\x1b[32m"
	ansiOrange  = "\x1b[38;5;208m" // 256-color; the basic palette has no orange
	ansiRed     = "\x1b[31m"
	ansiReset   = "\x1b[0m"
)

func list(dir string, args []string) error {
	if len(args) != 0 {
		return errors.New("usage: wt list")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	return renderWorktrees(ws, nil)
}

// listRow is one worktree with the metadata the table (and its ordering)
// needs.
type listRow struct {
	wt         git.Worktree
	isBase     bool
	baseBranch string
	created    time.Time
	createdOK  bool
	checkout   time.Time
	checkoutOK bool
}

// renderWorktrees prints the worktree table, optionally filtered by keep.
// Order: root repo first, then base worktrees, then the rest — within each
// group by most recent checkout, then most recently created, then name.
func renderWorktrees(ws *workspace, keep func(git.Worktree) bool) error {
	mainPath := ws.repo.Worktrees[0].Path
	var entries []listRow
	for _, wt := range ws.repo.Worktrees {
		if keep != nil && !keep(wt) {
			continue
		}
		row := listRow{wt: wt}
		row.baseBranch, row.isBase = git.ReadBaseMark(wt.Path)
		row.created, row.createdOK = git.CreatedAt(wt.Path)
		row.checkout, row.checkoutOK = git.CheckoutStamp(wt.Path)
		entries = append(entries, row)
	}

	group := func(r listRow) int {
		switch {
		case r.wt.Path == mainPath:
			return 0
		case r.isBase:
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if ga, gb := group(a), group(b); ga != gb {
			return ga < gb
		}
		if a.checkoutOK != b.checkoutOK {
			return a.checkoutOK
		}
		if a.checkoutOK && !a.checkout.Equal(b.checkout) {
			return a.checkout.After(b.checkout)
		}
		if a.createdOK != b.createdOK {
			return a.createdOK
		}
		if a.createdOK && !a.created.Equal(b.created) {
			return a.created.After(b.created)
		}
		return filepath.Base(a.wt.Path) < filepath.Base(b.wt.Path)
	})

	states := make([]string, len(entries))
	var wg sync.WaitGroup
	for i, e := range entries {
		if e.wt.Bare {
			states[i] = "bare"
			continue
		}
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			states[i] = worktreeState(path)
		}(i, e.wt.Path)
	}
	wg.Wait()
	for i, e := range entries {
		if state, ok := git.DepsState(e.wt.Path); ok && strings.HasPrefix(state, "copying") {
			states[i] = "syncing"
		}
	}

	current := ws.repo.Current()
	header := []string{"NAME", "BRANCH", "STATE", "COMMIT", "CREATED", "CHECKOUT"}
	rows := make([][]string, len(entries))
	rowColors := make([][]string, len(entries))
	markers := make([]string, len(entries))
	for i, e := range entries {
		wt := e.wt
		drifted := e.isBase && wt.Branch != e.baseBranch
		branch := wt.Branch
		if wt.Detached {
			branch = "(detached)"
		}
		if drifted {
			branch += "!" // keeps the drift visible without colors too
		}
		commit := shortSHA(wt.Head)
		if commit == "" {
			commit = "-"
		}
		rows[i] = []string{
			filepath.Base(wt.Path), branch, states[i], commit,
			when(e.created, e.createdOK), when(e.checkout, e.checkoutOK),
		}
		colors := make([]string, len(header))
		colors[0] = worktreeColor(wt.Path == mainPath, strings.HasPrefix(wt.Path, ws.root+"/"), e.isBase)
		if drifted {
			colors[1] = ansiRed
		}
		rowColors[i] = colors
		markers[i] = " "
		if current != nil && wt.Path == current.Path {
			markers[i] = "*"
		}
	}

	// Peek snapshots are invisible to git; show them as pseudo-rows so they
	// can't be forgotten. Only in the unfiltered listing, sorted last.
	if keep == nil {
		for _, p := range findPeeks(ws.root) {
			info, ok := readPeekInfo(p)
			if !ok {
				continue
			}
			state := "peek"
			if s, sok := git.DepsState(p); sok && strings.HasPrefix(s, "copying") {
				state = "syncing"
			}
			rows = append(rows, []string{
				filepath.Base(p), "(peek: " + info.Rev + ")", state,
				shortSHA(info.SHA), ago(info.Created), "-",
			})
			colors := make([]string, len(header))
			colors[0] = ansiRed
			rowColors = append(rowColors, colors)
			markers = append(markers, " ")
		}
	}

	widths := make([]int, len(header))
	for c, h := range header {
		widths[c] = len(h)
	}
	for _, row := range rows {
		for c, cell := range row {
			if n := utf8.RuneCountInString(cell); n > widths[c] {
				widths[c] = n
			}
		}
	}

	paint := colorEnabled()
	printRow := func(marker string, cells, colors []string) {
		var b strings.Builder
		b.WriteString(marker + " ")
		for c, cell := range cells {
			if c > 0 {
				b.WriteString("  ")
			}
			padded := pad(cell, widths[c])
			if paint && colors != nil && colors[c] != "" {
				padded = colors[c] + padded + ansiReset
			}
			b.WriteString(padded)
		}
		fmt.Fprintln(stdout, strings.TrimRight(b.String(), " "))
	}
	printRow(" ", header, nil)
	for i, row := range rows {
		printRow(markers[i], row, rowColors[i])
	}
	return nil
}

// worktreeColor picks the NAME color: cyan for the main checkout, orange for
// base worktrees, green for wt-managed worktrees (under the workspace root),
// magenta for worktrees created elsewhere by other tools.
func worktreeColor(main, managed, base bool) string {
	switch {
	case main:
		return ansiCyan
	case base:
		return ansiOrange
	case managed:
		return ansiGreen
	default:
		return ansiMagenta
	}
}

// colorEnabled reports whether stdout is a real terminal that wants color.
func colorEnabled() bool {
	f, ok := stdout.(*os.File)
	if !ok {
		return false
	}
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func pad(s string, width int) string {
	return s + strings.Repeat(" ", width-utf8.RuneCountInString(s))
}

func worktreeState(path string) string {
	out, err := git.Run(path, "status", "--porcelain")
	switch {
	case err != nil:
		return "error"
	case out == "":
		return "clean"
	default:
		return "dirty"
	}
}

func when(t time.Time, ok bool) string {
	if !ok {
		return "-"
	}
	return ago(t)
}

// ago renders a timestamp as a compact age: "now", "5m", "3h", "2d", "4mo".
func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
}
