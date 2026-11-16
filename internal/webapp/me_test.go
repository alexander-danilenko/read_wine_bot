package webapp_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
	"github.com/alexander-danilenko/read_wine_bot/internal/webapp"
)

const testBot = "test_wine_bot"

type meBody struct {
	Language    string  `json:"language"`
	FirstName   string  `json:"first_name"`
	Role        string  `json:"role"`
	PartnerLink string  `json:"partner_link"`
	AddLink     *string `json:"add_link"`
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
	if got.AddLink != nil {
		t.Errorf("add_link = %q, want none: the app no longer adds the bot to giveaway channels", *got.AddLink)
	}

	if got.PartnerLink != "https://t.me/"+testBot+"?startchannel&admin=manage_chat" {
		t.Errorf("partner_link = %q", got.PartnerLink)
	}
}

func TestMeRole(t *testing.T) {
	for _, role := range []giveaway.Role{giveaway.RoleFull, giveaway.RoleReadOnly, giveaway.RolePartner} {
		fake := &fakeGiveaways{role: role}
		mux := newMeMux(t, fake, &fakeUsers{})

		rec := call(t, mux, "/api/me", &models.User{ID: 42})
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: code = %d, body %s", role, rec.Code, rec.Body)
		}

		if got := decodeMe(t, rec); got.Role != string(role) || fake.userID != 42 {
			t.Errorf("role = %q for user %d, want %q for 42", got.Role, fake.userID, role)
		}
	}
}

func TestMeRoleLookupFails(t *testing.T) {
	mux := newMeMux(t, &fakeGiveaways{err: errors.New("telegram is down")}, &fakeUsers{})

	rec := call(t, mux, "/api/me", &models.User{ID: 42})
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"internal"`) {
		t.Errorf("code = %d, body %s; want 500 internal", rec.Code, rec.Body)
	}
}

func TestSetLanguage(t *testing.T) {
	users := &fakeUsers{saved: map[int64]string{42: "uk"}}
	mux := newMeMux(t, &fakeGiveaways{role: giveaway.RoleFull}, users)

	rec := send(t, mux, http.MethodPut, "/api/me/language", `{"language":"en"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body %s", rec.Code, rec.Body)
	}

	if got := decodeMe(t, rec); got.Language != "en" || got.Role != "full" || got.PartnerLink == "" {
		t.Errorf("answer = %+v, want the whole me body in en with role full", got)
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
	fake := &fakeGiveaways{channels: giveaway.ChannelLists{
		Mine: []giveaway.Channel{
			{ID: -1001, Title: "A", ExtraRights: []string{}},
			{ID: -1003, Title: "C", Username: "c_wine", ExtraRights: []string{"can_post_messages"}},
		},
		Partners: []giveaway.Channel{{ID: -1002, Title: "B", Username: "b_wine"}},
	}}
	mux := newMeMux(t, fake, &fakeUsers{})

	rec := call(t, mux, "/api/channels", &models.User{ID: 42})
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body %s", rec.Code, rec.Body)
	}

	type channel struct {
		ID          int64     `json:"id"`
		Title       string    `json:"title"`
		Username    string    `json:"username"`
		ExtraRights *[]string `json:"extra_rights"`
	}

	var got struct {
		Channels []channel `json:"channels"`
		Partners []channel `json:"partners"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}

	wantChannels := []channel{
		{ID: -1001, Title: "A", ExtraRights: &[]string{}},
		{ID: -1003, Title: "C", Username: "c_wine", ExtraRights: &[]string{"can_post_messages"}},
	}
	if fake.userID != 42 || !reflect.DeepEqual(got.Channels, wantChannels) {
		t.Errorf(
			"channels = %+v for user %d, want A with no extra rights and C with post for 42",
			got.Channels,
			fake.userID,
		)
	}

	if want := []channel{{ID: -1002, Title: "B", Username: "b_wine"}}; !reflect.DeepEqual(got.Partners, want) {
		t.Errorf("partners = %+v, want B with extra_rights null", got.Partners)
	}

	if fake.asPartner {
		t.Error("asPartner = true without the query, want false")
	}

	if rec := call(
		t,
		mux,
		"/api/channels?as_partner=true",
		&models.User{ID: 42},
	); rec.Code != http.StatusOK ||
		!fake.asPartner {
		t.Errorf("code = %d, asPartner = %v with as_partner=true, want 200 and true", rec.Code, fake.asPartner)
	}
}
