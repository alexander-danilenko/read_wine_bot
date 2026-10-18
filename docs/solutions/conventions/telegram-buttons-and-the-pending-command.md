---
title: Telegram buttons and the pending command
date: 2026-10-09
last_updated: 2026-10-19
category: conventions
module: internal/giveaway, internal/command, internal/keyboard, internal/user, internal/language
problem_type: convention
component: messaging
severity: medium
applies_when:
  - Adding an inline button, a callback data prefix or a callback handler
  - Adding a slash command or a message with inline buttons in the private chat
  - Adding a step that waits for the user's next message or tap
  - Adding a button that changes a giveaway row or the join button on a channel post
tags: [telegram, go-telegram-bot, inline-keyboard, command-menu, callback-data, pending-command, stale-button, idempotency, deleted-post, compensation]
---

# Telegram buttons and the pending command

## Context

Users drive the bot with slash commands and inline buttons, and the only state kept between their messages is one column, `users.pending_command`. The rules that make this work are spread over `main.go`, `internal/command`, `internal/keyboard`, `internal/user/middleware.go` and the giveaway setup and edit handlers. Each rule is small, but forgetting one gives a silent bug: a handler that never fires, a tap that runs twice, or a setup Cancel that deletes the wrong draft.

The database side of the same flows (scopes, guarded `UPDATE`s, inserting from a map) is in [Giveaway CRUD around the published flag](giveaway-crud-around-the-published-flag.md). This doc covers the chat side.

## Guidance

### Commands and inline buttons

| Kind | Where | What it sends | Routed by |
| --- | --- | --- | --- |
| Command | Telegram's command menu (≡ next to the message field), and tappable in the `/start` reply | `/new`, `/edit`, `/list`, `/language`, `/cancel` as a text message | One `bot.MatchTypeCommandStartOnly` handler per command (`main.go` `menuHandlers`) |
| Inline | Under one message, including the channel post | A callback query with `CallbackData` | One prefix or exact handler per callback namespace (`main.go`) |

The bot sends no reply keyboard.

### Commands

- `internal/command/command.go` names every command and lists the menu ones in `menu()`. `command.Register` publishes that list with `setMyCommands` in every locale at startup. Each description starts with an emoji.
- `/start` answers with the greeting and the menu as tappable commands. An edit link from `/list` also arrives as `/start edit_<channel>_<post>`, so `EditLinkHandler` is registered before the greeting handler: the first matching handler wins.
- `user.Middleware` runs before every handler. For any private message starting with `/` it clears `pending_command` and deletes the message with buttons (`internal/user/middleware.go`). A command therefore ends an unfinished setup or edit, and the handler that runs next may start a new one. `/cancel` only confirms that.
- Consequence: a setup or edit value can never start with `/`. The middleware treats it as a command.
- Adding a command means a constant and a `menu()` entry in `internal/command`, an `i18n` description key in every locale, and a `menuHandlers` line in `main.go`. Miss `main.go` and the command is listed in the menu but falls through to the default handler, which ignores it unless a step is pending.

### One message with buttons per chat

`keyboard.Tracker` (`internal/keyboard/keyboard.go`) keeps at most one message with inline buttons in each private chat and remembers it in `users.keyboard_message_id`. `Send` deletes the previous one before sending, and `Replace` edits the tapped message in place for a step within one screen. `Clear` and `Discard` delete it. Send every inline keyboard in the private chat through the tracker, or an old message keeps buttons that no longer work.

### Inline buttons: callback data

Each feature owns one namespace:

| Data | Handler match | Built in |
| --- | --- | --- |
| `giveaway:join` | exact | `internal/giveaway/handler.go` `JoinCallback` |
| `giveaway-setup:cancel`, `:confirm`, `:default:<step>` | prefix `giveaway-setup:` | `internal/giveaway/setup_handler.go:18-27` |
| `giveaway-edit:<action>[:<field>]:<giveaway ID>`, `giveaway-edit:stop` | prefix `giveaway-edit:` | `internal/giveaway/edit_handler.go:26-45` |
| `language:<code>` | prefix `language:` | `internal/language/language.go:17` |

