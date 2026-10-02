---
name: wt-cleanup
description: Remove the wt worktrees created in the current session. Run only when the user explicitly invokes /wt-cleanup or asks to clean up worktrees — never on the agent's own initiative, and never at the end of a task unless the user has set a standing preference for it.
argument-hint: "[worktree-name …]"
disable-model-invocation: true
---

# wt-cleanup — remove this session's worktrees

Removes the worktrees **this session** created with wt (`worktree create` /
`worktree fork`). The binary is `worktree`; `wt` is its shell function and may
not exist in a non-interactive shell.

## When this may run

Only on the user's request: they typed `/wt-cleanup`, asked for a clean-up in
plain words, or set a standing preference (project agent instructions such as
CLAUDE.md or AGENTS.md, memory, or an instruction earlier in the conversation
like "always remove your worktrees when you finish").

Never run it because a task ended, a branch was merged, or disk is tight. If
you got here without one of the triggers above, stop and ask.

## Scope

- **No arguments** — every worktree this session created, and nothing else.
- **Names given** (`/wt-cleanup api-fix docs-2`) — exactly those worktrees,
  whether or not this session created them.
- **Never in scope unless named:** worktrees that existed before the session
  or belong to other sessions or tools. **Never at all:** the main checkout
  and base worktrees (wt refuses them; `worktree base rm` is a separate,
  explicit request).

If you cannot tell which worktrees this session created — none were, or the
earlier context is gone — do not guess. Say so, show `worktree list`, and ask
which to remove.

## Steps

**1. Inspect each candidate.**

```sh
worktree list                              # does it still exist?
worktree status -g <name>                  # branch, path, uncommitted changes
cd <path> && worktree snap ls --all        # snapshots, if any — they die with the worktree
```

**2. Sort them.**

- Clean, no snapshots → remove.
- Uncommitted changes or snapshots → **do not remove.** List them with what
  would be lost and ask the user what to do.
- Deps still syncing (`worktree list` shows `syncing`) → wait, do not force.

**3. Remove, one per call, by exact name, from outside the worktree** (wt
refuses to remove the one you stand in):

```sh
cd <main-checkout> && worktree remove -b <name>
```

- `-b` also deletes the branch, safely (`git branch -d`). A branch with
  unmerged commits is kept and wt says `worktree removed, but the branch is
  kept` — that is the expected outcome, not a failure. Never follow up with
  `git branch -D` unasked.
- `-f` discards uncommitted changes for good: only when the user explicitly
  said to discard them.
- No glob patterns (`'dev-*'`): they ask y/N on stdin and abort without a
  terminal.

**4. Confirm and report.**

```sh
worktree list
```

Tell the user what was removed, what was kept and why (dirty, snapshots,
syncing), and which branches still exist.
