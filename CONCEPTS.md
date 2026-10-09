# Concepts

Shared domain vocabulary for this project: entities, named processes, and status concepts with project-specific meaning. Seeded with the terms known at bootstrap; it grows as ce-compound and ce-compound-refresh process learnings, and direct edits are fine. Glossary only, not a spec.

## Giveaways

### Giveaway

A prize draw the bot runs for the audience of the @read_wine Telegram channel. One giveaway is bound to one channel post: a channel admin creates it through the giveaway setup, and the bot attaches a join button to that post. A giveaway has an end time and a winner count. Until the admin confirms the setup, the giveaway is unpublished: its row holds the setup inputs and its post has no button yet. Winners are picked by the winner draw.

### Giveaway setup

The guided dialogue in the bot's private chat that creates a giveaway: the admin forwards the post, then enters the end date, end time, winner count and join button text, and confirms a summary. Nothing appears on the post until the admin confirms; cancelling discards the setup.

### Winner draw

The once-a-minute step that finishes a due giveaway: it picks distinct participants at random up to the winner count, passes each one through the candidate filter, records the winners, removes the join button and replies to the giveaway post with the results. A giveaway is drawn at most once; with too few eligible participants it finishes with fewer winners, possibly none.

### Candidate filter

The check every randomly picked participant passes before becoming a winner. A rejected candidate is set aside and the draw picks another. A check that cannot decide stops the giveaway's draw with nothing recorded, and the next draw tries again. It accepts everyone until filter rules are defined.

### Cancelled giveaway

A giveaway its creator took off the post: the join button is removed and the giveaway is unpublished again, but it keeps its participants and values. Only its creator can publish it again. A finished giveaway cannot be cancelled.

### Participant

A Telegram user who tapped a giveaway's join button. A user participates in a giveaway at most once. A participant can be marked as a winner.

### Finished giveaway

A giveaway whose winners are recorded. Passing the end time does not finish a giveaway: until winners are recorded it is due for drawing.

### Channel

The @read_wine Telegram channel the bot serves. Its audience takes part in giveaways.

## Users

### User profile

What Telegram tells the bot about a user: first and last name, username, Telegram language code, premium flag and whether the account is a bot. The language chosen in the bot's menu and the pending command are bot state, not part of the profile.

### User revision

A saved snapshot of a user profile. The bot writes a new revision only when the profile it receives differs from the saved one. Revisions count per user from 1, and the user row records the latest number.
