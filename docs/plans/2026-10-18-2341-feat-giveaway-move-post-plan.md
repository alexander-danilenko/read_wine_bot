---
title: Giveaway Survives a Deleted Post - Plan
type: feat
date: 2026-10-18
topic: giveaway-move-post
artifact_contract: ce-unified-plan/v1
product_contract_source: ce-brainstorm
execution: code
---

# Giveaway Survives a Deleted Post - Plan

## Goal Capsule

- **Objective:** A giveaway creator keeps full control of a giveaway, and keeps its participants, when its channel post is deleted or turns out to be the wrong one.
- **Product authority:** this Product Contract. Giveaway ID, channel and participants never change.
- **Means:** treat Telegram's "message not found" as a missing button (KTD1), and add a move step to the giveaway edit flow that rewrites only `message_id` (KTD2, KTD3).
- **Open blockers:** none.
- **Stop conditions:** stop and ask the owner if the move needs a schema change beyond updating `message_id`, or if `AutoMigrate` would alter `idx_giveaways_post`.
- **Execution profile:** two local commits on `main`, no push, no PR.

---

## Product Contract

### Summary

Cancel and finish work on a giveaway whose post was deleted. A new "Move to another post" action in the edit menu moves an active or cancelled giveaway to another post in the same channel, keeping its ID, channel, participants and settings.

### Problem Frame

A giveaway is bound to one post for its whole life. When the creator deletes that post without cancelling first, cancel fails with a "no rights" notice, because removing the button from a missing message errors and the cancel stops there. Finish still works: the draw cannot reply to the missing post and sends the results to the creator in private, which is the wanted fallback. A cancelled giveaway whose post is gone can never be published again, so the only way out today is a new giveaway, which loses the participants.

### Requirements

#### Deleted post

- R1. Cancelling a published giveaway whose post no longer exists succeeds: a missing post counts as a post without a button.
- R2. Finishing a giveaway whose post no longer exists keeps today's behavior: winners are drawn and recorded, and the results go to the creator in private with the post link.

#### Move to another post

- R3. The giveaway edit menu offers "Move to another post" for active and cancelled giveaways, never for finished ones or ones past their end time and waiting for the draw.
- R4. The creator names the new post the same way the giveaway setup asks for one (forward the post or send its link). The step has a Cancel button.
- R5. The new post must be in the giveaway's channel and must not carry another giveaway. Otherwise the step stays open with a notice that says what to send instead.
- R6. Sending the giveaway's current post changes nothing and ends the step with a notice.
- R7. Moving keeps the giveaway's ID, channel, participants, end time, winner count, button text and language. Only the post changes.
- R8. Moving an active giveaway puts the join button on the new post, then removes it from the old post. A failure to remove the old button (deleted post, no rights) does not fail the move.
- R9. Moving a cancelled giveaway only records the new post. Its button appears when the creator publishes it again.
- R10. If the button cannot be put on the new post, nothing changes and the creator gets a notice saying why.
- R11. The winner draw replies with the results under the post the giveaway is on at draw time.
- R12. Every new user-facing text exists in every locale.
- R13. When the bot cannot put the button on a post because the post no longer exists (publishing a cancelled giveaway, or moving onto a deleted post), the notice says the post is gone and suggests moving the giveaway, not that the bot lacks rights.

### Key Decisions

- **Active and cancelled giveaways can move; finished ones cannot.** (session-settled: user-directed — chosen over "active only" and "cancelled only": both states need it, and a cancelled giveaway with a deleted post has no other way back.) Governs R3, R8, R9.
- **Same channel only.** The user asked to keep the giveaway's chat ID. Governs R5.
- **No confirmation before moving.** The move is reversible by moving back. Governs R4.
- **The old button may stay behind if it cannot be removed.** Taps on it answer like any post without a giveaway. Blocking the move on it would strand giveaways whose old post is out of reach. Governs R8.

### Acceptance Examples

- AE1. **Covers R1.** Given a published giveaway whose post was deleted, when the creator confirms cancel, then the giveaway is cancelled and keeps its participants.
- AE2. **Covers R3, R8, R11.** Given an active giveaway with 40 participants whose post was deleted, when the creator moves it to a new post in the channel, then the new post shows the join button, the 40 participants remain, new taps add participants, and the draw replies under the new post.
- AE3. **Covers R9.** Given a cancelled giveaway, when the creator moves it and then publishes it, then the button appears on the new post.
- AE4. **Covers R5.** Given a post from another channel, or a post with another giveaway, when the creator sends it, then the step stays open with a notice.
- AE5. **Covers R3.** Given a giveaway past its end time and not drawn yet, when the creator taps a stale "Move" button, then the tap gets the stale response.

