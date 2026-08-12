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
`wt()` shell function that wraps the binary and runs `cd` itself.

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

`create` and `fork` both take `-c`/`--checkout` to cd straight into the new
worktree (flags go before the branch name).

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

```
$ wt fork
wt: created new branch 'main-2' from HEAD (4b9a96f)
wt: created worktree 'main-2' at /home/dev/worktrees/devbox/main-2
wt: carried over: 2 staged, 1 unstaged, 3 untracked file(s)
wt: switch with: wt ch main-2
```

### `wt ch <name>`

Jumps (cd's) to a worktree by its directory name; unique prefixes work too.
The repo name itself takes you back to the main checkout. No git state changes
hands here — every worktree permanently has its branch checked out.

### `wt remove [-f] [-b] <name>` (alias: `rm`)

Removes a worktree by name (prefix matching like `ch`). Refuses when the
worktree has local changes unless `-f` discards them. The branch is kept by
default; `-b` deletes it too — safely (`git branch -d`), so an unmerged branch
survives with a hint. The main checkout and the worktree you are standing in
cannot be removed.

### `wt status`

Shows where you are — project link, current worktree (with a `(main)` marker
in the main checkout), its path — followed by regular `git status` output:

```
$ wt status
project   devbox
worktree  main-2
path      /home/dev/worktrees/devbox/main-2

On branch main-2
nothing to commit, working tree clean
```

### `wt list` (alias: `ls`)

```
$ wt list
  NAME          BRANCH        STATE  PATH
* devbox        main          dirty  /devbox
  feature-auth  feature/auth  clean  /home/dev/worktrees/devbox/feature-auth
  main-2        main-2        clean  /home/dev/worktrees/devbox/main-2
```

## Where worktrees live

`~/worktrees/<linked-name>/<worktree>` — the project name comes from
`wt link`. Override the base directory with the `WT_ROOT` environment variable
or `git config wt.root` (per-repo or global); `~` and relative values resolve
against your home directory.

## Notes

- Git refuses to check out one branch in two worktrees at once — that's why
  argument-less `create` and `fork` always mint a new branch, and why
  `wt create <branch>` fails with a pointer to the worktree that already has
  that branch.
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
