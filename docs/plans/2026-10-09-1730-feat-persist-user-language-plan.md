---
title: "feat: persist each user's /language choice in the database"
date: 2026-10-09
type: feat
artifact_contract: ce-unified-plan/v1
product_contract_source: ce-plan-bootstrap
execution: code
depth: standard
---

# feat: persist each user's /language choice in the database

## Goal Capsule

- **Objective:** a user who picks a language with `/language` gets every later bot reply in that language, including after the bot restarts.
- **Means:** the bot's own database through GORM, SQLite or Postgres chosen by `DB_DRIVER`/`DB_DSN` (KTD1, KTD2), with a `users` table keyed by Telegram user ID (KTD3).
- **Authority:** this plan, then `CLAUDE.md` (Fail fast, Database, Dependencies, SOLID), then existing code patterns.
- **Stop conditions:** a change that needs a module outside the approved list; a destructive schema change; a settled decision proving unworkable.
- **Execution profile:** small Go change, 4 units, test-first for the store and config validation; handlers verified by build plus lint.
- **Shipping:** commits straight to `main`, pushed to `origin/main`, no branch and no pull request.

---

## Product Contract

### Summary

Wire the first database into the bot, store each user's `/language` choice in a `users` table, and use the saved language for the `/start` greeting and the `/language` replies. A user with no saved choice keeps today's behaviour: the Telegram app language, then English.

### Problem Frame

`/language` shows buttons and confirms the tap, but forgets the choice (`internal/language/language.go` says so in its package comment). Every reply uses `language_code` from the Telegram update, which is the app's language, not what the user picked. A Ukrainian speaker with an English Telegram app cannot get Ukrainian replies.

### Requirements

#### Configuration and startup

- R1. The bot reads `DB_DRIVER` (`sqlite` or `postgres`) and `DB_DSN` from the environment or `.env`, like `TELEGRAM_BOT_TOKEN`.
- R2. A missing or invalid `DB_DRIVER`, or a missing or invalid `DB_DSN` (a sqlite DSN that already holds `?`, per KTD2), stops the bot at startup with an error that names the setting, never its value; all invalid settings are reported in one error.
- R3. The bot connects to the database and pings it before it handles the first update; an unreachable database stops the bot.
- R4. The bot creates or updates the `users` table with `AutoMigrate` at startup.

#### Language choice

- R5. Tapping a language button saves that language for the tapping user; tapping again overwrites it.
- R6. The confirmation after a tap is in the newly chosen language.
- R7. `/start` and `/language` reply in the saved language when one exists, otherwise in the Telegram app language, otherwise English.
- R8. A database error while reading or saving the language is logged and the bot still replies; it does not crash or stay silent. A read error falls back per R7; a save error still sends the confirmation in the chosen language (R6).

### Scope Boundaries

- No new commands, no `setMyCommands` menu registration.
- No user profile beyond the language; no storing usernames or names.
- Group and channel behaviour unchanged: both handlers still ignore non-private chats.

### Acceptance Examples

- AE1. **Covers:** R5, R6, R7. **Given** a user whose Telegram app is English, **when** they tap 🇺🇦 Українська, **then** the confirmation is in Ukrainian, and a later `/start` is in Ukrainian.
- AE2. **Covers:** R7. **Given** a user who never used `/language` and whose app is Ukrainian, **when** they send `/start`, **then** the greeting is in Ukrainian.
- AE3. **Covers:** R2. **Given** `DB_DRIVER=mysql`, a valid `DB_DSN`, and no `TELEGRAM_BOT_TOKEN`, **when** the bot starts, **then** it exits with one error naming both `DB_DRIVER` and `TELEGRAM_BOT_TOKEN`.

---

## Planning Contract

### Key Technical Decisions