### Scope Boundaries

- Moving a giveaway to another channel.
- Announcing the move in the channel or to participants.
- Detecting post deletion automatically.

### Product Contract preservation

Changed: added R13. Planning found that Restore on a deleted post shows the "make me an admin" notice, which gives the wrong next step. R13 extends R10's "notice saying why" to that case.

### Sources / Research

- `internal/giveaway/edit.go` `Editor.Cancel`: `RemoveButton` error returns `EditNoRights` before the database write, which is the cancel bug.
- `internal/giveaway/draw.go` `Drawer.announce`: falls back to `sendToCreator` when the reply fails.
- `internal/giveaway/giveaway.go`: `(chat_id, message_id)` is unique (`idx_giveaways_post`), which enforces R5's "no other giveaway" at the database too.
- `internal/giveaway/setup.go`: the existing forward-or-link post step to reuse for R4.

---

## Planning Contract

### Key Technical Decisions

- KTD1. **Map Telegram's "message to edit not found" to a sentinel at the Telegram adapter.** `telegramAPI.RemoveButton` treats it as success, because a missing post carries no button. `telegramAPI.AttachButton` returns a new sentinel `ErrPostGone` for it, which `Editor` maps to the R13 notice. One fix in the adapter serves Cancel, the draw's `announce` and every future caller. The `RemoveButton` half ships in the fix commit. The `ErrPostGone` half ships with the move feature, because its notice points to the Move action. Governs R1, R13.
- KTD2. **The move is a field-like edit step, `edit:move:<id>`, saved as the pending command.** It reuses the edit flow's `parseEditStep`, its Cancel button (`Stop` already returns to the menu for any `edit:<what>:<id>`) and the post handling in `editHandler.message` (`postRef` covers forwards and links). There is no new handler and no new callback prefix. Governs R4.
- KTD3. **One store write, `Writer.Move(ctx, id, messageID)`.** It is an `UPDATE giveaways SET message_id = ? WHERE id = ? AND finished_at IS NULL AND <managed>`. `ErrNotFound` is returned when no row matches, and `ErrExists` on a violation of the unique index `idx_giveaways_post`. An `UPDATE` has no `ON CONFLICT`, so the violation arrives as a database error. Turning on GORM's `TranslateError` in `internal/db/db.go` lets the store map `gorm.ErrDuplicatedKey`, which the postgres driver already produces for code 23505, without importing `pgconn` directly. The unique index stays the authority on "one giveaway per post", including setup drafts that `Store.Post` does not return. No column or index changes, so `AutoMigrate` does nothing. Governs R5, R7.
- KTD4. **Order of side effects for an active giveaway: attach the new button, write the database, then remove the old button.** This mirrors `Cancel`, where Telegram comes first. A database failure after the attach leaves a button on the new post that joins nothing (`Service.Join` finds no giveaway there), and retrying the move fixes it. When the write returns `ErrExists` after an attach, the new post belongs to another row, so the move removes its own new button again, best-effort. The old button comes off last and best-effort. Governs R8, R10.
- KTD5. **The draw needs no change.** `Drawer` reads `message_id` from the row at draw time, so it already replies under the current post. Governs R11.

### Challenge protocol (schema)

- Access pattern: one `UPDATE` by primary key per move, issued by hand by a creator, a few times per giveaway at most.
- No new column, index or constraint. The existing unique index `idx_giveaways_post` enforces R5's "no other giveaway on the post". The in-memory fake mirrors it through `memStore.violates`.
- No rule in CLAUDE.md is broken, so no ADR is needed.

### Assumptions

- Telegram answers `editMessageReplyMarkup` on a deleted message with a 400 whose description contains `message to edit not found`. If the real text differs, the U1 match is the one place to correct.
- A move onto a post that carries someone's unfinished setup draft is rejected as taken (`ErrExists`), not taken over.

### Sequencing

U1 ships alone as `fix(giveaway)`. U2–U4 ship together as `feat(giveaway)`.

---

## Implementation Units

### U1. Missing post counts as a post without a button

- **Goal:** cancel succeeds on a deleted post.
- **Requirements:** R1, R2 (via KTD1).
- **Dependencies:** none.
- **Files:** `internal/giveaway/handler.go`, `internal/giveaway/handler_internal_test.go`, `main.go` (version 0.1.0 -> 0.1.1).
- **Approach:**
  1. Next to `ignoreNotModified`, add a matcher for the bad-request description `message to edit not found`.
  2. `RemoveButton` returns nil for it. `AttachButton` is unchanged in this unit.
