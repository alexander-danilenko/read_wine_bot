---
title: Giveaway CRUD around the published flag
date: 2026-10-09
last_updated: 2026-10-18
category: conventions
module: internal/giveaway
problem_type: convention
component: database
severity: medium
applies_when:
  - Adding a read, write or delete method to internal/giveaway/store.go
  - Adding a new giveaway state or a column that changes which state a row is in
  - Writing a boolean or nullable column that has a GORM default
  - Adding a CHECK constraint or a partial unique index to giveaways through a gorm tag
tags: [gorm, postgres, giveaway, published, soft-cancel, state-machine, idempotency, check-constraint, partial-index, automigrate]
---

# Giveaway CRUD around the published flag

## Context

One `giveaways` row holds three different things, told apart only by two columns: `published` and `cancelled_at`. A setup draft, a published giveaway and a cancelled giveaway share the table and the unique post index `(chat_id, message_id)`. `finished_at` is a third, independent axis. Each store method in `internal/giveaway/store.go` carries a piece of the rules. The database rejects an invalid combination of columns (see "The database enforces the states" below), but it cannot see which scope a query should read. A new method that forgets a scope still leaks drafts into the edit flow or lets a participant join a cancelled giveaway, and an insert from the struct still saves every draft as published.

The states, from `internal/giveaway/giveaway.go:13-40` and the scopes at `internal/giveaway/store.go:22-25`:

| State | `published` | `cancelled_at` | Join button on post |
| --- | --- | --- | --- |
| Draft (unfinished setup) | false | NULL | no |
| Published | true | NULL | yes |
| Cancelled | false | set | no |

Finished means `finished_at` is set. Only a published giveaway can finish, and `Cancel` refuses a finished one.

### The database enforces the states

The rules above are also constraints, declared in gorm tags in `internal/giveaway/giveaway.go:21-35` and created by `AutoMigrate`:

| Constraint | Rule |
| --- | --- |
| `chk_giveaways_cancelled` | `cancelled_at IS NULL OR NOT published`: a published row is never cancelled |
| `chk_giveaways_finished` | `finished_at IS NULL OR (published AND cancelled_at IS NULL)`: only a published giveaway finishes |
| `chk_giveaways_winners` | `winners >= 1`, drafts included |
| `idx_giveaways_draft` | partial unique index on `created_by` where `published = false AND cancelled_at IS NULL`: one draft per admin |

A store method that breaks one of these gets an SQL error instead of a bad row. Two consequences are easy to miss:

- **A map insert skips every GORM default, not only `published`.** `SaveDraft` writes each key of its map, so `winners` is saved as whatever the caller passed. A `Giveaway` built without `Winners` saves `0`, and `chk_giveaways_winners` rejects it even though the column has `default:1`. `Forward` (`internal/giveaway/setup.go:81-82`) therefore saves `minWinners` as a placeholder until the winners step. Any new caller of `SaveDraft` must set `Winners` too.
- **Delete the old draft before saving a new one.** `Forward` calls `DeleteDraft` before `SaveDraft` (`setup.go:74`), so the admin's previous draft on another post never collides with `idx_giveaways_draft`.

The in-memory fake (`internal/giveaway/fake_store_test.go`, `violates`) mirrors `chk_giveaways_winners` and `idx_giveaways_draft` and returns `errConstraint`, because tests never open a database. A new constraint needs a matching check in the fake, or tests pass on data production rejects. Test fixtures that build a `Giveaway` must set `Winners`; the `publish` helper defaults it to 1.

`AutoMigrate` cannot add a CHECK while an existing row violates it: the bot fails to start and the error names the constraint. Before deploying a new constraint, find and fix the violating rows in production. When these constraints shipped, drafts saved by the older code had `winners = 0` and needed `UPDATE giveaways SET winners = 1 WHERE winners < 1` first.

## Guidance

### Create: only drafts, inserted from a map

- The only insert is `SaveDraft` (`store.go:71`). A giveaway is never inserted as published. It becomes published through `Publish`.
- `Published` has `default:true` (`giveaway.go:40`) so rows created before the setup flow existed count as published. GORM leaves a `false` zero value out of an `INSERT` for a column with a default, so inserting the struct saves the draft as **published**. `SaveDraft` inserts a `map[string]any` that names `published: false` explicitly. Any new insert into this table must do the same.
- The upsert on the post index takes over the row only when it is a draft (`published = false AND cancelled_at IS NULL`, `store.go:84-87`). A published or cancelled giveaway on the post is left alone and `SaveDraft` returns `ErrExists`. Its `DoUpdates` list leaves `published` out, so the takeover keeps the row a draft.

