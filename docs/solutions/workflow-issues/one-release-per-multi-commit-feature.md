---
title: Ship a multi-commit feature as one release under the version-bump hook
date: 2026-11-16
category: workflow-issues
module: release workflow
problem_type: workflow_issue
component: development_workflow
severity: medium
applies_when:
  - "A feature is built in several commits, for example one per plan unit"
  - "More than one of those commits changes what ships (non-test Go, go.mod, go.sum, i18n locales, web/, Dockerfile)"
retire_when: "the version-bump hook in .claude/hooks/version-bump-gate.sh stops checking every commit, for example by checking only the branch tip; read the hook to confirm"
tags: [version-bump, release, git-amend, rebase, hooks]
---

# Ship a multi-commit feature as one release under the version-bump hook

## Context

`.claude/hooks/version-bump-gate.sh` blocks any `git commit` whose staged diff changes what ships unless `main.go`'s `const version` changes in that same commit. It compares against `HEAD`, or `HEAD~1` for `--amend`. Every version that reaches `main` is a release. Building a feature unit by unit, with one commit per unit, therefore forces one bump per unit. A three-unit `feat` would ship as three minor versions instead of one release.

## Guidance

Keep all code that ships in **one** commit that carries the single bump, and fold each later unit into it with `git commit --amend`. Put commits that change nothing shipped, such as docs, ADRs and the glossary, in separate commits: the hook lets them through without a bump.

While the feature commit is `HEAD`, amend it directly:

```bash
git add <unit files>
git commit --amend -F <message-file>   # hook compares to HEAD~1, which has no bump yet, so it passes
```

Once other commits sit on top, such as a docs commit or a commit of the owner's, a `fixup!` commit is blocked, because it changes shipped files without a bump. Fold the change in with a non-interactive edit stop instead:

```bash
git stash push -u -m unit-fixes                                    # only this run's changes
GIT_SEQUENCE_EDITOR="sed -i '' '1s/^pick/edit/'" git rebase -i origin/main  # stop at the feature commit (first on the branch)
git stash pop && git add -u                                        # or name the paths
git commit --amend -F <message-file>
git rebase --continue                                              # replays the docs/other commits on top
```

`sed -i ''` is the BSD/macOS form. On GNU sed, use `sed -i`. The rebase gives the commits above new SHAs, so do it only before anything is pushed.

## Why This Matters

Without this, an agent either bumps the version once per unit, which publishes several releases for one feature, or tries to get around the hook. `CLAUDE.md` forbids working around it.

## When to Apply

- Agent-driven builds that commit per unit, as `/ce-work` and `/lfg` do, on a branch the owner ships by merging to `main`.
- Skip it when the feature fits in one commit anyway.

## Examples

Issue #50 (primary channel and roles) shipped as `feat!:` at 3.0.0 in one commit holding seven units, plus a separate `docs:` commit. Units 2–6 were amended in. Unit 6 and the review fixes were folded in with the edit-stop rebase after a docs commit and an owner `chore` commit had landed on top.

Two related slips from the same run:

- In zsh, `git add $files` does not split a newline-separated variable into words, so git sees a single path that does not exist and nothing gets staged. Use `git add --pathspec-from-file=<file>` or `git add -u`. Then check `git diff --cached --name-only` before amending, because an amend with nothing staged silently rewrites the commit unchanged.
- Path-limited commits (`git commit -- <paths>`) leave other staged files, such as the owner's work in progress, out of the commit and still staged.

## Related

- `CLAUDE.md`, the Version section
- `.claude/hooks/version-bump-gate.sh`
