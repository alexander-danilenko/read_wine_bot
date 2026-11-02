# 0009. The Mini App API trusts only Telegram's signed launch data

Status: accepted, 2026-11-02.

## Context

The Mini App calls the bot's HTTP API from the Telegram client. Anyone can send requests to that API, so every request must prove which Telegram user sent it. Telegram gives the app `initData`: the launch data with the user's profile and launch time, signed with a key derived from the bot token. go-telegram's `ValidateWebappRequest` checks it, but it compares hashes in non-constant time, never checks the launch time, and decodes values twice, so a valid name with `+` or `%` fails.

## Decision

Every `/api/` request sends `initData` in `Authorization: tma <initData>`. `internal/webapp.Verify` checks it with the standard library: the HMAC-SHA256 key is derived from the bot token under `WebAppData`, the message is every other field sorted as `key=value` lines, and the hashes are compared with `hmac.Equal`. Launch data older than 24 hours, or more than a minute in the future, is rejected. The user ID comes only from the verified data. No session token is issued on top.

## Consequences

- Each request is checked on its own; the server keeps no session state.
- After 24 hours an open app gets 401 and asks the user to open it again from the bot. Telegram sets no limit; one hour is common, but an admin may keep the app open longer.
- Leaked launch data works until it expires. It is sent only over HTTPS to the bot's own domain.
- Unsigned parameters, such as a `?review=<id>` query on the app URL, choose what to show but never who the user is; each handler authorizes the giveaway by the verified user.
