package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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
	fs := flag.NewFlagSet("wt list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return errors.New("usage: wt list [--json]")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	if *asJSON {
		return writeWorktreesJSON(ws)
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
	state      string // clean, dirty, error or bare
	deps       string // raw deps state, empty when none was recorded
}

// drifted reports a base worktree that left the branch it is pinned to.
func (r listRow) drifted() bool {
	return r.isBase && r.wt.Branch != r.baseBranch
}

// peekRow is one peek snapshot; peeks are invisible to git, so they are
// listed from their marker files.
type peekRow struct {
	path string
	info peekInfo
	deps string
}

// collectWorktrees gathers the worktrees, optionally filtered by keep.
// Order: root repo first, then base worktrees, then the rest — within each
// group by most recent checkout, then most recently created, then name.
func collectWorktrees(ws *workspace, keep func(git.Worktree) bool) []listRow {
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
		row.deps, _ = git.DepsState(wt.Path)
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

	var wg sync.WaitGroup
	for i := range entries {
		if entries[i].wt.Bare {
			entries[i].state = "bare"
			continue
		}
		wg.Add(1)
		go func(row *listRow) {
			defer wg.Done()
			row.state = worktreeState(row.wt.Path)
		}(&entries[i])
	}
	wg.Wait()
	return entries
}

func collectPeeks(root string) []peekRow {
	var peeks []peekRow
	for _, p := range findPeeks(root) {
		info, ok := readPeekInfo(p)
		if !ok {
			continue
		}
		row := peekRow{path: p, info: info}
		row.deps, _ = git.DepsState(p)
		peeks = append(peeks, row)
	}
	return peeks
}

// syncing reports a deps state that says the background copy still runs.
func syncing(deps string) bool {
	return strings.HasPrefix(deps, "copying")
}

