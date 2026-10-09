package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// ErrInvalidDotenv means a .env file has a line that is not a comment, a blank line or KEY=VALUE.
var ErrInvalidDotenv = errors.New("invalid .env line, want KEY=VALUE")

// envKeyPattern matches a name a shell can export.
var envKeyPattern = regexp.MustCompile(`^[A-Za-z_]\w*$`)

// readDotenvChain reads .env in dir and in every parent directory up to the filesystem root,
// and merges them. A file nearer to dir overrides the same key from a file further up.
// A directory without .env is skipped.
func readDotenvChain(ctx context.Context, dir string) (map[string]string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", dir, err)
	}

	var dirs []string
	for {
		dirs = append(dirs, dir)

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	vars := make(map[string]string)
	// Root first, so nearer files overwrite the keys they share with files further up.
	for _, d := range slices.Backward(dirs) {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("read .env files: %w", err)
		}

		path := filepath.Join(d, ".env")

		data, err := fs.ReadFile(os.DirFS(d), ".env")
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}

		if err := parseDotenv(path, string(data), vars); err != nil {
			return nil, err
		}
	}

	return vars, nil
}

// parseDotenv adds the KEY=VALUE lines of a .env file to vars. It skips blank lines and # comments,
// allows an `export ` prefix and strips one pair of matching quotes around a value. Errors name
// the file and line, never the line's text, so a malformed secret stays out of the logs.
//
// ponytail: minimal dotenv subset; no escapes, multiline values, inline comments or ${VAR}
// expansion. Add them when a real .env needs one.
func parseDotenv(path, data string, vars map[string]string) error {
	for i, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")

		key = strings.TrimSpace(key)
		if !ok || !envKeyPattern.MatchString(key) {
			return fmt.Errorf("%s:%d: %w", path, i+1, ErrInvalidDotenv)
		}

		vars[key] = unquote(strings.TrimSpace(value))
	}

	return nil
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}

	return s
}
