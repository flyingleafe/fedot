---
name: improve-picoclaw
description: Use for ANY code change to the picoclaw codebase — bug fixes, new features, refactors, provider additions, config changes, CLI changes. Triggers on "fix X", "add support for Y", "implement Z", "can we have", "broken", "doesn't work", or any request that would result in editing a .go file. Always run before writing any code.
---

# Improve PicoClaw

Before touching any code for a new feature request, follow this workflow in order.

## Step 1: Sync with upstream

Check whether the local branch is up-to-date with upstream main (`github.com/sipeed/picoclaw`):

```bash
git fetch upstream main
git log HEAD..upstream/main --oneline
```

If the local branch is behind upstream:
1. Rebase onto upstream main
2. Re-read the user's request — the feature may already exist in the new commits
3. If it does, tell the user and stop

## Step 2: Search open PRs in upstream

Before implementing anything, check whether someone has already done the work:

```bash
gh pr list --repo sipeed/picoclaw --state open --limit 50
```

For each PR that looks relevant by title, read its description and diff:

```bash
gh pr view <number> --repo sipeed/picoclaw
gh pr diff <number> --repo sipeed/picoclaw
```

### If a matching PR exists

1. **Assess quality by reading** — does the implementation approach make sense? Is it clean?
2. **Test it locally** — check out the branch and build:
   ```bash
   git fetch upstream pull/<number>/head:pr-<number>
   git checkout pr-<number>
   make install
   # run the minimal test command relevant to the feature
   ```
3. **Decide**:
   - If the PR fully satisfies the user's request with no or minor changes needed → skip to step 4
   - If it needs **limited modifications** to satisfy the request → make those changes, then open a second-order PR *towards the author's fork branch* (not upstream main), and comment on the original PR explaining the rationale and linking your PR
   - If the implementation is fundamentally wrong or incomplete → implement from scratch (Step 3)
4. **React to the original PR** with a thumbs-up regardless:
   ```bash
   gh api repos/sipeed/picoclaw/pulls/<number>/reactions \
     -X POST -f content='+1'
   ```

## Step 3: Implement from scratch (if no matching PR)

If no relevant PR exists:

1. Create a dedicated feature branch from the current upstream-synced trunk:
   ```bash
   git checkout -b feat/<short-name> upstream/main
   ```
2. Implement the minimum changes needed — no scope creep
3. Build and test with the minimal test command
4. Open a PR from this branch towards `sipeed/picoclaw` main:
   ```bash
   gh pr create --repo sipeed/picoclaw --title "..." --body "..."
   ```
   Keep the PR focused — only commits related to this feature on top of upstream trunk.

## Notes

- `git` and `gh` (GitHub CLI) are required
- Always test before opening any PR — use the minimal command that exercises the new functionality
- When opening a second-order PR to a fork, target the author's feature branch, not their main
