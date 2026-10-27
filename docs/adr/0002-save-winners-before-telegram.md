# 0002. The winner draw saves winners before it calls Telegram

Status: accepted, 2026-10-09.

## Context

Every other giveaway action changes the join button on the post first and the row second (ADR 0006). The winner draw could follow the same order: remove the button, post the results, then save the winners. If a Telegram call fails after the pick, the next pass would draw again and pick different winners.

## Decision

`Store.Finish` saves the winners, `finished_at` and `results_pending` in one transaction before anything goes to Telegram. It is guarded on the giveaway still being due, so a cancel or a later end time saved after `Store.Due` wins. Only then does `announce` remove the button and send the results to review, under `context.WithoutCancel` so a shutdown does not stop it halfway. A finished giveaway is never drawn again.

## Consequences

- A failed Telegram call can never re-roll the winners.
- If the process dies between `Finish` and the review message, or the message fails, the creator gets nothing. The results stay pending and every channel admin finds them in `/list`. The button may stay on the post. Give the container a stop grace period well above Docker's 10-second default.
