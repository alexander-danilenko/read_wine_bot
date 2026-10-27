#!/usr/bin/env bash
# PreToolUse hook on Bash: blocks a `git commit` that changes the bot but not `const version` in
# main.go (exit 2, reason shown to Claude). The bot is what ships: non-test Go files, go.mod,
# go.sum, the files internal/i18n embeds, and the Dockerfile. Every bump on main is a GitHub
# Release, so a commit of anything else passes without one. See "Version" in CLAUDE.md.
set -euo pipefail

input="$(cat)"
# Runs before every Bash call, and most are not commits: leave before starting any program.
[[ $input == *commit* ]] || exit 0

# shellcheck source-path=SCRIPTDIR source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
require_tools jq rg >&2 || exit 0

command="$(jq -r '.tool_input.command // empty' <<<"$input")"
rg -q '\bgit\b[^;&|]*\bcommit\b' <<<"$command" || exit 0

cwd="$(jq -r '.cwd // empty' <<<"$input")"
repo_root="$(git -C "${cwd:-$PROJECT_DIR}" rev-parse --show-toplevel 2>/dev/null)" || exit 0
[[ -f "$repo_root/main.go" ]] || exit 0

# --amend replaces HEAD, so the commit is compared with its parent. -a stages tracked files at
# commit time, so the working tree is compared instead of the index.
base=HEAD
rg -q '\s--amend\b' <<<"$command" && base=HEAD~1
diff=(git -C "$repo_root" diff --cached "$base")
rg -q '\scommit\b.*\s(-[a-zA-Z]*a|--all)\b' <<<"$command" && diff=(git -C "$repo_root" diff "$base")

# Diffs are captured, not piped into rg: rg -q stops reading at the first match, and under
# pipefail the SIGPIPE that git then gets would fail the test.
files="$("${diff[@]}" --name-only 2>/dev/null)" || exit 0
shipped="$(rg -v '_test\.go$' <<<"$files" | rg '(\.go$|^go\.(mod|sum)$|^Dockerfile$|^internal/i18n/(schema|locales/[^/]+)\.json$)' || true)"
[[ -n $shipped ]] || exit 0
rg -qx 'main.go' <<<"$files" && rg -q '^\+const version = ' <<<"$("${diff[@]}" -- main.go)" && exit 0

current="$(rg -o --no-filename 'const version = "[^"]*"' "$repo_root/main.go" || echo unknown)"
echo "This commit changes the bot but not the version (${current} in main.go). Bump it per \"Version\" in CLAUDE.md (feat: minor, everything else: patch), stage main.go, then commit again. Stage it in a separate Bash call before the commit: this hook reads the index before your command runs." >&2
exit 2
