package cli

import (
	"path/filepath"
	"strings"
	"testing"

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

	var mainRow, sideRow string
	for _, line := range strings.Split(out.String(), "\n") {
		switch {
		case strings.Contains(line, filepath.Base(repo)):
			mainRow = line
		case strings.Contains(line, "side"):
			sideRow = line
		}
	}
	if !strings.HasPrefix(mainRow, "*") || !strings.Contains(mainRow, "clean") {
		t.Errorf("main row = %q, want current marker and clean state", mainRow)
	}
	if strings.HasPrefix(sideRow, "*") || !strings.Contains(sideRow, "dirty") {
		t.Errorf("side row = %q, want no marker and dirty state", sideRow)
	}
}
