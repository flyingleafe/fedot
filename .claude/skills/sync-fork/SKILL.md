# Skill: sync-fork

Sync this fork's `main` branch with upstream `main` while preserving all custom functionality that diverges from upstream.

## When to Use

Use this skill when:
- Upstream `main` has new commits
- The fork's `main` contains custom commits not in upstream
- You want to bring the fork up to date without losing custom work

## Pre-Check

This skill first runs a dry-run to show:
- How many commits upstream is ahead
- Which commits are fork-specific (not in upstream)
- The proposed new HEAD

## Sync Strategy

The cleanest approach for this repo:

```
1. Create temp branch from upstream/main
2. Cherry-pick fork-specific commits onto it  
3. Reset main to the result
```

This preserves the exact commit history and diff of custom work while aligning with upstream.

## Commands

### Dry-Run (Preview Only)

```bash
git fetch upstream
git log --oneline upstream/main..origin/main
```

### Full Sync

```bash
# 1. Ensure clean working tree
git status

# 2. Fetch latest upstream
git fetch upstream

# 3. Create sync branch from upstream main
git checkout -b sync/temp upstream/main

# 4. Cherry-pick fork-specific commits
#    (replace with actual commit range from dry-run)
git cherry-pick <first-commit-hash>..<last-commit-hash>

# 5. Reset main to the sync branch
git checkout main
git reset --hard sync/temp

# 6. Cleanup temp branch
git branch -d sync/temp
```

### Alternative: Rebase (if cleaner)

If fork commits apply cleanly on top of upstream:

```bash
git checkout main
git rebase upstream/main
```

## Safety Checks

- [ ] Working tree is clean before starting
- [ ] Backup branch created: `git branch backup/main-$(date +%Y%m%d)`
- [ ] Verify result: `git log --oneline -10`
- [ ] Test build: `make build`
- [ ] Force push only if explicitly requested

## Rollback

If something goes wrong:

```bash
git checkout main
git reset --hard backup/main-YYYYMMDD
```
