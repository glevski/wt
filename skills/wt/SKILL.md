---
name: wt
description: Manage git worktrees with the wt CLI (binary `worktree`). Use whenever a task involves creating, listing, switching, resetting or removing git worktrees, working on multiple branches in parallel, inspecting another branch without disturbing the current one, sharing dependencies (node_modules) across worktrees via hardlinks, checking worktree disk usage, recording a snapshot of work in progress when the user asks for one, or when the user mentions wt, worktrees, base branches, peeks, snapshots, deps linking, or ~/worktrees paths. Includes the default flow an agent should follow to create a new worktree. Prefer wt over raw `git worktree` commands in repos that are wt-linked.
argument-hint: "[wt command | request, e.g. new <branch>, cleanup, snap <message>, df]"
---

# wt — fast git worktree manager

`wt` creates and moves between git worktrees with one-word commands. The
binary is `worktree`; `wt` is a shell function installed by
`eval "$(worktree init zsh)"` (or `bash`) — it exists because a child process
cannot `cd` its parent shell, so jump commands print a tiny script the
function evals.

Check availability with `worktree --version` (prints e.g. `wt 0.0.14 (c66e121)`).

## Invoked by hand — `/wt …`

When the user runs this skill themselves, whatever they typed after `/wt` is
the request. Act on it right away instead of summarizing this document:

| typed | do |
|---|---|
| `/wt` (nothing) | run `worktree list` and `worktree status`, show both, ask what they want |
| `/wt new [branch]` | the **Default flow for agents** below; report the new path and its deps state |
| `/wt cleanup` (same as `/wt-cleanup`) | remove the worktrees **this session** created, following **Cleaning up**; names typed after it (`/wt cleanup api-fix`) target those instead |
| `/wt snap [message]`, `/wt snapshot [message]` | record a snapshot of the worktree you are working in — see **Snapshots**; report the number it got, or that nothing changed. `/wt snap ls`, `/wt snap diff`, `/wt snap show N` pass straight through |
| `/wt list` (or `/wt ls`) | run `worktree list` and show the table as is, then say what stands out: which one is current (`*`), which are `dirty`, any drifted base (`branch!`), any still `syncing` deps, and which belong to other tools (e.g. `.claude/worktrees/…`) |
| `/wt df` (or `/wt du`) | run `worktree df` and show the table, then say what stands out: the biggest worktrees, the TOTAL and how much sharing saves, and any worktree whose deps are `copied` rather than `linked`. Suggest what would reclaim space (`worktree deps link`, `/wt-cleanup`) — never act on it unasked |
| `/wt <other read-only command>` — `status [-g]`, `git-log`, `deps`, `base list`, `link`, `snap ls/show/diff`, `global ls/status/git-log` | run it with the `worktree` binary and show its output — these change nothing |
| `/wt <anything else>` — plain words, or a wt command that changes something | a **request**, not a command line to execute verbatim: carry it out under the rules of this document (default flow for new worktrees, **Cleaning up** for removals, **Safety rules** for everything destructive) |

Typing a command after `/wt` waives nothing. `/wt rm -f api`, `/wt reset
--hard`, `/wt deps purge` and the like are handled as any other destructive
request: say exactly what would be removed or discarded, get a clear yes, then
run it. If it is unclear whether the text is a command or a sentence
(`/wt reset my branch`), ask instead of guessing.

Jump commands (`/wt ch foo`, `/wt home`) cannot move the user's terminal from
here — give them the path, or the command to run in their own shell.

## Working conventions

- **stdout is machine-readable** (tables, paths, shell scripts); **all
  narration goes to stderr** prefixed `wt:`. Piping a command keeps the data
  clean.
- **Jump commands only move an interactive shell.** `checkout`/`ch`, `create -c`,
  `fork -c`, `home`, `switch`, `peek`, `unpeek`, `finish`, `global ch` and
  `root checkout` emit a `cd` script for the `wt()` function. Running them
  from a script or a non-interactive tool call prints the script instead of
  moving anywhere.
  **When automating, resolve the path first** — `wt list` or `wt status <name>`
  shows it — then use `git -C <path> …` or `cd <path> && …` in a single command.
