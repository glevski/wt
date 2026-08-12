// Command worktree manages git worktrees. See `worktree help`.
package main

import (
	"os"

	"wt/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
