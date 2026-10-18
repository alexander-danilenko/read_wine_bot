---
title: User profile revisions — what is tracked, how, and why
date: 2026-10-09
category: conventions
module: internal/user, internal/db
problem_type: convention
component: database
severity: medium
applies_when:
  - Adding, removing or renaming a Telegram profile field on user.Profile, user.User or user.Revision
  - Adding a users column that holds bot state, such as a new menu setting
  - Changing user.Store.Record or the user middleware's list of senders
  - Writing code that reads a user's history, such as a giveaway or spam check
tags: [user-revision, profile, history, gorm, postgres, conditional-update, telegram]
---

# User profile revisions — what is tracked, how, and why

## Context

The bot keeps a history of every Telegram user it sees, so later features can tell who a user was at a given moment: a giveaway participant's name when they joined, or a spammer's rename and username swaps. The `users` row holds the latest profile; `user_revisions` holds one full snapshot per change. The rules are spread over `internal/user/revision.go`, `internal/user/middleware.go` and `internal/db/db.go`, and several of them only hold because three or four places agree. This doc states them in one place, with the reasons that the code does not carry.

Alternatives rejected while designing it (session history): diff-only revision rows (a row cannot be read on its own), an audit log of every write (breaks "a revision only on change"), one Go struct embedded in both `User` and `Revision` (changes the schema shape), and a profile cache or `RETURNING` in `Record` (new state for a speedup nobody measured).

## Guidance

### What a revision is

- A **profile** is exactly the fields Telegram sends about another user: `is_bot`, `first_name`, `last_name`, `username`, `language_code`, `is_premium` (`user.Profile`, `internal/user/revision.go:14`). Flags Telegram fills only in the bot's own `getMe` reply, and the attachment-menu flag, are left out on purpose: they are never set for other users, so storing them would only store zeros.
- A **revision** is a full snapshot of the profile, not a diff, so any one row reads on its own (`user.Revision`, `internal/user/revision.go:26`). Revisions are only ever inserted.
- Numbers count **per user from 1**, with no gaps. `users.revision` holds the latest number (`0` means "none yet"). A unique index on `(user_id, number)` (`idx_user_revisions_number`) rejects a duplicate if a bug ever produces one.
- **Bot state never creates a revision.** The language chosen in the menu (`users.language`) and `users.pending_command` are not part of the profile. The pending command changes on every giveaway setup step and would flood the history; the menu language is the bot's setting, not who the user is.
- `created_at` on a revision is when the bot **recorded** that profile, not when the user changed it. Telegram does not say when a rename happened; the bot only learns it at the user's next update.

### How `Record` writes it

`Store.Record` (`internal/user/revision.go:60`) does everything in one transaction whose first statement is always a write:

1. `INSERT` the user with `revision = 1`, `ON CONFLICT DO NOTHING`. One row affected: a new user, so insert revision 1 and stop.
2. Otherwise one conditional `UPDATE users SET <profile>, revision = revision + 1 WHERE telegram_id = ? AND (<any profile column differs>)` (`changedProfile`, `internal/user/revision.go:54`). Zero rows affected: nothing changed, stop. The database does the comparison, so there is no read-compare-write race between two updates from the same user.
3. One row affected: read the updated row back and copy the snapshot **from the saved row**, not from the incoming `Profile`, because the saved row holds any language code kept in rule 4 below.

Why this shape:

- **Write first.** The conditional `UPDATE` takes the row lock before anything is read, so two concurrent `Record` calls for one user queue on it, and the second re-checks the changed-profile condition against the first one's committed row. Neither can read a stale revision number.
- **Empty language code keeps the saved one.** Telegram leaves `language_code` out of some updates. `keptLanguageCode` (`COALESCE(NULLIF(?, ''), language_code)`, `internal/user/revision.go:50`) is used in both the comparison and the update, so a missing code is neither a change nor an overwrite. Every other field is taken as sent: a removed last name or username **is** a change.

### Where profiles come from

The middleware (`internal/user/middleware.go:56`) records the sender of `Message` (any chat type), `EditedMessage`, `CallbackQuery`, `MyChatMember` and `ChatJoinRequest`. It skips a message with `SenderChat` set: posts made as the channel, anonymous group admins and the channel's automatic forwards into the discussion chat carry a placeholder `From` account, which would otherwise collect a revision history of its own. `chat_member` updates are out of scope. A `Record` error is logged and the handler still runs. Group messages only reach the bot when it is a group admin or has privacy mode off in BotFather; that is an operational step, not code.

### Users without a revision

A row created by `SetLanguage` or `SetPendingCommand` has revision 0 and an empty profile. Telegram always sends a first name, so its next `Record` differs and lands as revision 1 through step 2. The SQLite-era `BackfillRevisions` was dropped with the move to PostgreSQL, which started empty.

## Why This Matters

- A field added to `Profile` but missed in `changedProfile` is silently not tracked: a change to only that field matches no row, so the `users` row keeps the old value and no revision is written. Existing tests name only the current fields, so they are unlikely to catch it.
- A bot-state column added to `changedProfile` floods the history with a revision per setup step.
- Building the snapshot from the incoming `Profile` instead of the saved row stores an empty language code whenever Telegram omits it.
- Swapping the conditional `UPDATE` for "read the row, compare in Go, then write" makes the transaction read first, which lets two concurrent calls read the same revision number and collide on the unique `(user_id, number)` index.

## When to Apply

Read this before touching the user profile, its tables, the user middleware, or before building a feature on top of the history.

## Examples

Adding a profile field (say Telegram starts sending `emoji_status` for other users) means one edit in each of these places, then a manual check against a real database that a change to only that field writes a new revision (store SQL has no automated tests):

1. `Profile` and the middleware's `recordSender` mapping (`internal/user/middleware.go:85`).
2. `User` and `Revision`: a `not null` column with a default, because both tables already have rows.
3. `changedProfile` and its argument list in `Record`: the `?` placeholders are positional, so the order of arguments must match the order in the SQL string.
4. The `Updates` map in `Record` and the `Revision` literal at the end of it.
5. The test fixture that lists every field: the full profile in `TestMiddlewareRecordsFullProfile` (`internal/user/middleware_test.go`).

Adding a bot-state column (say a notification setting) touches `User` and its own setter only. It stays out of `Profile`, `Revision` and `changedProfile`.

## Related

- [Document columns with Go comments, not GORM comment tags](document-columns-with-go-comments-not-gorm-comment-tags.md): why `Revision.UserID` is not named `TelegramID`, and how the `users` columns are documented.
- [Telegram buttons and the pending command](telegram-buttons-and-the-pending-command.md): the pending command that revisions deliberately ignore.
- Plan: `docs/plans/2026-10-09-2205-feat-user-profile-revisions-plan.md`.
