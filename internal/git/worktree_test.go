package git

import "testing"

func TestParseWorktrees(t *testing.T) {
	raw := "worktree /repo\x00HEAD abc123\x00branch refs/heads/main\x00\x00" +
		"worktree /wt/det\x00HEAD def456\x00detached\x00\x00" +
		"worktree /srv/bare.git\x00bare\x00\x00" +
		"worktree /wt/locked\x00HEAD 789\x00branch refs/heads/fix/x\x00locked busy\x00\x00"

	got := parseWorktrees(raw)
	if len(got) != 4 {
		t.Fatalf("parsed %d worktrees, want 4: %+v", len(got), got)
	}

	main := got[0]
	if main.Path != "/repo" || main.Head != "abc123" || main.Branch != "main" || main.Bare || main.Detached {
		t.Errorf("main entry parsed wrong: %+v", main)
	}
	if det := got[1]; !det.Detached || det.Branch != "" {
		t.Errorf("detached entry parsed wrong: %+v", det)
	}
	if bare := got[2]; !bare.Bare || bare.Path != "/srv/bare.git" {
		t.Errorf("bare entry parsed wrong: %+v", bare)
	}
	if locked := got[3]; locked.Branch != "fix/x" {
		t.Errorf("locked entry parsed wrong: %+v", locked)
	}
}

func TestParseWorktreesEmpty(t *testing.T) {
	if got := parseWorktrees(""); len(got) != 0 {
		t.Fatalf("parsed %d worktrees from empty input", len(got))
	}
}