### Read: every query names its scope

Never look a giveaway up by bare id or post. Pick the scope that matches the caller:

- `draft` (`store.go:22`): the user's unfinished setup. Used by `Draft` and `DeleteDraft`.
- `managed` (`store.go:25`): published or cancelled, never a draft. Used by `Post`, `ByID` and `SaveFields`. A stale edit callback that carries a draft's id gets `ErrNotFound`.
- `published = true AND finished_at IS NULL`: only `Running` uses this, for the list of live giveaways.

Domain code still checks `g.Published` after a `managed` read, because `managed` includes cancelled rows. `Join` rejects a cancelled giveaway (`internal/giveaway/service.go:102`). `AskCancel`, `Cancel` and `Restore` in `internal/giveaway/edit.go` treat the wrong state as stale.

### Update: guard the source state in the WHERE clause

- A state change is one conditional `UPDATE` whose `WHERE` names the state it starts from. `RowsAffected == 0` returns `ErrNotFound`. A double tap on Confirm, Cancel or Publish therefore changes nothing the second time, without a read-then-write race.
  - `Publish`: `WHERE id = ? AND published = false` sets `published = true, cancelled_at = NULL` (`store.go:50-53`). The same method publishes a draft and restores a cancelled giveaway.
  - `Cancel`: `WHERE id = ? AND published = true AND finished_at IS NULL` sets `published = false, cancelled_at = now` (`store.go:207-210`).
- Change `published` and `cancelled_at` together in one `Updates` call. A row with `published = true` and `cancelled_at` set is not a valid state, and `chk_giveaways_cancelled` rejects it.
- Field edits (`SaveFields`, `store.go:189-192`) use `managed AND finished_at IS NULL`. A draft is edited through the setup flow, not here, and a finished giveaway is frozen.
- Write with `Updates(map[string]any{...})` and the `col*` constants (`store.go:28-34`). With a struct, GORM skips zero values, so `false` and `nil` would never be written.

### Delete: only drafts

- `DeleteDraft` (`store.go:116-117`) is the only hard delete, and it is scoped by `draft`.
- A published giveaway is never deleted. Cancel is a soft state change that keeps its participants and values, so the creator can publish it again.

### Order with Telegram: the button first, then the row

`publish` in `internal/giveaway/setup.go:257`, and `Cancel` and `Restore` in `edit.go`, change the join button on the post first and the row second. If the database write then fails, the row still describes the old state, and a retry repeats the Telegram call and completes the write. The conditional `UPDATE` makes that retry safe.

The winner draw (`Drawer` in `internal/giveaway/draw.go`) reverses this order on purpose: `Store.Finish` saves the winners and the finish time first, then the draw removes the button and posts the results. Saving first means a failed Telegram call can never lead to a second draw that picks different winners. `Finish` guards on the giveaway still being due, so a cancel or a later end time saved in the meantime wins.

## Why This Matters

- A missing scope on a read is silent. A draft shows up in the edit flow, or a cancelled giveaway accepts participants, and no test fails unless it covers that state.
- The `default:true` trap is invisible in code review. The struct insert looks correct and returns no error, yet the row is published.
- Read-then-write state changes race on a double tap. The guarded `UPDATE` makes the database decide which tap wins.

## When to Apply

- Every new `Store` method on `giveaways`.
- Before adding a state such as "drawing" or "archived": decide its column values, which scope includes it, and which transitions start from it. Then update the scope constants rather than adding conditions inline.
- Any table that keeps a boolean or nullable column with a non-zero GORM default.

## Examples

Wrong: inserting the struct saves a published row, and a lookup without a scope returns drafts.

```go
s.db.WithContext(ctx).Create(&Giveaway{ChatID: c, MessageID: m, Published: false}) // saved as published = true
s.db.WithContext(ctx).Where("id = ?", id).Take(&g)                                 // a draft id matches too
```

Right: insert from a map, read with a named scope, transition with a guarded `UPDATE`.

```go
s.db.WithContext(ctx).Model(&Giveaway{}).Create(map[string]any{colPublished: false /* ... */})
s.db.WithContext(ctx).Where("id = ? AND "+managed, id).Take(&g)

res := s.db.WithContext(ctx).Model(&Giveaway{}).
    Where("id = ? AND published = ? AND finished_at IS NULL", id, true).
    Updates(map[string]any{colPublished: false, colCancelled: time.Now()})
if res.RowsAffected == 0 {
    return ErrNotFound // already cancelled, finished, or missing
}
```

## Related

- [Describe every column, table and index in the database](describe-every-column-table-and-index-in-the-database.md): the other GORM schema convention in this repo.
