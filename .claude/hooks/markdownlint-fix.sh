#!/usr/bin/env bash
# PostToolUse hook: after Claude edits a .md file in this project, run markdownlint-cli2 --fix on
# that one file from the project root, so .markdownlint-cli2.yaml (and the .gitignore'd paths it
# skips) applies. Reports findings it could not fix and tells Claude when the file was rewritten.
# Exit 2 shows stderr to Claude; PostToolUse cannot block, so this only informs.
set -euo pipefail
# shellcheck source-path=SCRIPTDIR source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

require_tools jq >&2 || exit 2
file="$(edited_file)"
[[ "$file" == *.md ]] || exit 0
file="$(project_path "$file")" || exit 0
require_tools markdownlint-cli2 >&2 || exit 2

before="$(checksums "$file")"
status=0
output="$(cd "$PROJECT_DIR" && markdownlint-cli2 --fix "${file#"$PROJECT_DIR/"}" 2>&1)" || status=$?
rewritten="$(changed_files "$before" "$(checksums "$file")")"

[[ $status -eq 0 && -z "$rewritten" ]] && exit 0

{
	if [[ $status -ne 0 ]]; then
		echo "markdownlint findings it could not fix:"
		echo "$output" | grep -v -e '^markdownlint-cli2 v' -e '^Finding:' -e '^Linting:'
	fi
	if [[ -n "$rewritten" ]]; then
		echo "The hook reformatted this file; re-read it before editing it again: ${file#"$PROJECT_DIR/"}"
	fi
} >&2
exit 2