- **Never pipe a jump command** (`wt ch x | tee`): the shell function runs in a
  subshell and the `cd` is lost.
- Worktrees live in `~/worktrees/<project>/<worktree>`; override the base with
  `$WT_ROOT` or `git config wt.root`.
- Every jump also exports **`WT_HOME`** — the main checkout's path.

## Safety rules

- **Destructive and overriding actions need an explicit ask, each time.**
  That is: `remove`, `finish -d`, `base rm`, `reset` and `base reset`,
  `deps eject`, `deps purge`, `snap purge`, glob removal, and any `-f`,
  `--hard` or `--copy-deps`. Before running one, say what it deletes or
  discards. A request for one of them is not a request for the next.
- **Never answer a y/N prompt on the user's behalf** — no `echo y |`, no
  `yes |`. If a command prompts, it is the user's to confirm.
- **Only touch what this session made.** Worktrees from other sessions or
  tools (magenta in `wt list`, e.g. `.claude/worktrees/…`), base worktrees and
  the main checkout are off limits unless the user names them.
- **Never work in a base** or switch its branch — fork off it.
- When unsure whether a command changes anything, treat it as if it does.

## Setup, once per repo

```sh
cd ~/my-repo && wt link myproject      # required by create/fork
wt link -r myproject                   # …and register it for `wt global`
wt link                                # show current link + registration
```

The project name is deliberate, not derived from the directory: it decides
where worktrees live and keeps two repos both cloned as `app/` from colliding.

## Default flow for agents — a new worktree

Follow this whenever you need a new worktree and were not told otherwise. Call
the `worktree` binary directly: the `wt` function may not exist in a
non-interactive shell.

**1. Check the prerequisites — deps must be declared *before* you create.**

```sh
worktree link                    # shows the link; if unlinked: worktree link <project>
worktree deps                    # lists declared deps; if none, declare them now:
worktree deps add node_modules   # one per heavy git-ignored dir (.next, vendor, .venv, …)
```

Declaring is what puts a dependency in link mode. A declared dep is
**hardlinked** into the new worktree: near-zero disk, done in moments. An
undeclared `node_modules` is just another git-ignored path — copied in full,
synchronously, costing its whole size again in every worktree. Declaring
after the fact does not fix worktrees that already exist (repair them with
`worktree deps link`, below).

**2. Create it in link mode, waiting for deps.**

```sh
worktree create -w -n <name> [<branch>]   # clean worktree
worktree fork   -w -n <name> [<branch>]   # …carrying uncommitted changes along
```

- `-w` blocks until deps are in place, so a build or test can start right away.
- Link mode is the default — never pass `--copy-deps`, and leave
  `wt.depscopy` unset.
- Never pass `-c`: in a tool call it prints a `cd` script instead of moving.

**3. Read back the path and confirm the deps state.**

```sh
worktree status <name>           # path  …/worktrees/<project>/<name>    deps  linked
```

`create` prints nothing on stdout, and the name may differ from what you
expected (auto-named branch, or `<branch>-N` when the branch is checked out
elsewhere), so always read the path here. `deps linked` is the state you
want. `deps copied` means the source worktree is on another filesystem
(hardlinks cannot cross devices; wt warns at creation) — create from a base
or another worktree on the same disk instead, or repair in place:
`cd <path> && worktree deps link <source-worktree>`.

**4. Work there** with `git -C <path> …` or `cd <path> && <command>`.

Linked deps share file contents with the source worktree. Running the package
manager is safe (it replaces files, which un-shares them); a tool that edits
files **in place** inside a dep writes through to every linked worktree — tell
the user and offer `worktree deps eject` (private copies for that worktree)
before running such a tool.

**5. Leave it in place — never clean up on your own.** When the task is done,
report the worktree's name, path and branch, and stop there. Do not remove
it, even when the work is merged or abandoned: deleting worktrees is the
user's call. They can run `/wt-cleanup` (or ask) when they want it gone — see
**Cleaning up** below.

## Creating worktrees

```sh
wt create <branch>        # worktree for an existing local or remote branch
wt create                 # clean worktree off current HEAD, new auto-named branch
wt fork [new-branch]      # like create, but carries staged+unstaged+untracked changes
wt fork <existing-branch> # base off that branch's tip, auto-named new branch
wt checkout -b <branch>   # fork current state into a new worktree and jump (like git checkout -b)
```

