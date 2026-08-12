package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

	worktrees := ws.repo.Worktrees
	states := make([]string, len(worktrees))
	var wg sync.WaitGroup
	for i, wt := range worktrees {
		if wt.Bare {
			states[i] = "bare"
			continue
		}
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			states[i] = worktreeState(path)
		}(i, wt.Path)
	}
	wg.Wait()

	current := ws.repo.Current()
	header := []string{"NAME", "BRANCH", "STATE", "COMMIT", "CREATED", "CHECKOUT"}
	rows := make([][]string, len(worktrees))
	colors := make([]string, len(worktrees))
	markers := make([]string, len(worktrees))
	for i, wt := range worktrees {
		branch := wt.Branch
		if wt.Detached {
			branch = "(detached)"
		}
		commit := shortSHA(wt.Head)
		if commit == "" {
			commit = "-"
		}
		rows[i] = []string{
			filepath.Base(wt.Path), branch, states[i], commit,
			when(git.CreatedAt(wt.Path)), when(git.CheckoutStamp(wt.Path)),
		}
		colors[i] = worktreeColor(i == 0, strings.HasPrefix(wt.Path, ws.root+"/"))
		markers[i] = " "
		if current != nil && wt.Path == current.Path {
			markers[i] = "*"
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
	printRow := func(marker string, cells []string, color string) {
		var b strings.Builder
		b.WriteString(marker + " ")
		for c, cell := range cells {
			if c > 0 {
				b.WriteString("  ")
			}
			padded := pad(cell, widths[c])
			if c == 0 && color != "" {
				padded = color + padded + ansiReset
			}
			b.WriteString(padded)
		}
		fmt.Fprintln(stdout, strings.TrimRight(b.String(), " "))
	}
	printRow(" ", header, "")
	for i, row := range rows {
		color := ""
		if paint {
			color = colors[i]
		}
		printRow(markers[i], row, color)
	}
	return nil
}

// worktreeColor picks the NAME color: cyan for the main checkout, green for
// wt-managed worktrees (under the workspace root), magenta for worktrees
// created elsewhere by other tools.
func worktreeColor(main, managed bool) string {
	switch {
	case main:
		return ansiCyan
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
