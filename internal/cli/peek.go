package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"wt/internal/git"
)

const peekUsage = "usage: wt peek [<rev> | off [name]]"
const peekMarker = ".wt-peek"

// peekInfo is the .wt-peek marker: what the snapshot is and where it came
// from. It lives inside the peek directory — there is no git admin dir.
type peekInfo struct {
	Rev     string    `json:"rev"`
	SHA     string    `json:"sha"`
	Source  string    `json:"source"`
	Created time.Time `json:"created"`
}

// peek manages disposable read-only snapshots: `wt peek <rev>` exports the
// revision's files into a fake worktree (no checkout, no branch, no git
// registration) and jumps there; bare `wt peek` lists peeks; `wt peek off`
// jumps back and deletes. The user's real worktrees are never touched.
func peek(dir string, args []string) error {
	fs := flag.NewFlagSet("wt peek", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	env := envFlags(fs)
	if err := fs.Parse(args); err != nil || fs.NArg() > 2 {
		return errors.New(peekUsage)
	}
	rest := fs.Args()
	switch {
	case len(rest) == 0:
		return peekList(dir)
	case rest[0] == "off":
		return peekOff(dir, rest[1:])
	case len(rest) == 1:
		return peekAt(dir, rest[0], env())
	default:
		return errors.New(peekUsage)
	}
}

func peekAt(dir, rev string, env envOptions) error {
	if _, ok := findPeekRoot(dir); ok {
		return hintf("wt peek off first", "already inside a peek")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	if !ws.linked {
		return ws.requireLink()
	}
	sha, err := git.Run(ws.dir(), "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err != nil || sha == "" {
		return fmt.Errorf("'%s' does not resolve to a commit", rev)
	}

	dest := filepath.Join(ws.root, "peek-"+dirName(rev))
	if info, ok := readPeekInfo(dest); ok {
		logf("already peeking '%s' (%s) — wt peek off first for a fresh export", info.Rev, shortSHA(info.SHA))
		emitJump(dest, ws.repo.Worktrees[0].Path)
		return nil
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s exists but is not a peek directory", dest)
	}

	source := ws.repo.Worktrees[0].Path
	if cur := ws.repo.Current(); cur != nil {
		source = cur.Path
	}
	partial := dest + ".wt-partial"
	_ = os.RemoveAll(partial)
	if err := exportTree(ws.dir(), sha, partial); err != nil {
		_ = os.RemoveAll(partial)
		return err
	}
	if err := writePeekInfo(partial, peekInfo{Rev: rev, SHA: sha, Source: source, Created: time.Now()}); err != nil {
		return err
	}
	if err := os.Rename(partial, dest); err != nil {
		return err
	}
	copyEnvironment(ws, dest, env)
	logf("peeking '%s' (%s) — read-only snapshot, edits here are throwaway", rev, shortSHA(sha))
	logf("wt peek off returns and deletes it")
	emitJump(dest, ws.repo.Worktrees[0].Path)
	return nil
}

// peekList prints peeks on stderr — bare `wt peek` runs under the wrapper's
// stdout capture, which must stay script-or-empty.
func peekList(dir string) error {
	if root, ok := findPeekRoot(dir); ok {
		info, _ := readPeekInfo(root)
		logf("inside peek '%s': '%s' (%s), from %s, age %s",
			filepath.Base(root), info.Rev, shortSHA(info.SHA), info.Source, ago(info.Created))
		return nil
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}
	peeks := findPeeks(ws.root)
	if len(peeks) == 0 {
		logf("no peeks — try: wt peek <rev>")
		return nil
	}
	for _, p := range peeks {
		if info, ok := readPeekInfo(p); ok {
			logf("%s — '%s' (%s), age %s", filepath.Base(p), info.Rev, shortSHA(info.SHA), ago(info.Created))
		}
	}
	return nil
}

func peekOff(dir string, args []string) error {
	if len(args) > 1 {
		return errors.New(peekUsage)
	}
	if len(args) == 1 {
		// named form: delete without jumping anywhere
		ws, err := loadWorkspace(dir)
		if err != nil {
			return err
		}
		target := filepath.Join(ws.root, args[0])
		if _, ok := readPeekInfo(target); !ok {
			return hintf("wt peek lists them", "no peek named '%s'", args[0])
		}
		stopDepsWorker(target)
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		logf("removed peek '%s'", args[0])
		return nil
	}

	root, ok := findPeekRoot(dir)
	if !ok {
		return hintf("wt peek <rev> starts one", "not inside a peek directory")
	}
	info, _ := readPeekInfo(root)
	stopDepsWorker(root)
	if err := os.RemoveAll(root); err != nil {
		return err
	}
	logf("peek '%s' removed", filepath.Base(root))

	dest := info.Source
	if _, err := os.Stat(dest); dest == "" || err != nil {
		dest = filepath.Dir(root) // source gone: land in the project dir
	}
	home := ""
	if repo, err := git.Load(dest); err == nil {
		home = repo.Worktrees[0].Path
	}
	emitJump(dest, home)
	return nil
}

// exportTree writes rev's files into dest via `git archive | tar -x` — a
// pure file export: no checkout, no branch, nothing registered with git.
func exportTree(repoDir, sha, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	archive := exec.Command("git", "-C", repoDir, "archive", "--format=tar", sha)
	untar := exec.Command("tar", "-x", "-C", dest)
	pipe, err := archive.StdoutPipe()
	if err != nil {
		return err
	}
	untar.Stdin = pipe
	var archiveErr, untarErr strings.Builder
	archive.Stderr = &archiveErr
	untar.Stderr = &untarErr
	if err := untar.Start(); err != nil {
		return err
	}
	if err := archive.Run(); err != nil {
		_ = untar.Wait()
		return fmt.Errorf("git archive: %s", strings.TrimSpace(archiveErr.String()))
	}
	if err := untar.Wait(); err != nil {
		return fmt.Errorf("tar: %s", strings.TrimSpace(untarErr.String()))
	}
	return nil
}

func writePeekInfo(dir string, info peekInfo) error {
	raw, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, peekMarker), append(raw, '\n'), 0o644)
}

func readPeekInfo(dir string) (peekInfo, bool) {
	raw, err := os.ReadFile(filepath.Join(dir, peekMarker))
	if err != nil {
		return peekInfo{}, false
	}
	var info peekInfo
	if json.Unmarshal(raw, &info) != nil {
		return peekInfo{}, false
	}
	return info, true
}

// findPeekRoot walks up from dir to the enclosing peek directory, if any.
func findPeekRoot(dir string) (string, bool) {
	d, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		if _, ok := readPeekInfo(d); ok {
			return d, true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", false
		}
		d = parent
	}
}

// findPeeks lists peek directories directly under the project root.
func findPeeks(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var peeks []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(root, e.Name())
		if _, ok := readPeekInfo(p); ok {
			peeks = append(peeks, p)
		}
	}
	return peeks
}
