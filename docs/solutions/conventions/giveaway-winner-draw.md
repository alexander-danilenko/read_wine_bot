---
title: How the giveaway winner draw works, and what to keep when changing it
date: 2026-10-09
last_updated: 2026-10-23
category: conventions
module: internal/giveaway, internal/cron
problem_type: architecture_pattern
component: background_job
severity: medium
applies_when:
  - Changing how winners are picked, saved or announced
  - Adding a candidate filter rule, such as skipping bots or users who left the channel
  - Adding a retry, a re-draw or any second write to a finished giveaway
  - Changing results review: rerolls, publishing or the review message
  - Letting the creator edit, cancel or restore a giveaway near its end time
  - Changing the bot's shutdown or the container's stop timeout
tags: [giveaway, winner-draw, results-review, reroll, candidate-filter, cron, background-job, telegram, save-first, finished_at, shutdown, race]
---

# How the giveaway winner draw works, and what to keep when changing it

## Context

The winner draw finishes due giveaways without anyone acting. The owner made several product decisions in the 2026-10-09 brainstorm that the code follows but does not explain, and code review found gaps that were accepted rather than fixed. Neither is visible from the code alone. The full requirements and decisions are in [the plan](../../plans/2026-10-09-2254-feat-giveaway-winner-draw-plan.md). This doc keeps the reasoning and the known gaps for whoever changes the draw next.

## Guidance

### The pass, in order

`main` runs `Drawer.DrawDue` through `cron.Every(ctx, time.Minute, …)`. `cron.Every` runs it once at startup and then once a minute, and never overlaps two passes (`internal/cron/cron.go:12`). One pass:

1. `Store.Due` lists published, unfinished giveaways whose `ends_at` is at or before now. The startup pass therefore catches giveaways that ended while the bot was down. Drafts and cancelled giveaways are unpublished, so they are never due.
2. `pick` loads every participant, shuffles them once, and walks the list through the `Filter` until it has the winner count. A shuffle draws without replacement, so nobody can win twice and no "pick again on a repeat" loop is needed. With too few participants who pass, the giveaway finishes with fewer winners, possibly none.
3. `Store.Finish` sets `finished_at`, `results_pending` and the winners' `giveaway_participants.winner` marks and places in one transaction. It is guarded on the giveaway still being due, so a cancel or a later end time saved after step 1 wins, and `ErrNotFound` makes the pass skip the giveaway quietly.
4. Only then does `announce` remove the join button and hand the results to review, using `context.WithoutCancel` so a shutdown signal does not stop it halfway. The draw never posts to the channel. `ReviewNotifier.SendReview` sends the creator the results with a reroll button per winner and Publish. While the creator has a setup or edit step open, it sends a silent notice pointing to `/list` instead, so the step keeps its Cancel button.

### Results review (added 2026-10-23)

The owner checks winners by hand against partner channels the bot cannot read, so results reach the channel only when an admin of the giveaway's channel taps Publish. The decisions are in [the plan](../../plans/2026-10-23-1343-feat-giveaway-results-review-plan.md).

- **A reroll is chosen outside the transaction and checked inside it.** `Reviewer.pick` shuffles the eligible participants (`NOT winner AND rerolled_at IS NULL`) through the draw's filter. `Store.Reroll` then locks the giveaway row (`SELECT … FOR UPDATE`) and re-checks every guard. Two admins racing onto the same replacement get one success and one stale tap; a real Postgres run confirmed it.
- **A rerolled-out participant keeps `rerolled_at` and never wins that giveaway again.** Clearing `winner` alone would let the next reroll pick the same person.
- **Publish carries the rerolled-out count the admin saw.** `ClaimResults` locks the giveaway row and then counts in a separate statement, so a reroll it waited on is counted (a subquery inside the `UPDATE` would read the pre-reroll snapshot under READ COMMITTED). It matches the count, so a publish confirmed on a review older than another admin's reroll is stale and never posts a winner nobody checked.
- **Publish claims, then posts, then gives the claim back** when the first message fails. Posting and the release run under `context.WithoutCancel`. A deleted post gets the results as a standalone message (`AllowSendingWithoutReply`).
- **At most 98 winners.** Telegram allows 100 buttons on a message: 98 rerolls, Publish and Skip.

### Rules to keep

- **Winners are saved before anything is sent to Telegram.** This deliberately reverses the "button first, then the row" order used by Cancel and Restore (see [the published-flag convention](giveaway-crud-around-the-published-flag.md)). Saving first means a failed Telegram call can never lead to a second draw that picks different winners. A finished giveaway is never drawn again.
- **The participant's `winner` mark is the only record of a win.** The owner chose this over a separate `giveaway_winners` table. Do not add a second store of winners.
- **`finished_at` means "winners saved", `results_pending` means "not yet in the channel".** The draw never retries a channel post; an admin's Publish does, and a refused post leaves the results pending for another tap.
- **A filter error stops that giveaway's draw with nothing saved** (the `Filter` contract in `draw.go`). The giveaway stays due and the next minute shuffles again. A rule that calls Telegram, such as "still in the channel", should return an error on a failed call rather than `false`. Otherwise a network blip silently disqualifies people.

### Known gaps, accepted on 2026-10-09

- **Saved but never sent to review.** If the process dies between `Finish` and the review DM, or the DM fails, the creator gets nothing, but the results stay pending and every channel admin finds them in `/list`. The button may stay on the post in the first case. Give the container a stop grace period well above Docker's 10-second default.
- **An edit can put the button back.** `Editor.check` refuses only finished giveaways. This is the earlier decision that a giveaway can be edited until its winners are recorded. If the creator saves a new button text in the up-to-one-minute gap after the end time, `AttachButton` in the edit flow can run after the draw removed the button. The finished post then has a button whose taps answer "ended". Blocking edits once the end time passes would change that earlier decision, so ask the owner first.
- **A tap racing the draw.** `Service.Join` checks the end time and then inserts in a separate statement. A tap that passes the check just before the end can land after the draw read the participants. That user is recorded but cannot win. The window is milliseconds.
- **A partly posted result.** When a long result is split and the channel refuses a later message, the channel keeps the earlier part, the results count as published, and the admin who tapped gets a notice to post the rest by hand.
- **A claim lost in a crash.** If the process dies between the claim and the first channel message, the results count as published, leave `/list`, and nothing reaches the channel. The window is one Telegram call.
- **A double post after a timeout.** If Telegram accepts the first results message but the bot sees a timeout, the claim is given back and a second Publish posts the results again.

## Why This Matters

Each rule above has an alternative that looks simpler and breaks a guarantee. Posting before saving lets one bad Telegram call re-roll the winners. Reusing `finished_at` for "published" can never tell "saved" from "posted". A filter that returns `false` on errors removes real participants without a trace. The known gaps are cheap to rediscover only if someone remembers they were accepted. Otherwise the first unannounced giveaway looks like a new bug.

## When to Apply

Read this before touching `internal/giveaway/draw.go`, `review.go`, the `Due`, `Candidates`, `Finish`, `Reroll` or `ClaimResults` store methods, the join-after-end check, the edit flow's lock rule, or the shutdown sequence in `main.go`.

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
