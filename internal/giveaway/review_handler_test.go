package giveaway_test

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// resultsStore adds fixed results to memStore, for the review sent after a draw.
type resultsStore struct {
	*memStore

	results giveaway.Results
}

func (s resultsStore) Results(context.Context, uint) (giveaway.Results, error) { return s.results, nil }

func (resultsStore) Eligible(context.Context, uint) ([]user.User, error) { return nil, nil }

func (resultsStore) Reroll(context.Context, uint, int64, int64, time.Time) error {
	return giveaway.ErrNotFound
}

func (resultsStore) ClaimResults(context.Context, uint, int) error { return giveaway.ErrNotFound }

func (resultsStore) ReleaseResults(context.Context, uint) error { return nil }

func (h *handlerSetup) notifier(results giveaway.Results) *giveaway.ReviewNotifier {
	store := resultsStore{memStore: h.store, results: results}
	log := slog.New(slog.DiscardHandler)
	rv := giveaway.NewReviewer(store, store, store, giveaway.AcceptAll, rand.Shuffle, h.texts, time.Now, log)

	return giveaway.NewReviewNotifier(rv, h.users, h.texts, h.kb, log)
}

func TestSendReviewToCreator(t *testing.T) {
	h := newHandlerSetup(t)
	g := publish(t, h.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, Language: "en"})
	h.users.keyboard[adminID] = 77 // an older message with buttons

	results := giveaway.Results{Winners: []giveaway.Winner{{TelegramID: 5, FirstName: "Ann", Place: 1}}}
	h.notifier(results).SendReview(t.Context(), h.bot, g)

	sent := h.sent()
	if len(sent) != 1 || !strings.Contains(sent[0], `href="tg://user?id=5"`) {
		t.Fatalf("sent %q, want the review linking the winner", sent)
	}

	if !slices.Contains(h.deleted(), "77") {
		t.Errorf("deleted %v, want the older message with buttons gone", h.deleted())
	}

	rows := h.keyboard(t)
	if len(rows) != 2 || !strings.HasPrefix(rows[0][0].CallbackData, giveaway.ReviewCallbackPrefix) {
		t.Errorf("rows = %+v, want one reroll row and the publish row", rows)
	}
}

func TestSendReviewWaitsForOpenStep(t *testing.T) {
	h := newHandlerSetup(t)
	g := publish(t, h.store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, Language: "en"})
	h.users.keyboard[adminID] = 77 // the open step's prompt
	h.users.pending[adminID] = "edit:winners:1"

	h.notifier(giveaway.Results{}).SendReview(t.Context(), h.bot, g)

	h.wantSilent(t, h.texts.Text("en", i18n.ReviewWaiting, "https://t.me/c/1234567890/42"))

	if len(h.deleted()) != 0 {
		t.Errorf("deleted %v, want the open step's prompt kept", h.deleted())
	}
}