- **Patterns to follow:** `ignoreNotModified` and `TestIgnoreNotModified` (table test with wrapped `bot.ErrorBadRequest`), and the "not found" match in `internal/keyboard/keyboard.go`.
- **Test scenarios:**
  - Matcher table: "message to edit not found" matches; "message is not modified", "message can't be edited" and a Forbidden error do not.
  - Covers AE1. Cancel with a remover that returns nil for the missing post: the giveaway is cancelled and its participants are kept (existing `TestEditCancel` shape).
  - The existing `TestEditCancelWithoutRights` still gets `EditNoRights` for a generic remover error.
- **Verification:** a published giveaway whose post was deleted can be cancelled in the running bot.

### U2. Store write that moves a giveaway

- **Goal:** change a managed, unfinished giveaway's `message_id` in one statement.
- **Requirements:** R5, R7 (via KTD3).
- **Dependencies:** none.
- **Files:** `internal/giveaway/store.go`, `internal/giveaway/edit.go` (`Writer` interface), `internal/giveaway/fake_store_test.go`, `internal/db/db.go`.
- **Approach:**
  1. Set `TranslateError: true` in the `gorm.Config` in `internal/db/db.go`. Add `Store.Move` per KTD3, mapping `errors.Is(err, gorm.ErrDuplicatedKey)` to `ErrExists`.
  2. Add `Move` to `Writer`.
  3. Add `memStore.Move`, which applies the same row filter and returns `ErrExists` when another row (draft included) sits on the same `(chat_id, message_id)`.
- **Patterns to follow:** `Store.SaveFields` and `Store.Cancel` (`RowsAffected == 0` -> `ErrNotFound`). Check that no existing caller inspects raw driver errors before turning on `TranslateError`.
- **Test scenarios:** `Test expectation: none -- the store has no automated tests per CLAUDE.md; memStore behavior is exercised through U3's Editor tests, and the SQL gets the manual check in the Verification Contract.`
- **Verification:** `go build ./...` passes, and the manual run moves a row and keeps its participants.

### U3. Editor move rules

- **Goal:** `Editor` offers, validates and performs the move.
- **Requirements:** R3–R10, R13 (via KTD1, KTD2, KTD4). Settled decision: active and cancelled giveaways can move (Key Decision governing R3, R8, R9).
- **Dependencies:** U1, U2.
- **Files:** `internal/giveaway/edit.go`, `internal/giveaway/edit_test.go`, `internal/giveaway/handler.go`, `internal/giveaway/handler_internal_test.go`, `internal/i18n/i18n.go`, `internal/i18n/locales/en.json`, `internal/i18n/locales/uk.json`.
- **Approach:**
  1. Add the pseudo step `moveStep = "move"` and `AskMove(ctx, tg, userID, id)`. It loads and checks, returns stale for a published giveaway in state `Due`, then saves `editStep(moveStep, id)` as the pending command, as `Field` does.
  2. Add `Move(ctx, tg, rm, userID, chatID, messageID)`. It reads the pending step and requires `edit:move:<id>`, otherwise stale. It then loads and checks. A published giveaway in state `Due` is stale, repeated here as `FinishNow` repeats it, because the end time can pass while the step is open. It then applies the R5/R6 checks in order:
     - another channel -> `EditMoveOtherChannel`, step kept;
     - same post -> `EditMoveSame`, step cleared, menu shown;
     - `giveaways.Post` finds a giveaway -> `EditMoveTaken`, step kept.
  3. It claims the step as `Value` does. For a published giveaway it calls `AttachButton` on the new post (`ErrPostGone` -> `EditPostGone`, other error -> `EditNoRights`). Then `writer.Move` (`ErrExists` -> `EditMoveTaken`, after removing the new button again per KTD4; `ErrNotFound` -> `EditFinished`). Then `rm.RemoveButton` on the old post, logged on failure. Wherever the step is kept after the claim, the claim has already cleared the pending command, so `Move` saves `editStep(moveStep, id)` again before it returns.
  4. KTD1's second half: `telegramAPI.AttachButton` wraps "message to edit not found" as the package sentinel `ErrPostGone`, using U1's matcher. `Editor.Restore` maps it to the new notice `EditPostGone`, and other errors keep `EditNoRights`. Add `edit.post_gone` in both locales. It says the post is gone and to use "Move to another post" in the giveaway menu.
  5. On success it returns the notice `EditMoved` and the menu step.
  6. `Value` already rejects non-field steps, so a typed text at the move step asks again in U4's handler.
