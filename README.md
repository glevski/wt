# worktree (wt)

A fast, stdlib-only Go CLI that makes git worktrees ergonomic: create a
worktree from any local or remote branch, fork your current working state
(staged, unstaged and untracked changes included) into a fresh worktree, and
jump between worktrees with a two-letter command.

## Install

Prebuilt binary (macOS/Linux, checksum-verified, installs to `~/.local/bin`):

```sh
curl -fsSL https://github.com/nithenz/wt/raw/main/install.sh | sh
```

Or from a checkout:

```sh
make install                      # builds ~/.local/bin/worktree
```

Either way, wire up the shell function:

```sh
echo 'eval "$(worktree init zsh)"' >> ~/.zshrc   # or: init bash
```

`wt --version` reports the release tag and commit the binary was built from
(`make install` stamps them via `git describe`). Building outside a checkout —
from a `git archive` tarball, say, which carries no `.git` — leaves nothing to
stamp and yields `wt dev`; pass the values in instead:
`make install VERSION=0.0.11 COMMIT=7af004a`.

The `init` step is what makes `wt ch` able to actually `cd` you — a child
process can never change its parent shell's directory, so `init` emits a small
`wt()` shell function that wraps the binary and evals the tiny jump script it
prints (`cd …` plus environment updates).

Every wt jump also exports **`WT_HOME`** — the main checkout path of the repo
you jumped in — so `cd $WT_HOME`, `code $WT_HOME` and scripts always have the
project root at hand.

`init` also emits **tab completion**: subcommands complete on the first word,
and everywhere a command accepts an existing worktree or branch name —
`checkout`/`ch`, `status`, `remove`/`rm`, `create`, `fork`, `reset`, the `base`
and `root` subcommands — `<TAB>` offers the real candidates, computed live by
the binary (so `rm` only offers what is actually removable, and `create` also
lists remote-only branches). If your `.zshrc` runs the eval before `compinit`,
wt initializes the completion system itself.

Then link each repo you want to manage, once:

```sh
cd ~/my-repo && wt link myproject
```

The name is deliberate, not derived from the directory — it decides where the
repo's worktrees live and keeps two repos both cloned as `app/` from colliding.

## Shell prompt (optional)

`worktree prompt zsh` prints a ready-made `<project> (<branch>)` prompt
segment: the *project name* instead of the checkout's folder name (so
`~/worktrees/app/dev-2` shows as `app`; unlinked repos fall back to the repo
name, non-git directories to the plain path), with the branch colored in the
`wt list` palette — cyan in the root repo, green in wt worktrees, orange in
bases (red when drifted), magenta in external worktrees, and a red
`peek:<rev>` inside a peek. Wire it into your theme:

```zsh
setopt PROMPT_SUBST
_wt_prompt() { GIT_OPTIONAL_LOCKS=0 command worktree prompt zsh 2>/dev/null || print -rn -- '%B%F{cyan}%~%f%b'; }
PROMPT='%F{green}%n@%m%f: $(_wt_prompt) %B$%b '
```

The output is pure zsh prompt escapes (`%F{…}`, `%B`), so it renders with
your terminal's colors and costs one binary call per prompt (~5 ms). Literal
`%` in names is escaped, so directory names can't inject prompt codes.

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

Both commands also copy **git-ignored** files (`.env`, `node_modules`, …)
from the worktree you run them in, so new worktrees are runnable without
reinstalling anything. Turn it off with `git config wt.copyignored false`.

#### Deps — big dependencies, hardlinked in the background

Declare heavy ignored paths as **deps** and they stop blocking your jumps —
and stop costing disk:

```sh
wt deps add node_modules
```

On `create`/`fork`, deps are brought over by a detached background worker —
you're already cd'ed into the new worktree while `node_modules` appears. By
default every file is **hardlinked** to the source worktree (`cp -al` style:
real directories, shared file inodes), so a linked `node_modules` takes
near-zero extra space and lands much faster than a copy. Sources on another
filesystem fall back to copying automatically (hardlinks can't cross
devices), and wt says so upfront — a repo on a macOS bind mount with
worktrees on the container disk always copies; fork from a base or another
worktree (same filesystem) to get real links. `wt status` always shows a
`deps` line: `linked`, `copied`, `copying…`, `ejected`, or `declared: …`
where nothing was ever transferred (the root repo). Each dep is built in a `.wt-partial` sibling and renamed into
place, so a half-built directory never appears at its real path. `wt list`
shows `syncing` while it runs; `wt status` shows a `deps linked` line after.

