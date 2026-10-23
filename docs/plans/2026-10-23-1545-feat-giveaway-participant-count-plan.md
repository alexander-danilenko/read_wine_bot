---
title: Giveaway Participant Count and Menu Refresh - Plan
type: feat
date: 2026-10-23
topic: giveaway-participant-count
artifact_contract: ce-unified-plan/v1
product_contract_source: ce-plan-bootstrap
execution: code
---

# Giveaway Participant Count and Menu Refresh - Plan

## Goal Capsule

- **Objective:** A creator sees how many people joined each giveaway without leaving the chat, and can update the edit menu to the current numbers with one tap.
- **Product authority:** the owner's request of 2026-10-23.
- **Means:** one grouped count query over `giveaway_participants`, a participant line in the `/list` item and the edit menu, and a "🔄 Refresh" button in the edit menu that re-renders the menu in place.
- **Open blockers:** none.
- **Stop conditions:** none expected; the change adds no schema.
- **Execution profile:** local commits on `main`, no new branch, no push, no PR (session-settled, user-directed).
- **Done when:** the Definition of Done below holds.

---

## Product Contract

### Summary

`/list` shows the participant count on every running giveaway and draft line. The edit menu shows a "Participants: N" line. The edit menu gets a "🔄 Refresh" button that reloads the giveaway from the database and edits the same message in place.

### Problem Frame

The creator cannot see how many people joined a giveaway. The edit menu is a snapshot taken when it was sent, so its data goes stale while the giveaway runs.

### Requirements

- R1. Each running giveaway and draft line in `/list` shows the current participant count. Lines of giveaways whose results wait for review do not change.
- R2. The edit menu text shows the current participant count.
- R3. The edit menu has a "🔄 Refresh" button, in every locale, for published and cancelled giveaways alike.
- R4. A Refresh tap re-reads the giveaway and the count and edits the tapped menu message in place. When nothing changed, the tap still answers the callback and keeps the message (Telegram's "message is not modified" is not an error).
- R5. A Refresh tap on a giveaway the user may no longer edit (finished, not admin, missing) behaves like every other menu tap: the existing `load` checks answer it.

### Scope Boundaries

- Out: a count in the results review lines, a count in the channel post or button, live auto-refresh, a "refreshed at" timestamp.

### Assumptions

- The count is every row of `giveaway_participants` for the giveaway, including participants rerolled out during review: they did join.
- Button placement: Refresh is the last row of the menu, so the destructive buttons above it keep their place.

---

## Planning Contract

### Key Technical Decisions

- KTD1. Count with one `SELECT giveaway_id, count(*) ... WHERE giveaway_id IN ? GROUP BY giveaway_id` in `Store.Participants(ctx, ids) (map[uint]int, error)`. A missing ID maps to 0. The primary key `(giveaway_id, user_id)` serves the filter, so no index is needed. Rejected: a GORM read-only computed field on `Giveaway` (a subquery in every read of the model) and one count query per `/list` line (N queries).
- KTD2. Declare a new one-method interface `Counter` in `internal/giveaway/edit.go` and add it as a parameter of `NewEditor`. `Giveaways` already has three methods, the CLAUDE.md maximum.
- KTD3. Refresh is a new edit action `refresh` (`giveaway-edit:refresh:<id>`) mapped to `Editor.ShowMenu`. `ShowMenu` already runs the `load` checks and returns the menu step, and `inPlace` plus `Tracker.Replace` already edit the tapped message and ignore "message is not modified". Rejected: reusing the `no` action's callback data, which would tie two buttons to one meaning.
- KTD4. Session-settled (user-directed): work on the current branch `main`, no PR. Rejected: feature branch + PR.

### Challenge protocol

Access pattern: a read-only aggregate on `/list` (a creator's running giveaways, a handful of rows) and on every menu render (one ID). No write path, no schema change, no new constraint. No rule of the protocol applies.

---

## Implementation Units

### U1. Store count query

- **Files:** `internal/giveaway/store.go`, `internal/giveaway/fake_store_test.go`.
- **Approach:** add `Store.Participants` per KTD1; return an empty map for no IDs without a query. Add the same method to `memStore` over its `participants` map.
- **Test expectation:** Store SQL has no automated tests (CLAUDE.md); verify by hand against Postgres. The fake is exercised by U2 tests.

### U2. Count in `/list` and the edit menu

- **Files:** `internal/giveaway/edit.go`, `main.go`, `internal/giveaway/setup_test.go`, `internal/giveaway/move_test.go`, `internal/i18n/locales/en.json`, `internal/i18n/locales/uk.json`, `internal/i18n/schema.json`, `internal/giveaway/edit_test.go`, `internal/giveaway/handler_test.go` (`TestListHandler` compares the whole `/list` line and gets the count).
- **Approach:** `Counter` interface and `NewEditor` parameter (KTD2). `Menu` asks for the one count and passes it as a new `%[6]d` argument of `edit.menu`. `runningLines` gets the counts of all its giveaways in one call and passes a new `%[5]d` argument of `edit.list_item`. A count error fails `/list` and the menu like any other store error. Update the schema descriptions.
- **Test scenarios:**
  - `/list` with a published giveaway that has two participants and a draft with none shows "2" and "0" on their lines.
  - The edit menu of a giveaway with one participant shows the count 1; after another join, `Menu` shows 2.

### U3. Refresh button

- **Files:** `internal/giveaway/edit_handler.go`, `internal/i18n/i18n.go`, the locale and schema files, `internal/giveaway/handler_internal_test.go` (`parseEditData` is unexported), `internal/giveaway/handler_test.go`.
- **Approach:** add `editRefresh = "refresh"`, parse it like the other ID actions, route it in `act` to `ShowMenu`, add the last menu row with the new key `edit.refresh` ("🔄 Refresh" / "🔄 Оновити").
- **Test scenarios:**
  - `parseEditData("giveaway-edit:refresh:7")` returns the refresh action for giveaway 7; the existing callback-length and prefix tests cover the new data.
  - `TestEditForwardSendsMenu` expects `giveaway-edit:refresh:<id>` after `giveaway-edit:ask:<id>`; `TestEditMenuOfCancelled` expects Publish in the row above the last and Refresh in the last row.
  - Refresh on a finished giveaway returns the same reply as the other menu taps on a finished giveaway (`edit.finished` notice).

---

## Verification Contract

- `go test ./...`, `golangci-lint run ./...` (0 issues), `markdownlint-cli2 "**/*.md"`.
- Run `Store.Participants` against Postgres 18 (`docker compose up -d`) with zero, one and several IDs.

## Definition of Done

- R1-R5 hold; tests and lint pass; version bumped to `0.7.0` in the same commit (`feat`).
