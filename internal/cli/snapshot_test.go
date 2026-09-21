package cli

import (
	"slices"
	"strings"
	"testing"

	"wt/internal/gittest"
)

// snapFixture: a linked worktree "dev" with a tracked edit, an untracked
// file and an ignored node_modules — a working state ready to snapshot. Using
// a linked worktree (not the main checkout) exercises the per-worktree ref
// location.
func snapFixture(t *testing.T) (repo, wt string) {
	t.Helper()
	repo = linkedRepo(t)
	root := wtRoot(t)
	setupOutputs(t)
	gittest.WriteFile(t, repo, ".gitignore", "node_modules/\n")
	gittest.Commit(t, repo, "gitignore")
	wt = worktreePath(root, "proj", "dev")
	gittest.Git(t, repo, "worktree", "add", "-b", "dev", wt)
	gittest.WriteFile(t, wt, "README.md", "hello\nedited\n")
	gittest.WriteFile(t, wt, "notes.txt", "untracked")
	gittest.WriteFile(t, wt, "node_modules/x.js", "junk")
	return repo, wt
}

func snapRefs(t *testing.T, wt string) []string {
	t.Helper()
	out := gittest.Git(t, wt, "for-each-ref", "--format=%(refname)", snapRefPrefix)
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func TestSnapshotRecords(t *testing.T) {
	repo, wt := snapFixture(t)
	before := gittest.Git(t, wt, "status", "--short")
	head := gittest.Git(t, wt, "rev-parse", "HEAD")
	_, errOut := setupOutputs(t)

	if err := snap(wt, nil); err != nil {
		t.Fatal(err)
	}
	refs := snapRefs(t, wt)
	if len(refs) != 1 || refs[0] != snapRefPrefix+head+"/1" {
		t.Fatalf("refs = %v, want one under the HEAD-keyed series", refs)
	}
	if after := gittest.Git(t, wt, "status", "--short"); after != before {
		t.Errorf("working tree/index changed:\nbefore: %s\nafter:  %s", before, after)
	}
	first := loadSnapshots(wt)[head][0]
	files := gittest.Git(t, wt, "ls-tree", "-r", "--name-only", first.SHA)
	if !strings.Contains(files, "notes.txt") || strings.Contains(files, "node_modules") {
		t.Errorf("snapshot tree = %q; want untracked notes.txt in, ignored node_modules out", files)
	}
	if first.Message != "snapshot 1" {
		t.Errorf("message = %q, want the default", first.Message)
	}
	if !strings.Contains(errOut.String(), "snapshot 1 recorded (2 files") {
		t.Errorf("log:\n%s", errOut.String())
	}

	// a second iteration chains onto the first, with the given message
	gittest.WriteFile(t, wt, "second.txt", "more")
	if err := snap(wt, []string{"polish", "auth"}); err != nil {
		t.Fatal(err)
	}
	series := loadSnapshots(wt)[head]
	if len(series) != 2 || series[1].N != 2 || series[1].Message != "polish auth" {
		t.Fatalf("series = %+v", series)
	}
	if parent := gittest.Git(t, wt, "rev-parse", series[1].SHA+"^"); parent != series[0].SHA {
		t.Errorf("snapshot 2's parent = %s, want snapshot 1 (%s)", parent, series[0].SHA)
	}

	// invisible to normal git and to the main checkout
	if got := gittest.Git(t, repo, "for-each-ref", snapRefPrefix); got != "" {
		t.Errorf("main checkout sees the worktree's snapshots: %q", got)
	}
	if got := gittest.Git(t, wt, "log", "--oneline", "dev"); strings.Contains(got, "snapshot") {
		t.Errorf("snapshots leaked into the branch log:\n%s", got)
	}
}

func TestSnapshotNoop(t *testing.T) {
	_, wt := snapFixture(t)
	if err := snap(wt, nil); err != nil {
		t.Fatal(err)
	}
	_, errOut := setupOutputs(t)

	if err := snap(wt, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "nothing changed since snapshot 1") || len(snapRefs(t, wt)) != 1 {
		t.Errorf("unchanged tree should be a no-op:\n%s", errOut.String())
	}

	repo := linkedRepo(t) // a clean checkout has nothing to record
	_, errOut = setupOutputs(t)
	if err := snap(repo, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "matches HEAD") || len(snapRefs(t, repo)) != 0 {
		t.Errorf("clean tree should be a no-op:\n%s", errOut.String())
	}
}

func TestSnapshotMessageFlag(t *testing.T) {
	_, wt := snapFixture(t)
	if err := snap(wt, []string{"-m", "ls"}); err != nil {
		t.Fatal(err)
	}
	head := gittest.Git(t, wt, "rev-parse", "HEAD")
	if series := loadSnapshots(wt)[head]; len(series) != 1 || series[0].Message != "ls" {
		t.Errorf("-m did not force the message: %+v", series)
	}
}

func TestSnapshotSeriesResetOnCommit(t *testing.T) {
	_, wt := snapFixture(t)
	oldBase := gittest.Git(t, wt, "rev-parse", "HEAD")
	if err := snap(wt, nil); err != nil {
		t.Fatal(err)
	}
	gittest.Commit(t, wt, "commit the work")
	gittest.WriteFile(t, wt, "next.txt", "next round")
	if err := snap(wt, nil); err != nil {
		t.Fatal(err)
	}
	newBase := gittest.Git(t, wt, "rev-parse", "HEAD")
	byBase := loadSnapshots(wt)
	if len(byBase[oldBase]) != 1 || len(byBase[newBase]) != 1 || byBase[newBase][0].N != 1 {
		t.Fatalf("series after commit = %+v", byBase)
	}

	out, _ := setupOutputs(t)
	if err := snap(wt, []string{"ls"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), shortSHA(oldBase)) || !strings.Contains(out.String(), "snapshot 1") {
		t.Errorf("ls should show only the current series:\n%s", out.String())
	}

	out, _ = setupOutputs(t)
	if err := snap(wt, []string{"ls", "--all"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), shortSHA(oldBase)) || !strings.Contains(out.String(), shortSHA(newBase)+" commit the work (current)") {
		t.Errorf("ls --all should group both series:\n%s", out.String())
	}

	out, _ = setupOutputs(t)
	if err := snap(wt, []string{"ls", oldBase[:7]}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "(current)") || !strings.Contains(out.String(), shortSHA(oldBase)) {
		t.Errorf("ls <commit> should show that series only:\n%s", out.String())
	}

	err := snap(wt, []string{"ls", "0000000"})
	if err == nil || !strings.Contains(err.Error(), "no snapshot series") {
		t.Errorf("unknown series: err = %v", err)
	}

	// an old series stays addressable
	out, _ = setupOutputs(t)
	if err := snap(wt, []string{"show", oldBase[:7] + ":1", "--stat"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "notes.txt") {
		t.Errorf("show <commit>:1 wrong:\n%s", out.String())
	}
}

func TestSnapshotShowAndDiff(t *testing.T) {
	_, wt := snapFixture(t)
	if err := snap(wt, nil); err != nil { // 1: README edit + notes.txt
		t.Fatal(err)
	}
	gittest.WriteFile(t, wt, "second.txt", "more")
	if err := snap(wt, nil); err != nil { // 2: + second.txt
		t.Fatal(err)
	}

	run := func(args ...string) string {
		t.Helper()
		out, _ := setupOutputs(t)
		if err := snap(wt, args); err != nil {
			t.Fatalf("wt snap %v: %v", args, err)
		}
		return out.String()
	}
	only := func(got, name string, want, notWant string) {
		t.Helper()
		if !strings.Contains(got, want) || strings.Contains(got, notWant) {
			t.Errorf("%s: want %q and not %q in:\n%s", name, want, notWant, got)
		}
	}
	only(run("show", "--stat"), "show (latest)", "second.txt", "notes.txt")
	only(run("show", "2", "--stat"), "show 2", "second.txt", "notes.txt")
	only(run("diff", "2", "--stat"), "diff 2", "second.txt", "notes.txt")
	only(run("diff", "1", "2", "--stat"), "diff 1 2", "second.txt", "notes.txt")
	only(run("diff", "1", "--stat"), "diff 1", "notes.txt", "second.txt")

	full := run("diff", "--full", "2", "--stat")
	if !strings.Contains(full, "second.txt") || !strings.Contains(full, "notes.txt") {
		t.Errorf("diff --full 2 should be cumulative:\n%s", full)
	}

	// the gotcha: an untracked file added after the last snapshot must show
	// up in a diff against the working tree
	gittest.WriteFile(t, wt, "later.txt", "even later")
	only(run("diff", "--stat"), "diff (working tree)", "later.txt", "second.txt")
	everything := run("diff", "--full", "--stat")
	for _, f := range []string{"README.md", "notes.txt", "second.txt", "later.txt"} {
		if !strings.Contains(everything, f) {
			t.Errorf("diff --full missing %s:\n%s", f, everything)
		}
	}

	// no snapshots yet: fall back to the base with a note
	fresh := linkedRepo(t)
	gittest.WriteFile(t, fresh, "x.txt", "x")
	out, errOut := setupOutputs(t)
	if err := snap(fresh, []string{"diff", "--stat"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "x.txt") || !strings.Contains(errOut.String(), "no snapshots yet") {
		t.Errorf("diff without snapshots:\n%s\n%s", out.String(), errOut.String())
	}
}

func TestSnapshotPurge(t *testing.T) {
	_, wt := snapFixture(t)
	if err := snap(wt, nil); err != nil {
		t.Fatal(err)
	}
	gittest.Commit(t, wt, "commit")
	gittest.WriteFile(t, wt, "next.txt", "x")
	if err := snap(wt, nil); err != nil {
		t.Fatal(err)
	}
	setupOutputs(t)

	stdinInput(t, "n\n")
	if err := snap(wt, []string{"purge"}); err == nil || err.Error() != "aborted" {
		t.Fatalf("err = %v, want aborted", err)
	}
	if len(snapRefs(t, wt)) != 2 {
		t.Fatal("aborted purge deleted something")
	}

	stdinInput(t, "y\n")
	if err := snap(wt, []string{"purge"}); err != nil {
		t.Fatal(err)
	}
	if refs := snapRefs(t, wt); len(refs) != 1 || strings.Contains(refs[0], gittest.Git(t, wt, "rev-parse", "HEAD")) {
		t.Errorf("purge should drop only the current series, left %v", refs)
	}

	stdinInput(t, "y\n")
	if err := snap(wt, []string{"purge", "--all"}); err != nil {
		t.Fatal(err)
	}
	if len(snapRefs(t, wt)) != 0 {
		t.Error("purge --all left snapshots behind")
	}
}

func TestSnapshotRev(t *testing.T) {
	_, wt := snapFixture(t)
	if err := snap(wt, nil); err != nil {
		t.Fatal(err)
	}
	head := gittest.Git(t, wt, "rev-parse", "HEAD")
	out, _ := setupOutputs(t)

	if err := snap(wt, []string{"rev", "1"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != loadSnapshots(wt)[head][0].SHA {
		t.Errorf("rev 1 = %q", got)
	}
	err := snap(wt, []string{"rev", "9"})
	if err == nil || !strings.Contains(err.Error(), "no snapshot 9") {
		t.Errorf("err = %v", err)
	}
}

func TestSnapshotStatusLine(t *testing.T) {
	_, wt := snapFixture(t)
	if err := snap(wt, nil); err != nil {
		t.Fatal(err)
	}
	out, _ := setupOutputs(t)

	if err := status(wt, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `snapshots 1 (latest "snapshot 1", just now)`) {
		t.Errorf("status missing the snapshots line:\n%s", out.String())
	}
}

func TestSnapshotRemoveNote(t *testing.T) {
	repo, wt := snapFixture(t)
	if err := snap(wt, nil); err != nil {
		t.Fatal(err)
	}
	_, errOut := setupOutputs(t)

	if err := remove(repo, []string{"-f", "dev"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "1 snapshot(s) in 'dev' go with it") {
		t.Errorf("remove log:\n%s", errOut.String())
	}
}

func TestSnapshotCompletion(t *testing.T) {
	_, wt := snapFixture(t)
	if err := snap(wt, nil); err != nil {
		t.Fatal(err)
	}
	setupOutputs(t)

	if got := completionCandidates(wt, []string{"snap"}); !slices.Equal(got, snapMenu) {
		t.Errorf("snap submenu = %v", got)
	}
	if got := completionCandidates(wt, []string{"snap", "show"}); !slices.Contains(got, "1:snapshot 1 (now)") {
		t.Errorf("snapshot candidates = %v", got)
	}
	if got := completionCandidates(wt, []string{"snapshot", "ls"}); len(got) != 2 || got[0] != "--all:every series" {
		t.Errorf("series candidates = %v", got)
	}
}