Hardlink fine print: removing any worktree never breaks the others (shared
inodes live while one link remains), and package managers *replace* files on
install, which naturally un-shares them. But a tool editing files **in
place** inside a dep (build caches) writes through to every linked worktree —
when that matters, eject:

```sh
wt deps eject             # this worktree gets private copies of its deps
wt deps eject --no-copy   # just delete them; reinstall yourself
```

- `-w`/`--wait` — bring deps over synchronously (block until done)
- `--no-deps` — skip deps for this run, still copy other ignored files
- `--no-ignored` — copy nothing at all (no ignored files, no deps)
- `--copy-deps` (create/fork/peek/base add) or `git config wt.depscopy true` —
  real copies instead of hardlinks
- `wt deps [list | add <path> | rm <path> | sync [--copy] [name] |
  eject [--no-copy] [name] | link [source] | purge]` — manage the list
  (stored as multi-valued `git config wt.deps`); `sync` re-syncs deps into a
  worktree in the foreground (linking by default, `--copy` forces copies) —
  recovery after a failed background run, or onboarding a worktree created
  before deps were declared.
- `wt deps link [source]` — re-link the current worktree's deps as hardlinks
  to a source worktree; with no argument the source is the worktree holding
  the branch this one was created from (your base). The fix for worktrees
  that got copies because they were created from a cross-filesystem root:
  purge or eject, then `wt deps link staging`.
- `wt deps purge` — reclaim space: remove deps from every regular worktree
  (never the root, bases, or the one you stand in), after listing the
  targets and a y/N confirmation. `wt deps link`/`sync` bring them back.

`wt rm` refuses a worktree whose deps are still syncing; `wt rm -f` stops the
background copier first (its pid rides in the state marker) and then removes,
so a forced removal never races the copy.

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

### `wt checkout [name]` (alias: `ch`)

