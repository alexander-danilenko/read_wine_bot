package webapp_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
	"github.com/alexander-danilenko/read_wine_bot/internal/webapp"
)

// fakeGiveaways answers every giveaway operation with view, or with err when set, and records
// what it was called with.
type fakeGiveaways struct {
	view       giveaway.View
	err        error
	userID     int64
	lang       string
	fields     giveaway.Fields
	changes    giveaway.Changes
	link       string
	offset     int
	limit      int
	lastAction string
	review     giveaway.ReviewView
	winnerID   int64
	rerolled   int
}

func (f *fakeGiveaways) List(
	_ context.Context,
	_ giveaway.Telegram,
	userID int64,
	offset, limit int,
) (giveaway.Page, []giveaway.View, error) {
	f.userID, f.offset, f.limit = userID, offset, limit

	return giveaway.Page{Views: []giveaway.View{f.view}, Total: 21}, nil, f.err
}

func (f *fakeGiveaways) Channels(context.Context, giveaway.ChannelReader, ...[]giveaway.View) []giveaway.Channel {
	return nil
}

func (f *fakeGiveaways) Archived(_ context.Context, userID int64, offset, limit int) (giveaway.Page, error) {
	f.userID, f.offset, f.limit = userID, offset, limit

	return giveaway.Page{Views: []giveaway.View{f.view}, Total: 21}, f.err
}

func (f *fakeGiveaways) Get(_ context.Context, userID int64, _ uint) (giveaway.View, error) {
	f.userID = userID

	return f.view, f.err
}

func (f *fakeGiveaways) Create(
	_ context.Context, _ giveaway.AppTelegram, userID int64, lang string, fields giveaway.Fields,
) (giveaway.View, error) {
	f.userID, f.lang, f.fields = userID, lang, fields

	return f.view, f.err
}

func (f *fakeGiveaways) Edit(
	_ context.Context, _ giveaway.Telegram, userID int64, _ uint, c giveaway.Changes,
) (giveaway.View, error) {
	f.userID, f.changes = userID, c

	return f.view, f.err
}

func (f *fakeGiveaways) Move(
	_ context.Context,
	_ giveaway.AppTelegram,
	userID int64,
	_ uint,
	link string,
) (giveaway.View, error) {
	f.userID, f.link = userID, link

	return f.view, f.err
}

func (f *fakeGiveaways) Cancel(_ context.Context, _ giveaway.AppTelegram, userID int64, _ uint) (giveaway.View, error) {
	return f.act("cancel", userID)
}

func (f *fakeGiveaways) Restore(_ context.Context, _ giveaway.Telegram, userID int64, _ uint) (giveaway.View, error) {
	return f.act("restore", userID)
}

func (f *fakeGiveaways) FinishNow(_ context.Context, _ giveaway.Telegram, userID int64, _ uint) (giveaway.View, error) {
	return f.act("finish-now", userID)
}

func (f *fakeGiveaways) Review(
	_ context.Context,
	_ giveaway.ReviewTelegramAll,
	userID int64,
	_ uint,
) (giveaway.ReviewView, error) {
	f.userID = userID

	return f.review, f.err
}

func (f *fakeGiveaways) Reroll(
	_ context.Context, _ giveaway.ReviewTelegramAll, userID int64, _ uint, out int64,
) (giveaway.ReviewView, error) {
	f.userID, f.winnerID = userID, out

	return f.review, f.err
}

func (f *fakeGiveaways) Publish(
	_ context.Context,
	_ giveaway.ReviewTelegramAll,
	userID int64,
	_ uint,
	rerolled int,
) error {
	f.userID, f.rerolled, f.lastAction = userID, rerolled, "publish"

	return f.err
}

func (f *fakeGiveaways) Skip(
	_ context.Context,
	_ giveaway.ReviewTelegramAll,
	userID int64,
	_ uint,
	rerolled int,
) error {
	f.userID, f.rerolled, f.lastAction = userID, rerolled, "skip"

	return f.err
}

