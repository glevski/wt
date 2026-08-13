package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"wt/internal/config"
	"wt/internal/git"
)

const globalUsage = `usage: wt global <command>
  list | ls [project]      all projects, or one project's worktrees
  status [-g] <project>[/<worktree>]
  git-log <project>[/<worktree>] [git log args...]
  checkout | ch <project>[/<worktree>]`

// global works with registered projects from anywhere — no repo needed. The
// registry behind it is global git config (wt.project.<name>), maintained
// explicitly with wt link -r; stale entries are pruned on sight.
func global(dir string, args []string) error {
	if len(args) == 0 {
		return errors.New(globalUsage)
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list", "ls":
		switch len(rest) {
		case 0:
			return globalList(dir)
		case 1:
			root, wtName, err := resolveProject(dir, rest[0])
			if err != nil {
				return err
			}
			if wtName != "" {
				return errors.New("usage: wt global list [project]")
			}
			ws, err := loadWorkspace(root)
			if err != nil {
				return err
			}
			return renderWorktrees(ws, nil)
		default:
			return errors.New(globalUsage)
		}
	case "status":
		spec, others := splitSpec(rest)
		if spec == "" {
			return errors.New(globalUsage)
		}
		root, wtName, err := resolveProject(dir, spec)
		if err != nil {
			return err
		}
		if wtName != "" {
			others = append(others, wtName)
		}
		return status(root, others)
	case "git-log":
		spec, others := splitSpec(rest)
		if spec == "" {
			return errors.New(globalUsage)
		}
		root, wtName, err := resolveProject(dir, spec)
		if err != nil {
			return err
		}
		if wtName != "" {
			others = append([]string{wtName}, others...)
		}
		return gitLog(root, others)
	case "checkout", "ch":
		if len(rest) != 1 {
			return errors.New(globalUsage)
		}
		root, wtName, err := resolveProject(dir, rest[0])
		if err != nil {
			return err
		}
		if wtName != "" {
			return checkout(root, []string{wtName})
		}
		return home(root, nil)
	default:
		return errors.New(globalUsage)
	}
}

// globalList prints the projects table: name, worktree count (main checkout
// excluded), root repo path.
func globalList(dir string) error {
	projects := knownProjects(dir)
	sort.Slice(projects, func(i, j int) bool { return projects[i][0] < projects[j][0] })
	if len(projects) == 0 {
		logf("no registered projects — wt link -r registers one")
		return nil
	}
	counts := make([]string, len(projects))
	var wg sync.WaitGroup
	for i, p := range projects {
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			if repo, err := git.Load(path); err == nil {
				counts[i] = fmt.Sprintf("%d", len(repo.Worktrees)-1)
			} else {
				counts[i] = "?"
			}
		}(i, p[1])
	}
	wg.Wait()

	header := []string{"NAME", "WORKTREES", "PATH"}
	widths := make([]int, len(header))
	for c, h := range header {
		widths[c] = len(h)
	}
	rows := make([][]string, len(projects))
	for i, p := range projects {
		rows[i] = []string{p[0], counts[i], p[1]}
		for c, cell := range rows[i] {
			if n := utf8.RuneCountInString(cell); n > widths[c] {
				widths[c] = n
			}
		}
	}
	paint := colorEnabled()
	fmt.Fprintf(stdout, "%s  %s  %s\n", pad(header[0], widths[0]), pad(header[1], widths[1]), header[2])
	for _, row := range rows {
		name := pad(row[0], widths[0])
		if paint {
			name = ansiCyan + name + ansiReset
		}
		fmt.Fprintf(stdout, "%s  %s  %s\n", name, pad(row[1], widths[1]), row[2])
	}
	return nil
}

// resolveProject matches a <project>[/<worktree>] spec against the registry,
// unique name prefixes included, and hands back the project's root repo.
func resolveProject(dir, spec string) (root, worktree string, err error) {
	name, wtName, _ := strings.Cut(spec, "/")
	var prefixed [][2]string
	for _, p := range knownProjects(dir) {
		if p[0] == name {
			return p[1], wtName, nil
		}
		if strings.HasPrefix(p[0], name) {
			prefixed = append(prefixed, p)
		}
	}
	switch len(prefixed) {
	case 1:
		return prefixed[0][1], wtName, nil
	case 0:
		return "", "", hintf("wt global ls shows them; wt link -r registers one",
			"no registered project named '%s'", name)
	default:
		names := make([]string, len(prefixed))
		for i, p := range prefixed {
			names[i] = p[0]
		}
		return "", "", fmt.Errorf("'%s' is ambiguous: %s", name, strings.Join(names, ", "))
	}
}

// knownProjects is the registry with stale entries pruned: the recorded path
// must exist and still be linked under the same name.
func knownProjects(dir string) [][2]string {
	var valid [][2]string
	for _, p := range config.Projects(dir) {
		if projectValid(p[0], p[1]) {
			valid = append(valid, p)
			continue
		}
		config.UnregisterProject(dir, p[0])
		logf("pruned '%s' from the project registry (%s is gone or renamed)", p[0], p[1])
	}
	return valid
}

func projectValid(name, path string) bool {
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return false
	}
	got, ok := config.Name(path)
	return ok && got == name
}

// splitSpec pulls the <project>[/<worktree>] spec — the first non-flag
// argument — out of args, keeping the remaining arguments in order.
func splitSpec(args []string) (string, []string) {
	for i, a := range args {
		if !strings.HasPrefix(a, "-") {
			rest := append(append([]string{}, args[:i]...), args[i+1:]...)
			return a, rest
		}
	}
	return "", args
}
