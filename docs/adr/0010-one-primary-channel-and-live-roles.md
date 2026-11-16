# 0010. One primary channel, with roles read live from Telegram

Status: accepted, 2026-11-16.

## Context

The bot served any channel where it was an admin, and anyone who opened the Mini App got the admin interface. The app checked rights only when someone created a giveaway or acted in results review, and it accepted any channel where both the caller and the bot were admins. So any channel that added the bot with posting rights could run giveaways through it, and an admin of @read_wine whom the owner never meant to run giveaways could still create, cancel, reroll and publish them.

## Decision

The bot runs giveaways for one primary channel, named by its numeric chat ID in `PRIMARY_CHANNEL_ID`, so renaming the channel changes nothing. At startup the bot checks that it is an admin there with both Post messages and Edit messages of others, and refuses to start otherwise.

Every request reads the caller's member record in the primary channel from Telegram and maps it to full admin, read-only admin or partner. The role is never stored. Each operation in the giveaway domain checks the role it needs: reads need any primary admin, changes need a full admin, plus being the creator where that rule already existed. Changing the language and sending yourself the winner list stay open to read-only admins, because one saves a preference and the other messages only the caller.

Only a successful lookup of a non-admin user yields partner. Any Telegram error fails the request instead of downgrading to partner, so an outage never turns a full admin into a partner. Every other channel is a partner, and the bot's default admin rights when added are Manage chat only, so the startup check is the only guard for Post and Edit in the primary channel.

## Alternatives rejected

- Leaving channels the bot does not know, or asking the owner to approve each partner. Telegram cannot stop anyone adding the bot, and an approval flow is more to build for no gain.
- Keeping giveaway lists per creator. A read-only admin would see no giveaways at all, so every primary admin sees the whole channel.
- Starting without the rights and showing a notice in the app. A misconfigured bot is better caught at deploy time, which is the fail-fast rule.
- Caching roles or channel checks. Stale roles would delay a removal of rights, and requests stay stateless as in ADR 0009.
- A gate by HTTP method that blocks every write for read-only admins. It would also block the language setting and the winner list sent to the caller.

## Consequences

- Each request costs one `getChatMember` call to Telegram. Revisit caching if Telegram starts rate-limiting the bot.
- A change of someone's rights in Telegram applies on their next request.
- A bot removed from the primary channel exits at startup and crash-loops in Docker until it is re-added.
- A creator who lost the Post or Edit right still receives the results in private but cannot publish them.
- Giveaways in other channels are hidden and never drawn, but nothing is deleted.
