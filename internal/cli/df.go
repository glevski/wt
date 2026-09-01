package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"wt/internal/config"
	"wt/internal/git"
)

// df shows what each worktree actually costs on disk. Hardlink-shared bytes
// are charged to the first row that holds them (du-style, in canonical
// order: root, bases, then by last checkout), so a linked fork shows only
// its own few megabytes and the SIZE column sums exactly to TOTAL.
func df(dir string, args []string) error {
	if len(args) != 0 {
		return errors.New("usage: wt df")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	deps := config.Deps(ws.dir())

	type row struct {
		name, color, state string
		path               string
		isRoot, isBase     bool
		checkout           time.Time
		checkoutOK         bool
		created            time.Time
		createdOK          bool
		u                  treeUsage
		size, depsSize     int64 // attributed (first owner pays)
		shared, full       int64 // this tree's shared subset / du-alone size
	}
	mainPath := ws.repo.Worktrees[0].Path
	var rows []row
	for _, wt := range ws.repo.Worktrees {
		if wt.Bare {
			continue
		}
		_, isBase := git.ReadBaseMark(wt.Path)
		r := row{
			name:   filepath.Base(wt.Path),
			color:  worktreeColor(wt.Path == mainPath, strings.HasPrefix(wt.Path, ws.root+"/"), isBase),
			state:  depsStateCell(wt.Path),
			path:   wt.Path,
			isRoot: wt.Path == mainPath,
			isBase: isBase,
		}
		r.checkout, r.checkoutOK = git.CheckoutStamp(wt.Path)
		r.created, r.createdOK = git.CreatedAt(wt.Path)
		rows = append(rows, r)
	}
	for _, p := range findPeeks(ws.root) {
		rows = append(rows, row{name: filepath.Base(p), color: ansiGreen, state: depsStateCell(p), path: p})
	}

	errs := make([]error, len(rows))
	var wg sync.WaitGroup
	for i := range rows {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rows[i].u, errs[i] = diskUsage(rows[i].path, deps)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	// first-owner attribution, in canonical order
	order := make([]int, len(rows))
	for i := range order {
		order[i] = i
	}
	group := func(r row) int {
		switch {
		case r.isRoot:
			return 0
		case r.isBase:
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(order, func(a, b int) bool {
		ra, rb := rows[order[a]], rows[order[b]]
		if ga, gb := group(ra), group(rb); ga != gb {
			return ga < gb
		}
		if ra.checkoutOK != rb.checkoutOK {
			return ra.checkoutOK
		}
		if ra.checkoutOK && !ra.checkout.Equal(rb.checkout) {
			return ra.checkout.After(rb.checkout)
		}
		if ra.createdOK != rb.createdOK {
			return ra.createdOK
		}
		if ra.createdOK && !ra.created.Equal(rb.created) {
			return ra.created.After(rb.created)
		}
		return ra.name < rb.name
	})
	seen := map[inodeKey]bool{}
	var total, naive int64
	for _, i := range order {
		r := &rows[i]
		r.size, r.depsSize, r.full = r.u.own, r.u.ownDeps, r.u.own
		for k, bytes := range r.u.shared {
			r.full += bytes
			r.shared += bytes
			if !seen[k] {
				seen[k] = true
				r.size += bytes
				if _, isDep := r.u.sharedDeps[k]; isDep {
					r.depsSize += bytes
				}
			}
		}
		total += r.size
		naive += r.full
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].size > rows[j].size })

	header := []string{"NAME", "DEPS", "SIZE", "DEPS-SIZE", "SHARED"}
	cells := make([][]string, 0, len(rows)+1)
	for _, r := range rows {
		cells = append(cells, []string{r.name, r.state, humanBytes(r.size), dashBytes(r.depsSize), dashBytes(r.shared)})
	}
	totalCell := ""
	if savings := naive - total; savings > 0 {
		totalCell = "(" + humanBytes(savings) + " saved by sharing)"
	}
	cells = append(cells, []string{"TOTAL", "", humanBytes(total), "", totalCell})

	widths := make([]int, len(header))
	for c, h := range header {
		widths[c] = len(h)
	}
	for _, row := range cells {
		for c, cell := range row {
			if n := utf8.RuneCountInString(cell); n > widths[c] {
				widths[c] = n
			}
		}
	}
	paint := colorEnabled()
	printRow := func(row []string, color string) {
		var b strings.Builder
		b.WriteString("  ")
		for c, cell := range row {
			if c > 0 {
				b.WriteString("  ")
			}
			padded := pad(cell, widths[c])
			if paint && c == 0 && color != "" {
				padded = color + padded + ansiReset
			}
			b.WriteString(padded)
		}
		fmt.Fprintln(stdout, strings.TrimRight(b.String(), " "))
	}
	printRow(header, "")
	for i, row := range cells {
		color := ""
		if i < len(rows) {
			color = rows[i].color
		}
		printRow(row, color)
	}
	return nil
}

// depsStateCell maps a worktree's deps state marker for table display.
func depsStateCell(path string) string {
	state, ok := git.DepsState(path)
	if !ok {
		return "-"
	}
	switch {
	case strings.HasPrefix(state, "copying"):
		return "copying…"
	case state == "done":
		return "copied"
	}
	return state
}

type inodeKey struct{ dev, ino uint64 }

// treeUsage is one worktree's walk result: bytes owned outright (single-link
// files), plus each shared inode once — attribution across worktrees happens
// later, in a deterministic order.
type treeUsage struct {
	own, ownDeps int64
	shared       map[inodeKey]int64
	sharedDeps   map[inodeKey]struct{}
}

// diskUsage walks root, block-based like du and without following symlinks.
// The top-level .git entry is skipped (rows compare working files, not git's
// object store) and directory entries themselves are not counted. A shared
// inode hardlinked several times inside the same tree still counts once,
// matching du run on that tree alone.
func diskUsage(root string, deps []string) (treeUsage, error) {
	u := treeUsage{shared: map[inodeKey]int64{}, sharedDeps: map[inodeKey]struct{}{}}
	prefixes := make([]string, 0, len(deps))
	for _, d := range deps {
		prefixes = append(prefixes, strings.TrimSuffix(d, "/"))
	}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // a racing removal is not our problem
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		if rel == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil
		}
		bytes := st.Blocks * 512
		underDep := false
		for _, pre := range prefixes {
			if rel == pre || strings.HasPrefix(rel, pre+string(os.PathSeparator)) {
				underDep = true
				break
			}
		}
		if info.Mode().IsRegular() && st.Nlink > 1 {
			key := inodeKey{uint64(st.Dev), uint64(st.Ino)}
			u.shared[key] = bytes
			if underDep {
				u.sharedDeps[key] = struct{}{}
			}
			return nil
		}
		u.own += bytes
		if underDep {
			u.ownDeps += bytes
		}
		return nil
	})
	return u, err
}

// humanBytes renders like du -h: bytes, then K/M/G/T with one decimal
// under 10.
func humanBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	const units = "KMGT"
	v := float64(n)
	for i := 0; i < len(units); i++ {
		v /= 1024
		if v < 1024 || i == len(units)-1 {
			if v < 10 {
				return fmt.Sprintf("%.1f%c", v, units[i])
			}
			return fmt.Sprintf("%.0f%c", v, units[i])
		}
	}
	return ""
}

func dashBytes(n int64) string {
	if n == 0 {
		return "-"
	}
	return humanBytes(n)
}