Shared flags on `create`/`fork`:

| flag | effect |
|---|---|
| `-c`, `--checkout` | cd into the new worktree |
| `-n`, `--name <n>` | directory name; a leading dash appends to the branch (`-n -fix` → `<branch>-fix`) |
| `-w`, `--wait` | bring declared deps over synchronously instead of in the background |
| `--no-deps` | copy ignored files but skip declared deps |
| `--no-ignored` | copy no ignored files at all (and no deps) |
| `--copy-deps` | real copies of declared deps instead of the default hardlinks |

If the requested branch is already checked out elsewhere (git allows a branch
in only one worktree), `create` makes a fresh `<branch>-N` at that branch's tip
rather than failing.

**Environment copying:** new worktrees inherit the source worktree's
git-ignored files (`.env` and friends) so they run immediately. Declared deps
have their own pipeline — hardlinked, in the background — see Deps below.

## Moving around

```sh
wt checkout <name>    # jump by name; unique prefixes work (alias: ch)
wt checkout           # no name: pick from recent ones interactively
wt home               # back to the main checkout
wt switch             # toggle current ↔ last location, like cd - (per shell)
wt finish             # "done here": back where you came from (home if none)
wt finish -d          # …and delete the worktree (-b also deletes its branch, needs -d; -f discards changes)
```

## Inspecting

```sh
wt list                       # worktree table (alias: ls)
wt status [name]              # project, worktree, branch, commit, path, deps state
wt status -g [name]           # …plus real `git status` output as if run there
wt git-log [name] [git args]  # git log in a worktree; extra args pass through
```

`wt list` columns: NAME, BRANCH, STATE, COMMIT, CREATED, CHECKOUT. Rows are
ordered root repo → bases → the rest by most recent checkout. Names are
colored by kind: **cyan** main checkout, **orange** base, **green** wt-managed,
**magenta** worktrees made by other tools, **red** peeks; a drifted base shows
a red branch with a `!` suffix.

```sh
wt df                         # real disk cost per worktree, biggest first (alias: du)
```

`wt df` columns: NAME, DEPS, SIZE, DEPS-SIZE, SHARED, plus a TOTAL row. It is
hardlink-aware: shared bytes are charged once, to the first worktree holding
them, so a linked worktree shows only its own few megabytes. SIZE excludes
`.git` and sums to TOTAL; DEPS-SIZE is what `wt deps purge` would free there.

## Base branches — view-only launch pads

Long-lived branches (main/staging/dev) as permanent worktrees you look at and
fork off, never work in.

```sh
wt base add <branch>     # create it; --no-ignored skips the env copy, --deps [-w] adds deps
wt base list             # default when bare: wt base
wt base rm [-f] <name>   # remove the worktree; the branch is untouched
wt base update [name]    # fast-forward to upstream (skips dirty/drifted/diverged, loudly)
wt base reset [--hard] [name]  # hard-sync to upstream, re-checking-out the branch if drifted
```

`base add` seeds the base with the current worktree's git-ignored files but
**not** declared deps — a base is a launch pad, not a build directory. Note
that a worktree forked *off* a base therefore inherits no deps either; run
`wt deps sync <name>` if you need them.

`wt rm` refuses bases (use `wt base rm`); `wt reset` refuses to run inside one.
wt cannot stop git from switching a base's branch, so it reports drift instead.

## Snapshots — a private, diffable record of iterations

Local, per-worktree, commit-like records of the working state (tracked edits
**and** untracked files, ignored ones excluded). Never a commit on the branch,
invisible to normal git (only every-ref views like `git log --all` list them),
not carried by any ordinary push (only `git push --mirror` copies every ref);
they die with the worktree.

**Opt-in — do not snapshot on your own initiative.** Record a snapshot only
when the user asks for one ("snapshot this", "save this iteration") or runs
`/wt snap` / `/wt snapshot`. Not before a risky edit, not after each step, not
as a substitute for a commit. If the user asks for a snapshot per iteration,
keep doing it for that task and no longer. Reading existing snapshots
(`snap ls`, `show`, `diff`, `rev`) is fine whenever it helps.

