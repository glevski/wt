package cli

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"wt/internal/git"
	"wt/internal/gittest"
)

func TestList(t *testing.T) {
	repo := gittest.NewRepo(t)
	wtRoot(t)
	side := filepath.Join(t.TempDir(), "side")
	gittest.Git(t, repo, "worktree", "add", "-b", "side", side)
	gittest.WriteFile(t, side, "dirty.txt", "x")
	out, _ := setupOutputs(t)

	if err := list(repo, nil); err != nil {
		t.Fatal(err)
	}

	sha := gittest.Git(t, repo, "rev-parse", "HEAD")[:7]
	mainRow, sideRow := listRows(t, out.String(), filepath.Base(repo))
	if !strings.HasPrefix(mainRow, "*") || !strings.Contains(mainRow, "clean") {
		t.Errorf("main row = %q, want current marker and clean state", mainRow)
	}
	if !strings.Contains(mainRow, sha) || !strings.Contains(sideRow, sha) {
		t.Errorf("rows missing short commit %s:\nmain: %q\nside: %q", sha, mainRow, sideRow)
	}
	if strings.HasPrefix(sideRow, "*") || !strings.Contains(sideRow, "dirty") {
		t.Errorf("side row = %q, want no marker and dirty state", sideRow)
	}
	// side was just created but never jumped into: CREATED=now, CHECKOUT=-
	if !strings.Contains(sideRow, "now") || !strings.Contains(sideRow, "-") {
		t.Errorf("side row = %q, want created 'now' and checkout '-'", sideRow)
	}
	if strings.Contains(mainRow, side) || strings.Contains(mainRow, repo) {
		t.Errorf("paths should no longer be listed: %q", mainRow)
	}

	// a wt checkout stamps the worktree; its CHECKOUT cell fills in
	if err := checkout(repo, []string{"side"}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := list(repo, nil); err != nil {
		t.Fatal(err)
	}
	_, sideRow = listRows(t, out.String(), filepath.Base(repo))
	if strings.Contains(sideRow, "-") {
		t.Errorf("side row after checkout = %q, want no empty cells", sideRow)
	}
}

func TestListOrder(t *testing.T) {
	repo := linkedRepo(t)
	wtRoot(t)
	setupOutputs(t)
	gittest.Git(t, repo, "branch", "staging")
	if err := base(repo, []string{"add", "staging"}); err != nil {
		t.Fatal(err)
	}
	alpha := filepath.Join(t.TempDir(), "alpha")
	beta := filepath.Join(t.TempDir(), "beta")
	gittest.Git(t, repo, "worktree", "add", "-b", "alpha", alpha)
	gittest.Git(t, repo, "worktree", "add", "-b", "beta", beta)
	git.TouchCheckoutStamp(beta) // beta was visited, alpha never

	out, _ := setupOutputs(t)
	if err := list(repo, nil); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n")[1:] {
		names = append(names, strings.Fields(line[2:])[0])
	}
	want := []string{filepath.Base(repo), "staging", "beta", "alpha"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("row order = %v, want %v (root, bases, then by last checkout)", names, want)
	}
}

func TestListJSON(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	setupOutputs(t)
	gittest.Git(t, repo, "branch", "staging")
	if err := base(repo, []string{"add", "staging"}); err != nil {
		t.Fatal(err)
	}
	if err := create(repo, nil); err != nil { // managed, auto-named main-2
		t.Fatal(err)
	}
	side := filepath.Join(t.TempDir(), "side")
	gittest.Git(t, repo, "worktree", "add", "-b", "side", side)
	gittest.WriteFile(t, side, "dirty.txt", "x")
	if err := peek(repo, []string{"HEAD"}); err != nil {
		t.Fatal(err)
	}

	out, _ := setupOutputs(t)
	if err := list(repo, []string{"--json"}); err != nil {
		t.Fatal(err)
	}
	var doc listJSON
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if doc.Schema != 1 || doc.Project != "proj" || !doc.Linked || doc.Root != filepath.Join(root, "proj") {
		t.Errorf("header = %+v", doc)
	}

	rows := map[string]worktreeJSON{}
	var names []string
	for _, w := range doc.Worktrees {
		rows[w.Name] = w
		names = append(names, w.Name)
	}
	mainName := filepath.Base(repo)
	if len(names) != 4 || names[0] != mainName || names[1] != "staging" {
		t.Fatalf("worktrees = %v, want root and base first, like the table", names)
	}

	sha := gittest.Git(t, repo, "rev-parse", "HEAD")
	if w := rows[mainName]; w.Kind != "main" || !w.Current || w.Path != repo || w.Head != sha ||
		w.Branch != "main" || w.State != "clean" {
		t.Errorf("main = %+v", w)
	}
	if w := rows["staging"]; w.Kind != "base" || w.Pinned != "staging" || w.Drifted || w.Current {
		t.Errorf("base = %+v", w)
	}
	if w := rows["main-2"]; w.Kind != "managed" || w.Base != "main" ||
		w.Path != worktreePath(root, "proj", "main-2") {
		t.Errorf("managed = %+v", w)
	}
	if w := rows["side"]; w.Kind != "external" || w.State != "dirty" || w.Path != side ||
		w.Created.IsZero() || !w.Checkout.IsZero() {
		t.Errorf("external = %+v", w)
	}
	// committer date of each head; the fixture commits a moment ago
	when, err := time.Parse(time.RFC3339, gittest.Git(t, repo, "log", "-1", "--format=%cI"))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range doc.Worktrees {
		if !w.Committed.Equal(when) {
			t.Errorf("%s committed = %s, want %s", w.Name, w.Committed, when)
		}
	}
	if strings.Contains(out.String(), `"checkout": "0001`) {
		t.Errorf("unknown times must be omitted:\n%s", out.String())
	}

	if len(doc.Peeks) != 1 {
		t.Fatalf("peeks = %+v, want one", doc.Peeks)
	}
	if p := doc.Peeks[0]; p.Name != "peek-HEAD" || p.Rev != "HEAD" || p.SHA != sha ||
		p.Path != worktreePath(root, "proj", "peek-HEAD") || p.Source != repo {
		t.Errorf("peek = %+v", p)
	}
}

