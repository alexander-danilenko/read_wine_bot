# 0006. Change the join button first, then the giveaway row

Status: accepted, 2026-10-09.

## Context

Publish, cancel, restore, move and a button-text edit each change two things: the join button on a public channel post and the `giveaways` row. They cannot share a transaction.

## Decision

Change the button first and the row second, with a guarded `UPDATE` whose `WHERE` names the state it starts from (`RowsAffected == 0` is `ErrNotFound`). If Telegram refuses, nothing changed. If the database then fails transiently, a retry repeats the Telegram call (made idempotent by `ignoreNotModified`) and completes the write.

A write that fails because the state changed (`ErrNotFound`, `ErrExists`) is not retryable. When the Telegram change put a button on a post the row will not point to, undo it on every write failure; otherwise a live-looking button stays on the post and every tap fails in `Service.Join`. `Editor.move` does this for any store error.

## Consequences

- A double tap on Confirm, Cancel or Publish changes nothing the second time, without a read-then-write race.
- The winner draw reverses this order on purpose (ADR 0002).
- A deleted post: removing its button counts as success (`ignoreGone`), so a creator can cancel after deleting the post; putting a button on it returns `ErrPostGone`. Match Telegram's error texts against a real channel: channel posts and private messages answer differently.
