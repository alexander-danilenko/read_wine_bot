---
title: SQLite to PostgreSQL Migration - Plan
type: refactor
date: 2026-10-18
topic: postgres-migration
artifact_contract: ce-unified-plan/v1
product_contract_source: ce-plan-bootstrap
execution: code
---

# SQLite to PostgreSQL Migration - Plan

## Goal Capsule

- **Objective:** The bot stores users, revisions, giveaways and participants in an externally managed PostgreSQL database, configured by one `DATABASE_URL` setting, and no SQLite code or dependency remains.
- **Product authority:** the owner, in the 2026-10-18 `/lfg` request.
- **Means:** swap the GORM dialector to `gorm.io/driver/postgres`, replace `DB_PATH` with `DATABASE_URL`, remove SQLite-only workarounds, move the Service, Editor and handler tests onto hand-written in-memory fakes, and delete the SQL-only test files (KTD1-KTD4).
- **Open blockers:** none.
- **Stop conditions:** stop and report if `gorm.io/driver/postgres` cannot be imported without CGO, or if a store query needs SQL that GORM cannot express for Postgres without raw SQL the owner has not seen.
- **Execution:** `ce-work` implements unit by unit with a commit per unit on branch `refactor/postgres`.

---

## Product Contract

### Summary

Replace SQLite with PostgreSQL. The database runs outside this repo; the bot only receives its URL. Tests never open a database.

### Problem Frame

The owner decided to move the bot's data to a PostgreSQL server that is provisioned and configured elsewhere. The bot today opens a SQLite file named by `DB_PATH` and carries SQLite-specific workarounds (pragmas, a one-connection pool, write-first transactions, UTC text comparisons, `rowid` ordering). Many tests open a real SQLite file. After the switch, the bot must start only when it can reach Postgres and create its tables there, and the test suite must stay fully isolated: no SQLite, no Postgres, no network.

### Key Decisions

- KD1. **One `DATABASE_URL` setting.** (session-settled: user-directed — chosen over separate host/port/user/password settings or a Postgres service in this repo: the database is configured elsewhere.) Governs R1, R2.
- KD2. **Postgres only.** (session-settled: user-directed — chosen over supporting both SQLite and Postgres: the owner asked for a full migration.) Governs R3, R4.
- KD3. **Unit tests only.** (session-settled: user-directed — chosen over integration tests against a real Postgres: tests must stay isolated and never touch a database.) Governs R5, R6.

### Requirements

#### Configuration

- R1. `DATABASE_URL` replaces `DB_PATH`. It is required; a missing value or a value that is not a `postgres://` or `postgresql://` URL with a host stops the bot with an error that names `DATABASE_URL` and never prints its value.
- R2. `.env.example`, `CLAUDE.md` and the Dockerfile comment describe `DATABASE_URL` and no longer mention SQLite.

#### Database

- R3. `internal/db` opens Postgres from the URL, pings it, and runs `AutoMigrate` for the same four models at startup; any failure stops the bot.
- R4. No SQLite code, pragma, connection-pool workaround or module (`github.com/glebarez/sqlite`, `github.com/glebarez/go-sqlite`, `modernc.org/*`) remains in the code or `go.mod`.

#### Tests

- R5. `go test ./...` passes with no database and no network: no test opens a GORM connection.
- R6. The Service, Editor, setup and handler rules keep their behaviour tests, now running on hand-written in-memory fakes.

### Scope Boundaries

- No data migration from an existing SQLite file: Postgres starts empty.
- No Postgres container, compose service or provisioning in this repo.
- No new schema features (no column comments, no new indexes) beyond what the switch needs.

### Dependencies / Assumptions

- `gorm.io/driver/postgres` v1.6.3 (released 2026-09-14) is approved by the owner's request. It is pure Go (pgx v5), so `CGO_ENABLED=0` builds keep working.

### Sources / Research

- `internal/db/db.go`, `internal/config/config.go`, `internal/giveaway/store.go`, `internal/user/user.go`, `internal/user/revision.go`, and their tests.
- `docs/solutions/conventions/sqlite-one-connection-and-write-first-transactions.md`, `sqlite-time-comparisons-in-utc.md`, `document-columns-with-go-comments-not-gorm-comment-tags.md`, `user-profile-revisions.md`, `giveaway-winner-draw.md`.

