# Concepts

Shared domain vocabulary for this project: entities, named processes, and status concepts with project-specific meaning. Seeded with the terms known at bootstrap; it grows as ce-compound and ce-compound-refresh process learnings, and direct edits are fine. Glossary only, not a spec.

## Giveaways

### Giveaway

A prize draw the bot runs for the audience of the @read_wine Telegram channel. One giveaway is bound to one channel post: a channel admin creates it through the giveaway setup, and the bot attaches a join button to that post. A giveaway has an end time and a winner count. Until the admin confirms the setup, the giveaway is unpublished: its row holds the setup inputs and its post has no button yet. Winner selection is not defined yet.

### Giveaway setup

The guided dialogue in the bot's private chat that creates a giveaway: the admin forwards the post, then enters the end date, end time, winner count and join button text, and confirms a summary. Nothing appears on the post until the admin confirms; cancelling discards the setup.

### Participant

A Telegram user who tapped a giveaway's join button. A user participates in a giveaway at most once. A participant can be marked as a winner.

### Finished giveaway

A giveaway whose winners are recorded. Passing the end time does not finish a giveaway: until winners are recorded it is due for drawing.

### Channel

The @read_wine Telegram channel the bot serves. Its audience takes part in giveaways.
