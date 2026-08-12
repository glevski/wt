package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"wt/internal/git"
)

const baseUsage = "usage: wt base [add <branch> | list | rm [-f] <name> | update [name] | reset [--hard] [name]]"

// base manages base branches: permanent, view-only worktrees for long-lived
// branches (main/staging/dev). You jump in to look around, then fork real
// work off them — they are launch pads, not workbenches.
func base(dir string, args []string) error {
	if len(args) == 0 {
		return baseList(dir)
	}
	switch sub, rest := args[0], args[1:]; sub {
	case "add":
		return baseAdd(dir, rest)
	case "list":
		if len(rest) != 0 {
			return errors.New(baseUsage)
		}
		return baseList(dir)
	case "rm":
		return baseRemove(dir, rest)
	case "update":
		return baseUpdate(dir, rest)
	case "reset":
		return baseReset(dir, rest)
	default:
		return fmt.Errorf("unknown base command %q — %s", sub, baseUsage)
	}
}

func baseAdd(dir string, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: wt base add <branch>")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	if !ws.linked {
		return ws.requireLink()
	}
	branch := args[0]
	if wt := ws.repo.CheckedOut(branch); wt != nil {
		if _, isBase := git.ReadBaseMark(wt.Path); isBase {
			return hintf("wt ch "+filepath.Base(wt.Path), "'%s' is already a base", branch)
		}
		return hintf("move that checkout to another branch first, then retry",
			"branch '%s' is checked out at %s", branch, wt.Path)
	}
	refs, err := git.LookupBranch(ws.dir(), branch)
	if err != nil {
		return err
	}
	path, err := ws.targetPath(dirName(branch))
	if err != nil {
		return err
	}
	switch {
	case refs.Local:
		if _, err := git.Run(ws.dir(), "worktree", "add", path, branch); err != nil {
			return err
		}
	case len(refs.Remotes) > 0:
		remote, err := pickRemote(branch, refs.Remotes)
		if err != nil {
			return err
		}
		if _, err := git.Run(ws.dir(), "worktree", "add", "--track", "-b", branch, path, remote+"/"+branch); err != nil {
			return err
		}
		logf("branch '%s' created from %s/%s (tracking it)", branch, remote, branch)
	default:
		return hintf("fetch first: git fetch",
			"branch '%s' not found locally or on any remote", branch)
	}
	git.WriteBaseMark(path, branch)
	logf("added base '%s' at %s", branch, path)
	logf("jump with: wt ch %s — fork real work off it with: wt fork %s", dirName(branch), branch)
	return nil
}

func baseList(dir string) error {
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	isBase := func(wt git.Worktree) bool {
		_, ok := git.ReadBaseMark(wt.Path)
		return ok
	}
	for _, wt := range ws.repo.Worktrees {
		if isBase(wt) {
			return renderWorktrees(ws, isBase)
		}
	}
	logf("no base branches — add one: wt base add <branch>")
	return nil
}

