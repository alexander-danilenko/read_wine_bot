---
title: Hand-check Store SQL against the local database
date: 2026-10-27
category: conventions
module: internal/channel, internal/giveaway, internal/user, internal/db
problem_type: convention
component: database
severity: low
applies_when:
  - Adding or changing a query in any Store
  - Adding a model, constraint or upsert that AutoMigrate creates
tags: [postgres, gorm, store, manual-check]
---

# Hand-check Store SQL against the local database

## Context

Tests never open a database (`CLAUDE.md`), so a `Store` query is only checked by hand. Running the whole bot for that also needs Telegram and a real flow. A throwaway program that calls the store directly is faster and checks exactly the query you changed.

## Guidance

1. Start the local database: `docker compose up -d`.
2. Write `tmpcheck/main.go` in the repo root. It must sit inside the module, or it cannot import `internal/` packages. Load the config and open the database the way `main` does, so `AutoMigrate` and the table comments run too:

   ```go
   cfg, err := config.Load(ctx, ".")
   conn, err := db.Open(ctx, cfg.DatabaseURL)
   s := channel.NewStore(conn)
   ```

3. Call each changed method with edge cases: a repeat, an out-of-order write, a row that breaks a constraint. Print the results. Use made-up IDs (such as chat `-1000000000042`), and delete the rows at the end.
4. Run it and delete it in one command, so it never reaches a commit: `go run ./tmpcheck; rm -rf tmpcheck`.

## Example

The `bot_channels` upsert (#21) was checked this way: admin, then removed, then a late admin update with an older date. The last write had to be ignored, and `AdminChannels` had to return nothing.