func (f *fakeGiveaways) act(name string, userID int64) (giveaway.View, error) {
	f.userID, f.lastAction = userID, name

	return f.view, f.err
}

func newGiveawayMux(t *testing.T, fake *fakeGiveaways) *http.ServeMux {
	t.Helper()

	ops := webapp.NewOperations(
		nil,
		fake,
		webapp.NewPhotos(testToken, nil, slog.New(slog.DiscardHandler)),
		slog.New(slog.DiscardHandler),
	)

	return newTestMux(t, ops, map[int64]string{42: "uk"})
}

// send sends a JSON body as user 42 and returns the response.
func send(t *testing.T, mux http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	req.Header.Set("Authorization", authorize(t, models.User{ID: 42}))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	return rec
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) webapp.APIError {
	t.Helper()

	var p webapp.APIError
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}

	return p
}

func TestCreateGiveaway(t *testing.T) {
	fake := &fakeGiveaways{view: giveaway.View{ID: 7, Status: giveaway.StatusActive, EndsAt: "2026-12-01T20:00"}}
	mux := newGiveawayMux(t, fake)

	rec := send(t, mux, http.MethodPost, "/api/giveaways",
		`{"post_link":"https://t.me/c/1/42","ends_at":"2026-12-01T20:00","winners":2,"button_text":"Join"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, body %s", rec.Code, rec.Body)
	}

	want := giveaway.Fields{PostLink: "https://t.me/c/1/42", EndsAt: "2026-12-01T20:00", Winners: 2, ButtonText: "Join"}
	if fake.fields != want || fake.userID != 42 || fake.lang != "uk" {
		t.Errorf("Create got %+v as %d in %q, want %+v as 42 in uk", fake.fields, fake.userID, fake.lang, want)
	}

	var got struct {
		ID     uint   `json:"id"`
		Status string `json:"status"`
		EndsAt string `json:"ends_at"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}

	if got.ID != 7 || got.Status != "active" || got.EndsAt != "2026-12-01T20:00" {
		t.Errorf("body = %+v", got)
	}
}

func TestGiveawayErrors(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantCode  int
		wantError string
		wantField string
	}{
		{
			name:      "field problem",
			err:       &giveaway.AppError{Code: giveaway.CodeNotAdmin, Field: giveaway.FieldPostLink},
			wantCode:  http.StatusForbidden,
			wantError: "not_admin",
			wantField: "post_link",
		},
		{
			name:      "not found",
			err:       &giveaway.AppError{Code: giveaway.CodeNotFound},
			wantCode:  404,
			wantError: "not_found",
		},
		{name: "stale", err: &giveaway.AppError{Code: giveaway.CodeStale}, wantCode: 409, wantError: "stale"},
		{
			name:      "invalid winners",
			err:       &giveaway.AppError{Code: giveaway.CodeInvalidWinners, Field: giveaway.FieldWinners},
			wantCode:  422,
			wantError: "invalid_winners",
			wantField: "winners",
		},
		{name: "database down", err: context.DeadlineExceeded, wantCode: 500, wantError: "internal"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mux := newGiveawayMux(t, &fakeGiveaways{err: tc.err})

			rec := send(t, mux, http.MethodPost, "/api/giveaways/7/cancel", "")
			if rec.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d, body %s", rec.Code, tc.wantCode, rec.Body)
			}

			if p := decodeProblem(t, rec); p.Code != tc.wantError || p.Field != tc.wantField {
				t.Errorf("problem = %+v, want %s on %q", p, tc.wantError, tc.wantField)
			}
		})
	}
}

