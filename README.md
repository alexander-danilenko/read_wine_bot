<p align="center">
  <img src="docs/assets/header.svg" alt="read_wine_bot: one bot, one channel, many tools" width="100%">
</p>

<p align="center">
  <a href="https://t.me/read_wine_bot"><img src="https://img.shields.io/badge/Telegram-@read__wine__bot-26A5E4?logo=telegram&logoColor=white" alt="Telegram: @read_wine_bot"></a>
  <img src="https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white" alt="Go 1.27">
  <img src="https://img.shields.io/badge/PostgreSQL-18-4169E1?logo=postgresql&logoColor=white" alt="PostgreSQL 18">
  <img src="https://img.shields.io/badge/languages-🇺🇦%20uk%20·%20🇬🇧%20en-8B1E3F" alt="Languages: Ukrainian, English">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green" alt="License: MIT"></a>
</p>

# 🍷 read_wine_bot

One bot, one channel. [@read_wine_bot](https://t.me/read_wine_bot) is the helper of the [@read_wine](https://t.me/read_wine) Telegram channel: it does routine jobs for that channel and its admins. Giveaways are the first job; more will follow.

The bot is not a service for other channels. Each copy works for exactly one channel, its **primary channel**, set by `PRIMARY_CHANNEL_ID`. Other channels can only connect as giveaway partners. To get the same helper for your channel, [host your own copy](#-self-hosting) with your own bot token.

| | |
| --- | --- |
| **For** | One primary channel and its admins; partner channels take part in its giveaways |
| **Jobs** | Giveaways and channel analytics |
| **Languages** | 🇺🇦 Ukrainian (default) and 🇬🇧 English, picked per user in the Mini App's settings (`/language` opens them) |
| **Stack** | Go 1.27, PostgreSQL 18, a React Mini App embedded in one static binary |

## ✨ What it does for the channel

- 🎁 **Giveaways.** Adds a join button under a channel post, draws winners at random when the giveaway ends, and lets admins review the winners before it publishes the results under the post.
- 📈 **Channel analytics.** Shows how the channel grows: which invite links and ads bring subscribers who stay, and how each post affects subscriptions.

Admins work in a Telegram Mini App that opens from the chat with the bot.

## 👥 Who sees what

The bot reads each user's role from Telegram on every request, so a change of rights applies at once.

| Role | Who | What they can do |
| --- | --- | --- |
| **Full admin** | The owner of the primary channel, or its admin with both **Post messages** and **Edit messages of others** | Create giveaways, review and publish results. Changing or cancelling a giveaway also needs being its creator |
| **Read-only admin** | Any other admin of the primary channel | See every giveaway and results review, change nothing |
| **Partner** | Everyone else | See one screen: their channels connected to the bot, and the link to connect another |

## 🎁 How a giveaway works

```mermaid
flowchart LR
    A["📱 Admin opens the app<br/>and pastes a post link"] --> B["⚙️ Form<br/>end time, winners, button text"]
    B --> C["🎟️ Join button<br/>appears under the post"]
    C --> D["👆 Subscribers tap<br/>to join"]
    D --> E["⏰ End time:<br/>random draw"]
    E --> F["🔍 Admins review<br/>and replace winners"]
    F --> G["📣 Publish results<br/>under the post"]
    F --> H["🤫 Or close<br/>without publishing"]
```

During the review, admins see which partner channels each winner subscribes to.

## 🤝 Connect a partner channel

A partner channel lets the bot check which winners subscribe to it. The bot only reads subscriptions there and never posts.

1. Send `/start` to the bot and open the app.
2. Open the **Connect a channel** link in Telegram and pick your channel. It adds the bot as an admin with every permission off; Telegram shows subscriptions only to admins.
3. Tap **Refresh**. Your channel appears in the list, with a warning if the bot has more rights than it needs.

To end a partnership, remove the bot from your channel's admins.

## 🚀 Self-hosting

Each copy of the bot serves one channel, so your channel needs its own bot.

### Prerequisites

- A server reachable from the internet, with a domain that serves HTTPS and proxies to the bot's port (`8080` by default). Telegram loads the Mini App on users' devices, so `localhost` or a private address does not work.
- PostgreSQL 18.
- A bot token from [@BotFather](https://t.me/BotFather).
- Your channel's ID: forward a channel post to [@userinfobot](https://t.me/userinfobot). The bot replies with a negative number, for example `-1001234567890`. It stays the same when you rename the channel.

### Setup

1. Add the bot to your channel as an admin with only **Post messages** and **Edit messages of others**: `https://t.me/<your_bot>?startchannel&admin=post_messages+edit_messages`.
2. Download the archive for your platform from the [latest release](https://github.com/alexander-danilenko/read_wine_bot/releases/latest) and unpack it. The binary has the Mini App built in.
3. Copy `.env.example` to `.env` next to the binary and fill it in (table below).
4. Run `./read_wine_bot`.

| Variable | Required | Description |
| --- | --- | --- |
| `TELEGRAM_BOT_TOKEN` | yes | Bot token from [@BotFather](https://t.me/BotFather) |
| `DATABASE_URL` | yes | `postgres://user:password@host:port/database`; tables are created and migrated on start |
| `APP_URL` | yes | Public `https://` URL of the Mini App |
| `PRIMARY_CHANNEL_ID` | yes | Your channel's ID from [@userinfobot](https://t.me/userinfobot) |
| `LISTEN_ADDR` | no | Listen address for the Mini App and its API, default `:8080` |

The bot checks every setting, the database and its rights in the channel at startup, and exits with an error if any check fails.

## 🛠️ Development

```sh
git clone https://github.com/alexander-danilenko/read_wine_bot.git && cd read_wine_bot
npm --prefix web ci --ignore-scripts && npm --prefix web run build
go build .
cp .env.example .env    # fill in development values, with APP_URL=https://bot-local.readwine.com.ua
echo "127.0.0.1 bot-local.readwine.com.ua" | sudo tee -a /etc/hosts
sudo caddy start        # serves the Caddyfile: local HTTPS in front of :8080
./read_wine_bot
```

## 📄 License

[MIT](LICENSE)
