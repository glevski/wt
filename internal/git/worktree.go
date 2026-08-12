package git

import "strings"

// Worktree is one entry of `git worktree list`.
type Worktree struct {
	Path     string
	Head     string
	Branch   string // short branch name; empty when detached or bare
	Bare     bool
	Detached bool
}

// parseWorktrees reads `git worktree list --porcelain -z` output: NUL-terminated
// "key[ value]" fields, with an empty field closing each entry.
func parseWorktrees(raw string) []Worktree {
	var (
		all  []Worktree
		cur  Worktree
		open bool
	)
	flush := func() {
		if open {
			all = append(all, cur)
			cur, open = Worktree{}, false
		}
	}
	for _, field := range strings.Split(raw, "\x00") {
		if field == "" {
			flush()
			continue
		}
		open = true
		key, value, _ := strings.Cut(field, " ")
		switch key {
		case "worktree":
			cur.Path = value
		case "HEAD":
			cur.Head = value
		case "branch":
			cur.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "bare":
			cur.Bare = true
		case "detached":
			cur.Detached = true
		}
	}
	flush()
	return all
}
