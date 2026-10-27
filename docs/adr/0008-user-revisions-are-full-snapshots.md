# 0008. User revisions are full snapshots, written by a conditional update

Status: accepted, 2026-10-09.

## Context

The bot keeps a history of every Telegram user it sees, so later features can tell who a user was at a given moment. Rejected: diff-only rows (a row cannot be read on its own), an audit log of every write (breaks "a revision only on change"), one struct embedded in both `User` and `Revision` (changes the schema shape), and a profile cache.

## Decision

- A revision is a full snapshot of `user.Profile`, only ever inserted, numbered per user from 1 with no gaps (`idx_user_revisions_number`). `created_at` is when the bot recorded it, not when the user changed it.
- Bot state (`users.language`, `pending_command`, `keyboard_message_id`) is not part of the profile and never writes a revision.
- `Store.Record` writes first: an insert `ON CONFLICT DO NOTHING`, then one `UPDATE … WHERE <changedProfile>`. The database compares, and the row lock queues concurrent calls, so two of them can never read the same revision number. The snapshot is copied from the saved row, which keeps the language code when Telegram leaves it out.

## Consequences

Adding a profile field touches, together: `Profile` and the middleware's `recordSender` mapping; `User` and `Revision` (a `not null` column with a default); `changedProfile` and its positional arguments in `Record`; the `Updates` map and the `Revision` literal in `Record`; `TestMiddlewareRecordsFullProfile`. A field missed in `changedProfile` is silently not tracked. Then hand-check `Record` against a real database ([hand-check-store-sql](../hand-check-store-sql.md)).
