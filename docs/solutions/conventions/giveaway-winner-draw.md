---
title: How the giveaway winner draw works, and what to keep when changing it
date: 2026-10-09
category: conventions
module: internal/giveaway, internal/cron
problem_type: architecture_pattern
component: background_job
severity: medium
applies_when:
  - Changing how winners are picked, saved or announced
  - Adding a candidate filter rule, such as skipping bots or users who left the channel
  - Adding a retry, a re-draw or any second write to a finished giveaway
  - Letting the creator edit, cancel or restore a giveaway near its end time
  - Changing the bot's shutdown or the container's stop timeout
tags: [giveaway, winner-draw, candidate-filter, cron, background-job, telegram, save-first, finished_at, shutdown, race]
---

# How the giveaway winner draw works, and what to keep when changing it

## Context

The winner draw finishes due giveaways without anyone acting. The owner made several product decisions in the 2026-10-09 brainstorm that the code follows but does not explain, and code review found gaps that were accepted rather than fixed. Neither is visible from the code alone. The full requirements and decisions are in [the plan](../../plans/2026-10-09-2254-feat-giveaway-winner-draw-plan.md). This doc keeps the reasoning and the known gaps for whoever changes the draw next.

## Guidance

### The pass, in order

`main` runs `Drawer.DrawDue` through `cron.Every(ctx, time.Minute, …)` (`main.go:86`). `cron.Every` runs it once at startup and then once a minute, and never overlaps two passes (`internal/cron/cron.go:12`). One pass:

1. `Store.Due` lists published, unfinished giveaways whose `ends_at` is at or before now (`internal/giveaway/store.go:224`). The startup pass therefore catches giveaways that ended while the bot was down. Drafts and cancelled giveaways are unpublished, so they are never due.
2. `pick` loads every participant, shuffles them once, and walks the list through the `Filter` until it has the winner count (`internal/giveaway/draw.go:123-148`). A shuffle draws without replacement, so nobody can win twice and no "pick again on a repeat" loop is needed. With too few participants who pass, the giveaway finishes with fewer winners, possibly none.
3. `Store.Finish` sets `finished_at` and the winners' `giveaway_participants.winner` marks in one transaction (`store.go:257`). It is guarded on the giveaway still being due, so a cancel or a later end time saved after step 1 wins, and `ErrNotFound` makes the pass skip the giveaway quietly (`draw.go:106`).
4. Only then does `announce` remove the join button and reply to the post, using `context.WithoutCancel` so a shutdown signal does not stop it halfway (`draw.go:119`, `153`). If the channel refuses a message, the creator gets the post link and every results message privately, once (`draw.go:165`, `172`).

### Rules to keep

- **Winners are saved before anything is sent to Telegram.** This deliberately reverses the "button first, then the row" order used by Cancel and Restore (see [the published-flag convention](giveaway-crud-around-the-published-flag.md)). Saving first means a failed Telegram call can never lead to a second draw that picks different winners. A finished giveaway is never drawn again.
- **The participant's `winner` mark is the only record of a win.** The owner chose this over a separate `giveaway_winners` table. Do not add a second store of winners.
- **No retry.** The owner chose a single private fallback over retrying the channel post every minute. If a retry is ever wanted, it needs its own "results posted" state, because `finished_at` already means "winners saved".
- **A filter error stops that giveaway's draw with nothing saved** (`draw.go:30-32`). The giveaway stays due and the next minute shuffles again. A rule that calls Telegram, such as "still in the channel", should return an error on a failed call rather than `false`. Otherwise a network blip silently disqualifies people.
- **Times reach SQL in UTC.** See [Compare times in SQLite queries only in UTC](sqlite-time-comparisons-in-utc.md).

### Known gaps, accepted on 2026-10-09

- **Saved but never announced.** If the process dies between `Finish` and the announcement, the winners are saved, the giveaway is finished, the button stays on the post, and nothing posts the results. Each Telegram call can take up to the bot client's one-minute timeout, and `main` waits for the draw before closing the database (`main.go:90`). A container stop that kills the process sooner causes this. Give the container a stop grace period well above Docker's 10-second default. The same outcome follows when both the channel reply and the private fallback fail; then the only trace is an error log line.
- **An edit can put the button back.** `Editor.check` refuses only finished giveaways (`internal/giveaway/edit.go:506-511`). This is the earlier decision that a giveaway can be edited until its winners are recorded. If the creator saves a new button text in the up-to-one-minute gap after the end time, `AttachButton` (`edit.go:394`) can run after the draw removed the button. The finished post then has a button whose taps answer "ended". Blocking edits once the end time passes would change that earlier decision, so ask the owner first.
- **A tap racing the draw.** `Service.Join` checks the end time (`internal/giveaway/service.go:108`) and then inserts in a separate statement. A tap that passes the check just before the end can land after the draw read the participants. That user is recorded but cannot win. The window is milliseconds.
- **A partly posted result.** When a long result is split and the channel refuses a later message, the channel keeps the earlier part and the creator gets the full copy.

## Why This Matters

Each rule above has an alternative that looks simpler and breaks a guarantee. Posting before saving lets one bad Telegram call re-roll the winners. A retry that reuses `finished_at` can never tell "saved" from "announced". A filter that returns `false` on errors removes real participants without a trace. The known gaps are cheap to rediscover only if someone remembers they were accepted. Otherwise the first unannounced giveaway looks like a new bug.

## When to Apply

Read this before touching `internal/giveaway/draw.go`, the `Due`, `Candidates` or `Finish` store methods, the join-after-end check, the edit flow's lock rule, or the shutdown sequence in `main.go`.

## Examples

A future "still in the channel" filter, shaped to keep the error contract:

```go
func StillSubscribed(tg MemberChecker) Filter {
    return func(ctx context.Context, g Giveaway, u user.User) (bool, error) {
        ok, err := tg.IsMember(ctx, g.ChatID, u.TelegramID)
        if err != nil {
            return false, fmt.Errorf("check membership of user %d: %w", u.TelegramID, err) // the draw retries next minute
        }

        return ok, nil
    }
}
```

## Related

- [Giveaway CRUD around the published flag](giveaway-crud-around-the-published-flag.md): the states a giveaway row can be in, and the guarded `UPDATE` pattern `Finish` follows.
- [SQLite runs on one connection; transactions use only tx and start with a write](sqlite-one-connection-and-write-first-transactions.md): the rules the `Finish` transaction follows.
- [Compare times in SQLite queries only in UTC](sqlite-time-comparisons-in-utc.md).
