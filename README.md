<p align="center">
  <img src="docs/assets/header.svg" alt="read_wine_bot: giveaways for Telegram channels" width="100%">
</p>

<p align="center">
  <a href="https://t.me/read_wine_bot"><img src="https://img.shields.io/badge/Telegram-@read__wine__bot-26A5E4?logo=telegram&logoColor=white" alt="Telegram: @read_wine_bot"></a>
  <img src="https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white" alt="Go 1.27">
  <img src="https://img.shields.io/badge/PostgreSQL-18-4169E1?logo=postgresql&logoColor=white" alt="PostgreSQL 18">
  <img src="https://img.shields.io/badge/languages-🇺🇦%20uk%20·%20🇬🇧%20en-8B1E3F" alt="Languages: Ukrainian, English">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green" alt="License: MIT"></a>
</p>

# 🍷 read_wine_bot

[@read_wine_bot](https://t.me/read_wine_bot) is a Telegram bot that runs giveaways on channel posts. It was built for the [@read_wine](https://t.me/read_wine) channel and works in any channel that adds it as an admin.

> [!WARNING]
> The hosted [@read_wine_bot](https://t.me/read_wine_bot) has no guarantees. It can stop, lose data or change at any time. For your own channel, [host it yourself](#-run-your-own) with your own bot token.

| | |
| --- | --- |
| **What** | Giveaways on Telegram channel posts, with an admin review before the bot publishes the results |
| **Who** | Channel owners, their admins and giveaway partners |
| **Languages** | 🇺🇦 Ukrainian (default) and 🇬🇧 English, picked per user in the Mini App's settings (`/language` opens them) |
| **Stack** | Go 1.27, PostgreSQL 18; a multi-arch Docker image and static binaries for Linux, macOS and Windows |

## ✨ What it does

- 🎟️ **Join button.** Adds a join button under a channel post and collects participants.
- 🎲 **Random draw.** Draws winners at random when the giveaway ends.
- 🔍 **Review before publishing.** Channel admins see which channels each winner is subscribed to, and can replace a winner. Then they publish the results under the post, or close the giveaway without results.
- 📱 **Mini App.** Admins create, edit and review giveaways in a Telegram Mini App that opens from the chat with the bot. Its Help explains the bot and adds it to a channel; its Settings change the language and list your channels.

## 🔄 How a giveaway works

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

## 🔐 Add the bot to a channel

Send `/start` to [@read_wine_bot](https://t.me/read_wine_bot) and open the app: its **Help** screen has both links and explains each permission.

- 📢 **Channel that runs giveaways:** [add the bot as an admin](https://t.me/read_wine_bot?startchannel&admin=post_messages+edit_messages) with only **Post messages** (publishes the results) and **Edit messages of others** (adds the join button). Disable every other toggle.
- 🤝 **Giveaway partner:** [add the bot as an admin](https://t.me/read_wine_bot?startchannel&admin=manage_chat) with every permission disabled. Telegram shows subscriptions only to admins, and that is all the bot reads.

> [!NOTE]
> A partner channel appears in the partner list in the bot's app settings, which everyone who opens the app can see, and in the results review of every giveaway the bot runs, where admins of other channels see which winners subscribe to it. To end a partnership, remove the bot from your admins.

For your own bot, put its username in the links.

## 🚀 Run your own

Requirements:

- Docker with Compose.
- A bot token from [@BotFather](https://t.me/BotFather).
- A public HTTPS endpoint for the Mini App (reverse proxy or Cloudflare Tunnel) that forwards to the bot's `LISTEN_ADDR`. Register the same URL as the bot's Mini App URL in @BotFather. Telegram refuses `http://`, `localhost` and private addresses; for local work see [Test the Mini App locally](#-test-the-mini-app-locally).

Configuration is environment-only:

| Variable | Required | Description |
| --- | --- | --- |
| `TELEGRAM_BOT_TOKEN` | yes | Bot token from @BotFather |
| `DATABASE_URL` | yes | `postgres://user:password@host:port/database`; the bot creates and migrates its tables on start |
| `APP_URL` | yes | Public `https://` URL of the Mini App |
| `LISTEN_ADDR` | no | HTTP listen address for the Mini App and its API, default `:8080` |

The bot validates every setting, pings the database and calls `getMe` at startup, and exits with a descriptive error if any check fails.

### 🐳 Docker Compose

The image is `ghcr.io/alexander-danilenko/read_wine_bot`, built for `linux/amd64` and `linux/arm64`, tagged with each release version (`2.10.0`) and `latest`.

```yaml
services:
  db:
    image: postgres:18
    restart: unless-stopped
    environment: # not published outside the compose network
      POSTGRES_USER: pgusername
      POSTGRES_PASSWORD: pgpassword
      POSTGRES_DB: pgdatabasename
    volumes:
      - pgdata:/var/lib/postgresql # Postgres 18 keeps data in /var/lib/postgresql/18/docker

  bot:
    image: ghcr.io/alexander-danilenko/read_wine_bot:latest
    restart: unless-stopped
    environment:
      TELEGRAM_BOT_TOKEN: replace-with-the-bot-token
      DATABASE_URL: postgres://pgusername:pgpassword@db:5432/pgdatabasename?sslmode=disable
      APP_URL: https://bot.example.com
      LISTEN_ADDR: ":${PORT:-8080}"
    ports:
      - "${PORT:-8080}:${PORT:-8080}"
    depends_on:
      - db # the bot exits while the database is not ready, and the restart policy retries
    stop_grace_period: 2m # lets an in-flight winner draw finish its Telegram calls

  # Optional Cloudflare Tunnel: route its public hostname to http://bot:8080.
  # tunnel:
  #   image: cloudflare/cloudflared:latest
  #   restart: unless-stopped
  #   command: tunnel --no-autoupdate run
  #   environment:
  #     TUNNEL_TOKEN: replace-with-the-tunnel-token
  #   depends_on:
  #     - bot

volumes:
  pgdata:
```

Pin a version tag instead of `latest` for reproducible deploys. Upgrade with `docker compose pull && docker compose up -d`; the schema migrates on start.

<details>
<summary>📦 Release binary</summary>

[Releases](https://github.com/alexander-danilenko/read_wine_bot/releases) carry static binaries for Linux, macOS and Windows on `amd64` and `arm64`, plus `.env.example`. The bot reads `.env` from the working directory and each parent directory; a nearer file wins, and a non-empty environment variable overrides every file. On macOS, clear the quarantine flag once: `xattr -d com.apple.quarantine read_wine_bot`.

</details>

<details>
<summary>🐹 Build from source</summary>

Needs Go 1.27+ and Node.js 26. The binary embeds the Mini App, so build `web/` first; `go install` skips that step and the bot refuses to start.

```sh
npm --prefix web ci --ignore-scripts
npm --prefix web run build
go build .
```

Or build the image: `docker build -t read_wine_bot .`

</details>

### 🧪 Test the Mini App locally

The `Caddyfile` serves `https://bot-local.readwine.com.ua` with Caddy's local CA and proxies to `:8080`. This works in Telegram Desktop on the same machine only.

```sh
brew install caddy
echo "127.0.0.1 bot-local.readwine.com.ua" | sudo tee -a /etc/hosts
sudo caddy trust
sudo caddy run                               # from the repo root
docker compose up -d                         # local PostgreSQL from docker-compose.yml
npm --prefix web run build && go run .       # with APP_URL=https://bot-local.readwine.com.ua in .env
```

## 🧭 Where to read next

| File | What is in it |
| --- | --- |
| [CLAUDE.md](CLAUDE.md) | Developer notes, commands and conventions, for humans and coding agents |
| [GLOSSARY.md](GLOSSARY.md) | Domain terms, for example giveaway, winner draw, results review and reroll |
| [docs/adr/](docs/adr/) | Architecture decisions and the reasons for them |
| [docs/telegram-ux.md](docs/telegram-ux.md) | Rules for everything the user sees in Telegram |

## 📄 License

[MIT](LICENSE)
