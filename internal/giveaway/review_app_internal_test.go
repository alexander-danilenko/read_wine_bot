package giveaway

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// noCounts counts no participants; the review tests check winners, not counts.
type noCounts struct{}

func (noCounts) Participants(context.Context, []uint) (map[uint]int, error) {
	return map[uint]int{}, nil
}

// reviewTelegram answers the admin checks, records sent results and checks subscriptions.
type reviewTelegram struct {
	*fakeReviewTelegram
	*fakeSubscriptionTelegram
}

func newReviewApp(t *testing.T, store *fakeReviewStore) *App {
	t.Helper()

	texts, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.DiscardHandler)
	svc := &Service{texts: texts, kyiv: time.UTC, now: time.Now, log: log}

	return NewApp(&Editor{svc: svc, counter: noCounts{}}, newTestReviewer(t, store), nil)
}

func newReviewTelegram() reviewTelegram {
	return reviewTelegram{
		fakeReviewTelegram: newAdmins(),
		fakeSubscriptionTelegram: &fakeSubscriptionTelegram{
			titles:     map[int64]string{drawChannel: "Read Wine"},
			subscribed: map[int64][]int64{drawChannel: {1}},
			failing:    map[int64]bool{2: true},
		},
	}
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()

	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
}

// AE2: a channel admin who did not create the giveaway reviews and publishes it.
func TestAppReviewByOtherAdmin(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann(), bob()}, cid())
	app, tg := newReviewApp(t, store), newReviewTelegram()

	v, err := app.Review(t.Context(), tg, otherAdmin, reviewID)
	if err != nil {
		t.Fatal(err)
	}

	if len(v.Winners) != 2 || v.Winners[0].FirstName != "Ann" || v.Giveaway.Status != StatusReview {
		t.Fatalf("review = %+v", v)
	}

	// Ann is subscribed; Bob's check fails, which stays unknown, never "not subscribed".
	if got := v.Winners[0].Subscriptions; len(got) != 1 || got[0] != Subscribed {
		t.Errorf("Ann's subscriptions = %v, want subscribed", got)
	}

	if got := v.Winners[1].Subscriptions; len(got) != 1 || got[0] != SubscriptionUnknown {
		t.Errorf("Bob's subscriptions = %v, want unknown", got)
	}
}

// AE2: the other admin publishes once; a second publish and a later review find it closed.
func TestAppPublishOnce(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann(), bob()}, cid())
	app, tg := newReviewApp(t, store), newReviewTelegram()

	if err := app.Publish(t.Context(), tg, otherAdmin, reviewID, 0); err != nil {
		t.Fatal(err)
	}

	sent := len(tg.sent)
	if sent == 0 {
		t.Error("no results posted")
	}

	wantCode(t, app.Publish(t.Context(), tg, otherAdmin, reviewID, 0), CodeStale)

	if len(tg.sent) != sent {
		t.Error("a second publish posted again")
	}

	_, err := app.Review(t.Context(), tg, otherAdmin, reviewID)
	wantCode(t, err, CodeReviewClosed)
}

// AE3: a user who is no longer a channel admin cannot see or act on the review.
func TestAppReviewNotAdmin(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann()}, bob())
	app, tg := newReviewApp(t, store), newReviewTelegram()
	delete(tg.admins, otherAdmin)

	_, err := app.Review(t.Context(), tg, otherAdmin, reviewID)
	wantCode(t, err, CodeNotAdmin)

	_, err = app.Reroll(t.Context(), tg, otherAdmin, reviewID, 1)
	wantCode(t, err, CodeNotAdmin)

	wantCode(t, app.Skip(t.Context(), tg, otherAdmin, reviewID, 0), CodeNotAdmin)
}

func TestAppReroll(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann()}, bob())
	app, tg := newReviewApp(t, store), newReviewTelegram()

	v, err := app.Reroll(t.Context(), tg, creatorID, reviewID, 1)
	if err != nil {
		t.Fatal(err)
	}

	if v.Rerolled != 1 || len(v.Winners) != 1 || v.Winners[0].UserID != 2 {
		t.Fatalf("after reroll = %+v, want Bob in place of Ann", v)
	}

	// Ann was rerolled out already, so a second tap on her reroll is stale.
	_, err = app.Reroll(t.Context(), tg, creatorID, reviewID, 1)
	wantCode(t, err, CodeStale)

	// Publish with the count from before the reroll is stale: the winners changed since.
	wantCode(t, app.Publish(t.Context(), tg, creatorID, reviewID, 0), CodeStale)
}

func TestAppSkipClosesReview(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann()}, bob())
	app, tg := newReviewApp(t, store), newReviewTelegram()

	if err := app.Skip(t.Context(), tg, creatorID, reviewID, 0); err != nil {
		t.Fatal(err)
	}

	if len(tg.sent) != 0 {
		t.Error("skip posted results")
	}

	_, err := app.Reroll(t.Context(), tg, creatorID, reviewID, 1)
	wantCode(t, err, CodeStale)
}

func TestAppSendWinnersCannotMessage(t *testing.T) {
	store := newFakeReviewStore()
	store.drawn([]user.User{ann()})
	app, tg := newReviewApp(t, store), newReviewTelegram()

	if err := app.SendWinners(t.Context(), tg, creatorID, reviewID); err != nil {
		t.Fatal(err)
	}

	tg.sendFails = 2
	wantCode(t, app.SendWinners(t.Context(), tg, creatorID, reviewID), CodeCannotMessage)
}