func baseRemove(dir string, args []string) error {
	fs := flag.NewFlagSet("wt base rm", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	force := fs.Bool("f", false, "remove even when the worktree is dirty")
	if err := fs.Parse(args); err != nil || len(fs.Args()) != 1 {
		return errors.New("usage: wt base rm [-f] <name>")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	name := fs.Args()[0]
	target := findBase(ws, name)
	if target == nil {
		return hintf("wt base lists them", "no base named '%s'", name)
	}
	if cur := ws.repo.Current(); cur != nil && cur.Path == target.Path {
		return hintf("wt home first", "you are inside '%s'", name)
	}

	removeArgs := []string{"worktree", "remove"}
	if *force {
		removeArgs = append(removeArgs, "--force")
	}
	if _, err := git.Run(ws.dir(), append(removeArgs, target.Path)...); err != nil {
		if !*force {
			return hintf(fmt.Sprintf("wt base rm -f %s discards them", name), "%v", err)
		}
		return err
	}
	logf("removed base worktree '%s' (branch '%s' untouched)", name, target.Branch)
	return nil
}

// baseUpdate fast-forwards base worktrees to their upstreams: fetch each
// involved remote once, then ff-only merge per base — it can never lose or
// rewrite anything, so dirty, drifted or diverged bases are skipped loudly.
func baseUpdate(dir string, args []string) error {
	if len(args) > 1 {
		return errors.New("usage: wt base update [name]")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	type baseWorktree struct {
		wt     *git.Worktree
		branch string
	}
	var bases []baseWorktree
	for i := range ws.repo.Worktrees {
		wt := &ws.repo.Worktrees[i]
		branch, ok := git.ReadBaseMark(wt.Path)
		if !ok {
			continue
		}
		if len(args) == 1 && filepath.Base(wt.Path) != args[0] {
			continue
		}
		bases = append(bases, baseWorktree{wt, branch})
	}
	if len(bases) == 0 {
		if len(args) == 1 {
			return hintf("wt base lists them", "no base named '%s'", args[0])
		}
		logf("no base branches — add one: wt base add <branch>")
		return nil
	}

	remotes := map[string]bool{}
	for _, b := range bases {
		if up, ok := git.UpstreamOf(ws.dir(), b.branch); ok {
			if remote, _, found := strings.Cut(up.Name, "/"); found {
				remotes[remote] = true
			}
		}
	}
	for remote := range remotes {
		logf("fetching %s…", remote)
		if _, err := git.Run(ws.dir(), "fetch", remote); err != nil {
			return err
		}
	}

	for _, b := range bases {
		name := filepath.Base(b.wt.Path)
		if b.wt.Branch != b.branch {
			logf("%s: skipped (drifted to branch '%s')", name, b.wt.Branch)
			continue
		}
		if _, ok := git.UpstreamOf(ws.dir(), b.branch); !ok {
			logf("%s: skipped (no upstream)", name)
			continue
		}
		status, err := git.Run(b.wt.Path, "status", "--porcelain")
		if err != nil {
			return err
		}
		if status != "" {
			logf("%s: skipped (dirty)", name)
			continue
		}
		if _, err := git.Run(b.wt.Path, "merge", "--ff-only", b.branch+"@{upstream}"); err != nil {
			logf("%s: cannot fast-forward (diverged from upstream) — left untouched", name)
			continue
		}
		after, err := git.Run(b.wt.Path, "rev-parse", "HEAD")
		if err != nil {
			return err
		}
		if after == b.wt.Head {
			logf("%s: already up to date (%s)", name, shortSHA(after))
		} else {
			logf("%s: updated to %s (was %s)", name, shortSHA(after), shortSHA(b.wt.Head))
		}
	}
	return nil
}

// baseReset snaps a base branch back to its upstream's tip — the hard
// sibling of update: it moves the branch wherever the upstream is, backward
// included. Same safety contract as wt reset: local changes block it, --hard
// proceeds keeping only untracked files.
func baseReset(dir string, args []string) error {
	fs := flag.NewFlagSet("wt base reset", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	hard := fs.Bool("hard", false, "discard tracked changes")
	if err := fs.Parse(args); err != nil || fs.NArg() > 1 {
		return errors.New("usage: wt base reset [--hard] [name]")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}

	var target *git.Worktree
	var branch string
	if fs.NArg() == 1 {
		target = findBase(ws, fs.Arg(0))
		if target == nil {
			return hintf("wt base lists them", "no base named '%s'", fs.Arg(0))
		}
		branch, _ = git.ReadBaseMark(target.Path)
	} else {
		current, err := ws.currentWorktree()
		if err != nil {
			return err
		}
		marked, ok := git.ReadBaseMark(current.Path)
		if !ok {
			return hintf("wt base reset <name> targets one by name", "not inside a base worktree")
		}
		target, branch = current, marked
	}
	name := filepath.Base(target.Path)

	if target.Branch != branch {
		return hintf(fmt.Sprintf("git -C %s checkout %s brings it back", target.Path, branch),
			"'%s' has drifted to branch '%s'", name, target.Branch)
	}
	up, ok := git.UpstreamOf(ws.dir(), branch)
	if !ok {
		return fmt.Errorf("branch '%s' has no upstream to reset to", branch)
	}
	tip, err := git.Run(ws.dir(), "rev-parse", "--verify", "--quiet", up.Name)
	if err != nil || tip == "" {
		return fmt.Errorf("upstream '%s' does not resolve to a commit", up.Name)
	}

	if !*hard {
		status, err := git.Run(target.Path, "status", "--porcelain")
		if err != nil {
			return err
		}
		if status != "" {
			return hintf("wt base reset --hard discards tracked changes (untracked files are kept)",
				"base '%s' has local changes", name)
		}
	}

	if _, err := git.Run(target.Path, "reset", "--hard", tip); err != nil {
		return err
	}
	logf("reset base '%s' to %s (%s, was %s)", name, up.Name, shortSHA(tip), shortSHA(target.Head))
	return nil
}

func findBase(ws *workspace, name string) *git.Worktree {
	for i := range ws.repo.Worktrees {
		wt := &ws.repo.Worktrees[i]
		if _, ok := git.ReadBaseMark(wt.Path); !ok {
			continue
		}
		if filepath.Base(wt.Path) == name {
			return wt
		}
	}
	return nil
}
