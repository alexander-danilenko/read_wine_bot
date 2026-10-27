# 0001. Winner subscriptions are visible across channels

Status: accepted, 2026-10-27. Issue: #21.

## Context

Results review shows, next to each winner, which channels where the bot is an admin the winner is subscribed to (see Winner subscriptions in `GLOSSARY.md`). Any current admin of the giveaway's channel can open the review. So an admin of channel A sees whether a winner is subscribed to channel B, even when that admin has no role in B.

The alternative was to list only the channels where the viewer is also an admin: one more `getChatMember` per channel per review redraw, and no cross-channel disclosure.

## Decision

Every reviewing admin sees every channel where the bot is an admin. The bot exists to run giveaways across the @read_wine owner's own channels and their giveaway partners. A partner adds the bot precisely so its subscriptions show in other channels' reviews, and the README tells partners so before they add it.

## Consequences

- An automated security review flags `Reviewer.Subscriptions` (`internal/giveaway/subscriptions.go`) as cross-tenant disclosure. That is this decision, not a bug.
- If the bot ever serves channels with unrelated owners, revisit it: filter `AdminChannels` by whether the viewer is an admin of each channel, or group channels by owner in `bot_channels`.
- The audience log also covers every channel where the bot is an admin, partners included. The README discloses this to partners.
