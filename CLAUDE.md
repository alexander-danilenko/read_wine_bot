# read_wine_bot

General-purpose Telegram bot for the @read_wine channel. Planned features: giveaways with posted results, and spam detection in the chat; more may follow. Go 1.27, single binary.

The owner reads code well but is not a Go developer. When a change relies on a non-obvious Go idiom, name it in one line in your summary.

## Commands

| Command | Purpose |
| --- | --- |
| `go run .` | Run the bot |
| `go build .` | Build the bot binary `./read_wine_bot` |
| `go build ./...` | Compile everything |
| `go test ./...` | Run all tests |
| `golangci-lint run ./...` | Full lint; must report 0 issues |
| `golangci-lint run --fix ./internal/<pkg>/` | Autofix and format one package |
| `go mod tidy` | Sync `go.mod` and `go.sum` after changing imports |
| `markdownlint-cli2 --fix "**/*.md"` | Format all Markdown |

Tools: `brew install go golangci-lint markdownlint-cli2 jq`. The lint config was tested with golangci-lint 2.13; after a `brew upgrade`, new linters that `default: all` turns on may report new findings.

Config comes only from environment variables and `.env` files (the bot ships as a Docker container): copy `.env.example` to `.env`. `internal/config` reads `.env` in the working directory and in every parent directory up to `/`, then validates the result, see Fail fast. A nearer `.env` overrides the same key from a parent's, a non-empty environment variable overrides every file, and a malformed line in any of them stops the bot. In Docker, pass settings with `docker run --env-file .env` or Compose `env_file:`.

Lint a package directory, never a single `.go` file: `golangci-lint run file.go` treats the file as its own package and reports false `undefined (typecheck)` errors.

## Layout

Follows go.dev's "Organizing a Go module" for a single command: the main package sits in the repo root. No `cmd/`, no `pkg/`, no empty placeholder directories.

```text
*.go              # main package in the repo root: wiring only (config, logger, DB, bot, shutdown), no business logic
internal/<name>/  # every other package, one responsibility each
docs/plans/       # Compound Engineering plans
docs/solutions/   # documented solutions to past problems (bugs, best practices, workflow patterns), organized by category with YAML frontmatter (module, tags, problem_type)
CONCEPTS.md       # domain glossary; use its terms in code, identifiers and docs
```

## Dependencies: standard library first

- Solve it with the standard library first (`net/http`, `encoding/json`, `log/slog`, `context`, `errors`, `slices`, `maps`, `testing`).
- Approved modules: `gorm.io/gorm`, `gorm.io/driver/postgres`, `github.com/go-telegram/bot`. Add nothing else without the owner's approval.
- To propose a module, state what the standard library lacks, the module's transitive dependency count and its last release date.
- Keep `CGO_ENABLED=0` builds working: the Docker image is static.
- Tests use the standard `testing` package only: table-driven, hand-written fakes, no testify or mock generators. Tests never open a database, real or embedded: test the code above a store with in-memory fakes of its interfaces.

## Code style

The linter is the style guide: `.golangci.yml` enables every linter except a short list, each disabled with a reason.

- Fix findings. Never weaken `.golangci.yml` to get a pass.
- `//nolint` only for a confirmed false positive, naming the linter and the reason: `//nolint:gosec // G404: not security-sensitive`.
- Log with `log/slog` and key-value attributes. No `fmt.Print*` or `log.Print*`.
- Wrap errors with context: `fmt.Errorf("open db: %w", err)`. Declare sentinel errors as package variables and compare with `errors.Is`. No `panic` outside `main`.
- `context.Context` is the first parameter of every function that does I/O. Never store it in a struct.
- No package-level mutable state and no `init()`.
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

## Database

- PostgreSQL only, through GORM with `AutoMigrate` from Go structs. The database runs outside this repo; `DATABASE_URL` (`postgres://user:password@host:port/database`) is the only setting the bot needs. For local work, `docker compose up -d` starts PostgreSQL 18 with the superuser password, database, user and password from `.env`; `initdb/` creates the bot's user and database on the first start of an empty volume.
- `AutoMigrate` never drops or renames columns. Ask the owner before any destructive schema change.
- Store Telegram user and chat IDs as `int64`.

## Gotchas

- Store SQL has no automated tests. After changing a query in a `Store`, run the bot against a real `DATABASE_URL` and exercise the flow by hand.
- Hooks (`.claude/hooks/`, loaded when a session starts): after every `.go` edit, `go-lint-fix.sh` formats and autofixes that package and shows remaining findings. Before your turn ends, `go-lint-gate.sh` runs the full lint and blocks the stop until it passes, at most three times in a row before it hands the findings to the owner. Fix the finding; do not work around the hook.
- Write an import and its first use in the same edit. An unused import stops the package compiling, and the per-edit hook cannot autofix a package that does not compile.
- Markdown: one line per paragraph (no hard wrapping) and `-` bullets. `markdownlint-fix.sh` runs `markdownlint-cli2 --fix` on every edited `.md` file.

## Commits

Every commit follows [Conventional Commits 1.0](https://www.conventionalcommits.org/en/v1.0.0/): `<type>(<optional scope>): <imperative summary>`.

- Types: `feat`, `fix`, `refactor`, `perf`, `test`, `docs`, `build`, `ci`, `chore`, `style`, `revert`.
- Scope is the package or area touched, e.g. `feat(giveaway): pick winners at random`.
- Mark a breaking change with `!` after the type or a `BREAKING CHANGE:` footer.

## Compound Engineering

After a solved, verified problem, offer once to invoke the `ce-compound` skill at the completion checkpoint only when the work produced durable project reasoning that is not readily recoverable from the final code, tests, types, comments, or existing documentation, and losing it would plausibly cause recurrence, material risk, or substantial rediscovery. Apply this counterfactual: if the learning document disappeared, would a future engineer reading the final implementation still be likely to repeat the mistake or redo substantial investigation? If not, do not offer. Completion, effort, and diff size alone are not enough. Offer at the checkpoint so a qualifying learning can ship in the PR that produced it, and only where the repository treats captured learnings as tracked, committed knowledge.