Rules:

- No namespace may be a prefix of another. go-telegram/bot runs the first registered handler whose match succeeds (`process_update.go` `findHandler` in v1.27.0). A prefix handler for `giveaway:` would take every join tap. This is why setup and edit use `giveaway-setup:` and `giveaway-edit:`, not `giveaway:setup:`.
- Telegram limits callback data to 64 bytes. The longest payload today is `giveaway-edit:default:button_text:` plus a giveaway ID, at most 54 bytes. Keep IDs numeric and action names short.
- Callback data is input from the client. Parse it strictly and treat anything unknown, including buttons from an older bot version, as stale (`parseEditData`, `edit_handler.go:59-88`). Never trust it for authorization: every edit action reloads the giveaway and re-checks the creator, finished state, admin rights and the bot's rights (`Editor.load` and `Editor.check` in `internal/giveaway/edit.go`). A code review found the edit «Ні» button showing the menu without an owner check, so even read-only buttons go through `load` (session history).
- Every callback handler calls `AnswerCallbackQuery`, also on failure. Until it does, Telegram shows a spinner on the button.

### Stale buttons

Old messages keep their buttons, and the user can tap them at any time. The pending command decides whether a tap still applies:

- `pending_command` holds what the bot waits for next: a setup step (`giveaway:<step>`, `internal/giveaway/setup.go:16-26`) or an edit step (`edit:post`, `edit:<field>:<giveaway ID>`, `edit:move:<giveaway ID>`, `internal/giveaway/edit.go:16-40`). The edit step carries the giveaway ID, so no other edit state exists between messages.
- A tap or message that moves a step claims it with `Service.claim`, one conditional `UPDATE ... WHERE pending_command = <from>` (`Store.MovePendingCommand`, `internal/user/user.go:68-82`, called by `Service.claim`, `setup.go:377`). Of two taps on the same button, or two posts forwarded together, exactly one moves the step. A read-then-write here was a review finding (session history).
- A tap whose step has passed returns `Reply{Stale: true}`. The handler answers with the "outdated" popup and deletes the tapped message (`kb.Discard`, `setup_handler.go:178`, `edit_handler.go:265`). Nothing else changes.
- The popup exists only on the callback path (`internal/giveaway/edit_handler.go:253`). On the private-message path `editHandler.reply` returns early for a stale reply and sends nothing (`edit_handler.go:314`). A step that waits for a forwarded post or typed text must therefore answer a situation that changed meanwhile, such as an end time that passed, with a notice rather than `Stale`. A lost `claim` race is the one deliberately silent case: the other message of the pair already got the answer. The move step had this gap: a post forwarded after the giveaway's end time got no answer at all, until it returned the "finishing" notice (`internal/giveaway/edit.go:665`).
- `Service.claim` moves the pending command to its target, usually `""` (`internal/giveaway/setup.go:377`). A branch that fails after the claim and wants the user to send again must save the step back; otherwise the prompt still shows its Cancel button, but the next message is dropped because no step is pending. `Editor.move` does this in its `retry` closure (`edit.go:705`).
- Edit steps start with `edit:`, outside the setup prefix `giveaway:`, on purpose. Setup Cancel acts only when `inSetup` is true, then deletes the user's draft (`Service.Cancel`, `setup.go:280`). If edit steps shared the setup prefix, a setup Cancel tapped during an edit would delete an unrelated draft. The edit flow has its own `giveaway-edit:stop` instead.
- Pseudo steps `menu`, `cancel` and `finish` (`edit.go:32-37`) name a message to show. They are never saved as a pending command.

### Buttons that change a giveaway

