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
	"unicode/utf8"

	"wt/internal/config"
	"wt/internal/git"
)

// df shows per-worktree disk usage, hardlink-aware: naive per-tree sizes
// would count a linked node_modules in full everywhere and hide exactly the
// savings deps linking exists for.
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
		name   string
		color  string
		state  string
		size   int64
		deps   int64
		shared int64
	}
	mainPath := ws.repo.Worktrees[0].Path
	var rows []row
	var paths []string
	for _, wt := range ws.repo.Worktrees {
		if wt.Bare {
			continue
		}
		_, isBase := git.ReadBaseMark(wt.Path)
		rows = append(rows, row{
			name:  filepath.Base(wt.Path),
			color: worktreeColor(wt.Path == mainPath, strings.HasPrefix(wt.Path, ws.root+"/"), isBase),
			state: depsStateCell(wt.Path),
		})
		paths = append(paths, wt.Path)
	}
	for _, p := range findPeeks(ws.root) {
		rows = append(rows, row{name: filepath.Base(p), color: ansiGreen, state: depsStateCell(p)})
		paths = append(paths, p)
	}

	seen := newInodeSet()
	errs := make([]error, len(rows))
	var wg sync.WaitGroup
	for i := range rows {
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			rows[i].size, rows[i].deps, rows[i].shared, errs[i] = diskUsage(path, deps, seen)
		}(i, paths[i])
	}
	wg.Wait()
	var naive int64
	for i := range rows {
		if errs[i] != nil {
			return errs[i]
		}
		naive += rows[i].size
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].size > rows[j].size })

	header := []string{"NAME", "DEPS", "SIZE", "DEPS-SIZE", "SHARED"}
	cells := make([][]string, 0, len(rows)+1)
	for _, r := range rows {
		cells = append(cells, []string{r.name, r.state, humanBytes(r.size), dashBytes(r.deps), dashBytes(r.shared)})
	}
	totalCell := ""
	if savings := naive - seen.total; savings > 0 {
		totalCell = "(" + humanBytes(savings) + " saved by sharing)"
	}
	cells = append(cells, []string{"TOTAL", "", humanBytes(seen.total), "", totalCell})

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

// inodeSet deduplicates hardlinked files across parallel walks and keeps the
// running unique-bytes total.
type inodeSet struct {
	mu    sync.Mutex
	seen  map[inodeKey]struct{}
	total int64
}

type inodeKey struct{ dev, ino uint64 }

func newInodeSet() *inodeSet { return &inodeSet{seen: map[inodeKey]struct{}{}} }

// add counts one file toward the unique total — only the first time an inode
// appears; files with a single link skip the map entirely.
func (s *inodeSet) add(st *syscall.Stat_t, bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st.Nlink > 1 {
		key := inodeKey{uint64(st.Dev), uint64(st.Ino)}
		if _, ok := s.seen[key]; ok {
			return
		}
		s.seen[key] = struct{}{}
	}
	s.total += bytes
}

// diskUsage walks root, returning real (block-based) usage of its files, the
// subset under the declared dep prefixes, and the hardlink-shared subset;
// every file also feeds seen for the cross-worktree unique total. The
// top-level .git entry is skipped so rows compare working files, not git's
// object store; directory entries themselves are not counted.
func diskUsage(root string, deps []string, seen *inodeSet) (size, depsSize, shared int64, err error) {
	prefixes := make([]string, 0, len(deps))
	for _, d := range deps {
		prefixes = append(prefixes, strings.TrimSuffix(d, "/"))
	}
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
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
		size += bytes
		for _, pre := range prefixes {
			if rel == pre || strings.HasPrefix(rel, pre+string(os.PathSeparator)) {
				depsSize += bytes
				break
			}
		}
		if info.Mode().IsRegular() && st.Nlink > 1 {
			shared += bytes
		}
		seen.add(st, bytes)
		return nil
	})
	return size, depsSize, shared, err
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
