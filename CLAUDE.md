# read_wine_bot

General-purpose Telegram bot for the @read_wine channel. Planned features: giveaways with posted results, and spam detection in the chat; more may follow. Go 1.27, single binary.

The owner reads code well but is not a Go developer. When a change relies on a non-obvious Go idiom, name it in one line in your summary.

## Issue tracker

The private GitHub Project [Read Wine Bot](https://github.com/users/alexander-danilenko/projects/2) (#2, linked to this repo) is the primary tracker. File work as issues in `alexander-danilenko/read_wine_bot` and add them to it: `gh issue create ... --project "Read Wine Bot"`. Statuses: Backlog, Ready, In progress, In review, Done.

Use issue references without being asked:

- Before starting work, find its issue (`gh issue list --search`), or propose one. Check its blockers first: work on an issue whose blockers are still open only after the owner agrees.
- Record every dependency as a native "blocked by" link, never only in prose: `gh api -X POST repos/alexander-danilenko/read_wine_bot/issues/<N>/dependencies/blocked_by -F issue_id=<blocker's id from gh api repos/.../issues/<M> --jq .id>`. When you file several related issues, set the links between them in the same step. When work turns out to depend on something new, file that issue and link it.
- Reference the issue in plans (frontmatter or first line), in commit message footers (`Refs #N`, or `Closes #N` on the commit that finishes it), and in PR bodies.
- Move the issue's project status as the work moves: In progress when you start, In review when a review or PR is open, Done when it ships. `gh project item-edit 2 --owner alexander-danilenko --url <issue URL> --field Status --value "In progress"`.

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
| `actionlint` | Lint `.github/workflows`; CI also requires every `uses:` pinned to a commit SHA |

Tools: `brew install go golangci-lint markdownlint-cli2 jq actionlint`. The lint config was tested with golangci-lint 2.13; after a `brew upgrade`, new linters that `default: all` turns on may report new findings.

Config comes only from environment variables and `.env` files (the bot ships as a Docker container): copy `.env.example` to `.env`. `internal/config` reads `.env` in the working directory and in every parent directory up to `/`, then validates the result, see Fail fast. A nearer `.env` overrides the same key from a parent's, a non-empty environment variable overrides every file, and a malformed line in any of them stops the bot. In Docker, pass settings with `docker run --env-file .env` or Compose `env_file:`.

Lint a package directory, never a single `.go` file: `golangci-lint run file.go` treats the file as its own package and reports false `undefined (typecheck)` errors.

## Layout

Follows go.dev's "Organizing a Go module" for a single command: the main package sits in the repo root. No `cmd/`, no `pkg/`, no empty placeholder directories.

```text
*.go              # main package in the repo root: wiring only (config, logger, DB, bot, shutdown), no business logic
internal/<name>/  # every other package, one responsibility each
docs/channel-analytics.md  # ready SQL over the channel audience log
docs/telegram-ux.md        # rules for everything the user sees in Telegram
docs/hand-check-store-sql.md  # how to check a Store query against the local database
docs/adr/         # architecture decisions, one file each: NNNN-<slug>.md; read the ones that touch the area you change
docs/agents/      # how agent skills use the issue tracker and domain docs
GLOSSARY.md       # domain glossary; use its terms in code, identifiers and docs
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

## Database

- PostgreSQL only, through GORM with `AutoMigrate` from Go structs. The database runs outside this repo; `DATABASE_URL` (`postgres://user:password@host:port/database`) is the only setting the bot needs. For local work, `docker compose up -d` starts PostgreSQL 18 with `POSTGRES_USER`, `POSTGRES_PASSWORD` and `POSTGRES_DB` from `.env`, which the image applies on the first start of an empty volume.
- Backups belong to the hosting (the owner's Dokploy), not to this repo. Do not add a backup service, script or schedule here.
- Describe every column with a `comment:` in its `gorm` tag, and every table and named index in `descriptions()` in `internal/db/db.go`, so database tools show what each holds. `TestEverythingDescribed` enforces it.
- `AutoMigrate` never drops or renames columns. Ask the owner before any destructive schema change.
- Store Telegram user and chat IDs as `int64`.

### Challenge protocol (mandatory before any schema change)

Before writing a GORM model, a `Store` query or anything else that changes the shape of data:

1. State the access patterns it serves: the queries, the write paths and the expected row counts.
2. Check these rules. A rejected change is not implemented as asked.
   - A `jsonb` or array column whose data is filtered, joined, grows without limit or has its own lifecycle -> propose a table.
   - A relationship without a foreign key ("we validate in the app") -> reject. In GORM, declare it with an association field, as `Giveaway.Creator` does.
   - A uniqueness rule or invariant enforced only in Go -> propose a unique index or a `CHECK` constraint.
   - A nullable column whose `comment:` does not say what NULL means -> flag.
   - A polymorphic association (`entity_type` + `entity_id`) -> reject, propose separate foreign keys or a supertype table.
   - An EAV table (key/value rows for attributes) -> reject, unless users really define the attributes.
   - Money in `float`/`real` -> reject, use `numeric` or an integer of minor units.
   - Event time as `timestamp` without time zone -> reject, use `timestamptz` (GORM's default for `time.Time`).
   - A copied (denormalized) column without a named sync mechanism (same transaction, trigger) -> reject.
   - A query that loads rows and filters them in Go when a `WHERE` or an index can do it -> reject, propose the index or a new shape.
   - A row updated more than about once per second, sustained -> reject (row-lock contention), propose insert-only rows aggregated on read.
   - One write that fans out to many rows with no cap on how many -> flag.
   - A change that makes `AutoMigrate` rewrite or lock a large table (type change, new index on a big table: `AutoMigrate` cannot `CREATE INDEX CONCURRENTLY`) -> require an expand/contract plan.
   - A new `CHECK` or unique constraint -> first query production for rows that break it and fix them, or `AutoMigrate` cannot add the constraint and the bot fails at startup. Mirror the constraint in the store's in-memory fake, or tests pass on data the database rejects.
3. On a violation, reply with the rule, the concrete failure in production and the alternative. Implement only after the owner explicitly picks one option.
4. Every accepted rule break gets an ADR in `docs/adr/` (`NNNN-<slug>.md`: context, decision, consequences).

## Telegram UX

Before changing a handler, a button, a command or a text the user sees, read [docs/telegram-ux.md](docs/telegram-ux.md): the rules that keep the chat clean and every button working.

## Gotchas

- Store SQL has no automated tests. After changing a query in a `Store`, run it against the local database with a throwaway program: [docs/hand-check-store-sql.md](docs/hand-check-store-sql.md).
- CI (`.github/workflows/check.yml`) runs build, tests, `golangci-lint` and markdownlint on every push. Hooks (`.claude/hooks/`, loaded when a session starts): after every `.go` edit, `go-lint-fix.sh` formats and autofixes that package and shows remaining findings. Before your turn ends, `go-lint-gate.sh` runs the full lint and blocks the stop until it passes, at most three times in a row before it hands the findings to the owner. Fix the finding; do not work around the hook.
- Write an import and its first use in the same edit. An unused import stops the package compiling, and the per-edit hook cannot autofix a package that does not compile.
- Markdown: one line per paragraph (no hard wrapping) and `-` bullets. `markdownlint-fix.sh` runs `markdownlint-cli2 --fix` on every edited `.md` file.

## Commits

Every commit follows [Conventional Commits 1.0](https://www.conventionalcommits.org/en/v1.0.0/): `<type>(<optional scope>): <imperative summary>`.

- Types: `feat`, `fix`, `refactor`, `perf`, `test`, `docs`, `build`, `ci`, `chore`, `style`, `revert`.
- Scope is the package or area touched, e.g. `feat(giveaway): pick winners at random`.
- Mark a breaking change with `!` after the type or a `BREAKING CHANGE:` footer.
- Never open a pull request without the owner's explicit permission in the current conversation. This includes every skill that ends in a PR: stop after the local commits, report, and ask. A skill's default shipping step is not permission.

## Version

`const version` in `main.go` holds the bot's [semver](https://semver.org) version. The startup log (`bot started version=…`) and the `/start` reply show it, so the owner can check which build is deployed.

- Every commit that changes what ships bumps it, in that same commit: non-test `.go` files, `go.mod`, `go.sum`, the files `internal/i18n` embeds, or the `Dockerfile`. Any other commit leaves it alone, since each bump publishes a release.
- `feat` -> minor (`0.1.1` -> `0.2.0`), patch reset to 0. Every other type -> patch (`0.1.0` -> `0.1.1`). A breaking change (`!`) -> major once at `1.0.0` or later; before `1.0.0` it bumps minor.
- The `version-bump-gate.sh` hook (PreToolUse on Bash) blocks a `git commit` that breaks this rule. Bump, stage `main.go` and commit again; do not work around the hook.
- A bump reaching `main` is a release: once `check` passes, `.github/workflows/release.yml` tags `v<version>` and GoReleaser (`.goreleaser.yaml`) publishes a GitHub Release with binaries and a changelog from the Conventional Commit types. Never tag by hand. Try the config locally with `goreleaser release --snapshot --clean --skip=publish`.

## Agent skills

The owner drives work with Matt Pocock's engineering skills, one step at a time; no skill chains into the next on its own. Compound Engineering (`ce-*`, `/lfg`) is retired in this repo.

- **Issue tracker:** GitHub issues in `alexander-danilenko/read_wine_bot`, on the Read Wine Bot project (see Issue tracker above). See `docs/agents/issue-tracker.md`.
- **Domain docs:** single-context, `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.
