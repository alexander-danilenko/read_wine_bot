---
title: Compare times in SQLite queries only in UTC
date: 2026-10-09
category: conventions
module: internal/giveaway, internal/db
problem_type: convention
component: database
severity: high
applies_when:
  - Writing a WHERE or ORDER BY that compares a time column with a Go time.Time parameter
  - Comparing two time columns that may have been written in different time zones
  - Adding a time column whose rows other code will query by range
symptoms:
  - "A due-date query matches rows hours early or late, by the process's UTC offset"
  - Tests pass on a UTC machine or container and fail on a developer laptop, or the reverse
root_cause: wrong_api
resolution_type: code_fix
tags: [sqlite, glebarez, go-sqlite, gorm, time, utc, timezone, ends_at, finished_at, text-comparison]
---

# Compare times in SQLite queries only in UTC

## Context

SQLite has no time type. The driver this project uses (`github.com/glebarez/go-sqlite` v1.23.0, under `github.com/glebarez/sqlite`) binds every `time.Time` parameter as **text in the value's own zone**. `conn.formatTime` formats with `"2006-01-02 15:04:05.999999999-07:00"` (`sqlite.go:295-296`, `348-355`), and the bind path calls it for every `time.Time` (`sqlite.go:1135-1136`). A comparison such as `ends_at <= ?` is therefore a **string** comparison, and it is only correct when both sides carry the same offset.

The winner draw nearly shipped this bug. Giveaway end times are stored in UTC: `utcPtr` in `internal/giveaway/setup.go:334` converts every end time before saving. The draw passes `time.Now()` from `main`, which is local time. On a Mac set to Europe/Kyiv in summer (+03:00), `2026-10-24 14:00:01+03:00` already sorts after `2026-10-24 14:00:00+00:00`, so a giveaway ending at 14:00 UTC matches once the Kyiv clock passes 14:00, which is 11:00 UTC, three hours early. In winter (+02:00) it is two hours early. The plan's feasibility review caught it before any code was written, by reading the driver source. No test or lint would have flagged it.

The bug hides in production. The bot ships in a distroless container that sets no `TZ`, so unless `TZ` is passed in, Go's local zone there is typically UTC and the strings happen to agree. It only shows on a machine with a non-UTC zone, which is where you test by hand.

## Guidance

- **Convert every `time.Time` parameter to UTC before it reaches SQL that compares it.** `Store.Due` passes `now.UTC()` (`internal/giveaway/store.go:230`). `Store.Finish` converts `now` before both its guard and the value it writes (`store.go:258`).
- **Store time columns that other code queries by range in UTC**, as `utcPtr` does for `ends_at`. Then the stored text and the parameter text always share the `+00:00` offset.
- **Do not compare GORM's automatic timestamps across zones.** GORM fills `CreatedAt` and `UpdatedAt` with `time.Now().Local()` by default (GORM v1.31.2, `gorm.go:186-187` in the Go module cache). On a Kyiv machine these columns switch from `+03:00` to `+02:00` text at the end of daylight saving time, so their text order no longer matches time order across that change. Ordering by them is fine as a tie-breaker. A range query or `<` against a parameter needs UTC on both sides, or a `NowFunc` that returns UTC.
- **Test with a non-UTC `now`.** `TestDueComparesInstants` (`internal/giveaway/store_test.go:568`) passes a Europe/Kyiv time 30 minutes before a UTC end time and expects no match. It fails if either `.UTC()` call is removed.

## Why This Matters

Lexical comparison of zone-tagged text looks right in every code review: the query reads `ends_at <= ?` and the parameter is a real `time.Time`. Nothing fails loudly. Rows simply match at the wrong moment, offset by the process's zone. In this project that means a giveaway closes and announces winners hours before the time shown to the channel, and people who join in that window are silently excluded.

## When to Apply

Any new store method that filters or orders by a time column against a Go value: due reminders, expiries, "older than" cleanups, rate windows. It also applies when a column written by GORM's automatic timestamps is compared with anything.

## Examples

Before (matches up to 3 hours early on a Kyiv machine):

```go
s.db.WithContext(ctx).
    Where("published = ? AND finished_at IS NULL AND ends_at IS NOT NULL AND ends_at <= ?", true, now).
    Find(&gs)
```

After:

```go
// The driver binds a time as text with its zone offset and ends_at holds UTC text, so the
// comparison is only right for a UTC now.
s.db.WithContext(ctx).
    Where("published = ? AND finished_at IS NULL AND ends_at IS NOT NULL AND ends_at <= ?", true, now.UTC()).
    Find(&gs)
```

## Related

- [SQLite runs on one connection; transactions use only tx and start with a write](sqlite-one-connection-and-write-first-transactions.md): the other driver-level rules for this database.
- [Giveaway CRUD around the published flag](giveaway-crud-around-the-published-flag.md): the guarded `UPDATE` that `Store.Finish` also uses.