// A body that does not match the schema never reaches the domain.
func TestGiveawaySchemaValidation(t *testing.T) {
	fake := &fakeGiveaways{}
	mux := newGiveawayMux(t, fake)

	rec := send(
		t,
		mux,
		http.MethodPost,
		"/api/giveaways",
		`{"post_link":"x","ends_at":"y","winners":"two","button_text":"b"}`,
	)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, body %s", rec.Code, rec.Body)
	}

	if p := decodeProblem(t, rec); p.Code != webapp.CodeInvalid {
		t.Errorf("code = %q, want invalid", p.Code)
	}

	if fake.userID != 0 {
		t.Error("Create ran on an invalid body")
	}
}

func TestEditGiveawayPartial(t *testing.T) {
	fake := &fakeGiveaways{}
	mux := newGiveawayMux(t, fake)

	if rec := send(t, mux, http.MethodPatch, "/api/giveaways/7", `{"winners":3}`); rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body %s", rec.Code, rec.Body)
	}

	if fake.changes.Winners == nil || *fake.changes.Winners != 3 || fake.changes.EndsAt != nil ||
		fake.changes.ButtonText != nil {
		t.Errorf("changes = %+v, want only winners = 3", fake.changes)
	}
}

func TestListPages(t *testing.T) {
	for _, path := range []string{"/api/giveaways", "/api/giveaways/finished"} {
		t.Run(path, func(t *testing.T) { checkListPage(t, path) })
	}
}

// checkListPage checks that the list at path passes the page position on and answers with the
// total.
func checkListPage(t *testing.T, path string) {
	t.Helper()

	fake := &fakeGiveaways{}
	mux := newGiveawayMux(t, fake)

	rec := send(t, mux, http.MethodGet, path+"?offset=10&limit=10", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: code = %d, body %s", path, rec.Code, rec.Body)
	}

	if fake.offset != 10 || fake.limit != 10 {
		t.Errorf("%s: offset %d, limit %d, want 10 and 10", path, fake.offset, fake.limit)
	}

	var body struct {
		Giveaways []json.RawMessage `json:"giveaways"`
		Total     *int              `json:"total"`
		More      *bool             `json:"more"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}

	if len(body.Giveaways) != 1 || body.Total == nil || *body.Total != 21 || body.More != nil {
		t.Errorf("%s: body %s, want one giveaway, total 21 and no more", path, rec.Body)
	}

	if rec := send(t, mux, http.MethodGet, path+"?limit=101", ""); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("%s: limit 101 code = %d, want 422", path, rec.Code)
	}
}

func TestGiveawayRoutes(t *testing.T) {
	fake := &fakeGiveaways{}
	mux := newGiveawayMux(t, fake)

	for _, tc := range []struct{ method, path, body, wantAction string }{
		{method: http.MethodGet, path: "/api/giveaways"},
		{method: http.MethodGet, path: "/api/giveaways/finished?limit=10"},
		{method: http.MethodGet, path: "/api/giveaways/7"},
		{method: http.MethodPost, path: "/api/giveaways/7/cancel", wantAction: "cancel"},
		{method: http.MethodPost, path: "/api/giveaways/7/restore", wantAction: "restore"},
		{method: http.MethodPost, path: "/api/giveaways/7/finish-now", wantAction: "finish-now"},
		{method: http.MethodPost, path: "/api/giveaways/7/move", body: `{"post_link":"https://t.me/c/1/50"}`},
	} {
		fake.lastAction = ""

		rec := send(t, mux, tc.method, tc.path, tc.body)
		if rec.Code != http.StatusOK {
			t.Errorf("%s %s: code = %d, body %s", tc.method, tc.path, rec.Code, rec.Body)
		}

		if fake.lastAction != tc.wantAction {
			t.Errorf("%s %s: action %q, want %q", tc.method, tc.path, fake.lastAction, tc.wantAction)
		}
	}

	if fake.link != "https://t.me/c/1/50" {
		t.Errorf("move link = %q", fake.link)
	}

	if rec := call(t, mux, "/api/giveaways", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("without launch data: code = %d, want 401", rec.Code)
	}
}
