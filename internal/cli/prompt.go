package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"wt/internal/git"
)

// prompt prints the "<project><subpath> (<branch>)" segment for embedding in
// a shell prompt, using zsh prompt escapes. The branch is colored like
// wt list: cyan in the main checkout, green in wt-managed worktrees, orange
// in bases (red when drifted), magenta in external worktrees, red in peeks.
// The project name replaces the checkout's directory name, so every worktree
// of a repo shows the same identity; outside any repo it falls back to the
// plain ~-shortened path.
func prompt(dir string, args []string) error {
	if len(args) != 1 || args[0] != "zsh" {
		return errors.New("usage: wt prompt zsh")
	}
	fmt.Fprintln(stdout, promptSegment(dir))
	return nil
}

func promptSegment(dir string) string {
	if root, ok := findPeekRoot(dir); ok {
		info, _ := readPeekInfo(root)
		project := filepath.Base(filepath.Dir(root)) // peeks live in <root>/<project>
		return zshPath(project+subPath(root, dir)) + zshBranch("red", "peek:"+info.Rev)
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return zshPlainPath
	}
	wt := ws.repo.Current()
	if wt == nil || wt.Bare {
		return zshPlainPath
	}
	seg := zshPath(ws.name + subPath(wt.Path, dir))
	branch := wt.Branch
	if wt.Detached {
		branch = shortSHA(wt.Head)
	}
	if branch == "" {
		return seg
	}
	baseBranch, isBase := git.ReadBaseMark(wt.Path)
	color := promptColor(
		wt.Path == ws.repo.Worktrees[0].Path,
		strings.HasPrefix(wt.Path, ws.root+"/"),
		isBase,
		isBase && wt.Branch != baseBranch,
	)
	return seg + zshBranch(color, branch)
}

// zshPlainPath lets zsh render the ~-shortened cwd itself: with PROMPT_SUBST
// the substituted text goes through prompt expansion, so %~ works.
const zshPlainPath = "%B%F{cyan}%~%f%b"

// promptColor mirrors worktreeColor in zsh color names; drift wins, as in
// wt status.
func promptColor(main, managed, base, drifted bool) string {
	switch {
	case drifted:
		return "red"
	case main:
		return "cyan"
	case base:
		return "208" // orange, same as ansiOrange
	case managed:
		return "green"
	default:
		return "magenta"
	}
}

func zshPath(s string) string {
	return "%B%F{cyan}" + zshEscape(s) + "%f%b"
}

func zshBranch(color, name string) string {
	return " (%B%F{" + color + "}" + zshEscape(name) + "%f%b)"
}

// zshEscape doubles literal % so names cannot inject prompt escapes.
func zshEscape(s string) string {
	return strings.ReplaceAll(s, "%", "%%")
}

// subPath is dir's path below root ("" at root itself), tolerant of
// symlinked prefixes like a symlinked temp dir.
func subPath(root, dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	pairs := [][2]string{{root, abs}}
	if r, errRoot := filepath.EvalSymlinks(root); errRoot == nil {
		if d, errDir := filepath.EvalSymlinks(abs); errDir == nil {
			pairs = append(pairs, [2]string{r, d})
		}
	}
	for _, p := range pairs {
		rel, err := filepath.Rel(p[0], p[1])
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		if rel == "." {
			return ""
		}
		return "/" + rel
	}
	return ""
}
