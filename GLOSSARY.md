# Glossary

Shared domain vocabulary for this project: entities, named processes, and status concepts with project-specific meaning. It grows as terms are resolved (`/grill-with-docs`, `/domain-modeling`); direct edits are fine. Glossary only, not a spec.

## Giveaways

### Giveaway

A prize draw the bot runs for the audience of the @read_wine Telegram channel. One giveaway is bound to one channel post at a time: a channel admin creates it through the giveaway setup, and the bot attaches a join button to that post. Its creator can move it to another post of the same channel, for example after deleting the post; it keeps its participants. A giveaway has an end time and a winner count. Until the admin confirms the setup, the giveaway is unpublished: its row holds the setup inputs and its post has no button yet. Winners are picked by the winner draw.

### Giveaway setup

The guided dialogue in the bot's private chat that creates a giveaway: the admin forwards the post or sends its link, then enters the end date, end time, winner count and join button text, and confirms a summary. Nothing appears on the post until the admin confirms; cancelling discards the setup.

### Winner draw

The once-a-minute step that finishes a due giveaway: it picks distinct participants at random up to the winner count, passes each one through the candidate filter, records the winners, removes the join button and sends the results to the giveaway's creator in private for results review. A giveaway is drawn at most once; with too few eligible participants it finishes with fewer winners, possibly none.

### Results review

The step between the winner draw and the published results. The creator gets the results in private, and any current admin of the giveaway's channel can reroll winners one at a time, then publish the results as a reply to the giveaway post, or skip them to close the giveaway without posting. Publishing or skipping happens once and closes rerolls.

### Reroll

Replacing one winner during results review with a random participant who is neither a winner nor rerolled out. The replaced participant can never win that giveaway again. With nobody left to pick, the giveaway keeps one winner fewer.

### Candidate filter

The check every randomly picked participant passes before becoming a winner. A rejected candidate is set aside and the draw picks another. A check that cannot decide stops the giveaway's draw with nothing recorded, and the next draw tries again. It accepts everyone until filter rules are defined.

### Cancelled giveaway

A giveaway its creator took off the post: the join button is removed and the giveaway is unpublished again, but it keeps its participants and values. Only its creator can publish it again. A finished giveaway cannot be cancelled.

### Participant

A Telegram user who tapped a giveaway's join button. A user participates in a giveaway at most once. A participant can be marked as a winner.

### Finished giveaway

A giveaway whose winners are recorded. Passing the end time does not finish a giveaway: until winners are recorded it is due for drawing. A finished giveaway's results are unpublished until an admin publishes them in results review.

### Channel

A Telegram channel where the bot is an admin. The bot was built for @read_wine and can serve several channels. A giveaway belongs to the channel of its post: the giveaway's channel. Its audience takes part in giveaways.

### Giveaway partner

A channel that runs giveaways together with a giveaway's creator: winners are expected to subscribe to it as well. The bot is an admin there without the rights to run giveaways, only so it can check winner subscriptions. Like every channel, a partner shows in the results review of every giveaway, not only the shared ones.

### Winner subscriptions

The channels a winner is subscribed to, shown to admins in results review next to each winner. The bot checks them each time it shows the review, so a winner who unsubscribes after the draw shows up before the results are published. A channel whose check fails is shown as unknown, never as not subscribed. Every admin in results review sees every channel, including channels they do not administer.

## Users

### User profile

What Telegram tells the bot about a user: first and last name, username, Telegram language code, premium flag and whether the account is a bot. The language chosen in the bot's menu and the pending command are bot state, not part of the profile.

### User revision

A saved snapshot of a user profile. The bot writes a new revision only when the profile it receives differs from the saved one. Revisions count per user from 1, and the user row records the latest number.

## Channel audience

### Subscriber

A Telegram user subscribed to the channel. The Bot API cannot list subscribers, so the bot knows only those whose subscription changed while it was logging.

### Membership event

One change in who is subscribed: joined (with the invite link and its name at that moment), left, or removed by an admin. Events are only added, never changed, and one subscription is a joined event followed by the next left or removed event of the same user.

### Channel post

The time a post was published in the channel, kept so unsubscribes can be compared with the posts before them.
