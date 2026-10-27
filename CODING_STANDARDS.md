# Coding standards

The rules `/code-review` checks every diff against. `.golangci.yml` enforces what a linter can; these are the rest.

## Tests

- Standard `testing` package only: table-driven, hand-written fakes, no testify or mock generators.

## Style

The linter is the style guide: `.golangci.yml` enables every linter except a short list, each disabled with a reason.

- Fix findings. Never weaken `.golangci.yml` to get a pass.
- `//nolint` only for a confirmed false positive, naming the linter and the reason: `//nolint:gosec // G404: not security-sensitive`.
- Log with `log/slog` and key-value attributes.
- Wrap errors with context: `fmt.Errorf("open db: %w", err)`. Declare sentinel errors as package variables and compare with `errors.Is`. No `panic` outside `main`.
- `context.Context` is the first parameter of every function that does I/O. Never store it in a struct.
- Package names are short, lowercase and singular, and name what the package provides. Never `util`, `common`, `helpers` or `models`.

## Fail fast

A misconfigured bot must not start. Validate first: `main` checks everything it can before the bot handles its first update, and exits with an error that names the problem.

- `internal/config` validates every setting as it loads: required values present, formats and ranges correct. It reports all invalid settings in one error with `errors.Join`, so one run shows every problem.
- Check each external dependency at startup, not on first use: ping the database, call Telegram `getMe` with the token.
- No silent defaults for required settings. A setting gets a default only when that default is safe in production.
- Errors name the setting, never its value, so the bot token stays out of the logs.
- Packages return errors; only `main` logs them with `slog.Error` and calls `os.Exit(1)`.

## SOLID, the Go way

- **Single responsibility:** one package, one reason to change. Telegram handlers translate updates into calls to domain code; they hold no giveaway rules and no queries.
- **Open/closed:** extend with new types and composition, not with boolean flags on existing functions.
- **Liskov substitution:** every implementation of an interface honours the same contract for errors, nil values and context cancellation.
- **Interface segregation:** interfaces have 1-3 methods and are declared in the package that uses them, not the one that implements them.
- **Dependency inversion:** constructors take dependencies as parameters (`NewService(store Store, log *slog.Logger)`). Accept interfaces, return concrete types. Wiring happens only in the root `main` package.
- Do not create an interface for a single implementation unless a test needs a fake.
