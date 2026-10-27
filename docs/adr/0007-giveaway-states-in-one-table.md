# 0007. Draft, published and cancelled giveaways share one table

Status: accepted, 2026-10-09.

## Context

A setup draft, a published giveaway and a cancelled giveaway are told apart only by `published` and `cancelled_at`; `finished_at` is a third axis. They share the unique post index `(chat_id, message_id)`, so a draft can take over its post's row and a cancelled giveaway can be published again with its participants.

## Decision

Keep one table. The database rejects invalid combinations with CHECKs (`chk_giveaways_cancelled`, `chk_giveaways_finished`, `chk_giveaways_winners`, `chk_giveaways_results`) and allows one draft per admin (`idx_giveaways_draft`). Store queries name their scope: the `draft` and `managed` constants in `internal/giveaway/store.go`. A published giveaway is never deleted; only drafts are.

`published` has `default:true` so rows from before the setup flow count as published. GORM drops a `false` zero value for a column with a default, so a struct insert saves a draft as published: `SaveDraft` inserts from a map instead, and every new insert must too.

## Consequences

- A query without a scope is a silent bug: a draft shows up in the edit flow, or a cancelled giveaway accepts participants.
- A map insert skips every GORM default, not only `published`. Every caller of `SaveDraft` must set `Winners`, or `chk_giveaways_winners` rejects the row.
- Updates go through `Updates(map[string]any{...})` with the `col*` constants: a struct skips `false` and `nil`.
- The in-memory fake (`violates` in `internal/giveaway/fake_store_test.go`) mirrors the constraints. A new constraint needs a matching check there.
