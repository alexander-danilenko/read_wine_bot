package webapp_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
)

func TestGetReview(t *testing.T) {
	fake := &fakeGiveaways{review: giveaway.ReviewView{
		Giveaway: giveaway.View{ID: 7, Status: giveaway.StatusReview},
		Channels: []giveaway.ChannelInfo{{ID: -100, Title: "Read Wine", Username: "read_wine"}},
		Winners: []giveaway.WinnerView{{
			UserID: 1, FirstName: "Ann", Place: 1,
			Subscriptions: []giveaway.Subscription{giveaway.SubscriptionUnknown},
		}},
		Rerolled: 2,
	}}
	mux := newGiveawayMux(t, fake)

	rec := send(t, mux, http.MethodGet, "/api/reviews/7", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body %s", rec.Code, rec.Body)
	}

	var got struct {
		Rerolled int `json:"rerolled"`
		Winners  []struct {
			FirstName     string   `json:"first_name"`
			Subscriptions []string `json:"subscriptions"`
		} `json:"winners"`
		Channels []struct {
			Title string `json:"title"`
		} `json:"channels"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}

	if got.Rerolled != 2 || len(got.Winners) != 1 || got.Winners[0].Subscriptions[0] != "unknown" {
		t.Errorf("review = %+v, want rerolled 2 and an unknown subscription", got)
	}

	if len(got.Channels) != 1 || got.Channels[0].Title != "Read Wine" {
		t.Errorf("channels = %+v", got.Channels)
	}
}

func TestReviewActions(t *testing.T) {
	fake := &fakeGiveaways{}
	mux := newGiveawayMux(t, fake)

	if rec := send(t, mux, http.MethodPost, "/api/reviews/7/reroll", `{"winner_id":5}`); rec.Code != http.StatusOK {
		t.Errorf("reroll: code = %d, body %s", rec.Code, rec.Body)
	}

	if fake.winnerID != 5 {
		t.Errorf("rerolled winner %d, want 5", fake.winnerID)
	}

	for _, action := range []string{"publish", "skip"} {
		rec := send(t, mux, http.MethodPost, "/api/reviews/7/"+action, `{"rerolled":3}`)
		if rec.Code != http.StatusNoContent || fake.lastAction != action || fake.rerolled != 3 {
			t.Errorf("%s: code %d, action %q, rerolled %d", action, rec.Code, fake.lastAction, fake.rerolled)
		}
	}

	fake.err = &giveaway.AppError{Code: giveaway.CodeReviewClosed}

	rec := send(t, mux, http.MethodPost, "/api/reviews/7/publish", `{"rerolled":3}`)
	if rec.Code != http.StatusConflict || decodeProblem(t, rec).Code != "review_closed" {
		t.Errorf("closed review: code %d, body %s", rec.Code, rec.Body)
	}
}
