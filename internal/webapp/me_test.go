package webapp_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"

	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
	"github.com/alexander-danilenko/read_wine_bot/internal/webapp"
)

const testBot = "test_wine_bot"

type meBody struct {
	Language    string `json:"language"`
	FirstName   string `json:"first_name"`
	AddLink     string `json:"add_link"`
	PartnerLink string `json:"partner_link"`
}

func newMeMux(t *testing.T, fake *fakeGiveaways, users *fakeUsers) *http.ServeMux {
	t.Helper()

	log := slog.New(slog.DiscardHandler)
	ops := webapp.NewOperations(nil, fake, webapp.NewPhotos(testToken, nil, log), users, testBot, log)

	return newUsersMux(t, ops, users)
}

func decodeMe(t *testing.T, rec interface{ Result() *http.Response }) meBody {
	t.Helper()

	var got meBody

	res := rec.Result()
	defer res.Body.Close()

	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}

	return got
}

func TestMeLinks(t *testing.T) {
	mux := newMeMux(t, &fakeGiveaways{}, &fakeUsers{})

	rec := call(t, mux, "/api/me", &models.User{ID: 42})
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body %s", rec.Code, rec.Body)
	}

	got := decodeMe(t, rec)
	if got.AddLink != "https://t.me/"+testBot+"?startchannel&admin=post_messages+edit_messages" {
		t.Errorf("add_link = %q", got.AddLink)
	}

	if got.PartnerLink != "https://t.me/"+testBot+"?startchannel&admin=manage_chat" {
		t.Errorf("partner_link = %q", got.PartnerLink)
	}
}

func TestSetLanguage(t *testing.T) {
	users := &fakeUsers{saved: map[int64]string{42: "uk"}}
	mux := newMeMux(t, &fakeGiveaways{}, users)

	rec := send(t, mux, http.MethodPut, "/api/me/language", `{"language":"en"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body %s", rec.Code, rec.Body)
	}

	if got := decodeMe(t, rec); got.Language != "en" || got.AddLink == "" {
		t.Errorf("answer = %+v, want the me body in en", got)
	}

	if users.saved[42] != "en" {
		t.Errorf("saved = %q, want en", users.saved[42])
	}
}

func TestSetLanguageRejectsUnknown(t *testing.T) {
	for _, body := range []string{`{"language":"de"}`, `{"language":""}`, `{}`} {
		users := &fakeUsers{saved: map[int64]string{42: "uk"}}
		mux := newMeMux(t, &fakeGiveaways{}, users)

		rec := send(t, mux, http.MethodPut, "/api/me/language", body)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: code = %d, want 422", body, rec.Code)
		}

		if users.saved[42] != "uk" {
			t.Errorf("%s: saved = %q, want uk unchanged", body, users.saved[42])
		}
	}
}

func TestMyChannels(t *testing.T) {
	fake := &fakeGiveaways{
		channels: []giveaway.Channel{{ID: -1001, Title: "A"}, {ID: -1003, Title: "C", Username: "c_wine"}},
	}
	mux := newMeMux(t, fake, &fakeUsers{})

	rec := call(t, mux, "/api/channels", &models.User{ID: 42})
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body %s", rec.Code, rec.Body)
	}

	var got struct {
		Channels []struct {
			ID       int64  `json:"id"`
			Title    string `json:"title"`
			Username string `json:"username"`
		} `json:"channels"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}

	if fake.userID != 42 || len(got.Channels) != 2 || got.Channels[1].Title != "C" ||
		got.Channels[1].Username != "c_wine" {
		t.Errorf("channels = %+v for user %d, want A and C for 42", got.Channels, fake.userID)
	}
}