---

## Planning Contract

### Key Technical Decisions

- KTD1. **`gorm.io/driver/postgres` with default pool settings.** (session-settled: user-directed — chosen over keeping SQLite: owner asked for a full migration.) Postgres handles concurrent writers, so `SetMaxOpenConns(1)` and the pragmas go away. GORM maps `time.Time` to `timestamptz`, so times compare as instants and the UTC-text workaround in `Store.Due`/`Store.Finish` is no longer needed; the `.UTC()` calls can stay harmlessly or be dropped with their comments.
- KTD2. **`DATABASE_URL` validated with `net/url`.** (session-settled: user-directed — chosen over host/port/user/password settings: the database is configured elsewhere.) Accept only schemes `postgres`/`postgresql` with a non-empty host. Sentinel errors `ErrMissingDatabaseURL` and `ErrInvalidDatabaseURL`, reported through the existing `errors.Join` in `Config.validate`.
- KTD3. **Delete tests that only check SQL against SQLite.** (session-settled: user-directed — chosen over integration tests on a real Postgres: tests stay isolated.) `internal/db/db_test.go`, `internal/giveaway/store_test.go`, `internal/user/user_test.go` and `internal/user/revision_test.go` exercise GORM queries against a SQLite file; on Postgres they would test the wrong dialect, and the owner rejected a real test database. They are removed, not ported.
- KTD4. **Service and Editor tests run on an in-memory fake.** `internal/giveaway/setup_test.go` and `edit_test.go` currently build `Service` and `Editor` on real stores. A hand-written fake in package `giveaway_test` implements `Storage`, `Drafts`, `Giveaways` and `Writer` (giveaway rows with the published/cancelled/draft rules documented on `*Store`), and a map-backed user fake implements `Steps` and `Language` (so it also satisfies the handlers' `Users` interface). The shared constants `channelID`, `adminID`, `fanID` and a fake-backed `publish` helper move out of `store_test.go` into the fake file. Direct `s.db` writes in tests (marking a giveaway finished) become fake methods.
- KTD5. **`Candidates` orders by `p.created_at, p.user_id`.** Postgres has no `rowid`. `created_at` is microsecond-precise in Postgres, so ties are rare; `user_id` makes the order deterministic.
- KTD6. **Delete `user.BackfillRevisions`.** It upgraded SQLite databases saved before revisions existed. Postgres starts empty, so every user gets revision 1 from `Record`.

### High-Level Technical Design

Directional only:

```text
main -> config.Load (DATABASE_URL) -> db.Open(ctx, url)
db.Open: gorm.Open(postgres.Open(url), Logger: Discard) -> PingContext -> AutoMigrate(User, Revision, Giveaway, Participant)
```

### Assumptions

- `Running` orders by `ends_at`; Postgres sorts NULLs last in ascending order where SQLite sorted them first. Giveaways without an end time now appear after timed ones in the creator's list. Accepted as the more natural order; no test pins the old order.
- `Record`'s write-first transaction and `Finish`'s guarded update stay correct under Postgres READ COMMITTED: a blocked `UPDATE ... WHERE` re-checks its condition after the lock is released. Only the SQLite-specific comments change.
- `SaveDraft`'s `ON CONFLICT ... DO UPDATE ... WHERE giveaways.published = false ...` is valid Postgres and reports 0 rows when the `WHERE` skips the update.
- SQLite-only learnings under `docs/solutions/conventions/` are deleted (one-connection, UTC comparisons) or edited where they mention SQLite in passing; the comment-tag learning is updated to say Postgres now stores `comment:` tags.
- Local `.gitignore` entries for `*.db` files are removed with the rest of SQLite.

### Sequencing

U1 -> U2 -> U3 -> U4. Each unit compiles and passes `go test ./...` and `golangci-lint run ./...` on its own.

---

## Implementation Units

### U1. Config: `DATABASE_URL` replaces `DB_PATH`

- **Goal:** R1.
- **Files:** `internal/config/config.go`, `internal/config/config_test.go`, `main.go`, `.env.example`.
- **Patterns:** existing `validateToken`/`validateDBPath` and the table-driven `config_test.go`.
- **Test scenarios:** missing `DATABASE_URL` -> `ErrMissingDatabaseURL`; `postgres://u:p@host:5432/db?sslmode=disable` and `postgresql://host/db` accepted; `mysql://host/db`, `host=x dbname=y`, `postgres:///db` (no host) and an unparsable URL -> `ErrInvalidDatabaseURL`; error text never contains the URL or password; missing token and URL reported together.
- **Verification:** `go test ./internal/config/`.

### U2. Service and Editor tests on fakes

- **Goal:** R5, R6, KTD4. Done first so the Service and Editor tests no longer depend on the SQLite store when U3 removes it.
- **Files:** `internal/giveaway/setup_test.go`, `internal/giveaway/edit_test.go`, `internal/giveaway/handler_test.go`, `internal/giveaway/service_test.go` (shared constants), new `internal/giveaway/fake_store_test.go`.
- **Patterns:** existing `fakeTelegram` in the giveaway tests; contracts documented on `*Store` methods (`ErrNotFound`, `ErrExists`, draft/managed selection).
- **Test scenarios:** every existing test in `setup_test.go`, `edit_test.go` and `handler_test.go` keeps its assertions and passes on the fake.
- **Verification:** `go test ./internal/giveaway/` passes, also with `store_test.go` temporarily moved aside; only `store_test.go` imports `gorm` or calls `newStore`.

### U3. Switch to Postgres and drop SQLite

- **Goal:** R3, R4, R5, KTD1, KTD3, KTD5, KTD6.
- **Files:** `internal/db/db.go`; `internal/config/config.go` leftovers; `internal/giveaway/store.go` (`Candidates` order, `Due`/`Finish` comments); `internal/giveaway/giveaway.go` and `internal/user/user.go` (SQLite column-default comments); `internal/user/revision.go` (drop `BackfillRevisions`, transaction comment); delete `internal/db/db_test.go`, `internal/giveaway/store_test.go`, `internal/user/user_test.go`, `internal/user/revision_test.go`; `go.mod`, `go.sum`.
- **Patterns:** current `db.Open` shape (open, ping, migrate, close on failure).
- **Test scenarios:** none; these files only talk to a real database, which tests must not do (KTD3). `go build ./...` proves the wiring.
- **Verification:** `go mod tidy`, `go build ./...`, `go test ./...`; `rg -n 'rowid|SQLite|sqlite' --type go` and `rg -i 'sqlite|glebarez|modernc' go.mod` find nothing.

### U4. Docs

- **Goal:** R2.
- **Files:** `CLAUDE.md` (Dependencies, Database, Gotchas, config paragraph), `Dockerfile` comment, `.gitignore`, `.golangci.yml` (the `paralleltest` reason comment only; enabling the linter is out of scope), `docs/solutions/conventions/*` per Assumptions.
- **Verification:** `markdownlint-cli2 "**/*.md"`; `rg -il sqlite CLAUDE.md Dockerfile .env.example` finds nothing.

---

## Verification Contract

| Check | Command | Done when |
| --- | --- | --- |
| Build | `go build ./...` | exit 0 |
| Tests | `go test ./...` | pass with no database reachable |
| Lint | `golangci-lint run ./...` | 0 issues |
| SQLite gone | `rg -il --hidden 'sqlite\|glebarez\|modernc\|DB_PATH' --glob '!docs/plans/**' --glob '!.git/**'` | no code or config hits |

Manual, owner (store SQL has no automated check after KTD3): with a real `DATABASE_URL`, start the bot and check the tables are created; create a giveaway ending a few minutes ahead, join it from two accounts, let the per-minute draw pick and announce winners; forward the published post to `/giveaway` again and confirm a new draft is refused; restart the bot and confirm startup succeeds on the existing tables.

## Definition of Done

- All units committed on `refactor/postgres`, verification table green.
- Known gap recorded in the PR: store SQL is no longer tested automatically (KTD3).
- Deploy note recorded in the PR: Postgres starts empty, so existing SQLite users, revisions and giveaways are not carried over; deploy when no published, unfinished giveaway is running, and switch the deployment's env from `DB_PATH` to `DATABASE_URL` in the same release.
