package giveaway_test

import (
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
)

const appURL = "https://bot.example/app"

func TestSendReviewToCreator(t *testing.T) {
	h := newHandlerSetup(t)
	g := publish(t, h.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, Language: "en"})
	h.users.keyboard[adminID] = 77 // an older message with buttons

	giveaway.NewReviewNotifier(h.users, h.texts, h.kb, appURL, slog.New(slog.DiscardHandler)).
		SendReview(t.Context(), h.bot, g)

	sent := h.sent()
	if len(sent) != 1 || !strings.Contains(sent[0], "https://t.me/c/1234567890/42") {
		t.Fatalf("sent %q, want one message linking the post", sent)
	}

	if !slices.Contains(h.deleted(), "77") {
		t.Errorf("deleted %v, want the older message with buttons gone", h.deleted())
	}

	rows := h.keyboard(t)
	if len(rows) != 1 || len(rows[0]) != 1 {
		t.Fatalf("rows = %+v, want one button", rows)
	}

	want := appURL + "?review=" + strconv.FormatUint(uint64(g.ID), 10)
	if b := rows[0][0]; b.WebApp == nil || b.WebApp.URL != want {
		t.Errorf("button = %+v, want a web_app button to %s", b, want)
	}
}

func TestSendReviewInCreatorsLanguage(t *testing.T) {
	h := newHandlerSetup(t)
	g := publish(t, h.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, Language: "en"})
	h.users.language[adminID] = "uk"

	giveaway.NewReviewNotifier(h.users, h.texts, h.kb, appURL, slog.New(slog.DiscardHandler)).
		SendReview(t.Context(), h.bot, g)

	if rows := h.keyboard(t); len(rows) != 1 || rows[0][0].Text != h.texts.Text("uk", "review.open_app") {
		t.Errorf("rows = %+v, want the Ukrainian button", rows)
	}
}
