package webapp_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/user"
	"github.com/alexander-danilenko/read_wine_bot/internal/webapp"
)

const testToken = "123:test"

// signInitData builds launch data the way Telegram does: every field but hash, sorted by name
// as key=value lines, signed with HMAC-SHA256 under the key HMAC-SHA256("WebAppData", token).
func signInitData(t *testing.T, token string, fields url.Values) string {
	t.Helper()

	lines := make([]string, 0, len(fields))
	for k := range fields {
		lines = append(lines, k+"="+fields.Get(k))
	}

	slices.Sort(lines)

	key := hmac.New(sha256.New, []byte("WebAppData"))
	key.Write([]byte(token))
	mac := hmac.New(sha256.New, key.Sum(nil))
	mac.Write([]byte(strings.Join(lines, "\n")))

	signed := url.Values{}
	for k := range fields {
		signed.Set(k, fields.Get(k))
	}

	signed.Set("hash", hex.EncodeToString(mac.Sum(nil)))

	return signed.Encode()
}

func launchFields(t *testing.T, u models.User, authDate time.Time) url.Values {
	t.Helper()

	data, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}

	return url.Values{
		"user":      {string(data)},
		"auth_date": {strconv.FormatInt(authDate.Unix(), 10)},
		"query_id":  {"AAH"},
	}
}

func TestVerify(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	alice := models.User{ID: 42, FirstName: "A+B 100%", LanguageCode: "en"}

	valid := signInitData(t, testToken, launchFields(t, alice, now.Add(-time.Hour)))

	withSignature := launchFields(t, alice, now)
	withSignature.Set("signature", "ed25519-signature-for-third-parties")

	noUser := launchFields(t, alice, now)
	noUser.Del("user")

	altered, err := url.ParseQuery(valid)
	if err != nil {
		t.Fatal(err)
	}

	altered.Set("auth_date", strconv.FormatInt(now.Unix(), 10))

	tests := []struct {
		name     string
		initData string
		wantErr  error
	}{
		{name: "valid, name with + and %", initData: valid},
		{name: "signature field is signed too", initData: signInitData(t, testToken, withSignature)},
		{name: "23 hours old", initData: signInitData(t, testToken, launchFields(t, alice, now.Add(-23*time.Hour)))},
		{name: "empty", wantErr: webapp.ErrNoInitData},
		{name: "not a query", initData: "%zz", wantErr: webapp.ErrInitDataMalformed},
		{name: "no hash", initData: "auth_date=1&user=%7B%7D", wantErr: webapp.ErrInitDataMalformed},
		{name: "altered field", initData: altered.Encode(), wantErr: webapp.ErrInitDataSignature},
		{
			name:     "other bot's token",
			initData: signInitData(t, "456:other", launchFields(t, alice, now)),
			wantErr:  webapp.ErrInitDataSignature,
		},
		{
			name:     "25 hours old",
			initData: signInitData(t, testToken, launchFields(t, alice, now.Add(-25*time.Hour))),
			wantErr:  webapp.ErrInitDataExpired,
		},
		{
			name:     "from the future",
			initData: signInitData(t, testToken, launchFields(t, alice, now.Add(time.Hour))),
			wantErr:  webapp.ErrInitDataExpired,
		},
		{name: "no user", initData: signInitData(t, testToken, noUser), wantErr: webapp.ErrInitDataNoUser},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u, err := webapp.Verify(tc.initData, testToken, now)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}

			if err == nil && (u.ID != alice.ID || u.FirstName != alice.FirstName) {
				t.Errorf("user = %+v, want %+v", u, alice)
			}
		})
	}
}

type fakeUsers struct {
	recorded  []user.Profile
	saved     map[int64]string
	recordErr error
}

func (f *fakeUsers) Record(_ context.Context, p user.Profile) error {
	if f.recordErr != nil {
		return f.recordErr
	}

	f.recorded = append(f.recorded, p)

	return nil
}

func (f *fakeUsers) Language(_ context.Context, id int64) (string, error) {
	return f.saved[id], nil
}

func (f *fakeUsers) SetLanguage(_ context.Context, id int64, lang string) error {
	if f.saved == nil {
		f.saved = map[int64]string{}
	}

	f.saved[id] = lang

	return nil
}

// authorize returns the Authorization header value for u, launched now.
func authorize(t *testing.T, u models.User) string {
	t.Helper()

	return "tma " + signInitData(t, testToken, launchFields(t, u, time.Now()))
}

func TestAuthWrap(t *testing.T) {
	alice := models.User{ID: 42, FirstName: "Alice", LanguageCode: "en-US"}

	tests := []struct {
		name       string
		header     string
		saved      string
		recordErr  error
		wantCode   int
		wantLang   string
		wantCalled bool
	}{
		{
			name:       "saved language wins",
			header:     authorize(t, alice),
			saved:      "uk",
			wantCode:   200,
			wantLang:   "uk",
			wantCalled: true,
		},
		{name: "telegram language next", header: authorize(t, alice), wantCode: 200, wantLang: "en", wantCalled: true},
		{
			name:       "unsupported language falls back to uk",
			header:     authorize(t, models.User{ID: 42, LanguageCode: "de"}),
			wantCode:   200,
			wantLang:   "uk",
			wantCalled: true,
		},
		{name: "no header", wantCode: http.StatusUnauthorized},
		{name: "wrong scheme", header: strings.Replace(authorize(t, alice), "tma ", "Bearer ", 1), wantCode: 401},
		{name: "profile not saved", header: authorize(t, alice), recordErr: errors.New("db down"), wantCode: 500},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			users := &fakeUsers{saved: map[int64]string{42: tc.saved}, recordErr: tc.recordErr}
			auth := webapp.NewAuth(testToken, users, users, []string{"en", "uk"}, slog.New(slog.DiscardHandler))

			var got webapp.Caller

			called := false
			h := auth.Wrap(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				called = true
				got, _ = webapp.CallerFrom(r.Context())
			}))

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/me", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tc.wantCode || called != tc.wantCalled {
				t.Fatalf("code, called = %d, %v, want %d, %v", rec.Code, called, tc.wantCode, tc.wantCalled)
			}

			if !called {
				checkProblem(t, rec)

				return
			}

			if got.User.ID != 42 || got.Lang != tc.wantLang {
				t.Errorf("caller = %+v, want user 42, lang %q", got, tc.wantLang)
			}

			if len(users.recorded) != 1 || users.recorded[0].TelegramID != 42 {
				t.Errorf("recorded = %+v, want the caller's profile once", users.recorded)
			}
		})
	}
}

// checkProblem fails the test unless rec holds an error body with a code.
func checkProblem(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(body), `"code":`) {
		t.Errorf("error body %s has no code", body)
	}
}
