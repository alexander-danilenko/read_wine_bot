---
title: SQLite runs on one connection; transactions use only tx and start with a write
date: 2026-10-09
category: conventions
module: internal/db, internal/user
problem_type: convention
component: database
severity: high
applies_when:
  - Writing a GORM transaction (db.Transaction) in any store
  - Calling another store or the store's own db from code that may run inside a transaction
  - Iterating query results with Rows() or a cursor while issuing other queries
  - Changing the connection pool settings or the SQLite pragmas in internal/db
symptoms:
  - "A burst of concurrent updates fails with `database is locked (5) (SQLITE_BUSY)` after busy_timeout"
  - A handler hangs forever with no error after the pool was limited to one connection
root_cause: concurrency
resolution_type: config_change
tags: [sqlite, gorm, glebarez, connection-pool, setmaxopenconns, sqlite-busy, busy-timeout, transaction, deadlock, go-telegram-bot]
---

# SQLite runs on one connection; transactions use only tx and start with a write

## Context

`go-telegram/bot` handles every update in its own goroutine (`process_update.go:31` in v1.27.0: `go r(ctx, b, upd)`), and the user middleware writes to SQLite on every update. When `user.Store.Record` became a multi-statement transaction (insert or conditional update, then a revision insert), a burst of first-time users broke it. With the default pool, many connections each opened a transaction and waited in SQLite's busy handler for the single write lock. In a test of 200 concurrent `Record` calls, some failed with `database is locked (5) (SQLITE_BUSY)` once `busy_timeout(5000)` (`internal/db/db.go:18`) ran out. Two runs gave 39 and 6 failures, after about 5 seconds each. A failed `Record` is only logged, so the user got no `users` row and their giveaway join then failed its foreign key. The old single-statement upsert survived the same burst, which is why the problem only appeared with the transaction.

The fix is `sqlDB.SetMaxOpenConns(1)` in `db.Open` (`internal/db/db.go:38`): Go's pool now queues callers instead of SQLite failing them. The same 200-update burst finishes in about 30 ms with no errors. `TestOpenConcurrentRecords` (`internal/db/db_test.go:237`) guards this.

The one-connection pool makes some code patterns hang without any error, and the comment next to the setting is not where someone writing a new store would look. Those patterns are the reason for this doc.

## Guidance

1. **Keep the pool at one connection.** Do not raise `SetMaxOpenConns` to "speed up reads". Writes are what the bot does on every update, and SQLite allows only one writer at a time.
2. **Inside `db.Transaction(func(tx *gorm.DB) error { ... })`, use only `tx`.** A call to the store's own `s.db`, another store, or any helper that holds the outer `*gorm.DB` needs a second connection. With one connection, that call waits forever for the connection the transaction is holding, and no error ever appears. To reuse a store method inside a transaction, pass `tx` into it.
3. **Never keep a cursor open while running another query.** For example, loop over GORM `.Rows()`, or `*sql.Rows` from `Raw(...).Rows()`, and query inside the loop. The open cursor holds the only connection. Use `Find` into a slice first, then loop.
4. **Start every transaction with a write.** SQLite fails a transaction that read first and then tries to write while another writer holds the lock, and `busy_timeout` does not retry that case. `Record` puts its insert or conditional update first for this reason; see the comment at `internal/user/revision.go:62`. Even with one connection this still matters, because a second process such as an `sqlite3` shell can hold the lock.

## Why This Matters

- Breaking rule 1 brings back the original bug: under load, new users go unrecorded and giveaway joins fail.
- Breaking rule 2 or 3 does not fail any unit test that runs one call at a time. It shows up only in production, as a handler that never replies, and nothing is logged.
- Rule 4 keeps transactions correct even when something outside the bot holds the database.

## When to Apply

Whenever code in `internal/` touches GORM: a new store method, a new transaction, a pool or DSN change in `internal/db`, or a loop over query results.

## Examples

Before (deadlocks with one connection):

```go
return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    if err := tx.Create(&g).Error; err != nil {
        return err
    }
    return s.users.SetPendingCommand(ctx, g.CreatedBy, "") // uses its own *gorm.DB: waits forever
})
```

After:

```go
return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    if err := tx.Create(&g).Error; err != nil {
        return err
    }
    return tx.Model(&user.User{}).Where("telegram_id = ?", g.CreatedBy).
        Update("pending_command", "").Error // same connection, same transaction
})
```

## Related

- [Giveaway CRUD around the published flag](giveaway-crud-around-the-published-flag.md): guarded single-statement `UPDATE`s, the other way this codebase avoids read-then-write races.
- [Document columns with Go comments, not GORM comment tags](document-columns-with-go-comments-not-gorm-comment-tags.md).
