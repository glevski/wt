package cli

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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
