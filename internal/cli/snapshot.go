package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"wt/internal/git"
)

const snapUsage = `usage: wt snapshot [-m <msg>] [msg...]   (alias: snap)
       wt snap ls [--all | <commit>]
       wt snap show [N] [git show args]
       wt snap diff [--full] [A] [B] [git diff args]
       wt snap purge [--all]
       wt snap rev N`

// snapRefPrefix is where snapshots live: refs/worktree/ is git's per-worktree
// namespace (stored in the worktree's own admin dir), so snapshots are never
// on a branch, not carried by ordinary pushes (only --mirror copies every
// ref), invisible to other worktrees, and die with the worktree. Below it:
// <base commit sha>/<N>.
const snapRefPrefix = "refs/worktree/wt/snap/"

// snapshot is one recorded iteration: a commit object whose tree is the
// working files (tracked edits and untracked files, ignored ones excluded)
// and whose parent is the previous snapshot of its series — or the base
// commit for N=1 — so "git show" of it is exactly what that iteration added.
type snapshot struct {
	N       int
	SHA     string
	Tree    string
	Base    string // the commit the series sits on
	When    time.Time
	Message string
}

// snap records private, diffable snapshots of the worktree while you polish a
// change: no commit, nothing on the branch, nothing to push. A series is keyed
// by the commit you are on; committing starts a fresh one and the old series
// stays until purged.
func snap(dir string, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "ls", "list":
			return snapList(dir, args[1:])
		case "show":
			return snapShow(dir, args[1:])
		case "diff":
			return snapDiff(dir, args[1:])
		case "purge":
			return snapPurge(dir, args[1:])
		case "rev":
			return snapRev(dir, args[1:])
		}
	}
	return snapRecord(dir, args)
}