When asked to record one:

```sh
cd <worktree> && worktree snap <what this iteration changed>   # records the worktree the command runs in
cd <worktree> && worktree snap -m "show the new layout"        # -m when the message starts like a subcommand
```

- Give it a short message that says what changed; with none, wt names it
  `snapshot N`.
- Report what wt printed: `snapshot N recorded (…)`, or `nothing changed since
  snapshot N` — an unchanged tree records nothing, which is not an error.
- "What changed since the last snapshot?" → `worktree snap diff`. "What did
  iteration N do?" → `worktree snap show N`.
- To look at an old iteration without touching the worktree:
  `worktree peek $(worktree snap rev N)`.
- There is no restore command. To bring a file back, the snapshot is a commit:
  `git restore --source=$(worktree snap rev N) -- <path>` — it overwrites the
  working file, so only on the user's request.
- Leave `snap purge` to the user: it asks y/N and aborts without a terminal.

```sh
wt snap [message]             # record; "snapshot N" by default, numbered per commit (-m forces a message)
wt snap ls [--all|<commit>]   # current series / every series / one series
wt snap show [N]              # what iteration N added (default: latest); git show args pass through
wt snap diff [--full] [A] [B] # vs the snapshot before (working tree vs latest, N vs N-1);
                              # --full: vs the base commit; A B: any two; git diff args pass through
wt snap purge [--all]         # delete this series / every series (y/N)
wt snap rev N                 # the sha — e.g. wt peek $(wt snap rev N)
```

A commit starts a fresh series (numbering restarts at 1); older series stay
until purged and are addressed as `<commit>:N`. Recording an unchanged tree is
a no-op. Use `wt snap diff`, not `git diff <sha>`, to compare with the working
tree — plain git diff ignores untracked files.

## Peek — look at a revision without touching anything

```sh
wt peek <rev>      # disposable snapshot: a fake worktree, no branch, no checkout
wt peek            # list current peeks
wt peek off        # return and delete it (alias: wt unpeek)
```

A peek is `git archive` extracted into a directory — not registered with git,
so your worktrees are never disturbed and nothing can be lost. Edits there are
throwaway and git commands inside it fail (there is no repo). Env/deps flags
apply, so peeked code runs.

## Deps — big dependencies, hardlinked in the background

```sh
wt deps add node_modules      # declare (also .next, vendor, …) — do this before creating worktrees
wt deps rm <path>
wt deps list                  # default when bare: wt deps
wt deps sync [--copy] [name]  # bring deps into a worktree again, foreground (links by default)
wt deps link [source]         # re-link this worktree's deps to a source worktree (default: its base)
wt deps eject [--no-copy]     # give this worktree private copies (--no-copy: just delete them)
wt deps purge                 # remove deps from every regular worktree to reclaim space (y/N)
```

Only **declared** deps get this treatment, and only in worktrees created
after the declaration; anything else git-ignored is plainly copied. On
`create`/`fork` a detached worker brings declared deps over so `create -c`
returns instantly; `wt status` shows `deps copying…` and `wt list` shows
`syncing` until done. `-w`/`--wait` makes it synchronous. A dep is only
transferred if it exists in the source worktree **and** is git-ignored there.

**Link mode is the default:** every file is hardlinked to the source worktree
(real directories, shared inodes), so a linked `node_modules` costs near-zero
disk. `--copy-deps` or `git config wt.depscopy true` forces real copies. A
source on another filesystem falls back to copying with a warning — hardlinks
cannot cross devices. `wt status` shows the result on its `deps` line:
`linked`, `copied`, `copying…`, `ejected`, or `declared: …` when nothing was
transferred.

Removing a worktree never breaks the others, and a package-manager install
replaces files, which un-shares them. A tool that edits files in place inside
a dep writes through to every linked worktree — `wt deps eject` first.

Deps are **linked or copied, never installed** — wt never runs a package
manager.

## Removing

```sh
wt remove <name>        # alias: rm; the branch is kept
wt remove -b <name>     # also delete the branch (safe delete)
wt remove -f <name>     # discard local changes
wt rm 'dev-*'           # quoted glob: bulk-remove matching wt-managed worktrees
wt rm '*'               # …all of them; always lists matches and asks y/N first
```