- KTD1. **Own database through GORM.** Approved modules `gorm.io/gorm`, `github.com/glebarez/sqlite`, `gorm.io/driver/postgres` are already pinned in `go.mod` through `deps.go`. (session-settled: user-approved — chosen over Telegram Mini App CloudStorage/DeviceStorage: the Bot API cannot read those from a Go bot.)
- KTD2. **`DB_DRIVER` and `DB_DSN` are both required, no defaults.** `CLAUDE.md` allows a default only when it is safe in production; a SQLite file path inside a distroless container is not, because it needs a mounted volume. For `sqlite`, `DB_DSN` is a file path and the code appends `_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)` itself, so the `CLAUDE.md` pragma rule cannot be forgotten in config. A sqlite `DB_DSN` that already holds `?` is rejected as invalid. For `postgres`, `DB_DSN` passes through unchanged.
- KTD3. **`users` table: `telegram_id int64` primary key, `language string`, plus GORM timestamps.** Primary key on the Telegram ID makes the save an upsert (`ON CONFLICT (telegram_id) DO UPDATE SET language, updated_at`, via `clause.AssignmentColumns`), portable across SQLite and Postgres through `gorm.io/gorm/clause`. (session-settled: user-approved — chosen over relying only on `update.From.language_code`: that is the Telegram app language, not the user's choice; it stays as the fallback.)
- KTD4. **Package split.** `internal/db` opens the connection, pings it and runs `AutoMigrate` (one reason to change: storage wiring). `internal/user` owns the `User` model and a `Store` with `Language(ctx, telegramID)` and `SetLanguage(ctx, telegramID, lang)`. Handlers in `internal/greeting` and `internal/language` take small interfaces declared in their own packages, per the SOLID section of `CLAUDE.md`. `internal/db` imports `internal/user` only to pass the model to `AutoMigrate`.
- KTD5. **Fallback order lives in the handlers via one small helper per package user, not in `i18n`.** `i18n.Catalog.Text` already falls back from an unknown language to English; the handler only has to choose between the saved language and `language_code`. `i18n` stays free of storage.
- KTD6. **Ship straight to `main`.** (session-settled: user-directed — chosen over a feature branch and a draft PR: the owner's explicit instruction for this run.)

### Assumptions

- The Docker deployment mounts a writable volume for the SQLite file, or uses Postgres; the image itself is unchanged by this plan.
- Saved language codes are the button codes (`en`, `uk`); `Text` already maps unknown codes to English if a locale is later removed.

### Sequencing

U1 (config) and U2 (store) are independent; U3 (db wiring in `main.go`) needs both; U4 (handlers) needs U2 and U3.

---

## Implementation Units

### U1. Database settings in config

- **Goal:** load and validate `DB_DRIVER` and `DB_DSN`.
- **Requirements:** R1, R2.
- **Files:** `internal/config/config.go`, `internal/config/config_test.go`, `.env.example`.
- **Approach:** add `DBDriver`, `DBDSN` to `Config`; sentinel errors `ErrMissingDBDriver`, `ErrInvalidDBDriver`, `ErrMissingDBDSN`, `ErrInvalidDBDSN`; extend `validate()`'s `errors.Join`. Export driver name constants (`DriverSQLite`, `DriverPostgres`) for `internal/db` to switch on. Add both keys with comments to `.env.example`.
- **Patterns:** `validateToken` and the table test in `config_test.go`.
- **Test scenarios:** existing token cases still pass with valid DB env set; missing driver; unknown driver; missing DSN; sqlite DSN with `?`; postgres DSN with `?` accepted; token and driver both invalid gives one error matching both sentinels (AE3); error text never contains the DSN value.
- **Verification:** `go test ./internal/config/`.

### U2. User language store

- **Goal:** read and save a user's language.
- **Requirements:** R5, R8 (error contract).
- **Files:** `internal/user/user.go`, `internal/user/user_test.go`.
- **Approach:** `User{TelegramID int64 primaryKey autoIncrement:false; Language string not null; CreatedAt; UpdatedAt}`. `Store{db *gorm.DB}`, `NewStore(db)`. `Language` returns `""` and nil when no row (`errors.Is(err, gorm.ErrRecordNotFound)`), wraps other errors. `SetLanguage` upserts per KTD3.
- **Test scenarios:** unknown user returns `""`; set then get; set twice keeps the last value and moves `UpdatedAt` forward; tests run on in-memory SQLite (`glebarez/sqlite`, pure Go, no CGO).
- **Verification:** `go test ./internal/user/`.

### U3. Open, ping and migrate the database at startup

- **Goal:** fail fast on a broken database.
- **Requirements:** R3, R4.
- **Files:** `internal/db/db.go`, `internal/db/db_test.go`, `main.go`, `deps.go` (delete).
- **Approach:** `db.Open(ctx, driver, dsn) (*gorm.DB, error)`: pick `sqlite.Open(path + "?" + pragmas)` or `postgres.Open(dsn)`, set GORM's logger to `logger.Discard` (errors come back as return values and `main` logs them), `PingContext`, `AutoMigrate(&user.User{})`. `main.go` calls it after config and translations, closes the `sql.DB` on exit, builds `user.NewStore`. Delete `deps.go` once all three libraries have real imports, then `go mod tidy`.
- **Test scenarios:** open a temp-dir SQLite file, then confirm `PRAGMA foreign_keys` is 1 and the `users` table exists; unknown driver returns an error.
- **Verification:** `go test ./internal/db/`, `go build .`.

### U4. Handlers use the saved language

- **Goal:** save on tap, reply in the saved language.
- **Requirements:** R5, R6, R7, R8; AE1, AE2.
- **Files:** `internal/language/language.go`, `internal/greeting/greeting.go`, `main.go`.
- **Approach:** `language` declares an interface with `Language` and `SetLanguage`; `greeting` declares one with `Language` only. Each package has an unexported helper that returns the saved language or, on empty result or error (logged), `from.LanguageCode`. `ButtonHandler` saves first, logs a save error, and sends the confirmation in the chosen code. Update the comments that describe the old behaviour: the `language` package comment and the `ButtonHandler` doc (both say the choice is not saved), and the `greeting.Handler` doc (says it replies in the Telegram app language).
- **Test scenarios:** none new; handler wiring is covered by build and lint, store behaviour by U2.
- **Verification:** `go build ./...`, `golangci-lint run ./...`.

---

## Verification Contract

| Gate | Command | Applies to |
| --- | --- | --- |
| Tests | `go test ./...` | all units |
| Lint | `golangci-lint run ./...` reports 0 issues | all units |
| Build | `go build .` | U3, U4 |
| Modules | `go mod tidy` leaves `go.mod`/`go.sum` clean | U3 |

---

## Definition of Done

- All requirements R1–R8 met; `go test ./...`, `golangci-lint run ./...` and `go build .` pass.
- `deps.go` deleted; no blank imports remain.
- `.env.example` lists `DB_DRIVER` and `DB_DSN`.
- No dead code from abandoned approaches in the diff.
- Commits on `main` follow Conventional Commits and are pushed to `origin/main`.
