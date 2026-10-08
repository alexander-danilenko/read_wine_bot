#!/usr/bin/env bash
# Stop hook: before Claude ends a turn, lint the whole Go module. Findings block the stop (exit 2,
# reason shown to Claude) at most MAX_BLOCKS times in a row; after that the stop is allowed and the
# findings go to the owner instead, so an unfixable finding never traps Claude in a loop. The
# counter resets whenever a stop is allowed, so every turn gets the full budget.
set -euo pipefail
# shellcheck source-path=SCRIPTDIR source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

readonly MAX_BLOCKS=3

input="$(cat)"
session="no-session"
repo_root="$PROJECT_DIR"
if command -v jq >/dev/null 2>&1; then
	session="$(jq -r '.session_id // "no-session"' <<<"$input")"
	cwd="$(jq -r '.cwd // empty' <<<"$input")"
	# Follow the session into a worktree: lint the repository the session is working in.
	repo_root="$(git -C "${cwd:-$PROJECT_DIR}" rev-parse --show-toplevel 2>/dev/null || echo "$PROJECT_DIR")"
fi
counter="${TMPDIR:-/tmp}/claude-go-lint-gate-${session//[^A-Za-z0-9._-]/_}"

# No Go module yet (e.g. while bootstrapping): nothing to lint.
if [[ ! -f "$repo_root/go.mod" ]]; then
	rm -f "$counter"
	exit 0
fi

status=0
if ! output="$(require_tools jq go golangci-lint)"; then
	status=1
else
	# --allow-serial-runners waits for golangci-lint's global lock (held by another session, a
	# subagent's per-edit run or the IDE) instead of failing, which would burn a block.
	output="$(cd "$repo_root" && golangci-lint run --allow-serial-runners ./... 2>&1)" || status=$?
	# Exit 5 means the module has no Go files yet, which is not a finding. Exit 7 is not
	# excluded: it means golangci-lint logged an error, such as failing to load the module.
	[[ $status -eq 5 ]] && status=0
fi

if [[ $status -eq 0 ]]; then
	rm -f "$counter"
	exit 0
fi

blocks="$(cat "$counter" 2>/dev/null || echo 0)"
if ((blocks < MAX_BLOCKS)); then
	echo $((blocks + 1)) >"$counter"
	{
		echo "The end-of-turn lint gate failed. Fix these before ending the turn:"
		echo "$output"
	} >&2
	exit 2
fi

rm -f "$counter"
message="Lint gate gave up after ${MAX_BLOCKS} blocked stops; these findings remain:
${output}"
if command -v jq >/dev/null 2>&1; then
	jq -n --arg message "$message" '{systemMessage: $message}'
else
	echo '{"systemMessage": "Lint gate gave up: jq is not installed. Install with: brew install jq"}'
fi
exit 0
