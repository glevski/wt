// Package cli implements the worktree subcommands. Human-facing narration
// goes to stderr, machine-consumable output (paths, tables, shell code) to
// stdout — that is what lets the wt() shell wrapper capture `wt ch` safely.
package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"wt/internal/config"
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
                        appends to the branch (-n -fix → <branch>-fix);
                        -w/--wait copy deps synchronously; --no-deps skip
                        deps; --no-ignored copy no ignored files at all
  wt checkout [name]    jump to a worktree by name, unique prefixes work
                        (alias: ch; needs the wt() shell function); with no
                        name, pick from the most recent ones with arrow keys;
                        -b <new-branch> forks your current state into a new
                        worktree and jumps there (like git checkout -b)
  wt home               jump back to the main checkout (root repo)
  wt peek [rev]         jump into a disposable read-only snapshot of a
                        revision — a fake worktree with no branch or checkout;
                        env/deps flags apply; bare = list peeks;
                        wt peek off returns and deletes it (alias: wt unpeek)
  wt switch             toggle between the current and last-used location,
                        like cd - (state is per shell)
  wt root <cmd> [args]  run status, checkout, create or fork in the root
                        repo's context, e.g. wt root fork -c forks the root's
                        current state from wherever you stand
  wt global <cmd>       work with registered projects (wt link -r) from
                        anywhere, no repo needed: list [project] (all
                        projects, or one project's worktrees),
                        status [-g] <project>[/<wt>],
                        git-log <project>[/<wt>] [args...],
                        checkout|ch <project>[/<wt>] (jump there)
  wt base [cmd]         manage base branches — permanent view-only worktrees
                        for long-lived branches you fork real work off:
                        add <branch> (seeds it with the git-ignored files of
                        the current worktree; --no-ignored skips that,
                        --deps [-w] also copies declared deps),
                        list (default), rm [-f] <name>,
                        update [name] (fast-forward to upstream),
                        reset [--hard] [name] (hard-sync to upstream)
  wt list               list this repo's worktrees (alias: ls)
  wt status [name]      show a worktree (default: the current one) — project
                        link, worktree and path; -g/--git appends git status
                        output as if run there
  wt git-log [name]     run git log in a worktree (default: the current one);
                        extra arguments pass through to git log
  wt copy [-f] [--from <worktree>] <file> [dst]  copy a file into the current
                        worktree from the root repo (default) or the --from
                        worktree — for files git doesn't carry over (.env, …);
                        paths are worktree-relative, -f overwrites
  wt remove [-f] [-b] <name>  remove a worktree; -f discards local changes,
                        -b also deletes its branch when merged (alias: rm).
                        A quoted glob ('dev-*', '*') bulk-removes matching
                        wt-managed worktrees after listing and confirmation
  wt finish [-d] [-b] [-f]  wrap up the current worktree: jump back to the
                        last location this shell jumped from (home when there
                        is none); -d/--delete also removes the worktree,
                        -b/--branch also deletes its branch (needs -d),
                        -f/--force discards local changes
  wt reset [--hard] [base]  move the worktree's branch back to its base
                        branch's tip (recorded at creation), staying on the
                        branch; refuses with local changes unless --hard
  wt link [-r] [name]   link this repo to a project name (stored in git config
                        wt.name); -r/--register also adds it to the global
                        registry that wt global works from (bare wt link -r
                        registers an already linked repo); with no arguments,
                        show the current link
  wt deps [cmd]         manage dependency paths (node_modules, …) brought into
                        new worktrees in the background — hardlinked to the
                        source by default (near-zero space; cross-filesystem
                        falls back to copying; --copy-deps on create/fork or
                        git config wt.depscopy force copies):
                        add <path>, rm <path>, list (default),
                        sync [--copy] [name] (re-sync, foreground),
                        eject [--no-copy] [name] (own private copies instead
                        of sharing hardlinks; --no-copy just deletes them),
                        link [source] (re-link deps as hardlinks — default
                        source: the worktree holding your base branch),
                        purge (remove deps from all regular worktrees after
                        a y/N confirmation, to reclaim space)
  wt alias [cmd]        your own command aliases, git-style (stored as git
                        config wt.alias.*): add <name> <command...>, rm <name>,
                        list (default) — e.g. wt alias add cr create -c
  wt init <zsh|bash>    print the wt() shell function and tab completion;
                        add to your rc file: eval "$(worktree init zsh)"
  wt --version          print the wt version (release tag) and commit
  wt prompt zsh         print the "<project> (<branch>)" segment for your
                        shell prompt, colored like wt list — see the README

create and fork need a linked repo: worktrees are created under
~/worktrees/<linked-name>/, overridable with $WT_ROOT or "git config wt.root".
`

// stdin/stdout/stderr are swapped out by tests.
var (
	stdin  io.Reader = os.Stdin
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
)

// confirm asks a y/N question; only a plain "y" answer proceeds.
func confirm(question string) bool {
	fmt.Fprintf(stderr, "%s [y/N] ", question)
	line, _ := bufio.NewReader(stdin).ReadString('\n')
	return strings.TrimSpace(line) == "y"
}

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
	if cmd == "--version" || cmd == "version" {
		fmt.Fprintln(stdout, versionString())
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
	if cmd == "__deps-worker" {
		return depsWorker(args)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if handled, err := runBuiltin(cwd, cmd, args); handled {
		return err
	}
	if words := config.Alias(cwd, cmd); len(words) > 0 {
		if !builtinNames[words[0]] {
			return fmt.Errorf("alias '%s' expands to unknown command %q", cmd, words[0])
		}
		return dispatch(words[0], append(words[1:], args...))
	}
	return fmt.Errorf("unknown command %q, see: wt help", cmd)
}

func runBuiltin(cwd, cmd string, args []string) (bool, error) {
	run := func(err error) (bool, error) { return true, err }
	switch cmd {
	case "create":
		return run(create(cwd, args))
	case "fork":
		return run(fork(cwd, args))
	case "checkout", "ch":
		return run(checkout(cwd, args))
	case "home":
		return run(home(cwd, args))
	case "root":
		return run(root(cwd, args))
	case "global":
		return run(global(cwd, args))
	case "base":
		return run(base(cwd, args))
	case "list", "ls":
		return run(list(cwd, args))
	case "status":
		return run(status(cwd, args))
	case "git-log":
		return run(gitLog(cwd, args))
	case "copy":
		return run(copyCmd(cwd, args))
	case "remove", "rm":
		return run(remove(cwd, args))
	case "finish":
		return run(finish(cwd, args))
	case "reset":
		return run(reset(cwd, args))
	case "link":
		return run(link(cwd, args))
	case "deps":
		return run(deps(cwd, args))
	case "peek":
		return run(peek(cwd, args))
	case "unpeek":
		return run(peek(cwd, append([]string{"off"}, args...)))
	case "prompt":
		return run(prompt(cwd, args))
	case "alias":
		return run(alias(cwd, args))
	case "__jump-alias":
		return run(jumpAlias(cwd, args))
	case "complete":
		return run(complete(cwd, args))
	}
	return false, nil
}

// builtinNames are the commands an alias may expand to; builtins always win
// over an alias of the same name, like in git.
var builtinNames = map[string]bool{
	"create": true, "fork": true, "checkout": true, "ch": true, "home": true,
	"switch": true, "root": true, "global": true, "base": true, "list": true, "ls": true,
	"status": true, "remove": true, "rm": true, "reset": true, "link": true,
	"deps": true, "peek": true, "unpeek": true, "prompt": true, "alias": true,
	"git-log": true, "finish": true, "init": true, "copy": true,
}

// jumpCommands emit a cd script on stdout; the shell wrapper must capture
// and eval it. Keep in sync with the case list in shellinit.go.
var jumpCommands = map[string]bool{
	"checkout": true, "ch": true, "create": true, "fork": true,
	"home": true, "switch": true, "peek": true, "unpeek": true,
	"finish": true,
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