// renderWorktrees prints the worktree table, optionally filtered by keep.
func renderWorktrees(ws *workspace, keep func(git.Worktree) bool) error {
	mainPath := ws.repo.Worktrees[0].Path
	entries := collectWorktrees(ws, keep)

	current := ws.repo.Current()
	header := []string{"NAME", "BRANCH", "STATE", "COMMIT", "CREATED", "CHECKOUT"}
	rows := make([][]string, len(entries))
	rowColors := make([][]string, len(entries))
	markers := make([]string, len(entries))
	for i, e := range entries {
		wt := e.wt
		branch := wt.Branch
		if wt.Detached {
			branch = "(detached)"
		}
		if e.drifted() {
			branch += "!" // keeps the drift visible without colors too
		}
		commit := shortSHA(wt.Head)
		if commit == "" {
			commit = "-"
		}
		state := e.state
		if syncing(e.deps) {
			state = "syncing"
		}
		rows[i] = []string{
			filepath.Base(wt.Path), branch, state, commit,
			when(e.created, e.createdOK), when(e.checkout, e.checkoutOK),
		}
		colors := make([]string, len(header))
		colors[0] = worktreeColor(wt.Path == mainPath, strings.HasPrefix(wt.Path, ws.root+"/"), e.isBase)
		if e.drifted() {
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
		for _, p := range collectPeeks(ws.root) {
			state := "peek"
			if syncing(p.deps) {
				state = "syncing"
			}
			rows = append(rows, []string{
				filepath.Base(p.path), "(peek: " + p.info.Rev + ")", state,
				shortSHA(p.info.SHA), ago(p.info.Created), "-",
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

// listJSON is the `wt list --json` document. Fields are only ever added;
// schema is bumped when one changes meaning or goes away.
type listJSON struct {
	Schema    int            `json:"schema"`
	Project   string         `json:"project"`
	Linked    bool           `json:"linked"`
	Root      string         `json:"root"`
	Worktrees []worktreeJSON `json:"worktrees"`
	Peeks     []peekJSON     `json:"peeks"`
}

type worktreeJSON struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Branch    string    `json:"branch"`
	Head      string    `json:"head"`
	Detached  bool      `json:"detached"`
	Kind      string    `json:"kind"`
	Current   bool      `json:"current"`
	State     string    `json:"state"`
	Deps      string    `json:"deps,omitempty"`
	Base      string    `json:"base,omitempty"`   // branch it was created from
	Pinned    string    `json:"pinned,omitempty"` // branch a base worktree is pinned to
	Drifted   bool      `json:"drifted"`
	Created   time.Time `json:"created,omitzero"`
	Checkout  time.Time `json:"checkout,omitzero"`
	Committed time.Time `json:"committed,omitzero"` // committer date of head

	// What a dirty worktree holds on top of head, untracked files included.
	Files      int `json:"files,omitempty"`
	Insertions int `json:"insertions,omitempty"`
	Deletions  int `json:"deletions,omitempty"`
}

type peekJSON struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Rev     string    `json:"rev"`
	SHA     string    `json:"sha"`
	Source  string    `json:"source"`
	Created time.Time `json:"created,omitzero"`
	Deps    string    `json:"deps,omitempty"`
}

// writeWorktreesJSON prints what the table shows — same rows, same order —
// plus what it leaves out: paths, full SHAs, base branches and raw deps
// states. This is the interface for scripts and editor integrations.
func writeWorktreesJSON(ws *workspace) error {
	mainPath := ws.repo.Worktrees[0].Path
	current := ws.repo.Current()
	doc := listJSON{
		Schema:    1,
		Project:   ws.name,
		Linked:    ws.linked,
		Root:      ws.root,
		Worktrees: []worktreeJSON{},
		Peeks:     []peekJSON{},
	}
	entries := collectWorktrees(ws, nil)
	heads := make([]string, len(entries))
	for i, e := range entries {
		heads[i] = e.wt.Head
	}
	committed := commitDates(ws.dir(), heads)
	changes := make([]uncommitted, len(entries))
	var wg sync.WaitGroup
	for i, e := range entries {
		if e.state != "dirty" {
			continue
		}
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			changes[i] = uncommittedChanges(path)
		}(i, e.wt.Path)
	}
	wg.Wait()
	for i, e := range entries {
		wt := e.wt
		row := worktreeJSON{
			Name:      filepath.Base(wt.Path),
			Path:      wt.Path,
			Branch:    wt.Branch,
			Head:      wt.Head,
			Detached:  wt.Detached,
			Kind:      worktreeKind(wt.Path == mainPath, strings.HasPrefix(wt.Path, ws.root+"/"), e.isBase),
			Current:   current != nil && wt.Path == current.Path,
			State:     e.state,
			Deps:      e.deps,
			Pinned:    e.baseBranch,
			Drifted:   e.drifted(),
			Created:   e.created,
			Checkout:  e.checkout,
			Committed: committed[wt.Head],

			Files:      changes[i].files,
			Insertions: changes[i].insertions,
			Deletions:  changes[i].deletions,
		}
		row.Base, _ = git.BaseBranch(wt.Path)
		doc.Worktrees = append(doc.Worktrees, row)
	}
	for _, p := range collectPeeks(ws.root) {
		doc.Peeks = append(doc.Peeks, peekJSON{
			Name:    filepath.Base(p.path),
			Path:    p.path,
			Rev:     p.info.Rev,
			SHA:     p.info.SHA,
			Source:  p.info.Source,
			Created: p.info.Created,
			Deps:    p.deps,
		})
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// commitDates maps commits to their committer dates with a single git call —
// the worktrees share one object database. Commits git can't show (an unborn
// branch has none) are simply absent.
func commitDates(dir string, shas []string) map[string]time.Time {
	args := []string{"log", "--no-walk=unsorted", "--format=%H %cI"}
	for _, sha := range shas {
		if strings.Trim(sha, "0") != "" {
			args = append(args, sha)
		}
	}
	dates := map[string]time.Time{}
	if len(args) == 3 {
		return dates
	}
	out, err := git.Run(dir, args...)
	if err != nil {
		return dates
	}
	for _, line := range strings.Split(out, "\n") {
		sha, when, _ := strings.Cut(line, " ")
		if t, err := time.Parse(time.RFC3339, when); err == nil {
			dates[sha] = t
		}
	}
	return dates
}

// uncommitted sizes up a dirty worktree the way `git diff --shortstat` would
// if it also saw untracked files.
type uncommitted struct {
	files, insertions, deletions int
}

// uncommittedChanges counts tracked changes against head (staged or not) and
// adds every untracked file as wholly inserted. Read-only: unlike a snapshot
// it writes nothing to the object database, so it is safe to run on every
// refresh of an editor view.
func uncommittedChanges(path string) uncommitted {
	var c uncommitted
	if stat, err := git.Run(path, "diff", "--shortstat", "HEAD"); err == nil {
		c.files, c.insertions, c.deletions = parseShortstat(stat)
	}
	untracked, _ := git.UntrackedFiles(path)
	for _, file := range untracked {
		c.files++
		c.insertions += countLines(filepath.Join(path, file))
	}
	return c
}

// countLines counts a text file's lines like git does: binary files (a NUL
// byte early on) and anything over a megabyte count as none.
func countLines(file string) int {
	info, err := os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return 0
	}
	data, err := os.ReadFile(file)
	if err != nil || len(data) == 0 {
		return 0
	}
	if bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
		return 0
	}
	lines := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		lines++
	}
	return lines
}

// worktreeKind classifies a worktree: the main checkout, a base worktree, a
// wt-managed worktree (under the workspace root), or an external one created
// elsewhere by other tools.
func worktreeKind(main, managed, base bool) string {
	switch {
	case main:
		return "main"
	case base:
		return "base"
	case managed:
		return "managed"
	default:
		return "external"
	}
}

// worktreeColor picks the NAME color by kind: cyan for the main checkout,
// orange for base worktrees, green for managed ones, magenta for external.
func worktreeColor(main, managed, base bool) string {
	switch worktreeKind(main, managed, base) {
	case "main":
		return ansiCyan
	case "base":
		return ansiOrange
	case "managed":
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