- **Patterns to follow:** `Editor.Field` + `Editor.Value` (pending step, claim, `load`), `Editor.Cancel` (Telegram before database, `ButtonRemover` parameter for tests).
- **Test scenarios:**
  - Covers AE2. Active giveaway with participants moved to a new post in the same channel: the row's `message_id` changes, participants and ID are unchanged, the button is attached to the new post, removal is called for the old post, and the notice is `EditMoved`.
  - Covers AE3. Cancelled giveaway moved: the row's `message_id` changes and no attach or removal happens. A following `Restore` attaches to the new post.
  - Covers AE4. A post from another channel gets `EditMoveOtherChannel`, the pending command is still `edit:move:<id>` and the row is unchanged.
  - Covers AE4. A post carrying another giveaway gets `EditMoveTaken` and the pending command is still `edit:move:<id>`.
  - The write returns `ErrExists` after an attach, for a post holding a setup draft: `EditMoveTaken`, the new button is removed again, the row is unchanged and the step is restored.
  - The end time passes between `AskMove` and `Move`: `Move` is stale and the row is unchanged.
  - Same post as current gets `EditMoveSame` and nothing is called.
  - Covers AE5. `AskMove` on a published giveaway past its end time returns stale.
  - `AskMove` on a finished giveaway returns `EditFinished`.
  - Attach fails with `ErrPostGone`: notice `EditPostGone`, row unchanged, pending command restored to `edit:move:<id>`.
  - Restore where the fake `AttachButton` returns `ErrPostGone`: the notice is `EditPostGone`, the menu shows again and the giveaway stays cancelled.
  - The `AttachButton` error wrapping: "message to edit not found" becomes `ErrPostGone`, and other errors pass through unchanged.
  - Attach fails with another error: notice `EditNoRights`, row unchanged.
  - Old button removal fails: the move still succeeds with `EditMoved`.
  - `Move` with no pending move step, as from a second forward after success, returns stale.
- **Verification:** all scenarios pass, and `TestEditStop` still returns to the menu from the move step.

### U4. Edit menu button, step prompt and post routing

- **Goal:** the creator reaches the move from the menu and sends the post.
- **Requirements:** R3, R4, R12.
- **Dependencies:** U3.
- **Files:** `internal/giveaway/edit_handler.go`, `internal/giveaway/handler_test.go`, `internal/i18n/i18n.go`, `internal/i18n/locales/en.json`, `internal/i18n/locales/uk.json`, `CONCEPTS.md`, `main.go` (version 0.1.1 -> 0.2.0).
- **Approach:**
  1. Add the edit action `editMove = "move"` carrying the giveaway ID, and route it in `act` to `AskMove`.
  2. In `menu`, add a `EditMoveGiveaway` row for published and cancelled giveaways, above Finish/Cancel or Publish.
  3. In `prompt`, `moveStep` shows `EditMovePrompt` with the Cancel button.
  4. In `editHandler.message`, a step whose `what` is `moveStep`, combined with `isPost`, calls `Move`. A non-post message at that step re-prompts.
  5. Add the locale keys `edit.move_giveaway`, `edit.move_prompt`, `edit.move_other_channel`, `edit.move_same`, `edit.move_taken` and `edit.moved` in `en` and `uk`.
  6. In `CONCEPTS.md`, update the Giveaway entry: a giveaway is bound to one post at a time, and its creator can move it to another post in the same channel.
- **Patterns to follow:** the existing `editAskFinish` action wiring, and callback-data tests in `handler_test.go` (64-byte limit, prefix).
- **Test scenarios:**
  - `parseEditData` accepts `giveaway-edit:move:<id>` and rejects it without an ID.
  - Move callback data for the largest giveaway ID fits 64 bytes and starts with `EditCallbackPrefix`.
  - The i18n catalog test passes with every new key in both locales.
- **Verification:** in the running bot, the menu shows "Move to another post", forwarding a new post moves the giveaway, and Cancel at the step returns to the menu.

---

## Verification Contract

| Gate | Command or action | Applies to |
| --- | --- | --- |
| Build | `go build ./...` | U1–U4 |
| Tests | `go test ./...` | U1, U3, U4 |
| Lint | `golangci-lint run ./...` reports 0 issues | U1–U4 |
| Markdown | `markdownlint-cli2 "**/*.md"` | `CONCEPTS.md`, this plan |
| Manual store check | run the bot against a real `DATABASE_URL`: move an active giveaway onto a new post, cancel one whose post was deleted, publish a cancelled one after moving | U1, U2 |

---

## Definition of Done

- Every unit's test scenarios exist and pass, and lint reports 0 issues.
- Two commits on `main`: `fix(giveaway): …` with version 0.1.1, then `feat(giveaway): …` with version 0.2.0. Nothing pushed.
- No `AutoMigrate`-visible schema change.
- No abandoned-attempt code, debug logging or unused locale keys remain in the diff.
- The manual store check is either done or listed as outstanding for the owner in the final report.
