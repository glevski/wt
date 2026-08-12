package git

import (
	"errors"
	"path/filepath"
	"strings"
)

// Repo is the worktree context loaded once per command invocation.
type Repo struct {
	Dir       string     // directory the command was invoked from
	Worktrees []Worktree // [0] is always the main worktree (or the bare repo)
	Name      string     // repo name: basename of the main worktree path
}

// Load reads the repo's worktree list starting from dir.
func Load(dir string) (*Repo, error) {
	out, err := Run(dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		if strings.Contains(err.Error(), "not a git repository") {
			return nil, errors.New("not inside a git repository")
		}
		return nil, err
	}
	worktrees := parseWorktrees(out)
	if len(worktrees) == 0 {
		return nil, errors.New("not inside a git repository")
	}
	name := strings.TrimSuffix(filepath.Base(worktrees[0].Path), ".git")
	return &Repo{Dir: dir, Worktrees: worktrees, Name: name}, nil
}

// Current returns the worktree containing the invocation directory, or nil
// when it is in none of them (e.g. inside a bare repo's .git directory).
func (r *Repo) Current() *Worktree {
	dir, err := filepath.Abs(r.Dir)
	if err != nil {
		return nil
	}
	dir = canonical(dir)
	for {
		for i := range r.Worktrees {
			if canonical(r.Worktrees[i].Path) == dir {
				return &r.Worktrees[i]
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

// CheckedOut returns the worktree that has branch checked out, if any.
func (r *Repo) CheckedOut(branch string) *Worktree {
	if branch == "" {
		return nil
	}
	for i := range r.Worktrees {
		if r.Worktrees[i].Branch == branch {
			return &r.Worktrees[i]
		}
	}
	return nil
}

func canonical(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}
