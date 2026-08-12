# worktree (wt)

A fast, stdlib-only Go CLI that makes git worktrees ergonomic: create a
worktree from any local or remote branch, fork your current working state
(staged, unstaged and untracked changes included) into a fresh worktree, and
jump between worktrees with a two-letter command.

## Install

```sh
make install                      # builds ~/.local/bin/worktree
echo 'eval "$(worktree init zsh)"' >> ~/.zshrc   # or: init bash
```

The `init` step is what makes `wt ch` able to actually `cd` you — a child
process can never change its parent shell's directory, so `init` emits a small
`wt()` shell function that wraps the binary and evals the tiny jump script it
prints (`cd …` plus environment updates).

Every wt jump also exports **`WT_HOME`** — the main checkout path of the repo
you jumped in — so `cd $WT_HOME`, `code $WT_HOME` and scripts always have the
project root at hand.

Then link each repo you want to manage, once:

```sh
cd ~/my-repo && wt link myproject
```

The name is deliberate, not derived from the directory — it decides where the
repo's worktrees live and keeps two repos both cloned as `app/` from colliding.

## Commands

### `wt link [name]`

Links the repo to a project name, stored in `git config wt.name` (shared
`.git/config` — visible from every worktree of the repo, never tracked, and
per-clone, so a fresh clone needs linking again). `create` and `fork` refuse
to run in unlinked repos; everything else works regardless. Without an
argument it shows the current link. Relinking is allowed — existing worktrees
keep working since their paths live in git's own registry. Unlink with
`git config --unset wt.name`.

### `wt create <branch>`

Adds a worktree for an existing branch — from the local branch if it exists,
otherwise from a remote (preferring `origin`), as a new tracking branch. Logs
where the branch came from and how it relates to its upstream. Purely local:
it never fetches; if the branch is unknown, it tells you to `git fetch` first.

If the branch is already checked out in another worktree (git forbids checking
one branch out twice), `create` starts a fresh auto-suffixed branch at its tip
instead: `wt create main` while main is busy gives you branch `main-2` in a
new worktree — with `-n` naming the directory as usual.

`create` and `fork` share two flags (flags go before the branch name):

- `-c`/`--checkout` — cd straight into the new worktree.
- `-n`/`--name` — pick the worktree's directory name instead of deriving it
  from the branch. A leading dash appends to the branch name: on
  `feature/auth`, `-n -fix` gives `feature-auth-fix`; with auto-named branches
  (`wt fork` with no argument on `main`) `-n -exp` gives `main-exp`.

```
$ wt create feature/auth
wt: branch 'feature/auth' found locally (origin/feature/auth: ahead 2, behind 1)
wt: created worktree 'feature-auth' at /home/dev/worktrees/myrepo/feature-auth
wt: switch with: wt ch feature-auth
```

### `wt create` (no argument)

Adds a *clean* worktree at the current commit on a new auto-named branch
(`main` → `main-2`, `main-3`, …). Your uncommitted changes stay where they
are — use `fork` to bring them along.

### `wt fork [new-branch]`

Forks your current state: new branch at HEAD, new worktree, and your staged,
unstaged and untracked changes carried over — with the staged/unstaged split
preserved exactly. The original worktree is left untouched (copy, not move).
Without an argument the branch is auto-named like `create`.

Passing an **existing** branch makes it the base instead: `wt fork main` from
a dirty `dev` cuts an auto-named `main-2` at main's tip and carries your
changes onto it — "take my WIP onto a fresh main". If the changes don't apply
cleanly there, the worktree stays and the error tells you how to finish the
apply manually.

```
$ wt fork
wt: created new branch 'main-2' from HEAD (4b9a96f)
wt: created worktree 'main-2' at /home/dev/worktrees/devbox/main-2
wt: carried over: 2 staged, 1 unstaged, 3 untracked file(s)
wt: switch with: wt ch main-2
```

### `wt checkout <name>` (alias: `ch`)