func TestListJSONUncommittedChanges(t *testing.T) {
	repo := gittest.NewRepo(t) // README.md: "hello\n"
	wtRoot(t)
	clean := filepath.Join(t.TempDir(), "clean")
	gittest.Git(t, repo, "worktree", "add", "-b", "clean", clean)

	gittest.WriteFile(t, repo, "README.md", "changed\nmore\n") // tracked, unstaged: +2 −1
	gittest.WriteFile(t, repo, "staged.txt", "a\nb\n")         // staged new file: +2
	gittest.Git(t, repo, "add", "staged.txt")
	gittest.WriteFile(t, repo, "notes.txt", "no newline") // untracked: +1
	gittest.WriteFile(t, repo, "blob.bin", "a\x00b\n")    // untracked binary: a file, no lines

	out, _ := setupOutputs(t)
	if err := list(repo, []string{"--json"}); err != nil {
		t.Fatal(err)
	}
	var doc listJSON
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if w := doc.Worktrees[0]; w.State != "dirty" || w.Files != 4 || w.Insertions != 5 || w.Deletions != 1 {
		t.Errorf("dirty = %d files +%d −%d (%s), want 4 files +5 −1", w.Files, w.Insertions, w.Deletions, w.State)
	}
	if w := doc.Worktrees[1]; w.State != "clean" || w.Files != 0 || w.Insertions != 0 || w.Deletions != 0 {
		t.Errorf("clean = %+v", w)
	}
	if strings.Contains(out.String(), `"files": 0`) || strings.Contains(out.String(), `"insertions": 0`) {
		t.Errorf("a clean worktree carries no change counts:\n%s", out.String())
	}
	// counting must not touch the index or the working files
	if got := gittest.Git(t, repo, "status", "--porcelain"); !strings.Contains(got, "?? notes.txt") || !strings.Contains(got, "A  staged.txt") {
		t.Errorf("status after listing = %q", got)
	}
}

func TestListJSONDriftedBase(t *testing.T) {
	repo := linkedRepo(t)
	root := wtRoot(t)
	setupOutputs(t)
	gittest.Git(t, repo, "branch", "staging")
	if err := base(repo, []string{"add", "staging"}); err != nil {
		t.Fatal(err)
	}
	gittest.Git(t, worktreePath(root, "proj", "staging"), "checkout", "-b", "elsewhere")

	out, _ := setupOutputs(t)
	if err := list(repo, []string{"--json"}); err != nil {
		t.Fatal(err)
	}
	var doc listJSON
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if w := doc.Worktrees[1]; !w.Drifted || w.Pinned != "staging" || w.Branch != "elsewhere" {
		t.Errorf("drifted base = %+v", w)
	}
}

func TestListUsage(t *testing.T) {
	repo := gittest.NewRepo(t)
	setupOutputs(t)
	if err := list(repo, []string{"extra"}); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("err = %v, want usage", err)
	}
}

func TestWorktreeColor(t *testing.T) {
	if got := worktreeColor(true, false, false); got != ansiCyan {
		t.Errorf("main = %q, want cyan", got)
	}
	if got := worktreeColor(false, true, false); got != ansiGreen {
		t.Errorf("managed = %q, want green", got)
	}
	if got := worktreeColor(false, false, false); got != ansiMagenta {
		t.Errorf("external = %q, want magenta", got)
	}
	if got := worktreeColor(false, true, true); got != ansiOrange {
		t.Errorf("base = %q, want orange", got)
	}
}

func listRows(t *testing.T, table, mainName string) (mainRow, sideRow string) {
	t.Helper()
	for _, line := range strings.Split(table, "\n") {
		switch {
		case strings.Contains(line, mainName):
			mainRow = line
		case strings.Contains(line, "side"):
			sideRow = line
		}
	}
	if mainRow == "" || sideRow == "" {
		t.Fatalf("table missing expected rows:\n%s", table)
	}
	return mainRow, sideRow
}
