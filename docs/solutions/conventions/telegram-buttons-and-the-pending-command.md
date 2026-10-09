---
title: Telegram buttons and the pending command
date: 2026-10-09
category: conventions
module: internal/giveaway, internal/menu, internal/user, internal/language
problem_type: convention
component: messaging
severity: medium
applies_when:
  - Adding an inline button, a callback data prefix or a callback handler
  - Adding or renaming a menu keyboard button or its locale label
  - Adding a step that waits for the user's next message or tap
  - Adding a button that changes a giveaway row or the join button on a channel post
tags: [telegram, go-telegram-bot, inline-keyboard, reply-keyboard, callback-data, pending-command, stale-button, idempotency]
---

# Telegram buttons and the pending command

## Context

The bot has no slash commands besides `/start`. Users drive it with two kinds of buttons, and the only state kept between their messages is one column, `users.pending_command`. The rules that make this work are spread over `main.go`, `internal/menu`, `internal/user/middleware.go` and the giveaway setup and edit handlers. Each rule is small, but forgetting one gives a silent bug: a handler that never fires, a tap that runs twice, or a setup Cancel that deletes the wrong draft.

The database side of the same flows (scopes, guarded `UPDATE`s, inserting from a map) is in [Giveaway CRUD around the published flag](giveaway-crud-around-the-published-flag.md). This doc covers the button side.

## Guidance

### Two kinds of buttons

| Kind | Where | What a tap sends | Routed by |
| --- | --- | --- | --- |
| Menu (reply keyboard) | Under the message field in the private chat | Its label, as an ordinary text message | One exact-match text handler per label per language (`main.go` `menuHandlers`, `labelHandlers`) |
| Inline | Under one message, including the channel post | A callback query with `CallbackData` | One prefix or exact handler per callback namespace (`main.go` `handlers`) |

### Menu buttons

- The keyboard (`internal/menu/menu.go` `Keyboard`) is sent by `/start` (`internal/greeting/greeting.go:34`) and again after a language change (`internal/language/language.go:99`). Telegram cannot relabel a keyboard that is already shown, so a new language needs a new keyboard message.
- A tap arrives as plain text, so the label is the route. Each label in every language gets its own `bot.MatchTypeExact` handler. An old keyboard in the previous language keeps working because every language's labels are registered.
- `menu.IsLabel` covers all languages. The user middleware uses it to clear `pending_command` before the handler runs (`internal/user/middleware.go:35-55`). A menu tap or any `/command` therefore ends an unfinished setup or edit, and the handler that runs next may start a new one.
- Consequence: a user can never enter a menu label as a setup or edit value, such as a join button text. The middleware treats it as a menu tap.
- Adding a menu button means a new locale key in `en.json` and `uk.json`, a row in `Keyboard`, the key in `IsLabel`, and a `labelHandlers` line in `main.go`. Miss `IsLabel` and the new button does not end a running setup. Miss `main.go` and the tap falls through to the default handler, which ignores it unless a step is pending.

### Inline buttons: callback data

Each feature owns one namespace:

| Data | Handler match | Built in |
| --- | --- | --- |
| `giveaway:join` | exact | `internal/giveaway/handler.go` `JoinCallback` |
| `giveaway-setup:cancel`, `:confirm`, `:default:<step>` | prefix `giveaway-setup:` | `internal/giveaway/setup_handler.go:17-24` |
| `giveaway-edit:<action>[:<field>]:<giveaway ID>`, `giveaway-edit:stop` | prefix `giveaway-edit:` | `internal/giveaway/edit_handler.go:17-35` |
| `language:<code>` | prefix `language:` | `internal/language/language.go:17` |

Rules:

- No namespace may be a prefix of another. go-telegram/bot runs the first registered handler whose match succeeds (`process_update.go` `findHandler` in v1.27.0). A prefix handler for `giveaway:` would take every join tap. This is why setup and edit use `giveaway-setup:` and `giveaway-edit:`, not `giveaway:setup:`.
- Telegram limits callback data to 64 bytes. The longest payload today is `giveaway-edit:default:button_text:` plus a giveaway ID, at most 54 bytes. Keep IDs numeric and action names short.
- Callback data is input from the client. Parse it strictly and treat anything unknown, including buttons from an older bot version, as stale (`parseEditData`, `edit_handler.go:44-77`). Never trust it for authorization: every edit action reloads the giveaway and re-checks the creator, finished state, admin rights and the bot's rights (`Editor.load` and `Editor.check` in `internal/giveaway/edit.go`). A code review found the edit «Ні» button showing the menu without an owner check, so even read-only buttons go through `load` (session history).
- Every callback handler calls `AnswerCallbackQuery`, also on failure. Until it does, Telegram shows a spinner on the button.

### Stale buttons

Old messages keep their buttons, and the user can tap them at any time. The pending command decides whether a tap still applies:

- `pending_command` holds what the bot waits for next: a setup step (`giveaway:<step>`, `internal/giveaway/setup.go:16-25`) or an edit step (`edit:post`, `edit:<field>:<giveaway ID>`, `internal/giveaway/edit.go:16-20`). The edit step carries the giveaway ID, so no other edit state exists between messages.
- A tap or message that moves a step claims it with `Service.claim`, one conditional `UPDATE ... WHERE pending_command = <from>` (`internal/user/user.go:67-79`, `setup.go:360-373`). Of two taps on the same button, or two posts forwarded together, exactly one moves the step. A read-then-write here was a review finding (session history).
- A tap whose step has passed returns `Reply{Stale: true}`. The handler answers with the "outdated" popup and removes that message's inline keyboard (`removeButtons`, `setup_handler.go:258-270`). Nothing else changes.
- Edit steps start with `edit:`, outside the setup prefix `giveaway:`, on purpose. Setup Cancel acts only when `inSetup` is true, then deletes the user's draft (`setup.go:276-300`). If edit steps shared the setup prefix, a setup Cancel tapped during an edit would delete an unrelated draft. The edit flow has its own `giveaway-edit:stop` instead.
- Pseudo steps `menu` and `cancel` (`edit.go:31-36`) name a message to show. They are never saved as a pending command.

### Buttons that change a giveaway

- The join button on the channel post is the outside effect. Change it first and the row second (`Editor.Cancel`, `Editor.Restore`, `Service.publish`, and a button text edit in `Editor.save`). If Telegram refuses, the row is unchanged. If the database then fails, a retry repeats the Telegram call and completes the write.
- `ignoreNotModified` (`internal/giveaway/handler.go`) turns Telegram's "message is not modified" into success, so repeating the same button edit is safe.
- A join tap on an old post may arrive as an inaccessible message. `post` reads the chat and message ID from either form (`handler.go`). The join data needs no giveaway ID because the post identifies the giveaway.

## Why This Matters

- A prefix collision or a missing label handler fails silently: the tap goes to the wrong handler or to none, and no error is logged.
- Without the conditional claim, a double tap on Confirm or a field value applies twice.
- Telegram never expires buttons. Every handler must expect a tap from any earlier step, and from a different flow.
- Button and row order decides what a partial failure leaves behind. The order above makes every failure retryable.

## When to Apply

- Before adding a callback namespace: check it against every prefix registered in `main.go`.
- Before adding a waiting step: give it a prefix that matches its flow, claim it with `MovePendingCommand`, and decide what a stale tap shows.
- Before adding a menu button: update the locales, `Keyboard`, `IsLabel` and `main.go` together.

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