Jumps (cd's) to a worktree by its directory name; unique prefixes work too.
No git state changes hands here — every worktree permanently has its branch
checked out.

`wt ch -b <new-branch>` works like `git checkout -b`: it forks your current
state (staged, unstaged and untracked changes included) into a new worktree
on that branch and jumps there — shorthand for `wt fork -c <new-branch>`.

### `wt home`

Jumps back to the main checkout (the root repo), from wherever you are —
no name needed.

### `wt root <cmd>` — root mode

Runs a command in the **root repo's context** from wherever you stand:

- `wt root status [-g]` — the root's status, as if run there
- `wt root checkout` — jump to the root (same as `wt home`); with a name or
  `-b` it behaves like `checkout` run at the root
- `wt root create …` — create based on the root's current branch
- `wt root fork [-c] …` — fork the root's current state (its branch, its
  staged/unstaged/untracked changes), even while you're in another worktree

All flags pass through unchanged (`wt root fork -c -n -exp`, …).

### `wt switch`

Toggles between where you are and where your last wt jump left from — like
`cd -`. Run it twice and you're back. The history lives in a plain shell
variable inside the `wt()` function, so every terminal has its own,
independent toggle state.

### `wt reset [--hard] [base]`

Only inside a worktree (never the main checkout). Moves the worktree's branch
back to the tip of its **base branch** — the branch it was created from,
recorded automatically by `create`/`fork` (`main-2` remembers `main`;
remote-created worktrees remember `origin/<branch>`) — while staying on the
same branch. Any local changes block it; `--hard` proceeds, discarding tracked
changes (untracked files are kept, standard `git reset --hard` semantics).
Worktrees created before this feature (or by other tools) have no recording —
name the base explicitly: `wt reset <branch>` always works and overrides.

### `wt remove [-f] [-b] <name>` (alias: `rm`)

Removes a worktree by name (prefix matching like `ch`). Refuses when the
worktree has local changes unless `-f` discards them. The branch is kept by
default; `-b` deletes it too — safely (`git branch -d`), so an unmerged branch
survives with a hint. The main checkout and the worktree you are standing in
cannot be removed.

### `wt status [name]`

Shows a worktree — the current one by default, or any worktree by name
(prefix matching like `checkout`) — as project link, worktree name (with a
`(home)` marker for the main checkout), and path. `-g`/`--git` appends regular
`git status` output as if run in that worktree:

```
$ wt status
project   devbox
worktree  main-2
path      /home/dev/worktrees/devbox/main-2

$ wt status -g
project   devbox
worktree  main-2
path      /home/dev/worktrees/devbox/main-2

On branch main-2
nothing to commit, working tree clean
```

### `wt list` (alias: `ls`)

```
$ wt list
  NAME          BRANCH        STATE  COMMIT   CREATED  CHECKOUT
* devbox        main          dirty  4b9a96f  -        now
  feature-auth  feature/auth  clean  163a116  2d       5h
  main-2        main-2        clean  4b9a96f  3h       -
```

`CREATED` is the worktree's age (from git's own metadata; the main checkout
shows `-`). `CHECKOUT` is the last time wt jumped there — via `checkout`/`ch`
or a `-c` flag — recorded as a stamp file in the worktree's git admin dir, so
it starts as `-` and travels/dies with the worktree.

On a terminal, names are colored by kind — in `list` and in `status`'s
worktree line alike: **cyan** for the main checkout, **green** for wt-managed
worktrees (under the workspace root), **magenta** for worktrees created
elsewhere by other tools (e.g. `.claude/worktrees`). Piped output and
`NO_COLOR` stay plain.

## Where worktrees live

`~/worktrees/<linked-name>/<worktree>` — the project name comes from
`wt link`. Override the base directory with the `WT_ROOT` environment variable
or `git config wt.root` (per-repo or global); `~` and relative values resolve
against your home directory.

## Notes

- Git refuses to check out one branch in two worktrees at once — that's why
  argument-less `create` and `fork` always mint a new branch, and why
  `wt create <branch>` falls back to a fresh `<branch>-N` branch when the
  branch is already checked out elsewhere.
- A worktree is a completely normal checkout: commit, push and open PRs from
  it as usual, then `wt rm` it once merged.
- `fork` moves state via `git stash create` + `stash apply --index` (never
  touching your original worktree), plus a file copy for untracked files.
- Every command talks to the real `git` binary; stdout carries only
  machine-readable output (paths, tables), all narration goes to stderr.

## Development

```sh
make test
make vet
make fmt
```
