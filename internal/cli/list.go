package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"text/tabwriter"
	"time"

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
	fmt.Fprintln(tw, "  NAME\tBRANCH\tSTATE\tCOMMIT\tCREATED\tCHECKOUT")
	for i, wt := range worktrees {
		marker := " "
		if current != nil && wt.Path == current.Path {
			marker = "*"
		}
		branch := wt.Branch
		if wt.Detached {
			branch = "(detached)"
		}
		commit := shortSHA(wt.Head)
		if commit == "" {
			commit = "-"
		}
		fmt.Fprintf(tw, "%s %s\t%s\t%s\t%s\t%s\t%s\n",
			marker, filepath.Base(wt.Path), branch, states[i], commit,
			when(git.CreatedAt(wt.Path)), when(git.CheckoutStamp(wt.Path)))
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

func when(t time.Time, ok bool) string {
	if !ok {
		return "-"
	}
	return ago(t)
}

// ago renders a timestamp as a compact age: "now", "5m", "3h", "2d", "4mo".
func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
}
