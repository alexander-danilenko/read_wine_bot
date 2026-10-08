#!/usr/bin/env bash
# PostToolUse hook: after Claude edits a .go file in this project, run golangci-lint with --fix on
# that file's package (autofixes plus formatters), then report the findings left and the files it
# rewrote. Exit 2 shows stderr to Claude; PostToolUse cannot block, so this only informs.
# Never lint a single file: golangci-lint then reports false "undefined (typecheck)" errors.
set -euo pipefail
# shellcheck source-path=SCRIPTDIR source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

require_tools jq >&2 || exit 2
file="$(edited_file)"
[[ "$file" == *.go ]] || exit 0
file="$(project_path "$file")" || exit 0
require_tools go golangci-lint >&2 || exit 2

# The package is the file's directory; golangci-lint must run from the module root above it.
pkg_dir="$(dirname "$file")"
module_root="$pkg_dir"
until [[ -f "$module_root/go.mod" ]]; do
	[[ "$module_root" == "$PROJECT_DIR" ]] && exit 0 # not inside a Go module
	module_root="$(dirname "$module_root")"
done

pkg="${pkg_dir#"$module_root"}"
pkg="./${pkg#/}"

before="$(checksums "$pkg_dir"/*.go)"
status=0
output="$(cd "$module_root" && golangci-lint run --fix --allow-serial-runners "$pkg" 2>&1)" || status=$?
rewritten="$(changed_files "$before" "$(checksums "$pkg_dir"/*.go)")"

# Exit 5 and 7 mean there is no Go code to analyze yet, which is not a finding.
[[ $status -eq 5 || $status -eq 7 ]] && status=0
[[ $status -eq 0 && -z "$rewritten" ]] && exit 0

{
	if [[ $status -ne 0 ]]; then
		echo "golangci-lint findings in ${pkg_dir#"$PROJECT_DIR/"}/:"
		echo "$output"
		if [[ "$output" == *"(typecheck)"* ]]; then
			echo "The package does not compile yet, so nothing was autofixed; typecheck findings clear once the edit is complete."
		fi
	fi
	if [[ -n "$rewritten" ]]; then
		echo "The hook rewrote these files; re-read them before editing them again:"
		echo "$rewritten"
	fi
} >&2
exit 2
