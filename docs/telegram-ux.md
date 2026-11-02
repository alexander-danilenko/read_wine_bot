# Telegram UX

The chat is the bot's only UI. Keep it clean and keep every button working.

- One live message with buttons per private chat. Send inline buttons only through `keyboard.Tracker`: `Send` deletes the previous message with buttons and remembers the new one in `users.keyboard_message_id`. Any command deletes it too (`user.Middleware`). Notices without buttons stay as the history of what happened.
- A tap whose next step is another prompt edits the tapped message in place (`Tracker.Replace`). A tap that ends in a notice deletes the tapped message, and the notice goes below.
- A tap on a bot link (`t.me/<bot>?start=<payload>`) sends `/start <payload>` as the user. Its handler deletes that message first (`dropLinkTap`), and its next prompt is a new message, because the command already deleted the live message with buttons. A link that no longer applies answers with a notice: a message shows no popup.
- Notices about the user's own action go without a sound (`disable_notification`). Before a reply that waits on Telegram, such as checking a forwarded post, show "typing…" (`sendChatAction`).
- `/cancel` leaves any step, like every command does. Register every command with `setMyCommands` in every locale (`command.Register`), at startup.
- Answer every callback query, even when nothing changes, or Telegram keeps a spinner on the button. The default handler answers buttons no handler matches, such as those left from 1.x.
- Every step that waits for input has a Cancel button. An action that cannot be undone asks for confirmation first, and a second tap on the same confirmation is stale, not a repeat.
- Offer the likely value as a button. A typed value that does not parse keeps the step and names the expected format.
- Every text the user sees comes from `internal/i18n` in every locale. Error texts say what to do next and never show internals.
- Callback data fits Telegram's 64 bytes and starts with its handler's prefix, which overlaps no other prefix. Start-link payloads follow the same rule within Telegram's 64 characters of `A-Za-z0-9_-`. Tests enforce both.
- Messages with links use HTML parse mode and disable link previews.
- Command replies (`command.reply`) and screens with buttons are HTML. Notices (`notify` in `internal/giveaway`) are plain text, because some carry what the user typed, such as a button label: a notice text holds no markup.
- Name a Telegram admin right exactly as Telegram's UI labels it in that locale:

| Right | en | uk |
| --- | --- | --- |
| `can_post_messages` | Post messages | Публікувати повідомлення |
| `can_edit_messages` | Edit messages of others | Змінювати повідомлення інших |
| `can_delete_messages` | Delete messages of others | Видаляти повідомлення інших |
| `can_invite_users` | Add members | Додавати учасників |
| `can_restrict_members` | Ban users | Блокувати користувачів |

- Main actions live in Telegram's command menu (≡ next to the message field), each with an emoji at the start of its description so the eye finds it. `/start` lists them as tappable commands. The bot sends no reply keyboard.
- The Mini App opens only with an inline `web_app` button or the menu button, the launch types that carry signed `initData` ([ADR 0009](adr/0009-mini-app-api-trusts-only-initdata.md)): the chat's menu button (`command.SetMenuButton`), the button under the `/start` reply, the buttons under the replies to `/help` (`APP_URL?help`), `/list` (`APP_URL`), `/new` (`APP_URL?new`) and `/language` (`APP_URL?settings`) (`command.ScreenHandler`), and "Review in the app" under the results review sent after a draw (`APP_URL?review=<id>`). Never open it from a reply-keyboard `web_app` button: Telegram sends that launch no `initData`.
- Inline `web_app` buttons work only in private chats, so a channel post never carries one; the join button stays a callback button.

## How it is wired

The chat keeps `/start`, the commands that open a Mini App screen and the join button; everything else, help and the language included, happens in the Mini App, whose API trusts only Telegram's signed launch data ([ADR 0009](adr/0009-mini-app-api-trusts-only-initdata.md)). The database side of giveaway changes is in [ADR 0006](adr/0006-button-first-then-row.md) and [ADR 0007](adr/0007-giveaway-states-in-one-table.md).

### Commands

- Adding a command means a constant and a `menu()` entry in `internal/command`, an `i18n` description key in every locale, and a line in `handlers` in `main.go`. Miss `main.go` and the command shows in the menu but gets no answer.
- `user.Middleware` runs before every handler. A private message starting with `/` deletes the live message with buttons, so only the command's answer is live.

### Callback data

- Each feature owns one namespace (`giveaway:join`, `language:`). No namespace may be a prefix of another: a prefix handler for `giveaway:` would take every join tap.
- Callback data is client input. Parse it strictly and treat anything unknown, including buttons from an older bot version, as stale.

### Mini App actions

- Every API action reloads the giveaway and re-checks the creator, its state and the admin and bot rights (`Editor.load`, `Editor.check`, `Reviewer.load`), so a query parameter such as `?review=<id>` only chooses a screen.
- An action that cannot be undone (publish or close the results, end now, cancel) asks for confirmation in the app first. A review that changed since it was shown answers `stale`, and the app reloads it.
