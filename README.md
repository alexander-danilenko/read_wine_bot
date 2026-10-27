# read_wine_bot

[@read_wine_bot](https://t.me/read_wine_bot) is a Telegram bot that runs giveaways on channel posts. It was built for the [@read_wine](https://t.me/read_wine) channel and works in any channel that adds it as an admin.

What it does today:

- Adds a join button under a channel post and collects participants.
- Draws winners at random when the giveaway ends.
- Lets channel admins review the winners before anything is posted: check which channels each winner is subscribed to, replace a winner, then publish the results under the post or close the giveaway quietly.
- Logs who subscribes to and leaves the channel, so the owner can compare unsubscribes with posts.

The bot speaks Ukrainian and English. Each user picks the language with /language.

## Who uses the bot

| Who | What they do | What they grant |
| --- | --- | --- |
| Channel owner | Adds the bot to the channel as an admin. | The giveaway rights below. |
| Channel admin | Creates, edits and reviews giveaways in a private chat with the bot. | Nothing. Telegram tells the bot who the channel's admins are. |
| Giveaway partner | Runs a giveaway together with a channel owner. Winners are expected to subscribe to the partner's channel too. | The partner rights below. |
| Participant | Taps the join button under the giveaway post. | Nothing. |
| Subscriber | Nothing. The bot logs subscribes and leaves in the background. | Nothing. |

## Rights to grant

Grant only the rights listed here and leave every other toggle off. The bot never asks for more.

### Channel that runs giveaways

In the channel, go to **Administrators → Add Admin → @read_wine_bot**:

- **Manage messages → Post messages**: posts the giveaway results as a reply to the giveaway post.
- **Manage messages → Edit messages of others**: adds the join button under your post, and removes it when the giveaway ends.

The bot checks both rights when an admin starts a giveaway, and stops the setup if either is missing.

Being an admin also lets the bot see who subscribes and who leaves, and check winner subscriptions. Telegram shares neither with bots that are not admins.

Leave these off. The bot does not use them:

- Manage messages → Delete messages of others
- Change channel info
- Send welcome messages
- Manage stories
- Add members
- Manage live streams
- Manage direct messages
- Add new admins
- Ban users

### Giveaway partner

A partner's channel needs **no rights at all**. Add @read_wine_bot as an admin and turn every toggle off. Telegram answers "is this user subscribed?" only to a channel's admins. That status is the only thing the bot needs from you.

With no rights, the bot cannot post, edit, delete or ban anything in your channel. It also cannot run giveaways there, because that needs Edit messages of others.

Keep in mind:

- Your channel appears in the results review of every giveaway the bot runs, in every channel. The review lists each winner's subscriptions to it. Admins of other channels see these subscriptions.
- Being an admin, the bot also logs your channel's subscribes, leaves and post times, the same way it does for the channels that run giveaways. See [What the bot stores](#what-the-bot-stores).

To end a partnership, remove the bot from your admins. It stops checking your channel right away.

## What the bot stores

The bot keeps its data in its own PostgreSQL database:

- **Users the bot sees** (admins, participants, subscribers): Telegram ID, first and last name, username, Telegram app language, Premium flag, and whether the account is a bot. When any of these change, the bot saves a new revision and keeps the old ones.
- **Giveaways**: the channel post, end time, number of winners, button text, participants, winners and replaced winners.
- **Channel audience**, for every channel where the bot is an admin: each subscribe, leave and removal with its time, the invite link a subscriber joined through and that link's name, and the time of every channel post.
- **Channels**: which channels the bot is an admin of. Titles and usernames are not stored; the bot reads them from Telegram when it shows them.

The bot cannot list a channel's existing subscribers. Telegram does not allow it. It only knows about changes that happen while it is an admin.

## Run your own

You need Go 1.27, Docker and a bot token from [@BotFather](https://t.me/BotFather).

1. `cp .env.example .env`, then set `TELEGRAM_BOT_TOKEN`.
2. `docker compose up -d` starts PostgreSQL 18.
3. `go run .` starts the bot.

The bot reads settings only from environment variables and `.env` files. It checks every setting, the database and the token before it starts, and it exits with an error that names the problem. In production, set `DATABASE_URL` to an external PostgreSQL and run the image from the `Dockerfile` with `docker run --env-file .env`.

Developer notes, commands and conventions are in [CLAUDE.md](CLAUDE.md). The domain terms are in [CONCEPTS.md](CONCEPTS.md).
