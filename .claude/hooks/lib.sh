# shellcheck shell=bash
# Shared helpers for the hooks in this directory. Source this file; do not run it.

# PROJECT_DIR is the project root with symlinks resolved (macOS /tmp is /private/tmp).
PROJECT_DIR="$(cd "${CLAUDE_PROJECT_DIR:-$PWD}" && pwd -P)"
readonly PROJECT_DIR

# require_tools prints which of the given tools are missing, with the Homebrew command that
# installs them, and fails when any is missing.
require_tools() {
	local missing=() tool
	for tool in "$@"; do
		command -v "$tool" >/dev/null 2>&1 || missing+=("$tool")
	done
	if ((${#missing[@]} > 0)); then
		echo "Missing tool(s): ${missing[*]}. Install with: brew install ${missing[*]}"
		return 1
	fi
}

# edited_file reads the hook input on stdin and prints the path of the file the tool edited.
edited_file() {
	jq -r '.tool_input.file_path // .tool_input.notebook_path // empty'
}

# project_path prints the absolute, symlink-resolved path of an existing file, and fails when
# the file lies outside the project. Hooks fire for every edit in the session, wherever the
# file is, so this keeps the repo's rules off memory files, scratch notes and other repos.
project_path() {
	local dir
	dir="$(cd "$(dirname "$1")" 2>/dev/null && pwd -P)" || return 1
	case "$dir/" in
	"$PROJECT_DIR"/*) printf '%s/%s\n' "$dir" "$(basename "$1")" ;;
	*) return 1 ;;
	esac
}

# checksums prints a checksum line per existing file, so callers can tell which files a tool rewrote.
checksums() {
	local file
	for file in "$@"; do
		[[ -f "$file" ]] && cksum "$file"
	done
	return 0
}

# changed_files prints, relative to the project, the paths whose checksum line differs between
# two checksums outputs.
changed_files() {
	local file
	while IFS= read -r file; do
		[[ -n "$file" ]] && printf '%s\n' "${file#"$PROJECT_DIR/"}"
	done < <(comm -13 <(sort <<<"$1") <(sort <<<"$2") | cut -d' ' -f3-)
	return 0
}