Guards: the main checkout, base worktrees, and the worktree you are standing
in are never removed; a worktree still syncing deps needs `-f`.

## Cleaning up — only when the user asks

**Never delete a worktree on your own initiative.** That covers `remove`,
`finish -d`, `base rm` and `deps purge` alike — not when a task ends, not to
save disk, not because a branch was merged. Remove worktrees only when one of
these holds:

- the user asks for it in this conversation ("clean up", "remove that worktree");
- the user runs `/wt-cleanup` or `/wt cleanup`;
- the user has set a standing preference — in the project's agent instructions
  (CLAUDE.md, AGENTS.md), in memory, or earlier in the conversation ("always
  remove your worktrees when you finish"). Only then may you clean up at the
  end of a task without being asked again.

**What a clean-up covers:** the worktrees *this session* created — nothing
else. Worktrees that existed before the session, belong to other sessions or
tools, bases and the main checkout are out of scope unless the user names
them. If you cannot tell which ones this session created (nothing was created,
or the earlier context is gone), say so, show `worktree list`, and ask.

**How:**

```sh
worktree status -g <name>                              # 1. per candidate: branch, path, uncommitted changes
cd <path> && worktree snap ls --all                    #    …and any snapshots (they die with the worktree)
cd <main-checkout> && worktree remove -b <name>        # 2. one per call, by exact name, from outside it
worktree list                                          # 3. confirm, then report what was removed and what was kept
```

- **Uncommitted changes or snapshots → do not remove.** Report them and ask;
  `-f` discards changes for good, so only on the user's explicit say-so.
- `-b` is a safe delete (`git branch -d`): a branch with unmerged commits is
  kept and reported (`worktree removed, but the branch is kept`) — that is the
  expected outcome, not a failure. Never follow up with `git branch -D`
  unasked.
- A worktree still syncing deps is refused: wait for it, do not force it.
- Glob patterns (`'dev-*'`) and `deps purge` ask y/N on stdin and abort
  without a terminal — remove by exact name instead.
- Removing a worktree never breaks deps linked elsewhere.

## Resetting

```sh
wt reset [base]         # move this worktree's branch back to its base branch's tip
wt reset --hard         # …discarding local tracked changes (untracked kept)
```

The base branch is the one recorded at creation; pass it explicitly when
unrecorded. You stay on your own branch.

## Working from anywhere — `wt global`

Needs `wt link -r` per project. Works outside any repo.

```sh
wt global ls                       # all registered projects: NAME, WORKTREES, PATH
wt global ls <project>             # that project's worktree table
wt global status <project>[/<wt>]  # -g works too
wt global git-log <project>[/<wt>] [git args]
wt global ch <project>[/<wt>]      # jump to the root repo, or into a worktree
```

Project names prefix-match. The registry is `git config --global wt.project.*`;
entries whose repo vanished are pruned automatically.

## Root mode

```sh
wt root status | checkout | create | fork   # run in the root repo's context from anywhere in the project
```

## Customization

```sh
wt alias add cr create -c     # wt cr feature → wt create -c feature
wt alias rm cr
wt alias                      # list; stored as git config wt.alias.*
```

Builtins always win over an alias of the same name. Aliases tab-complete like
the command they expand to, and an alias of a jump command still cd's.

```sh
wt prompt zsh                 # prompt segment: "<project> (<branch>)", colored like wt list
```

Config keys: `wt.name` (project), `wt.root` (worktree base dir),
`wt.copyignored` (default true), `wt.deps` (multi-valued),
`wt.depscopy` (default false — true copies deps instead of hardlinking),
`wt.alias.*`, `wt.project.*` (global registry).

## Troubleshooting

- **"your wt() shell function is outdated"** — re-run `eval "$(worktree init zsh)"`
  or start a new shell; the binary version-checks the function.
- **A jump printed `cd '…'` instead of moving** — the `wt()` function is not
  installed in that shell (or the command was piped/run non-interactively).
- **"this repository is not linked"** — run `wt link <name>`.
- **`wt --version` says `dev`** — the binary was built outside a git checkout;
  build with `make install VERSION=<tag> COMMIT=<sha>`.
