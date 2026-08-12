package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"text/tabwriter"

	"wt/internal/git"
)

func list(dir string, args []string) error {
	if len(args) != 0 {
		return errors.New("usage: wt list")
	}
	ws, err := loadWorkspace(dir)
	if err != nil {
		return err
	}

	worktrees := ws.repo.Worktrees
	states := make([]string, len(worktrees))
	var wg sync.WaitGroup
	for i, wt := range worktrees {
		if wt.Bare {
			states[i] = "bare"
			continue
		}
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			states[i] = worktreeState(path)
		}(i, wt.Path)
	}
	wg.Wait()

	current := ws.repo.Current()
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  NAME\tBRANCH\tSTATE\tPATH")
	for i, wt := range worktrees {
		marker := " "
		if current != nil && wt.Path == current.Path {
			marker = "*"
		}
		branch := wt.Branch
		if wt.Detached {
			branch = "(detached " + shortSHA(wt.Head) + ")"
		}
		fmt.Fprintf(tw, "%s %s\t%s\t%s\t%s\n", marker, filepath.Base(wt.Path), branch, states[i], wt.Path)
	}
	return tw.Flush()
}

func worktreeState(path string) string {
	out, err := git.Run(path, "status", "--porcelain")
	switch {
	case err != nil:
		return "error"
	case out == "":
		return "clean"
	default:
		return "dirty"
	}
}
