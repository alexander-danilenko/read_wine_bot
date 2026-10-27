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
| `actionlint` | Lint `.github/workflows`; CI requires every third-party `uses:` pinned to a commit SHA; GitHub's `actions/*` stay on their latest major tag |

Tools: `brew install go golangci-lint markdownlint-cli2 jq actionlint`. The lint config was tested with golangci-lint 2.13; after a `brew upgrade`, new linters that `default: all` turns on may report new findings.

Config comes only from environment variables and `.env` files (the bot ships as a Docker container): copy `.env.example` to `.env`. `internal/config` reads `.env` in the working directory and in every parent directory up to `/`, then validates the result (Fail fast in [CODING_STANDARDS.md](CODING_STANDARDS.md)). A nearer `.env` overrides the same key from a parent's, a non-empty environment variable overrides every file, and a malformed line in any of them stops the bot. In Docker, pass settings with `docker run --env-file .env` or Compose `env_file:`.

Lint a package directory, never a single `.go` file: `golangci-lint run file.go` treats the file as its own package and reports false `undefined (typecheck)` errors.

## Layout

Follows go.dev's "Organizing a Go module" for a single command: the main package sits in the repo root. No `cmd/`, no `pkg/`, no empty placeholder directories.

```text
*.go              # main package in the repo root: wiring only (config, logger, DB, bot, shutdown), no business logic
internal/<name>/  # every other package, one responsibility each
docs/channel-analytics.md  # ready SQL over the channel audience log
docs/telegram-ux.md        # rules for everything the user sees in Telegram
docs/hand-check-store-sql.md  # how to check a Store query against the local database
docs/schema-challenge.md      # checks every schema or Store change must pass first
docs/adr/         # architecture decisions, one file each: NNNN-<slug>.md; read the ones that touch the area you change
docs/agents/      # how agent skills use the issue tracker and domain docs
GLOSSARY.md       # domain glossary; use its terms in code, identifiers and docs
CODING_STANDARDS.md  # Go style, fail-fast and SOLID rules that /code-review enforces
```

## Dependencies: standard library first

- Solve it with the standard library first (`net/http`, `encoding/json`, `log/slog`, `context`, `errors`, `slices`, `maps`, `testing`).
- Approved modules: `gorm.io/gorm`, `gorm.io/driver/postgres`, `github.com/go-telegram/bot`. Add nothing else without the owner's approval.
- To propose a module, state what the standard library lacks, the module's transitive dependency count and its last release date.
- Keep `CGO_ENABLED=0` builds working: the Docker image is static.
- Tests never open a database, real or embedded: test the code above a store with in-memory fakes of its interfaces.

## Database

- PostgreSQL only, through GORM with `AutoMigrate` from Go structs. The database runs outside this repo; `DATABASE_URL` (`postgres://user:password@host:port/database`) is the only setting the bot needs. For local work, `docker compose up -d` starts PostgreSQL 18 with `POSTGRES_USER`, `POSTGRES_PASSWORD` and `POSTGRES_DB` from `.env`, which the image applies on the first start of an empty volume.
- Backups belong to the hosting (the owner's Dokploy), not to this repo. Do not add a backup service, script or schedule here.
- Describe every column with a `comment:` in its `gorm` tag, and every table and named index in `descriptions()` in `internal/db/db.go`, so database tools show what each holds. `TestEverythingDescribed` enforces it.
- `AutoMigrate` never drops or renames columns. Ask the owner before any destructive schema change.
- Store Telegram user and chat IDs as `int64`.
- Before writing a GORM model, a `Store` query or anything else that changes the shape of data, run the challenge protocol in [docs/schema-challenge.md](docs/schema-challenge.md).

## Coding standards

[CODING_STANDARDS.md](CODING_STANDARDS.md) holds the Go style, fail-fast and SOLID rules. Read it before writing Go; `/code-review` checks every diff against it.

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

The owner drives work with Matt Pocock's engineering skills, one step at a time.

- **Issue tracker:** GitHub issues in `alexander-danilenko/read_wine_bot`, on the Read Wine Bot project (see Issue tracker above). See `docs/agents/issue-tracker.md`.
- **Domain docs:** single-context, `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.
