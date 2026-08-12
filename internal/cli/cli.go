// Package cli implements the worktree subcommands. Human-facing narration
// goes to stderr, machine-consumable output (paths, tables, shell code) to
// stdout — that is what lets the wt() shell wrapper capture `wt ch` safely.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
)

const usage = `worktree — fast git worktree manager (alias it to wt via "init")

Usage:
  wt create [flags] [branch]    add a worktree for an existing local or remote
                        branch; with no argument, a clean worktree off the
                        current HEAD on a new auto-named branch
  wt fork [flags] [new-branch]  add a worktree off the current HEAD carrying
                        over all staged, unstaged and untracked changes; an
                        existing branch argument bases the fork off its tip
                        with an auto-named new branch
                        Flags on both: -c/--checkout cd into the new worktree;
                        -n/--name set its directory name, where a leading dash
                        appends to the branch (-n -fix → <branch>-fix)
  wt checkout <name>    jump to a worktree by name, unique prefixes work
                        (alias: ch; needs the wt() shell function);
                        -b <new-branch> forks your current state into a new
                        worktree and jumps there (like git checkout -b)
  wt home               jump back to the main checkout (root repo)
  wt switch             toggle between the current and last-used location,
                        like cd - (state is per shell)
  wt list               list this repo's worktrees (alias: ls)
  wt status [name]      show a worktree (default: the current one) — project
                        link, worktree and path; -g/--git appends git status
                        output as if run there
  wt remove [-f] [-b] <name>  remove a worktree; -f discards local changes,
                        -b also deletes its branch when merged (alias: rm)
  wt reset [--hard] [base]  move the worktree's branch back to its base
                        branch's tip (recorded at creation), staying on the
                        branch; refuses with local changes unless --hard
  wt link [name]        link this repo to a project name (stored in git config
                        wt.name); without an argument, show the current link
  wt init <zsh|bash>    print the wt() shell function; add to your rc file:
                        eval "$(worktree init zsh)"

create and fork need a linked repo: worktrees are created under
~/worktrees/<linked-name>/, overridable with $WT_ROOT or "git config wt.root".
`

// stdout/stderr are swapped out by tests.
var (
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
)

// Run dispatches a command line and returns the process exit code.
func Run(args []string) int {
	if v := os.Getenv("WT_WRAPPER_VERSION"); v != "" && v != wrapperVersion {
		logf("your wt() shell function is outdated — restart the shell or re-run: eval \"$(worktree init zsh)\"")
	}
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	cmd, rest := args[0], args[1:]
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if err := dispatch(cmd, rest); err != nil {
		fmt.Fprintf(stderr, "wt: %v\n", err)
		var withHint *hintError
		if errors.As(err, &withHint) {
			fmt.Fprintf(stderr, "hint: %s\n", withHint.hint)
		}
		return 1
	}
	return 0
}

func dispatch(cmd string, args []string) error {
	if cmd == "init" {
		return shellInit(args)
	}
	if cmd == "switch" {
		return switchTo(args)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	switch cmd {
	case "create":
		return create(cwd, args)
	case "fork":
		return fork(cwd, args)
	case "checkout", "ch":
		return checkout(cwd, args)
	case "home":
		return home(cwd, args)
	case "list", "ls":
		return list(cwd, args)
	case "status":
		return status(cwd, args)
	case "remove", "rm":
		return remove(cwd, args)
	case "reset":
		return reset(cwd, args)
	case "link":
		return link(cwd, args)
	default:
		return fmt.Errorf("unknown command %q, see: wt help", cmd)
	}
}

// logf prints one line of human-facing narration.
func logf(format string, args ...any) {
	fmt.Fprintf(stderr, "wt: "+format+"\n", args...)
}

// hintError renders as the usual "wt: <msg>" line followed by "hint: <hint>".
type hintError struct {
	msg  string
	hint string
}

func (e *hintError) Error() string { return e.msg }

func hintf(hint, format string, args ...any) error {
	return &hintError{msg: fmt.Sprintf(format, args...), hint: hint}
}