func snapRecord(dir string, args []string) error {
	fs := flag.NewFlagSet("wt snapshot", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	message := fs.String("m", "", "message (use when it would read as a subcommand)")
	if err := fs.Parse(args); err != nil {
		return errors.New(snapUsage)
	}
	if *message == "" {
		*message = strings.TrimSpace(strings.Join(fs.Args(), " "))
	}
	wt, err := snapWorktree(dir)
	if err != nil {
		return err
	}
	base, err := git.Run(wt.Path, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	series := loadSnapshots(wt.Path)[base]
	tree, err := snapTree(wt.Path)
	if err != nil {
		return err
	}

	n, parent := 1, base
	if len(series) > 0 {
		last := series[len(series)-1]
		if last.Tree == tree {
			logf("nothing changed since snapshot %d", last.N)
			return nil
		}
		n, parent = last.N+1, last.SHA
	} else if headTree, _ := git.Run(wt.Path, "rev-parse", "HEAD^{tree}"); headTree == tree {
		logf("nothing to snapshot — the working tree matches HEAD")
		return nil
	}
	if *message == "" {
		*message = fmt.Sprintf("snapshot %d", n)
	}
	sha, err := git.Run(wt.Path, "commit-tree", tree, "-p", parent, "-m", *message)
	if err != nil {
		return err
	}
	if _, err := git.Run(wt.Path, "update-ref", snapRef(base, n), sha); err != nil {
		return err
	}
	logf("snapshot %d recorded (%s)", n, changeSummary(wt.Path, parent, sha))
	return nil
}

func snapList(dir string, args []string) error {
	fs := flag.NewFlagSet("wt snap ls", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	all := fs.Bool("all", false, "every series")
	if err := fs.Parse(args); err != nil || fs.NArg() > 1 || (*all && fs.NArg() == 1) {
		return errors.New("usage: wt snap ls [--all | <commit>]")
	}
	wt, err := snapWorktree(dir)
	if err != nil {
		return err
	}
	byBase := loadSnapshots(wt.Path)
	if len(byBase) == 0 {
		logf("no snapshots — record one: wt snap [message]")
		return nil
	}
	head, _ := git.Run(wt.Path, "rev-parse", "HEAD")

	var bases []string
	switch {
	case *all:
		bases = seriesOrder(byBase, head)
	case fs.NArg() == 1:
		base, err := resolveSeries(byBase, fs.Arg(0))
		if err != nil {
			return err
		}
		bases = []string{base}
	default:
		if _, ok := byBase[head]; !ok {
			logf("no snapshots on %s — %d earlier series: wt snap ls --all", shortSHA(head), len(byBase))
			return nil
		}
		bases = []string{head}
	}

	paint := colorEnabled()
	for i, base := range bases {
		if *all || fs.NArg() == 1 { // an explicit choice gets its label; the implicit current series doesn't
			if i > 0 {
				fmt.Fprintln(stdout)
			}
			label := "on " + shortSHA(base) + " " + commitSubject(wt.Path, base)
			if base == head {
				label += " (current)"
			}
			if paint {
				label = ansiCyan + label + ansiReset
			}
			fmt.Fprintln(stdout, label)
		}
		series := byBase[base]
		rows := make([][]string, 0, len(series))
		for j := len(series) - 1; j >= 0; j-- { // newest first, like git log
			s := series[j]
			rows = append(rows, []string{strconv.Itoa(s.N), ago(s.When), changeSummary(wt.Path, snapParent(series, j), s.SHA), s.Message})
		}
		printTable([]string{"N", "AGE", "FILES", "MESSAGE"}, rows)
	}
	return nil
}

func snapShow(dir string, args []string) error {
	wt, err := snapWorktree(dir)
	if err != nil {
		return err
	}
	byBase, head := loadSnapshots(wt.Path), snapHead(wt.Path)
	spec, rest := splitSpec(args)
	var target snapshot
	if spec != "" {
		if target, err = resolveSnapshot(byBase, head, spec); err != nil {
			return err
		}
	} else {
		series := byBase[head]
		if len(series) == 0 {
			return hintf("record one: wt snap [message]", "no snapshots on this commit")
		}
		target = series[len(series)-1]
	}
	return git.RunPassthrough(wt.Path, stdout, stderr, append([]string{"show", target.SHA}, rest...)...)
}

// snapDiff compares against "the snapshot right before": the working tree vs
// the latest snapshot, or snapshot N vs N-1. --full moves that reference to
// the base commit — everything since the commit, like git diff but with
// untracked files. Two snapshots compare directly. Extra args go to git diff.
func snapDiff(dir string, args []string) error {
	var (
		full  bool
		specs []string
		rest  []string
	)
	for _, a := range args {
		switch {
		case a == "--full":
			full = true
		case len(specs) < 2 && isSnapSpec(a):
			specs = append(specs, a)
		default:
			rest = append(rest, a)
		}
	}
	wt, err := snapWorktree(dir)
	if err != nil {
		return err
	}
	byBase, head := loadSnapshots(wt.Path), snapHead(wt.Path)

	var left, right string
	switch len(specs) {
	case 2:
		a, err := resolveSnapshot(byBase, head, specs[0])
		if err != nil {
			return err
		}
		b, err := resolveSnapshot(byBase, head, specs[1])
		if err != nil {
			return err
		}
		left, right = a.SHA, b.SHA
	case 1:
		s, err := resolveSnapshot(byBase, head, specs[0])
		if err != nil {
			return err
		}
		right = s.SHA
		left = s.Base
		if !full {
			left = snapParent(byBase[s.Base], s.N-1)
		}
	default:
		// The working tree must be turned into a tree object first: a plain
		// `git diff <sha>` against the files would treat every untracked
		// file as deleted.
		if right, err = snapTree(wt.Path); err != nil {
			return err
		}
		series := byBase[head]
		switch {
		case full:
			left = head
		case len(series) == 0:
			logf("no snapshots yet — comparing with HEAD")
			left = head
		default:
			left = series[len(series)-1].SHA
		}
	}
	return git.RunPassthrough(wt.Path, stdout, stderr, append([]string{"diff", left, right}, rest...)...)
}

func snapPurge(dir string, args []string) error {
	fs := flag.NewFlagSet("wt snap purge", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	all := fs.Bool("all", false, "every series")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return errors.New("usage: wt snap purge [--all]")
	}
	wt, err := snapWorktree(dir)
	if err != nil {
		return err
	}
	byBase, head := loadSnapshots(wt.Path), snapHead(wt.Path)
	var victims []snapshot
	if *all {
		for _, series := range byBase {
			victims = append(victims, series...)
		}
	} else {
		victims = byBase[head]
		if len(victims) == 0 && len(byBase) > 0 {
			return hintf("wt snap purge --all clears every series", "no snapshots on this commit")
		}
	}
	if len(victims) == 0 {
		logf("no snapshots to purge")
		return nil
	}
	if *all {
		logf("%d snapshot(s) across %d series", len(victims), len(byBase))
	}
	if !confirm(fmt.Sprintf("delete %d snapshot(s)?", len(victims))) {
		return errors.New("aborted")
	}
	for _, s := range victims {
		if _, err := git.Run(wt.Path, "update-ref", "-d", snapRef(s.Base, s.N), s.SHA); err != nil {
			return err
		}
	}
	logf("purged %d snapshot(s) — the objects expire with git's next gc", len(victims))
	return nil
}

func snapRev(dir string, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: wt snap rev <N | commit:N>")
	}
	wt, err := snapWorktree(dir)
	if err != nil {
		return err
	}
	s, err := resolveSnapshot(loadSnapshots(wt.Path), snapHead(wt.Path), args[0])
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, s.SHA)
	return nil
}

// snapWorktree is the worktree a snapshot command applies to — the one you
// stand in; snapshots always cover it whole, whatever subdirectory you are in.
func snapWorktree(dir string) (*git.Worktree, error) {
	if _, ok := findPeekRoot(dir); ok {
		return nil, hintf("a peek is a throwaway export — wt unpeek", "snapshots need a real worktree")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return nil, err
	}
	return ws.currentWorktree()
}

func snapHead(wtPath string) string {
	head, _ := git.Run(wtPath, "rev-parse", "HEAD")
	return head
}

// snapTree turns the working files into a tree object without touching the
// real index: a copy of it (so the stat cache keeps add -A fast) receives
// `add -A` — tracked edits and untracked files, ignored ones excluded, no
// hooks — and write-tree hands back the sha.
func snapTree(wtPath string) (string, error) {
	index, err := git.Run(wtPath, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp("", "wt-snap-index-*")
	if err != nil {
		return "", err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if data, err := os.ReadFile(index); err == nil {
		if err := os.WriteFile(tmp.Name(), data, 0o600); err != nil {
			return "", err
		}
	}
	env := []string{"GIT_INDEX_FILE=" + tmp.Name()}
	if _, err := git.RunEnv(wtPath, env, "add", "-A"); err != nil {
		return "", err
	}
	return git.RunEnv(wtPath, env, "write-tree")
}

// loadSnapshots reads every series of the worktree, each sorted by N.
func loadSnapshots(wtPath string) map[string][]snapshot {
	out, err := git.Run(wtPath, "for-each-ref",
		"--format=%(refname)%09%(objectname)%09%(tree)%09%(committerdate:unix)%09%(subject)", snapRefPrefix)
	if err != nil || out == "" {
		return nil
	}
	byBase := map[string][]snapshot{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\t", 5)
		if len(f) < 5 {
			continue
		}
		base, nStr, ok := strings.Cut(strings.TrimPrefix(f[0], snapRefPrefix), "/")
		n, err := strconv.Atoi(nStr)
		if !ok || err != nil {
			continue
		}
		secs, _ := strconv.ParseInt(f[3], 10, 64)
		byBase[base] = append(byBase[base], snapshot{N: n, SHA: f[1], Tree: f[2], Base: base, When: time.Unix(secs, 0), Message: f[4]})
	}
	for _, series := range byBase {
		sort.Slice(series, func(i, j int) bool { return series[i].N < series[j].N })
	}
	return byBase
}

func snapRef(base string, n int) string {
	return snapRefPrefix + base + "/" + strconv.Itoa(n)
}

// snapParent is what snapshot series[i] diffs against: the one before it, or
// the base commit for the first.
func snapParent(series []snapshot, i int) string {
	if i <= 0 {
		return series[0].Base
	}
	return series[i-1].SHA
}

// seriesOrder lists bases with the current one first, then by most recent
// snapshot.
func seriesOrder(byBase map[string][]snapshot, head string) []string {
	bases := make([]string, 0, len(byBase))
	for base := range byBase {
		bases = append(bases, base)
	}
	newest := func(base string) time.Time {
		series := byBase[base]
		return series[len(series)-1].When
	}
	sort.Slice(bases, func(i, j int) bool {
		if (bases[i] == head) != (bases[j] == head) {
			return bases[i] == head
		}
		return newest(bases[i]).After(newest(bases[j]))
	})
	return bases
}

// resolveSeries matches a base commit by unique sha prefix.
func resolveSeries(byBase map[string][]snapshot, spec string) (string, error) {
	var matches []string
	for base := range byBase {
		if strings.HasPrefix(base, spec) {
			matches = append(matches, base)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", hintf("wt snap ls --all lists the series", "no snapshot series on '%s'", spec)
	default:
		sort.Strings(matches)
		short := make([]string, len(matches))
		for i, m := range matches {
			short[i] = shortSHA(m)
		}
		return "", fmt.Errorf("'%s' is ambiguous: %s", spec, strings.Join(short, ", "))
	}
}

// resolveSnapshot finds "N" in the current series or "<commit>:N" elsewhere.
func resolveSnapshot(byBase map[string][]snapshot, head, spec string) (snapshot, error) {
	base, nStr := head, spec
	if b, n, ok := strings.Cut(spec, ":"); ok {
		var err error
		if base, err = resolveSeries(byBase, b); err != nil {
			return snapshot{}, err
		}
		nStr = n
	}
	n, err := strconv.Atoi(nStr)
	if err != nil || n < 1 {
		return snapshot{}, fmt.Errorf("'%s' is not a snapshot number (N, or <commit>:N)", spec)
	}
	for _, s := range byBase[base] {
		if s.N == n {
			return s, nil
		}
	}
	return snapshot{}, hintf("wt snap ls shows them", "no snapshot %d on %s", n, shortSHA(base))
}

func isSnapSpec(arg string) bool {
	base, n, ok := strings.Cut(arg, ":")
	if !ok {
		base, n = "", arg
	}
	if _, err := strconv.Atoi(n); err != nil || n == "" {
		return false
	}
	for _, r := range base {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// changeSummary compacts `git diff --shortstat` to "3 files +40 −2".
func changeSummary(wtPath, from, to string) string {
	stat, err := git.Run(wtPath, "diff", "--shortstat", from, to)
	if err != nil || stat == "" {
		return "-"
	}
	var out []string
	for _, part := range strings.Split(stat, ",") {
		fields := strings.Fields(part)
		if len(fields) < 2 {
			continue
		}
		switch {
		case strings.HasPrefix(fields[1], "file"):
			out = append(out, fields[0]+" "+fields[1])
		case strings.HasPrefix(fields[1], "insertion"):
			out = append(out, "+"+fields[0])
		case strings.HasPrefix(fields[1], "deletion"):
			out = append(out, "−"+fields[0])
		}
	}
	return strings.Join(out, " ")
}

func commitSubject(wtPath, sha string) string {
	subject, _ := git.Run(wtPath, "log", "-1", "--format=%s", sha)
	return truncate(subject, 40)
}

// printTable renders header + rows on stdout with the two-space padded
// layout the other tables use.
func printTable(header []string, rows [][]string) {
	widths := make([]int, len(header))
	for c, h := range header {
		widths[c] = len(h)
	}
	for _, row := range rows {
		for c, cell := range row {
			if n := len([]rune(cell)); n > widths[c] {
				widths[c] = n
			}
		}
	}
	line := func(cells []string) {
		var b strings.Builder
		for c, cell := range cells {
			if c > 0 {
				b.WriteString("  ")
			}
			if c == len(cells)-1 {
				b.WriteString(cell) // last column: no trailing padding
			} else {
				b.WriteString(pad(cell, widths[c]))
			}
		}
		fmt.Fprintln(stdout, b.String())
	}
	line(header)
	for _, row := range rows {
		line(row)
	}
}

func countSnapshots(wtPath string) int {
	n := 0
	for _, series := range loadSnapshots(wtPath) {
		n += len(series)
	}
	return n
}

// agoPhrase reads naturally in a sentence: "5m ago", "just now".
func agoPhrase(t time.Time) string {
	if a := ago(t); a != "now" {
		return a + " ago"
	}
	return "just now"
}