Jumps (cd's) to a worktree by its directory name; unique prefixes work too.
No git state changes hands here — every worktree permanently has its branch
checked out.

Bare `wt ch` opens an **interactive picker**: the 10 most recently used
worktrees (latest checkout stamp, falling back to creation time), newest
first — arrows or j/k to move, Enter to jump, Esc/q to cancel. It renders on
the terminal directly, so it composes with the shell wrapper like any other
jump.

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

### `wt global <cmd>` — your projects from anywhere

Works outside any repo. Register a project once with `wt link -r` (or a bare
`wt link -r` in an already linked repo — registration is always explicit),
then from wherever you stand:

```sh
wt global ls                       # all registered projects:
                                   #   NAME  WORKTREES  PATH
wt global ls tickets-app           # that project's full worktree table
wt global status tickets-app/dev   # wt status as if run there (-g works)
wt global git-log tickets-app      # git log of the root repo…
wt global git-log tickets-app/dev --oneline -5   # …or a worktree, args pass through
wt global ch tickets-app           # jump to the root repo
wt global ch tickets-app/dev       # jump into a worktree
```

Project names prefix-match like worktree names do, and tab completion offers
them. The registry is plain global git config (`wt.project.<name>` → root
repo path); entries whose repo vanished or was renamed are pruned
automatically with a note. `wt link` shows whether the repo is registered.

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

A glob pattern bulk-removes: `wt rm 'dev-*'` or `wt rm '*'` (quote it so your
shell doesn't expand it). Wildcards only ever match **wt-managed** worktrees —
never the root, bases, external tools' worktrees, or the one you're standing
in — and always print the matches and ask `[y/N]` first; only a plain `y`
proceeds. `-f`/`-b` apply to every match.

### `wt finish [-d] [-b] [-f]`

The "I'm done here" command. Jumps back to the last location this shell
jumped from (the same per-shell state `wt switch` uses), or home when there
is none — and, on request, cleans up on the way out:

```sh
wt finish            # just go back — the worktree stays
wt finish -d         # …and delete the worktree (branch kept)
wt finish -d -b      # …and delete its branch too (safe delete)
wt finish -d -f      # discard local changes; -f -b force-deletes nothing extra
```

`-b` needs `-d`. Local changes block `-d` unless `-f`; the main checkout and
(with `-d`) base worktrees refuse. If the remembered location is inside the
worktree being deleted, finish lands you home instead.

### `wt status [name]`

Shows a worktree — the current one by default, or any worktree by name
(prefix matching like `checkout`) — as project link, worktree name (with a
`(home)` marker for the main checkout), and path. `-g`/`--git` appends regular
`git status` output as if run in that worktree:

```
$ wt status
project   devbox
worktree  main-2
branch    main-2
commit    4b9a96f  Kirill G, 2 hours ago
path      /home/dev/worktrees/devbox/main-2

$ wt status -g
project   devbox
worktree  main-2
branch    main-2
commit    4b9a96f  Kirill G, 2 hours ago
path      /home/dev/worktrees/devbox/main-2

On branch main-2
nothing to commit, working tree clean
```

### `wt git-log [name]`

`git log` for any worktree without leaving where you stand — `wt git-log
feature-auth` (unique prefixes work), no name means the current worktree.
Extra arguments pass straight through, so `wt git-log dev --oneline -5` does
what you'd expect. Passthrough like `status -g`: your pager, colors and log
config all apply.

### `wt copy [-f] [--from <worktree>] <file> [dst]`

Copies one file into the current worktree — from the root repo by default,
or from any worktree via `--from` (prefix matching like `checkout`). Made for
the files git doesn't carry across worktrees: `.env`, local configs,
credentials.

```sh
wt copy .env                      # root repo's .env → here
wt copy --from dev-2 .env.local   # from another worktree
wt copy .env config/.env.dev      # different destination path
```

Paths are relative to each worktree's root; parent directories are created;
symlinks and executable bits survive. An existing destination is refused
unless `-f` overwrites it.

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

Rows are ordered for scanning: the root repo first, then base branches, then
everything else by most recent checkout (most recently created as tiebreak).

### `wt df` (alias: `du`)

What each worktree actually costs on disk, sorted biggest-first — and
**hardlink-aware**: shared bytes are charged to the first worktree that holds
them (du-style; canonical order root → bases → rest by last checkout), so a
linked fork shows only its own few megabytes:

```
$ wt df
  NAME      DEPS    SIZE    DEPS-SIZE  SHARED
  staging   copied  512M    498M       498M
  main-2    linked  14M     ~          498M
  TOTAL             526M               (498M saved by sharing)
```

`SIZE` is the attributed real (block-based) usage excluding `.git` — the
column sums exactly to `TOTAL`; `DEPS-SIZE` the attributed subset under
declared deps — what `deps purge` would free there; `SHARED` this tree's
hardlink-shared content (informational, not charged again). Walking big
trees takes a moment (rows are computed in parallel).

On a terminal, names are colored by kind — in `list` and in `status`'s
worktree line alike: **cyan** for the main checkout, **green** for wt-managed
worktrees (under the workspace root), **magenta** for worktrees created
elsewhere by other tools (e.g. `.claude/worktrees`). Piped output and
`NO_COLOR` stay plain.

### `wt alias` — your own commands

Git-style aliases: name your favorite invocations and wt expands them, typed
arguments appended. Builtins always win over an alias of the same name.

```sh
wt alias add cr create -c      # wt cr feature  →  wt create -c feature
wt alias add stg status -g
wt alias rm cr
wt alias                       # list them
```

`add`/`rm` write your **global** git config (`wt.alias.<name>`) — aliases are
a habit, not a project property — but any config scope works, so a per-repo
alias is just `git config wt.alias.stg "status -g"`. Aliases tab-complete
like the command they expand to, and an alias of a jump command (`create`,
`checkout`, `peek`, …) cd's exactly like the real thing: the shell wrapper
asks the binary whether an unknown first word is a jump alias.

## Snapshots — a private, diffable record of your iterations

Polishing a change takes several rounds, and committing each one is overkill
— but without a record you lose track of what changed when. `wt snapshot`
(alias `snap`) records the worktree's current state — tracked edits **and**
untracked files, ignored files excluded — as a local, commit-like object:
never on the branch, never pushed, invisible to `git log`, `git branch` and
other worktrees (only `git log --all`, which lists every ref, shows them —
as it shows your stash), gone with the worktree.

```sh
wt snap                    # record "snapshot 1"; the next call is "snapshot 2", …
wt snap polish the auth    # …or with a message (-m if it would read as a subcommand)
wt snap ls                 # this series: N, AGE, FILES, MESSAGE
wt snap show 2             # what iteration 2 added — git show, pager and colors
wt snap diff               # working tree vs the latest snapshot: what changed since you last recorded
wt snap diff 2             # snapshot 2 vs 1: what iteration 2 added
wt snap diff 1 3           # any two
wt snap diff --full        # working tree vs the commit — like git diff, but untracked files included
wt snap diff --full 2      # everything up to snapshot 2
wt snap purge              # drop this series (y/N); --all drops every series
wt snap rev 2              # the sha — for hand-rolled git, or: wt peek $(wt snap rev 2)
```

Snapshots are numbered per commit: `git commit` (or amend, rebase) starts a
fresh series at 1, and the old one stays until purged. `wt snap ls --all`
shows every series grouped by its base commit, `wt snap ls <commit>` one of
them, and `<commit>:N` addresses a snapshot in it (`wt snap show a1b2c3d:2`).
Extra arguments to `show` and `diff` pass through to git (`--stat`, paths, …).

Under the hood a snapshot is a commit whose tree is built from a throwaway
index and stored under `refs/worktree/wt/snap/…` — git's per-worktree ref
namespace — so two worktrees on the same commit never mix, and no ordinary
push (`git push`, `--all`, `--tags`, force pushes) ever carries one. The one
exception is `git push --mirror`, which by definition copies every ref:
purge first if you mirror a worktree. Recording an unchanged tree is a no-op.
Untracked scratch files get recorded too; gitignore what you don't want kept.
`wt status` shows the count, and `wt rm` says how many go with the worktree.

## Peek — look at any revision without touching anything

```sh
wt peek origin/main      # or a tag, a sha, HEAD~3 …
```

Exports the revision's files into a **disposable fake worktree** under
`~/worktrees/<project>/peek-…` — a plain directory with no branch, no
checkout, and no git registration — and jumps you there. Your real worktree
is never written to, so peeking is safe from any state: dirty, mid-rebase,
whatever. The environment comes along (ignored files synchronously, declared
deps in the background — `-w`, `--no-deps`, `--no-ignored` work like on
`create`/`fork`), so the peeked code runs.

It's a viewer: git commands inside it fail (there's no repo), and edits are
throwaway. `wt peek off` (alias: `wt unpeek`) jumps you back to where you
came from and deletes the directory; `wt peek off <name>` drops one from
anywhere; bare `wt peek` lists open peeks, which also appear in `wt ls` as
red `peek` rows. `wt status` inside a peek shows the peek view with the exit
hint at the bottom. If looking
turns into working, that's what `wt fork`/`wt create` are for.

## Base branches

A **base branch** is a long-lived branch (main, staging, dev, …) you promote
to a permanent, view-only worktree named after the branch:

```sh
wt base add staging
```

The problem it solves: you're mid-work on `dev` with uncommitted changes and
a staging issue comes up. `wt ch staging` drops you into a persistent staging
checkout — inspect, run the code, even make exploratory tweaks — with zero
impact on your dev worktree and no new worktree minted per visit. The moment
looking becomes real work, fork off it: `wt fork -c` from inside the base
carries your tweaks into a properly named worktree and jumps there.

- `wt base` / `wt base list` — the worktree table filtered to bases
- `wt base add <branch>` — promote an existing (local or remote) branch;
  refuses branches currently checked out elsewhere
- `wt base rm [-f] <name>` — remove the base worktree; the branch itself is
  never touched. Regular `wt rm` refuses bases outright.
- `wt base update [name]` — fetch and fast-forward bases to their upstreams
  (ff-only: can never lose anything; dirty, drifted or diverged bases are
  skipped with a note)
`base add` seeds the new base with the git-ignored files of the worktree you
run it from (`.env` and friends), so it is readable and runnable right away —
but **not** the declared [deps](#deps--big-dependencies-copied-in-the-background):
a base is a launch pad, not a build directory. `--deps` (with `-w` to wait)
opts them in, `wt deps sync <name>` adds them later, and `--no-ignored` copies
nothing at all.

- `wt base reset [--hard] [name]` — restore a base to its pristine state:
  a drifted base gets its own branch checked out again, then the branch is
  hard-synced to the upstream tip, backward moves included (accidental local
  commit, force-pushed upstream). Local changes block it; `--hard` discards
  tracked changes and keeps untracked files. No name needed when standing
  inside the base.

Bases show **orange** names in `list`/`status`. wt can't stop git from
switching a base's branch — instead it *tells* you: a drifted base (checked
out branch no longer matches its name) gets a red BRANCH cell and a `!`
suffix in `wt list`, and `(base: staging, drifted)` in `wt status`.
`wt reset` refuses to run inside a base — other worktrees reset *to* it.

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