- The join button on the channel post is the outside effect. Change it first and the row second (`Editor.Cancel`, `Editor.Restore`, `Service.publish`, `Editor.move`, and a button text edit in `Editor.save`). If Telegram refuses, the row is unchanged. If the database then fails with a transient error, a retry repeats the Telegram call and completes the write.
- A retry does not help when the write fails because the row's state changed: the guarded `UPDATE` matches nothing (`ErrNotFound`, for example the winner draw finished the giveaway between the edit's checks and the write), or a unique index refuses it (`ErrExists`). When the Telegram change put a join button on a post the row will not point to, undo that change on every write failure. Otherwise a live-looking button stays on a public channel post, and every subscriber tap fails in `Service.Join`. `Editor.move` removes the button it just attached whenever the store returns any error (`edit.go:728`). A code review of the move found the first version undid it only for `ErrExists`.
- Telegram answers an edit of a deleted channel post with a 400 `MESSAGE_ID_INVALID`, and a deleted message in a private chat with `message to edit not found`; `isGone` (`internal/giveaway/handler.go:195`) matches both. Check such texts against a real channel: the first fix matched only the private-chat text, and cancel kept failing in production. Removing a button from such a post counts as success (`ignoreGone`, `internal/giveaway/handler.go:176`), so a creator can cancel a giveaway after deleting its post. Putting a button on one returns `ErrPostGone` (`markGone`, `handler.go:185`), which the edit flow shows as "this post no longer exists, move the giveaway", not as missing admin rights. Before this, cancel stopped on the error and told the creator to grant rights the bot already had.
- `ignoreNotModified` (`internal/giveaway/handler.go`) turns Telegram's "message is not modified" into success, so repeating the same button edit is safe.
- A join tap on an old post may arrive as an inaccessible message. `post` reads the chat and message ID from either form (`handler.go`). The join data needs no giveaway ID because the post identifies the giveaway.

## Why This Matters

- A prefix collision or a missing command handler fails silently: the tap goes to the wrong handler or to none, and no error is logged.
- Without the conditional claim, a double tap on Confirm or a field value applies twice.
- Telegram never expires buttons. Every handler must expect a tap from any earlier step, and from a different flow.
- Button and row order decides what a partial failure leaves behind. The order above makes a transient failure retryable. A failure caused by changed state is not retryable, so the Telegram change is undone instead.
- A stale reply is silent outside the callback path, so a message-driven step that returns one leaves the user with no answer and no error in the log.

## When to Apply

- Before adding a callback namespace: check it against every prefix registered in `main.go`.
- Before adding a waiting step: give it a prefix that matches its flow, claim it with `MovePendingCommand`, and decide what a stale tap shows.
- Before adding a command: update `internal/command`, the locales and `main.go` together.
- Before adding an action that puts a button on a post and then writes the row: list every error the write can return and undo the button for each one a retry cannot fix.
- Before returning `Stale` from a function a message handler calls: check whether the user would see anything.

## Examples

A new edit action, end to end:

```go
// edit_handler.go: data under the existing namespace, ID last.
const editArchive = "archive"
button(i18n.EditArchive, editData(editArchive, rawID)) // giveaway-edit:archive:42

// parseEditData already accepts <action>:<ID>; act routes it.
case editArchive:
    return h.ed.Archive(ctx, tg, userID, a.id)

// edit.go: reload and re-check, refuse the wrong state as stale.
func (e *Editor) Archive(ctx context.Context, tg Telegram, userID int64, id uint) Reply {
    g, r, ok := e.load(ctx, tg, userID, id)
    if !ok {
        return r
    }
    if !g.Published {
        return Reply{Stale: true}
    }
    // Telegram first, then a guarded UPDATE; ErrNotFound -> Reply{Stale: true}.
}
```

Wrong: a namespace that swallows another.

```go
bot.WithCallbackQueryDataHandler("giveaway:", bot.MatchTypePrefix, archiveHandler) // also matches giveaway:join
```

## Related

- [Giveaway CRUD around the published flag](giveaway-crud-around-the-published-flag.md): scopes, guarded state changes and the `default:true` insert trap behind these buttons.
