package cli

import (
	"errors"
	"fmt"

	"wt/internal/git"
)

const rootUsage = "usage: wt root <status|checkout|create|fork> [args]"

// root runs a wt command in the root repo's context instead of the cwd's:
// every command takes its invocation directory explicitly, so pointing that
// at the main checkout is all "root mode" is.
func root(dir string, args []string) error {
	if len(args) == 0 {
		return errors.New(rootUsage)
	}
	repo, err := git.Load(dir)
	if err != nil {
		return err
	}
	rootPath := repo.Worktrees[0].Path

	switch sub, rest := args[0], args[1:]; sub {
	case "status":
		return status(rootPath, rest)
	case "checkout", "ch":
		if len(rest) == 0 {
			return home(dir, nil)
		}
		return checkout(rootPath, rest)
	case "create":
		return create(rootPath, rest)
	case "fork":
		return fork(rootPath, rest)
	default:
		return fmt.Errorf("unknown root command %q — %s", sub, rootUsage)
	}
}
