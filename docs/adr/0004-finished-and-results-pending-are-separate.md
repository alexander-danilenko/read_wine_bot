# 0004. finished_at means "winners saved", results_pending means "not yet in the channel"

Status: accepted, 2026-10-23. Results review.

## Context

The owner checks winners by hand against partner channels the bot cannot read, so results reach the channel only when an admin of the giveaway's channel taps Publish. Reusing `finished_at` for "published" could never tell "saved" from "posted".

## Decision

`finished_at` is set by the draw; `results_pending` stays true until an admin publishes or skips. The draw never posts to the channel and never retries a post. Publish is its own guarded write:

- `ClaimResults` locks the giveaway row, then counts rerolled-out participants in a separate statement (a subquery inside the `UPDATE` would read the pre-reroll snapshot under READ COMMITTED). The count must match what the admin saw, so a publish confirmed on a review older than another admin's reroll is stale.
- Publish claims, posts, and gives the claim back when the first message fails. A deleted post gets the results as a standalone message.
- A reroll is chosen outside the transaction and re-checked inside it under `SELECT … FOR UPDATE`, so two admins racing onto the same replacement get one success and one stale tap.

## Consequences

Accepted gaps, each one Telegram call wide:

- A claim lost in a crash between the claim and the first channel message: the results count as published and nothing reaches the channel.
- Telegram accepts the first results message but the bot sees a timeout: the claim is given back and a second Publish posts the results again.
- A long result split into several messages, and the channel refuses a later one: the earlier part stays, the results count as published, and the admin is told to post the rest by hand.
